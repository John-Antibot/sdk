//! DataDome: tags, interstitial and slider payloads.
//!
//! ```no_run
//! use john_antibot::datadome;
//!
//! # async fn run(cid: String, ddk: String) -> john_antibot::Result<()> {
//! let key = std::env::var("JOHNANTIBOT_API_KEY").unwrap();
//! let client = datadome::Client::new(&key);
//! let mut session = client.session(datadome::SessionOptions {
//!     referer: Some("https://www.example.com/".into()),
//!     ..Default::default()
//! });
//! let tags = session
//!     .generate_tags(datadome::TagsInput {
//!         event: datadome::Event::Ch,
//!         cookie: cid,
//!         site_key: ddk,
//!         loaded_font_families: vec!["Inter".into()],
//!         ..Default::default()
//!     })
//!     .await?;
//! println!("{} {}", tags.payload, session.user_agent());
//! # Ok(())
//! # }
//! ```

use std::sync::Arc;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use serde::Deserialize;
use serde_json::{Map, Value};
use sha2::{Digest, Sha256};

use crate::http::{Builder, Core, put};
use crate::{Error, Result};

mod page;
pub use page::*;

const DEFAULT_TIMEOUT: Duration = Duration::from_secs(60);

/// Sends generator calls. Cheap to clone; safe to share across tasks.
#[derive(Debug, Clone)]
pub struct Client(Arc<Core>);

impl Client {
    /// A client for the hosted API with your API key.
    ///
    /// # Panics
    ///
    /// When the TLS backend cannot be initialized (as `reqwest::Client::new`). Use [`Client::builder`] to handle it.
    pub fn new(api_key: &str) -> Self {
        Self::builder(api_key)
            .build()
            .expect("john_antibot: build HTTP client")
    }

    /// A builder: `org_key`, `endpoint`, `timeout` (one call; default 60 s), `http_client`.
    pub fn builder(api_key: &str) -> Builder<Client> {
        Builder::new(api_key)
    }

    /// The API root this client calls.
    pub fn endpoint(&self) -> &str {
        &self.0.endpoint
    }

    /// A new browser identity. Use one per proxy and cookie jar.
    pub fn session(&self, options: SessionOptions) -> Session {
        let some = |v: Option<String>| v.filter(|s| !s.is_empty());
        Session {
            client: self.clone(),
            token: some(options.session),
            device: some(options.device),
            user_agent: some(options.user_agent).unwrap_or_else(|| LATEST_USER_AGENT.to_string()),
            referer: some(options.referer),
            timezone: some(options.timezone),
            timezone_offset: options.timezone_offset,
        }
    }
}

impl Builder<Client> {
    /// Builds the client.
    pub fn build(self) -> Result<Client> {
        Ok(Client(Arc::new(self.core(
            "datadome",
            "/v1/datadome",
            DEFAULT_TIMEOUT,
        )?)))
    }
}

/// The identity a [`Session`] starts with. Every field is optional.
#[derive(Debug, Clone, Default)]
pub struct SessionOptions {
    /// The session token of an earlier session, to resume it.
    pub session: Option<String>,
    /// Pins a device profile by name.
    pub device: Option<String>,
    /// The browser's user agent; empty is [`LATEST_USER_AGENT`].
    pub user_agent: Option<String>,
    /// The default page URL of calls.
    pub referer: Option<String>,
    /// The IANA zone of the session, for example `America/New_York`.
    pub timezone: Option<String>,
    /// JavaScript's `getTimezoneOffset()` for that zone, in minutes (`240` for EDT).
    pub timezone_offset: Option<i32>,
}

/// One browser identity. It learns its session token from each reply; send [`Session::user_agent`] on every request.
#[derive(Debug, Clone)]
pub struct Session {
    client: Client,
    token: Option<String>,
    device: Option<String>,
    user_agent: String,
    referer: Option<String>,
    timezone: Option<String>,
    timezone_offset: Option<i32>,
}

/// A tags event: [`Event::Ch`] first, then [`Event::Le`], on one session.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum Event {
    /// `ch`.
    #[default]
    Ch,
    /// `le`.
    Le,
}

impl Event {
    /// The wire value: `ch` or `le`.
    pub fn as_str(self) -> &'static str {
        match self {
            Event::Ch => "ch",
            Event::Le => "le",
        }
    }
}

