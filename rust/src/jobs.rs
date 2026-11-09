use std::time::Duration;

use serde::Deserialize;
use serde::de::DeserializeOwned;
use serde_json::{Map, Value};

use crate::http::{Core, escape};
use crate::{Error, Result};

/// How long a poll asks the server to hold it for a job that has not ended (the server's most is 25 seconds).
const POLL_WAIT: Duration = Duration::from_secs(25);

/// How many polls in a row may fail (the network, a 5xx) before `wait` gives the job up. The job runs on, and `job` can ask again.
const POLL_RETRIES: u32 = 3;

/// A job as the API reports it. `R` is the service's result: [`cloudflare::Solution`](crate::cloudflare::Solution) or
/// [`recaptcha::Solution`](crate::recaptcha::Solution).
#[derive(Debug, Clone, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Job<R> {
    /// The job's id.
    #[serde(default)]
    pub id: String,
    /// `queued`, `running`, `done`, `failed` or `declined`.
    #[serde(default)]
    pub state: String,
    /// The task: `challenge`, `turnstile`, `jsd`, `v3`, `v3-enterprise`.
    #[serde(default)]
    pub task: String,
    /// When failed or declined: why.
    #[serde(default)]
    pub error: String,
    /// When declined: the attempt was turned down; send a new job.
    #[serde(default)]
    pub retry: bool,
    /// When done: the result.
    pub result: Option<R>,
    /// When done: the API no longer holds the result.
    #[serde(default)]
    pub result_expired: bool,
    /// The proxy the job runs through, without its credentials.
    pub proxy: Option<String>,
    /// When the job was sent (RFC 3339).
    pub created_at: Option<String>,
    /// When the job started (RFC 3339).
    pub started_at: Option<String>,
    /// When the job ended (RFC 3339).
    pub finished_at: Option<String>,
}

impl<R> Job<R> {
    /// The job has ended.
    pub fn is_terminal(&self) -> bool {
        self.state != "queued" && self.state != "running"
    }

    /// The end of a terminal job: its result, or why it has none.
    fn outcome(self) -> Result<R> {
        let message = match (self.state.as_str(), self.result) {
            ("done", Some(r)) => return Ok(r),
            ("done", None) if self.result_expired => {
                "the job is done but the API no longer holds its result".to_string()
            }
            ("done", None) => "the job is done without a result".to_string(),
            _ => self.error,
        };
        Err(Error::Job {
            id: self.id,
            state: self.state,
            message,
            retry: self.retry,
        })
    }
}

pub(crate) async fn submit<R: DeserializeOwned>(
    core: &Core,
    path: &str,
    body: Map<String, Value>,
) -> Result<Job<R>> {
    let job: Job<R> = core.post(path, body).await?;
    if job.id.is_empty() {
        return Err(Error::Decode(format!(
            "{}: the API answered without a job id",
            core.service
        )));
    }
    Ok(job)
}

pub(crate) async fn get<R: DeserializeOwned>(
    core: &Core,
    id: &str,
    wait: Duration,
) -> Result<Job<R>> {
    if id.is_empty() {
        return Err(Error::Input(format!(
            "{}: a job id is required",
            core.service
        )));
    }
    let mut path = format!("/jobs/{}", escape(id, false));
    if !wait.is_zero() {
        path += &format!("?wait={}", wait.as_millis().div_ceil(1000));
    }
    core.get(&path).await
}

pub(crate) async fn wait<R: DeserializeOwned + Clone>(core: &Core, job: &Job<R>) -> Result<R> {
    if job.is_terminal() {
        return job.clone().outcome();
    }
    let mut failures = 0;
    loop {
        match get::<R>(core, &job.id, POLL_WAIT).await {
            Ok(next) if next.is_terminal() => return next.outcome(),
            Ok(_) => failures = 0,
            Err(e) if retryable(&e) && failures < POLL_RETRIES => {
                failures += 1;
                tokio::time::sleep(Duration::from_secs(1)).await;
            }
            Err(e) => return Err(e),
        }
    }
}

/// A poll that failed for a reason that may not be there next time: the network, or a server that had a bad moment.
fn retryable(err: &Error) -> bool {
    match err {
        Error::Api { status, .. } => *status >= 500,
        Error::Input(_) => false,
        _ => true,
    }
}
