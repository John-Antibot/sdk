//! Client for the [John Antibot](https://antibot.johnavenue.com) DataDome, Cloudflare and reCAPTCHA APIs and your account's balance.
//!
//! | Module | |
//! |---|---|
//! | [`datadome`] | Tags, interstitial and slider payloads. |
//! | [`cloudflare`] | Cloudflare challenge, Turnstile and JSD solves, as jobs through your proxy. |
//! | [`recaptcha`] | reCAPTCHA v3 tokens, classic and Enterprise, as jobs through your proxy. |
//! | [`account`] | The balance of your key's account. |
//!
//! Every client is built from your API key, is cheap to clone and is `Send + Sync`. Every call returns [`Result`], whose
//! [`Error`] has the predicates (`is_hardblock`, `is_declined`, ...) that say what went wrong.
//!
//! ```no_run
//! use john_antibot::cloudflare;
//!
//! # async fn run() -> john_antibot::Result<()> {
//! let key = std::env::var("JOHNANTIBOT_API_KEY").unwrap();
//! let cf = cloudflare::Client::new(&key);
//! let res = cf
//!     .solve_challenge(cloudflare::ChallengeInput {
//!         url: "https://www.example.com/".into(),
//!         proxy: "http://user:pass@host:port".into(),
//!         timezone: Some("America/New_York".into()),
//!         ..Default::default()
//!     })
//!     .await?;
//! println!("{:?} {}", res.clearance(), res.user_agent);
//! # Ok(())
//! # }
//! ```
#![forbid(unsafe_code)]
#![warn(missing_docs)]

pub mod account;
pub mod cloudflare;
pub mod datadome;
mod http;
mod jobs;
pub mod recaptcha;

pub use http::{Builder, DEFAULT_ENDPOINT};
pub use jobs::Job;

/// The result of every call.
pub type Result<T, E = Error> = std::result::Result<T, E>;

/// Everything a call can fail with.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    /// A non-2xx reply of the API. `message` is its `error` text (else the body, else the status line).
    #[error("HTTP {status}: {message}")]
    Api {
        /// The HTTP status.
        status: u16,
        /// The API's error text.
        message: String,
        /// DataDome: the site blocked the session outright (`422`, `"hardblock": true`).
        hardblock: bool,
    },
    /// A job that ended without a result: `failed` (the price is returned) or `declined` (not charged; send another).
    #[error("job {id} {state}: {message}")]
    Job {
        /// The job's id.
        id: String,
        /// `failed`, `declined`, or `done` when the result is missing.
        state: String,
        /// The API's text.
        message: String,
        /// `true` when declined: a new job is a new attempt.
        retry: bool,
    },
    /// The input was refused before any request was sent.
    #[error("{0}")]
    Input(String),
    /// The request did not complete: connection, TLS, or the per-call timeout ([`Error::is_timeout`]).
    #[error("request: {0}")]
    Transport(#[source] reqwest::Error),
    /// The reply could not be read.
    #[error("{0}")]
    Decode(String),
}

impl Error {
    fn api_status(&self, want: u16) -> bool {
        matches!(self, Error::Api { status, .. } if *status == want)
    }

    fn job_says(&self, text: &str) -> bool {
        matches!(self, Error::Job { message, .. } if message.contains(text))
    }

    /// DataDome: the site blocked the session outright, with no slider to solve. Change the proxy and the session. Free.
    pub fn is_hardblock(&self) -> bool {
        matches!(self, Error::Api { hardblock, message, .. } if *hardblock || message.contains("hardblock"))
    }

    /// DataDome: a puzzle captcha, not a slider. Free.
    pub fn is_puzzle_unsupported(&self) -> bool {
        matches!(self, Error::Api { message, .. } if message.contains("puzzle slider unsupported"))
    }

    /// `429`: wait, then try again.
    pub fn is_rate_limited(&self) -> bool {
        self.api_status(429)
    }

    /// `402`: the balance is below the price.
    pub fn is_insufficient_balance(&self) -> bool {
        self.api_status(402)
    }

    /// `403`: the site is not approved for your key.
    pub fn is_site_not_approved(&self) -> bool {
        self.api_status(403)
    }

    /// `400`: the input was refused (the proxy, the user agent, a field).
    pub fn is_bad_request(&self) -> bool {
        self.api_status(400)
    }

    /// `401`: the key is missing, unknown or revoked, or its account is disabled.
    pub fn is_bad_key(&self) -> bool {
        self.api_status(401)
    }

    /// A job the site (or the API) declined. Not charged: send another.
    pub fn is_declined(&self) -> bool {
        matches!(self, Error::Job { state, .. } if state == "declined")
    }

    /// Cloudflare: the page serves no challenge.
    pub fn is_no_challenge(&self) -> bool {
        self.job_says("serves no challenge")
    }

    /// Cloudflare: the proxy exits from a different address on each connection. Use a sticky session or a static proxy.
    pub fn is_rotating_proxy(&self) -> bool {
        self.job_says("different address on each connection")
    }

    /// The per-call timeout ended the request.
    pub fn is_timeout(&self) -> bool {
        matches!(self, Error::Transport(e) if e.is_timeout())
    }
}
