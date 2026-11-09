//! reCAPTCHA v3 tokens, classic and Enterprise, as a job through your proxy.
//!
//! ```no_run
//! use john_antibot::recaptcha;
//!
//! # async fn run() -> john_antibot::Result<()> {
//! let rc = recaptcha::Client::new(&std::env::var("JOHNANTIBOT_API_KEY").unwrap());
//! let res = rc
//!     .solve_v3(recaptcha::Input {
//!         url: "https://www.example.com/login".into(),
//!         site_key: "6Lc...".into(),
//!         action: Some("login".into()),
//!         proxy: "http://user:pass@host:port".into(),
//!         ..Default::default()
//!     })
//!     .await?;
//! println!("{} {:?}", res.token, res.headers());
//! # Ok(())
//! # }
//! ```

use std::sync::Arc;
use std::time::Duration;

use reqwest::header::{HeaderMap, HeaderName, HeaderValue};
use serde::Deserialize;
use serde_json::Map;

use crate::http::{Builder, Core, put, put_millis};
use crate::{Error, Result, jobs};

const DEFAULT_TIMEOUT: Duration = Duration::from_secs(40);

/// A reCAPTCHA job.
pub type Job = crate::Job<Solution>;

/// Sends API calls. Cheap to clone; safe to share across tasks.
#[derive(Debug, Clone)]
pub struct Client(Arc<Core>);

impl Builder<Client> {
    /// Builds the client.
    pub fn build(self) -> Result<Client> {
        Ok(Client(Arc::new(self.core(
            "recaptcha",
            "/v1/recaptcha",
            DEFAULT_TIMEOUT,
        )?)))
    }
}

/// A page that uses reCAPTCHA v3, and the proxy the token is made through.
#[derive(Debug, Clone, Default)]
pub struct Input {
    /// The page that asks for the token. The site must be approved for your key.
    pub url: String,
    /// The site's reCAPTCHA key.
    pub site_key: String,
    /// What the page passes to `execute`, for example `login`; absent is `submit`.
    pub action: Option<String>,
    /// The proxy the solve goes through, in any form a provider prints. It must have a public address: send the token from it.
    pub proxy: String,
    /// The job's wall, from 10 seconds to the most the API allows (60 on the hosted API); absent is 30.
    pub timeout: Option<Duration>,
}

/// What a finished job holds: the token, and the browser it was made as.
#[derive(Debug, Clone, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct Solution {
    /// The `g-recaptcha-response` value, or what the page's `execute` call would have returned.
    pub token: String,
    /// When Google stops accepting the token (RFC 3339); `None` when the API gave none.
    pub expires_at: Option<String>,
    /// The browser's user agent.
    pub user_agent: String,
    /// The browser's `accept-language`.
    pub accept_language: String,
    /// `sec-ch-ua`.
    pub sec_ua: String,
    /// `sec-ch-ua-mobile`.
    pub sec_ua_mobile: String,
    /// `sec-ch-ua-platform`.
    pub sec_ua_platform: String,
}

impl Solution {
    /// `user-agent`, `accept-language` and the client hints of the browser the token was made as: send them in the request that
    /// carries the token. A header without a value is left out.
    pub fn headers(&self) -> HeaderMap {
        let mut h = HeaderMap::new();
        for (name, value) in [
            ("user-agent", &self.user_agent),
            ("accept-language", &self.accept_language),
            ("sec-ch-ua", &self.sec_ua),
            ("sec-ch-ua-mobile", &self.sec_ua_mobile),
            ("sec-ch-ua-platform", &self.sec_ua_platform),
        ] {
            match HeaderValue::from_str(value) {
                Ok(v) if !value.is_empty() => _ = h.insert(HeaderName::from_static(name), v),
                _ => {}
            }
        }
        h
    }
}

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

    /// A builder: `org_key`, `endpoint`, `timeout` (one call; default 40 s), `http_client`.
    pub fn builder(api_key: &str) -> Builder<Client> {
        Builder::new(api_key)
    }

    /// The API root this client calls.
    pub fn endpoint(&self) -> &str {
        &self.0.endpoint
    }

    /// A token for a page that loads the classic script (`api.js`), waited for.
    pub async fn solve_v3(&self, input: Input) -> Result<Solution> {
        let job = self.submit_v3(input).await?;
        self.wait(&job).await
    }

    /// A token for a page that loads reCAPTCHA Enterprise (`enterprise.js`), waited for.
    pub async fn solve_v3_enterprise(&self, input: Input) -> Result<Solution> {
        let job = self.submit_v3_enterprise(input).await?;
        self.wait(&job).await
    }

    /// Sends a classic v3 job and returns it queued. The price is charged now and returned if the job fails.
    pub async fn submit_v3(&self, input: Input) -> Result<Job> {
        self.submit("/v3", input).await
    }

    /// Sends an Enterprise job and returns it queued.
    pub async fn submit_v3_enterprise(&self, input: Input) -> Result<Job> {
        self.submit("/v3-enterprise", input).await
    }

    /// Reads a job. With `wait` above zero the call returns as soon as the job has ended, or after `wait` (up to 25 seconds)
    /// with the state it has.
    pub async fn job(&self, id: &str, wait: Duration) -> Result<Job> {
        jobs::get(&self.0, id, wait).await
    }

    /// Follows a job until it ends. A job that failed or was declined is an [`Error::Job`]. Dropping the future ends the wait,
    /// not the job.
    pub async fn wait(&self, job: &Job) -> Result<Solution> {
        jobs::wait(&self.0, job).await
    }

    async fn submit(&self, route: &str, input: Input) -> Result<Job> {
        if input.url.is_empty() || input.site_key.is_empty() || input.proxy.is_empty() {
            return Err(Error::Input(
                "recaptcha: a solve needs a URL, a site key and a proxy".into(),
            ));
        }
        let mut body = Map::new();
        put(&mut body, "url", Some(&input.url));
        put(&mut body, "siteKey", Some(&input.site_key));
        put(&mut body, "proxy", Some(&input.proxy));
        put(&mut body, "action", input.action.as_deref());
        put_millis(&mut body, input.timeout);
        jobs::submit(&self.0, route, body).await
    }
}
