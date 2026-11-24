//! Kasada payloads: the `/tl` body and headers from an ips.js your client fetched.
//!
//! ```no_run
//! use john_antibot::kasada;
//!
//! # async fn run() -> john_antibot::Result<()> {
//! let client = kasada::Client::new(&std::env::var("JOHNANTIBOT_API_KEY").unwrap());
//! let res = client
//!     .generate_payload(kasada::PayloadInput {
//!         script: "<ips.js body>".into(),
//!         ips_link: Some("/<id>/<id>/ips.js?...".into()),
//!         page_url: "https://www.example.com/".into(),
//!         user_agent: "Mozilla/5.0 ...".into(),
//!         ip: "203.0.113.1".into(),
//!         accept_language: "en-US,en;q=0.9".into(),
//!         ..Default::default()
//!     })
//!     .await?;
//! println!("{} {} bytes", res.tl_url, res.payload.len());
//! # Ok(())
//! # }
//! ```

use std::collections::BTreeMap;
use std::sync::Arc;
use std::time::Duration;

use base64::Engine;
use serde::Deserialize;
use serde_json::Map;
use sha2::{Digest, Sha256};

use crate::http::{Builder, Core, put};
use crate::{Error, Result};

const DEFAULT_TIMEOUT: Duration = Duration::from_secs(60);

/// Sends API calls. Cheap to clone; safe to share across tasks.
#[derive(Debug, Clone)]
pub struct Client(Arc<Core>);

impl Builder<Client> {
    /// Builds the client.
    pub fn build(self) -> Result<Client> {
        Ok(Client(Arc::new(self.core(
            "kasada",
            "/v1/kasada",
            DEFAULT_TIMEOUT,
        )?)))
    }
}

/// One Kasada fingerprint solve.
#[derive(Debug, Clone, Default)]
pub struct PayloadInput {
    /// The ips.js body, exactly as fetched.
    pub script: String,
    /// The block or `/fp` page that named ips.js. Send this or `ips_link`.
    pub html: Option<String>,
    /// The ips.js path and query from that page. Send this or `html`.
    pub ips_link: Option<String>,
    /// The URL of the block or `/fp` page. The site must be approved for your key.
    pub page_url: String,
    /// The browser fetching and posting as.
    pub user_agent: String,
    /// The address the resulting `/tl` request will be posted from.
    pub ip: String,
    /// That browser's Accept-Language value.
    pub accept_language: String,
    /// Continue a browser identity from an earlier result.
    pub session: Option<String>,
}

/// The p.js whose `x-kpsdk-v` value is wanted.
#[derive(Debug, Clone, Default)]
pub struct VersionInput {
    /// The p.js body, exactly as fetched.
    pub script: String,
    /// The URL of the page that loaded p.js.
    pub page_url: String,
    /// Optionally chooses the collected browser to run as.
    pub user_agent: Option<String>,
    /// That browser's Accept-Language value.
    pub accept_language: Option<String>,
    /// Continue a browser identity from an earlier result.
    pub session: Option<String>,
}

/// What `x-kpsdk-cd` is made from besides the session. They change only when the site's p.js does.
#[derive(Debug, Clone, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct CdParams {
    /// The sum of the rounds' targets.
    pub difficulty: u32,
    /// How many rounds of proof of work.
    pub subchallenge_count: u32,
    /// The site's key, hashed into the first round.
    pub crypto_customer_key: String,
    /// An extra string hashed after the key; usually empty.
    pub seed_suffix: String,
    /// What p.js hashes first, for example `tp-v2-input`.
    pub label: String,
    /// Whether the label is followed by the first 16 characters of `ct` (a desktop browser's is).
    pub ct_prefix: bool,
}

fn sha256_hex(text: &str) -> String {
    Sha256::digest(text.as_bytes())
        .iter()
        .map(|b| format!("{b:02x}"))
        .collect()
}

