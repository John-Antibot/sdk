//! Cloudflare: clear a challenge page, solve a Turnstile widget or earn a JSD clearance, as a job through your proxy.
//!
//! ```no_run
//! use john_antibot::cloudflare;
//!
//! # async fn run() -> john_antibot::Result<()> {
//! let cf = cloudflare::Client::new(&std::env::var("JOHNANTIBOT_API_KEY").unwrap());
//! let res = cf
//!     .solve_challenge(cloudflare::ChallengeInput {
//!         url: "https://www.example.com/".into(),
//!         proxy: "http://user:pass@host:port".into(),
//!         timezone: Some("America/New_York".into()),
//!         ..Default::default()
//!     })
//!     .await?;
//! println!("{:?} {} {}", res.clearance(), res.user_agent, res.exit_ip);
//! # Ok(())
//! # }
//! ```

use std::sync::Arc;
use std::time::Duration;

use serde::Deserialize;
use serde_json::Map;

use crate::http::{Builder, Core, put, put_millis};
use crate::{Error, Result, jobs};

const DEFAULT_TIMEOUT: Duration = Duration::from_secs(40);

/// A Cloudflare job.
pub type Job = crate::Job<Solution>;

/// Sends API calls. Cheap to clone; safe to share across tasks.
#[derive(Debug, Clone)]
pub struct Client(Arc<Core>);

impl Builder<Client> {
    /// Builds the client.
    pub fn build(self) -> Result<Client> {
        Ok(Client(Arc::new(self.core(
            "cloudflare",
            "/v1/cloudflare",
            DEFAULT_TIMEOUT,
        )?)))
    }
}

/// A page that serves a Cloudflare challenge.
#[derive(Debug, Clone, Default)]
pub struct ChallengeInput {
    /// The page that serves the challenge. The site must be approved for your key.
    pub url: String,
    /// The proxy the solve goes through, in any form a provider prints (`http://user:pass@host:port`, `socks5://...`,
    /// `host:port:user:pass`, ...). It must have a public address and keep it: the clearance is for the address it exits from.
    pub proxy: String,
    /// Your client's user agent, to get the clearance for it (the exact user agent of a Chrome build on macOS the API runs).
    /// Absent: use [`Solution::user_agent`].
    pub user_agent: Option<String>,
    /// The IANA zone of the proxy's location, for example `America/New_York`.
    pub timezone: Option<String>,
    /// Names the device: the same session is the same device.
    pub session: Option<String>,
    /// The job's wall, from 10 to 120 seconds; absent is 60.
    pub timeout: Option<Duration>,
}

/// A page whose JavaScript detections grant a `cf_clearance`: the fields of a challenge, with `url` a page the detection script is
/// appended to.
pub type JsdInput = ChallengeInput;

/// A Turnstile widget on a page.
#[derive(Debug, Clone, Default)]
pub struct TurnstileInput {
    /// The page with the widget. The site must be approved for your key.
    pub page_url: String,
    /// The widget's site key.
    pub site_key: String,
    /// As in [`ChallengeInput::proxy`].
    pub proxy: String,
    /// The widget's `data-action`.
    pub action: Option<String>,
    /// The widget's `data-cdata`.
    pub c_data: Option<String>,
    /// The widget's `chlPageData`.
    pub chl_page_data: Option<String>,
    /// As in [`ChallengeInput::user_agent`].
    pub user_agent: Option<String>,
    /// As in [`ChallengeInput::timezone`].
    pub timezone: Option<String>,
    /// As in [`ChallengeInput::session`].
    pub session: Option<String>,
    /// As in [`ChallengeInput::timeout`].
    pub timeout: Option<Duration>,
}

/// A cookie the solve ended with.
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
#[serde(default)]
pub struct Cookie {
    /// Its name.
    pub name: String,
    /// Its value.
    pub value: String,
    /// Its domain.
    pub domain: String,
    /// Its path.
    pub path: String,
}

