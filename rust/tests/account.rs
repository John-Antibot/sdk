mod common;

use std::time::{Duration, Instant};

use common::{Mock, reply, serve, slow};
use john_antibot::account::{self, Balance};
use john_antibot::{DEFAULT_ENDPOINT, Error};

async fn serve_once(status: u16, body: &'static str) -> (account::Client, Mock) {
    let m = serve(move |_, _| reply(status, body)).await;
    (
        account::Client::builder("key")
            .endpoint(format!("{}/", m.url))
            .build()
            .unwrap(),
        m,
    )
}

#[tokio::test]
async fn balance_reads_the_account_of_the_key() {
    let (c, m) = serve_once(200, r#"{"metered":true,"balanceMicros":4821000}"#).await;
    let b = c.balance().await.unwrap();
    assert_eq!(
        b,
        Balance {
            metered: true,
            balance_micros: 4_821_000
        }
    );
    assert_eq!(b.balance_usd(), 4.821);
    // a GET of /v1/balance, with the key in the header the API reads
    let req = m.last();
    assert_eq!(
        (req.method.as_str(), req.path.as_str(), req.query.as_deref()),
        ("GET", "/v1/balance", None)
    );
    assert_eq!(
        (
            req.header("x-api-key"),
            req.header("x-org-key"),
            req.header("content-type")
        ),
        (Some("key"), None, None)
    );
    assert!(req.body.is_empty());
}

#[tokio::test]
async fn a_key_that_is_not_metered_has_no_balance() {
    let (c, _m) = serve_once(200, r#"{"metered":false}"#).await;
    assert_eq!(
        c.balance().await.unwrap(),
        Balance {
            metered: false,
            balance_micros: 0
        }
    );
}

#[tokio::test]
async fn refusals_are_typed_and_keep_the_apis_text() {
    let (c, _m) = serve_once(401, r#"{"error":"gateway: missing or bad x-api-key"}"#).await;
    let err = c.balance().await.unwrap_err();
    assert!(err.is_bad_key(), "{err}");
    assert!(
        matches!(&err, Error::Api { status: 401, message, .. } if message == "gateway: missing or bad x-api-key")
    );
    // other failures are not a bad key
    let (c, _m) = serve_once(503, "We cannot load the billing data.").await;
    let err = c.balance().await.unwrap_err();
    assert!(!err.is_bad_key());
    assert!(
        matches!(&err, Error::Api { status: 503, message, .. } if message == "We cannot load the billing data.")
    );
}

#[tokio::test]
async fn a_default_endpoint_and_a_timeout() {
    assert_eq!(account::Client::new("k").endpoint(), DEFAULT_ENDPOINT);
    let m = serve(|_, _| slow(Duration::from_secs(10))).await;
    let c = account::Client::builder("k")
        .endpoint(&m.url)
        .timeout(Duration::from_millis(100))
        .build()
        .unwrap();
    let start = Instant::now();
    let err = c.balance().await.unwrap_err();
    assert!(
        err.is_timeout() && start.elapsed() < Duration::from_secs(3),
        "{err:?}"
    );
}