/// p.js's proof of work: per round, the first nonce `n` whose `sha256("<n>, <previous hash>")` scores at least
/// `difficulty / rounds` (score = 2^52 over the first 13 hex digits, plus one); the passing hash starts the next round.
fn cd_answers(first: &str, difficulty: u32, rounds: u32) -> Vec<u32> {
    let target = f64::from(difficulty) / f64::from(rounds);
    let mut hash = sha256_hex(first);
    let mut answers = Vec::new();
    while answers.len() < rounds as usize {
        let mut n = 1u32;
        loop {
            let next = sha256_hex(&format!("{n}, {hash}"));
            let prefix = u64::from_str_radix(&next[..13], 16).unwrap_or(0);
            if (1u64 << 52) as f64 / (prefix + 1) as f64 >= target {
                answers.push(n);
                hash = next;
                break;
            }
            n += 1;
        }
    }
    answers
}

/// Random bytes without a dependency: std's per-process random hasher keys, mixed with the clock.
fn random_u64() -> u64 {
    use std::hash::{BuildHasher, Hasher};
    let mut h = std::collections::hash_map::RandomState::new().build_hasher();
    h.write_u128(
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map_or(0, |d| d.as_nanos()),
    );
    h.finish()
}

impl CdParams {
    /// `x-kpsdk-cd` for one protected request. Computed here: about ten SHA-256 hashes, no API call, no charge.
    ///
    /// `ct` is the session's current `x-kpsdk-ct` (the one sent on this very request; each reply rotates it) and `st`
    /// the `x-kpsdk-st` header `/tl` answered with, in milliseconds. Make a new header for every request.
    pub fn header(&self, ct: &str, st: i64) -> Result<String> {
        if self.difficulty < 1 || self.subchallenge_count < 1 {
            return Err(Error::Input(
                "kasada: difficulty and subchallenge_count must be positive".into(),
            ));
        }
        if st <= 0 {
            return Err(Error::Input(
                "kasada: st must be the x-kpsdk-st value /tl answered with".into(),
            ));
        }
        let mut label = if self.label.is_empty() {
            "tp-v2-input".to_string()
        } else {
            self.label.clone()
        };
        if self.ct_prefix {
            if ct.len() < 16 || !ct.is_char_boundary(16) {
                return Err(Error::Input(
                    "kasada: ct must be the x-kpsdk-ct value (at least 16 characters)".into(),
                ));
            }
            label.push_str(&ct[..16]);
        }
        let id = format!("{:016x}{:016x}", random_u64(), random_u64());
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map_or(0, |d| d.as_millis() as i64);
        let work_time = now - 10 - (random_u64() % 41) as i64;
        let mut first = format!("{label}, {work_time}, {id}");
        if !self.crypto_customer_key.is_empty() {
            first.push_str(", ");
            first.push_str(&self.crypto_customer_key);
        }
        if !self.seed_suffix.is_empty() {
            first.push_str(", ");
            first.push_str(&self.seed_suffix);
        }
        // a real page's clock reads st a little before or after the server's, the same for the whole session
        let d = i64::from(Sha256::digest(st.to_string().as_bytes())[0]) * 90 / 255 - 10;
        let duration = (200 + random_u64() % 551) as f64 / 10.0;
        serde_json::to_string(&serde_json::json!({
            "workTime": work_time, "id": id,
            "answers": cd_answers(&first, self.difficulty, self.subchallenge_count),
            "duration": duration, "d": d, "st": st, "rst": st + d,
        }))
        .map_err(|e| Error::Decode(e.to_string()))
    }
}

/// The `x-kpsdk-v` value p.js stamps on `/fp`, and what `x-kpsdk-cd` is made from (see [`VersionResult::cd_header`]).
#[derive(Debug, Clone, Default)]
pub struct VersionResult {
    /// For example `j-1.2.1020`.
    pub version: String,
    /// `None` when p.js's proof of work could not be read.
    pub cd: Option<CdParams>,
    /// The same value under the API's generic payload field.
    pub payload: String,
    /// Send this as the next call's `session` to continue the identity.
    pub session: String,
    /// The browser the version was read as.
    pub user_agent: String,
}

