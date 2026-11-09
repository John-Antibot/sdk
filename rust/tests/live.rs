//! Against the hosted API. Opt in: `JOHNANTIBOT_LIVE=1 JOHNANTIBOT_API_KEY=... cargo test --test live -- --ignored`.
//! `JOHNANTIBOT_ENDPOINT` overrides the API root. With `JOHNANTIBOT_LIVE_URL` and `JOHNANTIBOT_PROXY` it also clears a
//! Cloudflare challenge (a paid call).

use john_antibot::{account, cloudflare};

fn env(name: &str) -> Option<String> {
    std::env::var(name).ok().filter(|v| !v.is_empty())
}

/// The key, when the live run was asked for.
fn live_key() -> Option<String> {
    if env("JOHNANTIBOT_LIVE").as_deref() != Some("1") {
        eprintln!("set JOHNANTIBOT_LIVE=1 and JOHNANTIBOT_API_KEY to run against the API");
        return None;
    }
    Some(env("JOHNANTIBOT_API_KEY").expect("JOHNANTIBOT_API_KEY"))
}

#[tokio::test]
#[ignore = "calls the hosted API: set JOHNANTIBOT_LIVE=1 and JOHNANTIBOT_API_KEY"]
async fn live() {
    let Some(key) = live_key() else { return };
    let endpoint = env("JOHNANTIBOT_ENDPOINT").unwrap_or_default();

    let acct = account::Client::builder(&key)
        .endpoint(&endpoint)
        .build()
        .unwrap();
    let balance = acct.balance().await.expect("balance");
    eprintln!(
        "balance: metered {} ${:.4}",
        balance.metered,
        balance.balance_usd()
    );

    let (Some(url), Some(proxy)) = (env("JOHNANTIBOT_LIVE_URL"), env("JOHNANTIBOT_PROXY")) else {
        return;
    };
    let cf = cloudflare::Client::builder(&key)
        .endpoint(&endpoint)
        .build()
        .unwrap();
    let res = cf
        .solve_challenge(cloudflare::ChallengeInput {
            url,
            proxy,
            timezone: env("JOHNANTIBOT_TIMEZONE"),
            ..Default::default()
        })
        .await
        .expect("challenge");
    assert!(res.clearance().is_some_and(|c| !c.is_empty()), "{res:?}");
    assert!(!res.user_agent.is_empty());
}