/// The inputs of [`Session::generate_tags`].
#[derive(Debug, Clone, Default)]
pub struct TagsInput {
    /// `ch` or `le`.
    pub event: Event,
    /// The current `datadome` cookie value.
    pub cookie: String,
    /// The site's DataDome key (`ddjskey`, required).
    pub site_key: String,
    /// The tags.js version (`ddv`).
    pub version: Option<String>,
    /// The source of the tags.js the page loaded: that build runs. Only its SHA-256 is sent. Absent: the current build.
    pub script: Option<String>,
    /// The page URL; defaults to the session's referer.
    pub referer: Option<String>,
    /// The `@font-face` families the page loaded (`document.fonts`). Required for most sites.
    pub loaded_font_families: Vec<String>,
    /// The site's tags.js, when it is not `https://datadome.<site>/tags.js`.
    pub tags_script_url: Option<String>,
}

/// The reply of [`Session::generate_tags`].
#[derive(Debug, Clone, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct TagsResponse {
    /// Form body for the site's `/js/`.
    pub payload: String,
    /// The session token.
    pub session: String,
    /// The session's user agent.
    pub user_agent: String,
    /// When it was made, in Unix milliseconds.
    pub generated_at: i64,
}

/// The inputs of [`Session::generate_interstitial`].
#[derive(Debug, Clone, Default)]
pub struct InterstitialInput {
    /// The interstitial page.
    pub html: String,
    /// The URL `html` was loaded from (required).
    pub device_link: String,
    /// The blocked page URL; defaults to the session's referer.
    pub referer: Option<String>,
    /// When `html` was loaded.
    pub fetched_at: Option<SystemTime>,
    /// The source of the page's external challenge script ([`challenge_script_url`]); absent when the page inlines it.
    /// Only its SHA-256 is sent.
    pub script: Option<String>,
}

/// The reply of [`Session::generate_interstitial`].
#[derive(Debug, Clone, Default)]
pub struct InterstitialResponse {
    /// Form body for `https://geo.captcha-delivery.com/interstitial/`.
    pub form_body: String,
    /// The session token.
    pub session: String,
    /// The session's user agent.
    pub user_agent: String,
}

/// The inputs of [`Session::generate_slider`].
#[derive(Debug, Clone, Default)]
pub struct SliderInput {
    /// The captcha page.
    pub html: String,
    /// The URL `html` was loaded from (required).
    pub captcha_url: String,
    /// The page that showed the captcha; absent: read from `html`.
    pub parent_url: Option<String>,
    /// When `html` was loaded.
    pub fetched_at: Option<SystemTime>,
    /// The page's external captcha script, as in [`InterstitialInput::script`].
    pub script: Option<String>,
}

/// The reply of [`Session::generate_slider`].
#[derive(Debug, Clone, Default)]
pub struct SliderResponse {
    /// The `/captcha/check` URL: GET it.
    pub check_url: String,
    /// The session token.
    pub session: String,
    /// The session's user agent.
    pub user_agent: String,
}

#[derive(Deserialize, Default)]
#[serde(rename_all = "camelCase", default)]
struct Wire {
    payload: String,
    session: String,
    user_agent: String,
}

impl Session {
    /// The user agent to send on every request of this session.
    pub fn user_agent(&self) -> &str {
        &self.user_agent
    }

    /// The session token, once the API gave one (or the one it was resumed with).
    pub fn token(&self) -> Option<&str> {
        self.token.as_deref()
    }

    /// The default page URL of calls.
    pub fn referer(&self) -> Option<&str> {
        self.referer.as_deref()
    }

    /// One tags payload.
    pub async fn generate_tags(&mut self, input: TagsInput) -> Result<TagsResponse> {
        if input.site_key.is_empty() {
            return Err(Error::Input(
                "datadome: TagsInput.site_key is required: the site's DataDome key".into(),
            ));
        }
        let mut req = self.identity();
        req.insert("cid".into(), input.cookie.into());
        req.insert("jsType".into(), input.event.as_str().into());
        req.insert("ddk".into(), input.site_key.into());
        if !input.loaded_font_families.is_empty() {
            req.insert(
                "loadedFontFamilies".into(),
                input.loaded_font_families.into(),
            );
        }
        put(&mut req, "tagsScriptUrl", input.tags_script_url.as_deref());
        put(&mut req, "version", input.version.as_deref());
        put(
            &mut req,
            "scriptHash",
            input
                .script
                .as_deref()
                .filter(|s| !s.is_empty())
                .map(script_hash)
                .as_deref(),
        );
        let Some(referer) = pick(input.referer.as_deref(), self.referer.as_deref()) else {
            return Err(Error::Input(
                "datadome: TagsInput.referer (or SessionOptions.referer) is required: the page the tags run on".into(),
            ));
        };
        req.insert("referer".into(), referer.into());
        let out: TagsResponse = self.client.0.post("/tags", req).await?;
        self.learn(&out.session, &out.user_agent);
        Ok(out)
    }