impl VersionResult {
    /// `x-kpsdk-cd` for one protected request, computed locally (no API call, no charge); see [`CdParams::header`].
    pub fn cd_header(&self, ct: &str, st: i64) -> Result<String> {
        self.cd
            .as_ref()
            .ok_or_else(|| {
                Error::Input(
                    "kasada: this version has no cd parameters (p.js's proof of work could not be read)".into(),
                )
            })?
            .header(ct, st)
    }
}

#[derive(Deserialize, Default)]
#[serde(rename_all = "camelCase", default)]
struct WireVersion {
    version: String,
    payload: String,
    cd: Option<CdParams>,
    session: String,
    user_agent: String,
}

/// The binary `/tl` artifact and the browser it was made as.
#[derive(Debug, Clone, Default)]
pub struct PayloadResult {
    /// The decoded binary `/tl` request body. Post it unchanged.
    pub payload: Vec<u8>,
    /// `payload` in the API's base64 form.
    pub payload_base64: String,
    /// The `x-kpsdk-*` headers for the `/tl` POST.
    pub headers: BTreeMap<String, String>,
    /// The absolute `/tl` endpoint.
    pub tl_url: String,
    /// Send this as the next call's `session` to continue the identity.
    pub session: String,
    /// The browser the payload was made as.
    pub user_agent: String,
}

#[derive(Deserialize, Default)]
#[serde(rename_all = "camelCase", default)]
struct WireResult {
    payload_base64: String,
    headers: BTreeMap<String, String>,
    tl_url: String,
    session: String,
    user_agent: String,
}

impl Client {
    /// A client for the hosted API with your API key.
    ///
    /// # Panics
    ///
    /// When the TLS backend cannot be initialized. Use [`Client::builder`] to handle it.
    pub fn new(api_key: &str) -> Self {
        Self::builder(api_key)
            .build()
            .expect("john_antibot: build HTTP client")
    }

    /// A builder: `org_key`, `endpoint`, `timeout`, `http_client`.
    pub fn builder(api_key: &str) -> Builder<Client> {
        Builder::new(api_key)
    }

    /// The API root this client calls.
    pub fn endpoint(&self) -> &str {
        &self.0.endpoint
    }

    /// Reads the `x-kpsdk-v` value by running p.js and reading its `/mfc` header.
    pub async fn version(&self, input: VersionInput) -> Result<VersionResult> {
        if input.script.is_empty() || input.page_url.is_empty() {
            return Err(Error::Input(
                "kasada: a version needs script and page_url".into(),
            ));
        }
        let mut body = Map::new();
        put(&mut body, "script", Some(&input.script));
        put(&mut body, "pageUrl", Some(&input.page_url));
        put(&mut body, "userAgent", input.user_agent.as_deref());
        put(
            &mut body,
            "acceptLanguage",
            input.accept_language.as_deref(),
        );
        put(&mut body, "session", input.session.as_deref());
        let wire: WireVersion = self.0.post("/version", body).await?;
        if wire.version.is_empty() {
            return Err(Error::Decode(
                "kasada: the API answered without a version".into(),
            ));
        }
        Ok(VersionResult {
            payload: if wire.payload.is_empty() {
                wire.version.clone()
            } else {
                wire.payload
            },
            version: wire.version,
            cd: wire
                .cd
                .filter(|c| c.difficulty > 0 && c.subchallenge_count > 0),
            session: wire.session,
            user_agent: wire.user_agent,
        })
    }

