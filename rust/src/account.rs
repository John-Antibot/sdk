//! The balance your key is charged against. Each paid call takes its price when it starts and gets a `402` when the balance is less.
//!
//! ```no_run
//! # async fn run() -> john_antibot::Result<()> {
//! let account = john_antibot::account::Client::new(&std::env::var("JOHNANTIBOT_API_KEY").unwrap());
//! let balance = account.balance().await?;
//! println!("${:.2}", balance.balance_usd());
//! # Ok(())
//! # }
//! ```

use std::sync::Arc;
use std::time::Duration;

use serde::Deserialize;

use crate::Result;
use crate::http::{Builder, Core};

const DEFAULT_TIMEOUT: Duration = Duration::from_secs(20);

/// Sends API calls. Cheap to clone; safe to share across tasks.
#[derive(Debug, Clone)]
pub struct Client(Arc<Core>);

impl Builder<Client> {
    /// Builds the client.
    pub fn build(self) -> Result<Client> {
        Ok(Client(Arc::new(self.core(
            "account",
            "/v1",
            DEFAULT_TIMEOUT,
        )?)))
    }
}

/// What the account of a key has to spend.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Deserialize)]
#[serde(default)]
pub struct Balance {
    /// The key is charged. A key that is not metered has no account: `balance_micros` is zero.
    pub metered: bool,
    /// The balance in millionths of a dollar, the unit of a price in the catalog (2000 is $0.002).
    #[serde(rename = "balanceMicros")]
    pub balance_micros: i64,
}

impl Balance {
    /// The balance in dollars, for display.
    pub fn balance_usd(&self) -> f64 {
        self.balance_micros as f64 / 1e6
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

    /// A builder: `endpoint`, `timeout` (one call; default 20 s), `http_client`.
    pub fn builder(api_key: &str) -> Builder<Client> {
        Builder::new(api_key)
    }

    /// The API root this client calls.
    pub fn endpoint(&self) -> &str {
        &self.0.endpoint
    }

    /// The balance of the key's account. Free. A bad key is [`Error::is_bad_key`](crate::Error::is_bad_key).
    pub async fn balance(&self) -> Result<Balance> {
        self.0.get("/balance").await
    }
}
