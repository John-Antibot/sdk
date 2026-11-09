use std::io::Write;
use std::marker::PhantomData;
use std::time::Duration;

use flate2::Compression;
use flate2::write::GzEncoder;
use reqwest::Method;
use serde::de::DeserializeOwned;
use serde_json::{Map, Value};

use crate::{Error, Result};

/// The hosted API, used when no endpoint is set.
pub const DEFAULT_ENDPOINT: &str = "https://antibot.johnavenue.com";

/// Request bodies from this size up are gzipped.
const COMPRESS_ABOVE: usize = 32 << 10;

/// Builds a client: `datadome::Client::builder(&key).org_key(..).endpoint(..).timeout(..).build()?`.
#[must_use]
pub struct Builder<C> {
    api_key: String,
    org_key: Option<String>,
    endpoint: Option<String>,
    timeout: Option<Duration>,
    http: Option<reqwest::Client>,
    _client: PhantomData<fn() -> C>,
}

impl<C> Builder<C> {
    pub(crate) fn new(api_key: &str) -> Self {
        Builder {
            api_key: api_key.to_string(),
            org_key: None,
            endpoint: None,
            timeout: None,
            http: None,
            _client: PhantomData,
        }
    }

    /// Sent as `x-org-key`: software distributed under an organization sends its key next to each user's own API key.
    pub fn org_key(mut self, key: impl Into<String>) -> Self {
        self.org_key = Some(key.into()).filter(|k| !k.is_empty());
        self
    }

    /// The API root, for example `https://antibot.johnavenue.com` (the default).
    pub fn endpoint(mut self, url: impl Into<String>) -> Self {
        self.endpoint = Some(url.into());
        self
    }

    /// The deadline of one HTTP call (not of a solve: wrap the call in `tokio::time::timeout` for that).
    pub fn timeout(mut self, timeout: Duration) -> Self {
        self.timeout = Some(timeout).filter(|t| !t.is_zero());
        self
    }

    /// The HTTP client for calls to the API (its proxy, TLS settings). It carries your calls to the API, not the solve.
    pub fn http_client(mut self, client: reqwest::Client) -> Self {
        self.http = Some(client);
        self
    }

    pub(crate) fn core(
        self,
        service: &'static str,
        prefix: &'static str,
        default_timeout: Duration,
    ) -> Result<Core> {
        let endpoint = self.endpoint.unwrap_or_default();
        let endpoint = match endpoint.trim_end_matches('/') {
            "" => DEFAULT_ENDPOINT.to_string(),
            e => e.to_string(),
        };
        match reqwest::Url::parse(&endpoint) {
            Ok(u) if u.scheme() == "http" || u.scheme() == "https" => {}
            _ => {
                return Err(Error::Input(format!(
                    "{service}: endpoint {endpoint:?} is not an http(s) URL"
                )));
            }
        }
        let http = match self.http {
            Some(c) => c,
            None => reqwest::Client::builder()
                .build()
                .map_err(Error::Transport)?,
        };
        Ok(Core {
            http,
            endpoint,
            prefix,
            service,
            api_key: self.api_key,
            org_key: self.org_key,
            timeout: self.timeout.unwrap_or(default_timeout),
        })
    }
}

/// What every client shares: the connection to the API.
#[derive(Debug)]
pub(crate) struct Core {
    http: reqwest::Client,
    pub(crate) endpoint: String,
    prefix: &'static str,
    pub(crate) service: &'static str,
    api_key: String,
    org_key: Option<String>,
    timeout: Duration,
}

impl Core {
    pub(crate) async fn get<T: DeserializeOwned>(&self, path: &str) -> Result<T> {
        self.call(Method::GET, path, None).await
    }

    pub(crate) async fn post<T: DeserializeOwned>(
        &self,
        path: &str,
        body: Map<String, Value>,
    ) -> Result<T> {
        self.call(Method::POST, path, Some(body)).await
    }

    async fn call<T: DeserializeOwned>(
        &self,
        method: Method,
        path: &str,
        body: Option<Map<String, Value>>,
    ) -> Result<T> {
        let url = format!("{}{}{}", self.endpoint, self.prefix, path);
        let mut req = self.http.request(method, url).timeout(self.timeout);
        if !self.api_key.is_empty() {
            req = req.header("x-api-key", &self.api_key);
        }
        if let Some(org) = &self.org_key {
            req = req.header("x-org-key", org);
        }
        if let Some(body) = body {
            let mut bytes = serde_json::to_vec(&body)
                .map_err(|e| Error::Input(format!("{}: encode request: {e}", self.service)))?;
            req = req.header("content-type", "application/json");
            if bytes.len() >= COMPRESS_ABOVE {
                bytes = gzip(&bytes);
                req = req.header("content-encoding", "gzip");
            }
            req = req.body(bytes);
        }
        let res = req.send().await.map_err(Error::Transport)?;
        let status = res.status();
        let data = res.bytes().await.map_err(Error::Transport)?;
        if !status.is_success() {
            let failure: Value = serde_json::from_slice(&data).unwrap_or(Value::Null);
            let mut message = failure
                .get("error")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .to_string();
            if message.is_empty() {
                message = String::from_utf8_lossy(&data).trim().to_string();
            }
            if message.is_empty() {
                message = status.to_string();
            }
            let hardblock = failure
                .get("hardblock")
                .and_then(Value::as_bool)
                .unwrap_or(false);
            return Err(Error::Api {
                status: status.as_u16(),
                message,
                hardblock,
            });
        }
        serde_json::from_slice(&data)
            .map_err(|e| Error::Decode(format!("{}: decode response: {e}", self.service)))
    }
}

fn gzip(data: &[u8]) -> Vec<u8> {
    let mut enc = GzEncoder::new(Vec::with_capacity(data.len() / 2), Compression::fast());
    // (writing to a Vec cannot fail)
    enc.write_all(data).expect("gzip to memory");
    enc.finish().expect("gzip to memory")
}

/// Puts `value` under `key` unless it is absent or empty: an empty optional is left out.
pub(crate) fn put(body: &mut Map<String, Value>, key: &str, value: Option<&str>) {
    if let Some(v) = value.filter(|v| !v.is_empty()) {
        body.insert(key.to_string(), Value::from(v));
    }
}

/// A job's wall in the API's unit; zero is left out (the API's default).
pub(crate) fn put_millis(body: &mut Map<String, Value>, timeout: Option<Duration>) {
    if let Some(t) = timeout.filter(|t| !t.is_zero()) {
        body.insert("timeoutMs".into(), Value::from(t.as_millis() as u64));
    }
}

/// `s` with every byte but `A-Z a-z 0-9 - _ . ~` percent-encoded; a space is `+` when `plus` (a query) and `%20` otherwise (a path).
pub(crate) fn escape(s: &str, plus: bool) -> String {
    let mut out = String::with_capacity(s.len());
    for b in s.bytes() {
        match b {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                out.push(b as char)
            }
            b' ' if plus => out.push('+'),
            _ => out.push_str(&format!("%{b:02X}")),
        }
    }
    out
}