    /// Runs ips.js as a collected device and returns the `/tl` body and headers.
    pub async fn generate_payload(&self, input: PayloadInput) -> Result<PayloadResult> {
        if input.script.is_empty()
            || input.page_url.is_empty()
            || input.user_agent.is_empty()
            || input.ip.is_empty()
            || input.accept_language.is_empty()
            || (input.html.as_deref().unwrap_or_default().is_empty()
                && input.ips_link.as_deref().unwrap_or_default().is_empty())
        {
            return Err(Error::Input(
                "kasada: a payload needs script, html or ips_link, page_url, user_agent, ip and accept_language".into(),
            ));
        }
        let mut body = Map::new();
        put(&mut body, "script", Some(&input.script));
        put(&mut body, "html", input.html.as_deref());
        put(&mut body, "ipsLink", input.ips_link.as_deref());
        put(&mut body, "pageUrl", Some(&input.page_url));
        put(&mut body, "userAgent", Some(&input.user_agent));
        put(&mut body, "ip", Some(&input.ip));
        put(&mut body, "acceptLanguage", Some(&input.accept_language));
        put(&mut body, "session", input.session.as_deref());
        let wire: WireResult = self.0.post("/payload", body).await?;
        let payload = base64::engine::general_purpose::STANDARD
            .decode(&wire.payload_base64)
            .map_err(|e| Error::Decode(format!("kasada: decode payloadBase64: {e}")))?;
        Ok(PayloadResult {
            payload,
            payload_base64: wire.payload_base64,
            headers: wire.headers,
            tl_url: wire.tl_url,
            session: wire.session,
            user_agent: wire.user_agent,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const NIKE_KEY: &str = "1760fa26d0fa116e2f26e90dc8b1379c3d9e946f650852b7f9925457ab77a14a";

    // Two cds real Chrome stamped on nike's cart writes (2026-11-24), with the x-kpsdk-ct each request carried.
    #[test]
    fn answers_match_real_chrome() {
        assert_eq!(
            cd_answers(
                &format!(
                    "tp-v2-input045g90GkupSRHwee, 1795536575353, ce1a0639146179bcf6ab456ac1925359, {NIKE_KEY}"
                ),
                10,
                2
            ),
            vec![13, 5]
        );
        assert_eq!(
            cd_answers(
                &format!(
                    "tp-v2-input01pDYjEopgLXL7X4, 1795536579918, 2d455b029dc31363685cb443dce15ea4, {NIKE_KEY}"
                ),
                10,
                2
            ),
            vec![5, 1]
        );
    }

    #[test]
    fn header_solves_its_own_input_and_keeps_d_per_session() {
        let params = CdParams {
            difficulty: 10,
            subchallenge_count: 2,
            crypto_customer_key: NIKE_KEY.into(),
            label: "tp-v2-input".into(),
            ct_prefix: true,
            ..Default::default()
        };
        let ct = "045g90GkupSRHweeAkVbaHmX2luTJgqAGpoWX3q5D4o";
        let st = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_millis() as i64
            - 300;
        let mut offsets = std::collections::BTreeSet::new();
        for _ in 0..10 {
            let cd: serde_json::Value =
                serde_json::from_str(&params.header(ct, st).unwrap()).unwrap();
            let first = format!(
                "tp-v2-input{}, {}, {}, {NIKE_KEY}",
                &ct[..16],
                cd["workTime"],
                cd["id"].as_str().unwrap()
            );
            assert_eq!(cd["answers"], serde_json::json!(cd_answers(&first, 10, 2)));
            let d = cd["d"].as_i64().unwrap();
            assert_eq!(cd["rst"].as_i64().unwrap() - cd["st"].as_i64().unwrap(), d);
            assert!((-10..=80).contains(&d));
            let duration = cd["duration"].as_f64().unwrap();
            assert!((20.0..=75.0).contains(&duration));
            offsets.insert(d);
        }
        assert_eq!(offsets.len(), 1);
        assert!(VersionResult::default().cd_header(ct, st).is_err());
    }
}