/// What a finished job holds.
#[derive(Debug, Clone, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct Solution {
    /// `cf_clearance=<value>` for a challenge or JSD; the token for Turnstile.
    pub payload: String,
    /// Send it in each request to the site, from the same proxy.
    pub user_agent: String,
    /// The device the solve ran as: send it as `session` in the next solve for the same site session.
    pub session: String,
    /// The cookies a browser on the page would send now.
    pub cookies: Vec<Cookie>,
    /// The address the site saw.
    pub exit_ip: String,
    /// How long the solve took, in milliseconds.
    pub ms: i64,
}

impl Solution {
    /// The `cf_clearance` value, from the payload or the cookies.
    pub fn clearance(&self) -> Option<&str> {
        self.payload.strip_prefix("cf_clearance=").or_else(|| {
            self.cookies
                .iter()
                .find(|c| c.name == "cf_clearance")
                .map(|c| c.value.as_str())
        })
    }

    /// The Turnstile token, for the form field `cf-turnstile-response`.
    pub fn token(&self) -> &str {
        &self.payload
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

    /// Clears the challenge page and waits for the clearance.
    pub async fn solve_challenge(&self, input: ChallengeInput) -> Result<Solution> {
        let job = self.submit_challenge(input).await?;
        self.wait(&job).await
    }

    /// Solves the Turnstile widget and waits for the token.
    pub async fn solve_turnstile(&self, input: TurnstileInput) -> Result<Solution> {
        let job = self.submit_turnstile(input).await?;
        self.wait(&job).await
    }

    /// Runs the page's detection script and waits for the clearance.
    pub async fn solve_jsd(&self, input: JsdInput) -> Result<Solution> {
        let job = self.submit_jsd(input).await?;
        self.wait(&job).await
    }

    /// Sends a challenge job and returns it queued. The price is charged now and returned if the job fails or is declined.
    pub async fn submit_challenge(&self, input: ChallengeInput) -> Result<Job> {
        if input.url.is_empty() || input.proxy.is_empty() {
            return Err(Error::Input(
                "cloudflare: a challenge needs a URL and a proxy".into(),
            ));
        }
        jobs::submit(&self.0, "/challenge", page_body(&input)).await
    }

    /// Sends a JSD job and returns it queued.
    pub async fn submit_jsd(&self, input: JsdInput) -> Result<Job> {
        if input.url.is_empty() || input.proxy.is_empty() {
            return Err(Error::Input(
                "cloudflare: a jsd solve needs a URL and a proxy".into(),
            ));
        }
        jobs::submit(&self.0, "/jsd", page_body(&input)).await
    }

    /// Sends a Turnstile job and returns it queued.
    pub async fn submit_turnstile(&self, input: TurnstileInput) -> Result<Job> {
        if input.page_url.is_empty() || input.site_key.is_empty() || input.proxy.is_empty() {
            return Err(Error::Input(
                "cloudflare: a turnstile solve needs a page URL, a site key and a proxy".into(),
            ));
        }
        let mut body = Map::new();
        put(&mut body, "pageUrl", Some(&input.page_url));
        put(&mut body, "sitekey", Some(&input.site_key));
        put(&mut body, "proxy", Some(&input.proxy));
        put(&mut body, "action", input.action.as_deref());
        put(&mut body, "cData", input.c_data.as_deref());
        put(&mut body, "chlPageData", input.chl_page_data.as_deref());
        put(&mut body, "userAgent", input.user_agent.as_deref());
        put(&mut body, "timezone", input.timezone.as_deref());
        put(&mut body, "session", input.session.as_deref());
        put_millis(&mut body, input.timeout);
        jobs::submit(&self.0, "/turnstile", body).await
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
}

/// The body of a challenge or JSD job.
fn page_body(input: &ChallengeInput) -> Map<String, serde_json::Value> {
    let mut body = Map::new();
    put(&mut body, "url", Some(&input.url));
    put(&mut body, "proxy", Some(&input.proxy));
    put(&mut body, "userAgent", input.user_agent.as_deref());
    put(&mut body, "timezone", input.timezone.as_deref());
    put(&mut body, "session", input.session.as_deref());
    put_millis(&mut body, input.timeout);
    body
}