    /// The interstitial form to POST to `GEO_ORIGIN/interstitial/`.
    pub async fn generate_interstitial(
        &mut self,
        input: InterstitialInput,
    ) -> Result<InterstitialResponse> {
        if input.device_link.is_empty() {
            return Err(Error::Input(
                "datadome: InterstitialInput.device_link is required: the interstitial URL you loaded".into(),
            ));
        }
        require_script(&input.html, input.script.as_deref())?;
        let mut req = self.identity();
        req.insert("html".into(), input.html.into());
        req.insert("deviceUrl".into(), input.device_link.into());
        put(
            &mut req,
            "referer",
            pick(input.referer.as_deref(), self.referer.as_deref()),
        );
        put_fetched_at(&mut req, input.fetched_at);
        put(
            &mut req,
            "scriptHash",
            input
                .script
                .as_deref()
                .filter(|s| !s.is_empty())
                .map(script_hash)
                .as_deref(),
        );
        let wire: Wire = self.client.0.post("/interstitial", req).await?;
        if !wire.payload.starts_with("cid=") {
            return Err(Error::Decode(
                "datadome: service returned no ready-to-post form; it predates the form-body wire"
                    .into(),
            ));
        }
        self.learn(&wire.session, &wire.user_agent);
        Ok(InterstitialResponse {
            form_body: wire.payload,
            session: wire.session,
            user_agent: wire.user_agent,
        })
    }

    /// The `/captcha/check` URL to GET.
    pub async fn generate_slider(&mut self, input: SliderInput) -> Result<SliderResponse> {
        if input.captcha_url.is_empty() {
            return Err(Error::Input(
                "datadome: SliderInput.captcha_url is required: the captcha URL you loaded".into(),
            ));
        }
        require_script(&input.html, input.script.as_deref())?;
        let mut req = self.identity();
        req.insert("html".into(), input.html.into());
        req.insert("captchaUrl".into(), input.captcha_url.into());
        put(&mut req, "parentUrl", input.parent_url.as_deref());
        put_fetched_at(&mut req, input.fetched_at);
        put(
            &mut req,
            "scriptHash",
            input
                .script
                .as_deref()
                .filter(|s| !s.is_empty())
                .map(script_hash)
                .as_deref(),
        );
        let wire: Wire = self.client.0.post("/slider", req).await?;
        let Some(check_url) = normalize_check_url(&wire.payload) else {
            return Err(Error::Decode(
                "datadome: service returned no check URL; it predates the check-URL wire".into(),
            ));
        };
        self.learn(&wire.session, &wire.user_agent);
        Ok(SliderResponse {
            check_url,
            session: wire.session,
            user_agent: wire.user_agent,
        })
    }

    fn learn(&mut self, session: &str, user_agent: &str) {
        if !session.is_empty() {
            self.token = Some(session.to_string());
        }
        if self.user_agent.is_empty() && !user_agent.is_empty() {
            self.user_agent = user_agent.to_string();
        }
    }

    fn identity(&self) -> Map<String, Value> {
        let mut m = Map::new();
        if let Some(token) = &self.token {
            m.insert("session".into(), token.as_str().into());
            m.insert("sessionKey".into(), token.as_str().into());
        }
        put(&mut m, "device", self.device.as_deref());
        put(&mut m, "userAgent", Some(&self.user_agent));
        put(&mut m, "timezone", self.timezone.as_deref());
        if let Some(offset) = self.timezone_offset {
            m.insert("timezoneOffset".into(), offset.into());
        }
        m
    }
}

/// The first of `a`, `b` that is set and not empty.
fn pick<'a>(a: Option<&'a str>, b: Option<&'a str>) -> Option<&'a str> {
    a.filter(|s| !s.is_empty()).or(b.filter(|s| !s.is_empty()))
}

fn put_fetched_at(req: &mut Map<String, Value>, at: Option<SystemTime>) {
    if let Some(ms) = at.and_then(|t| t.duration_since(UNIX_EPOCH).ok()) {
        req.insert(
            "challengeFetchedAtMs".into(),
            (ms.as_millis() as u64).into(),
        );
    }
}

/// What the API takes of a page script: its SHA-256, as lowercase hex.
pub fn script_hash(script: &str) -> String {
    Sha256::digest(script.as_bytes())
        .iter()
        .map(|b| format!("{b:02x}"))
        .collect()
}

/// Refuses a page that loads an external script when none was passed.
fn require_script(html: &str, script: Option<&str>) -> Result<()> {
    match challenge_script_url(html) {
        Some(u) if script.is_none_or(str::is_empty) => Err(Error::Input(format!(
            "datadome: script is required: load {u} right after the page and pass its source"
        ))),
        _ => Ok(()),
    }
}
