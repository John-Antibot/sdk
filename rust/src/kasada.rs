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

/// The `x-kpsdk-v` value p.js stamps on `/fp`.
#[derive(Debug, Clone, Default)]
pub struct VersionResult {
    /// For example `j-1.2.1020`.
    pub version: String,
    /// The same value under the API's generic payload field.
    pub payload: String,
    /// Send this as the next call's `session` to continue the identity.
    pub session: String,
    /// The browser the version was read as.
    pub user_agent: String,
}

#[derive(Deserialize, Default)]
#[serde(rename_all = "camelCase", default)]
struct WireVersion {
    version: String,
    payload: String,
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
