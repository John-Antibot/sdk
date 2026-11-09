mod common;

use std::time::{Duration, Instant};

use common::{Mock, Reply, Request, hang_up, keys, reply, serve, slow};
use john_antibot::Error;
use john_antibot::cloudflare::{
    self, ChallengeInput, Cookie, Job, JsdInput, Solution, TurnstileInput,
};

const DONE_JOB: &str = r#"{"id":"job_1","state":"done","task":"challenge","createdAt":"2026-10-23T10:00:00Z","proxy":"8.8.8.8:80","result":{"payload":"cf_clearance=abc123","userAgent":"UA/1","session":"sess_x","exitIp":"9.9.9.9","ms":7000,"cookies":[{"name":"cf_clearance","value":"abc123","domain":"example.com","path":"/"},{"name":"__cf_bm","value":"bm","domain":"example.com","path":"/"}]}}"#;

/// The API: a submit answers 202 with job_1, the first poll says running, the next says `terminal`.
async fn api(terminal: &'static str) -> Mock {
    serve(
        move |r: &Request, n| match (r.method.as_str(), r.path.as_str()) {
            ("POST", _) => reply(202, r#"{"id":"job_1","state":"queued"}"#),
            ("GET", "/v1/cloudflare/jobs/job_1") if n == 1 => {
                reply(200, r#"{"id":"job_1","state":"running"}"#)
            }
            ("GET", "/v1/cloudflare/jobs/job_1") => reply(200, terminal),
            _ => reply(404, r#"{"error":"not found"}"#),
        },
    )
    .await
}

fn client(m: &Mock) -> cloudflare::Client {
    cloudflare::Client::builder("key")
        .org_key("org")
        .endpoint(format!("{}/", m.url))
        .build()
        .unwrap()
}

fn challenge() -> ChallengeInput {
    ChallengeInput {
        url: "https://www.example.com/".into(),
        proxy: "8.8.8.8:80".into(),
        ..Default::default()
    }
}

#[tokio::test]
async fn solve_challenge_submits_and_waits() {
    let m = api(DONE_JOB).await;
    let res = client(&m)
        .solve_challenge(ChallengeInput {
            url: "https://www.example.com/".into(),
            proxy: "u:p@8.8.8.8:80".into(),
            timezone: Some("America/New_York".into()),
            timeout: Some(Duration::from_secs(90)),
            ..Default::default()
        })
        .await
        .unwrap();
    assert_eq!(res.clearance(), Some("abc123"));
    assert_eq!(
        (
            res.user_agent.as_str(),
            res.exit_ip.as_str(),
            res.session.as_str(),
            res.ms
        ),
        ("UA/1", "9.9.9.9", "sess_x", 7000)
    );
    assert_eq!(
        res.cookies[1],
        Cookie {
            name: "__cf_bm".into(),
            value: "bm".into(),
            domain: "example.com".into(),
            path: "/".into()
        }
    );
    let reqs = m.requests();
    assert_eq!(reqs.len(), 3);
    let submit = &reqs[0];
    assert_eq!(
        (
            submit.method.as_str(),
            submit.path.as_str(),
            submit.query.as_deref()
        ),
        ("POST", "/v1/cloudflare/challenge", None)
    );
    assert_eq!(
        (submit.header("x-api-key"), submit.header("x-org-key")),
        (Some("key"), Some("org"))
    );
    assert_eq!(
        (
            submit.header("content-type"),
            submit.header("content-encoding")
        ),
        (Some("application/json"), None)
    );
    // the fields given, none of the ones left empty
    assert_eq!(
        submit.keys(),
        keys(&["proxy", "timeoutMs", "timezone", "url"])
    );
    let got = submit.json();
    assert_eq!(
        (got["url"].as_str(), got["proxy"].as_str()),
        (Some("https://www.example.com/"), Some("u:p@8.8.8.8:80"))
    );
    assert_eq!(
        (got["timezone"].as_str(), got["timeoutMs"].as_u64()),
        (Some("America/New_York"), Some(90000))
    );
    // it polled with the server's wait, twice (running, then done), with the same keys
    for poll in &reqs[1..] {
        assert_eq!(
            (
                poll.method.as_str(),
                poll.path.as_str(),
                poll.query.as_deref()
            ),
            ("GET", "/v1/cloudflare/jobs/job_1", Some("wait=25"))
        );
        assert_eq!(
            (poll.header("x-api-key"), poll.header("x-org-key")),
            (Some("key"), Some("org"))
        );
        assert!(poll.body.is_empty());
    }
}

#[tokio::test]
async fn solve_turnstile() {
    let m = api(r#"{"id":"job_1","state":"done","task":"turnstile","result":{"payload":"0.token","userAgent":"UA/1"}}"#).await;
    let c = client(&m);
    let res = c
        .solve_turnstile(TurnstileInput {
            page_url: "https://www.example.com/login".into(),
            site_key: "0x4AAA".into(),
            proxy: "8.8.8.8:80".into(),
            action: Some("login".into()),
            ..Default::default()
        })
        .await
        .unwrap();
    assert_eq!((res.token(), res.clearance()), ("0.token", None));
    let submit = &m.requests()[0];
    assert_eq!(submit.path, "/v1/cloudflare/turnstile");
    assert_eq!(
        submit.keys(),
        keys(&["action", "pageUrl", "proxy", "sitekey"])
    );
    assert_eq!(
        (
            submit.json()["pageUrl"].as_str(),
            submit.json()["sitekey"].as_str()
        ),
        (Some("https://www.example.com/login"), Some("0x4AAA"))
    );

    // every optional field, by its wire name
    c.submit_turnstile(TurnstileInput {
        page_url: "https://www.example.com/login".into(),
        site_key: "0x4AAA".into(),
        proxy: "8.8.8.8:80".into(),
        action: Some("login".into()),
        c_data: Some("cd".into()),
        chl_page_data: Some("pd".into()),
        user_agent: Some("UA/1".into()),
        timezone: Some("Europe/Paris".into()),
        session: Some("s-1".into()),
        timeout: Some(Duration::from_millis(15500)),
    })
    .await
    .unwrap();
    let got = m.last();
    assert_eq!(
        got.keys(),
        keys(&[
            "action",
            "cData",
            "chlPageData",
            "pageUrl",
            "proxy",
            "session",
            "sitekey",
            "timeoutMs",
            "timezone",
            "userAgent"
        ])
    );
    assert_eq!(
        (
            got.json()["cData"].as_str(),
            got.json()["chlPageData"].as_str()
        ),
        (Some("cd"), Some("pd"))
    );
    assert_eq!(got.json()["timeoutMs"], 15500);
}

#[tokio::test]
async fn solve_jsd() {
    let m = api(r#"{"id":"job_1","state":"done","task":"jsd","result":{"payload":"cf_clearance=jsd1","userAgent":"UA/1","exitIp":"9.9.9.9"}}"#).await;
    let res = client(&m)
        .solve_jsd(JsdInput {
            url: "https://www.example.com/".into(),
            proxy: "8.8.8.8:80".into(),
            user_agent: Some("UA/1".into()),
            session: Some("s-1".into()),
            ..Default::default()
        })
        .await
        .unwrap();
    assert_eq!(
        (res.clearance(), res.exit_ip.as_str()),
        (Some("jsd1"), "9.9.9.9")
    );
    let submit = &m.requests()[0];
    assert_eq!(
        (submit.method.as_str(), submit.path.as_str()),
        ("POST", "/v1/cloudflare/jsd")
    );
    assert_eq!(
        submit.keys(),
        keys(&["proxy", "session", "url", "userAgent"])
    );
}

#[tokio::test]
async fn ended_jobs_are_errors() {
    for (name, terminal, check, state, retry) in [
        (
            "declined",
            r#"{"id":"job_1","state":"declined","error":"the challenge was not passed","retry":true}"#,
            Error::is_declined as Check,
            "declined",
            true,
        ),
        (
            "failed",
            r#"{"id":"job_1","state":"failed","error":"the proxy is refused: x is not a public address"}"#,
            |e: &Error| !e.is_declined(),
            "failed",
            false,
        ),
        (
            "no wall",
            r#"{"id":"job_1","state":"failed","error":"the page serves no challenge (status 200)"}"#,
            Error::is_no_challenge,
            "failed",
            false,
        ),
        (
            "rotating proxy",
            r#"{"id":"job_1","state":"failed","error":"the proxy exits from a different address on each connection (1.1.1.1, then 2.2.2.2): a clearance is for one address"}"#,
            Error::is_rotating_proxy,
            "failed",
            false,
        ),
        (
            "expired",
            r#"{"id":"job_1","state":"done","resultExpired":true}"#,
            |e: &Error| e.to_string().contains("no longer holds its result"),
            "done",
            false,
        ),
        (
            "no result",
            r#"{"id":"job_1","state":"done"}"#,
            |e: &Error| e.to_string().contains("without a result"),
            "done",
            false,
        ),
    ] {
        let m = api(terminal).await;
        let err = client(&m).solve_challenge(challenge()).await.unwrap_err();
        assert!(check(&err), "{name}: {err}");
        match &err {
            Error::Job {
                id,
                state: s,
                message,
                retry: r,
            } => {
                assert_eq!(
                    (id.as_str(), s.as_str(), *r),
                    ("job_1", state, retry),
                    "{name}"
                );
                assert!(!message.is_empty(), "{name}");
            }
            other => panic!("{name}: {other:?}"),
        }
        assert!(!err.is_bad_request() && !err.is_hardblock(), "{name}");
    }
}

#[tokio::test]
async fn refusals() {
    let predicates: [(u16, Check); 4] = [
        (402, Error::is_insufficient_balance),
        (403, Error::is_site_not_approved),
        (429, Error::is_rate_limited),
        (400, Error::is_bad_request),
    ];
    for (status, check) in predicates {
        let m = serve(move |_, _| reply(status, r#"{"error":"gateway: nope"}"#)).await;
        let err = client(&m).solve_challenge(challenge()).await.unwrap_err();
        assert!(
            matches!(&err, Error::Api { status: s, message, hardblock: false } if *s == status && message == "gateway: nope"),
            "{err:?}"
        );
        assert!(check(&err), "{status}");
        // one predicate per refusal
        assert_eq!(
            predicates.iter().filter(|(_, p)| p(&err)).count(),
            1,
            "{status}"
        );
        assert!(!err.is_declined());
        // a refusal is not a job: nothing was waited for
        assert_eq!(m.count(), 1);
    }
    // an input without its required fields is refused before any call
    let m = serve(|_, _| reply(202, r#"{"id":"job_1","state":"queued"}"#)).await;
    let c = client(&m);
    let err = c
        .submit_challenge(ChallengeInput {
            url: "https://www.example.com/".into(),
            ..Default::default()
        })
        .await
        .unwrap_err();
    assert!(
        matches!(&err, Error::Input(m) if m.contains("proxy")),
        "{err}"
    );
    assert!(
        c.submit_turnstile(TurnstileInput {
            page_url: "https://www.example.com/".into(),
            proxy: "8.8.8.8:80".into(),
            ..Default::default()
        })
        .await
        .is_err()
    );
    assert!(
        c.submit_jsd(JsdInput {
            proxy: "8.8.8.8:80".into(),
            ..Default::default()
        })
        .await
        .is_err()
    );
    assert!(matches!(
        c.job("", Duration::ZERO).await,
        Err(Error::Input(_))
    ));
    assert_eq!(m.count(), 0);
}

#[tokio::test]
async fn a_reply_without_a_job_id_is_an_error() {
    let m = serve(|_, _| reply(202, r#"{"state":"queued"}"#)).await;
    let err = client(&m).submit_challenge(challenge()).await.unwrap_err();
    assert!(
        matches!(&err, Error::Decode(m) if m.contains("without a job id")),
        "{err}"
    );
    let m = serve(|_, _| reply(202, "not json")).await;
    assert!(matches!(
        client(&m).submit_challenge(challenge()).await,
        Err(Error::Decode(_))
    ));
}

#[tokio::test]
async fn wait_survives_a_bad_poll_and_stops_when_dropped() {
    // a 502, then a dropped connection, then the end
    let m = serve(|_, n| match n {
        0 => reply(502, ""),
        1 => hang_up(),
        _ => reply(200, DONE_JOB),
    })
    .await;
    let c = client(&m);
    let res = c
        .wait(&Job {
            id: "job_1".into(),
            state: "queued".into(),
            ..Default::default()
        })
        .await
        .unwrap();
    assert_eq!((res.clearance(), m.count()), (Some("abc123"), 3));

    // a job that never ends: the caller's timeout ends the wait
    let hang = serve(|_, _| reply(200, r#"{"id":"job_1","state":"running"}"#)).await;
    let c = client(&hang);
    let start = Instant::now();
    let job = Job {
        id: "job_1".into(),
        state: "running".into(),
        ..Default::default()
    };
    assert!(
        tokio::time::timeout(Duration::from_millis(150), c.wait(&job))
            .await
            .is_err()
    );
    assert!(start.elapsed() < Duration::from_secs(3));
}

#[tokio::test]
async fn wait_gives_up_after_three_bad_polls_in_a_row() {
    let m = serve(|_, _| reply(503, r#"{"error":"billing unavailable; retry shortly"}"#)).await;
    let err = client(&m)
        .wait(&Job {
            id: "job_1".into(),
            state: "running".into(),
            ..Default::default()
        })
        .await
        .unwrap_err();
    assert!(matches!(&err, Error::Api { status: 503, .. }), "{err}");
    assert_eq!(m.count(), 4);
}

#[tokio::test]
async fn not_found_is_not_retried() {
    let m = serve(|_, _| reply(404, r#"{"error":"not found"}"#)).await;
    let err = client(&m)
        .wait(&Job {
            id: "job_x".into(),
            state: "running".into(),
            ..Default::default()
        })
        .await
        .unwrap_err();
    assert!(
        matches!(&err, Error::Api { status: 404, message, .. } if message == "not found"),
        "{err}"
    );
    assert_eq!(m.count(), 1);
}

#[tokio::test]
async fn a_terminal_job_is_not_polled() {
    let m = serve(|_, _| reply(500, "")).await;
    let job: Job = serde_json::from_str(DONE_JOB).unwrap();
    assert_eq!(
        (
            job.task.as_str(),
            job.proxy.as_deref(),
            job.created_at.as_deref()
        ),
        (
            "challenge",
            Some("8.8.8.8:80"),
            Some("2026-10-23T10:00:00Z")
        )
    );
    assert!(job.is_terminal());
    assert_eq!(
        client(&m).wait(&job).await.unwrap().clearance(),
        Some("abc123")
    );
    assert_eq!(m.count(), 0);
}

#[tokio::test]
async fn job_asks_the_server_to_wait() {
    let m = serve(|_, _| reply(200, r#"{"id":"job_1","state":"running"}"#)).await;
    let c = client(&m);
    let job = c.job("job_1", Duration::from_millis(1500)).await.unwrap();
    assert!(!job.is_terminal() && job.result.is_none());
    assert_eq!(m.last().query.as_deref(), Some("wait=2"));
    c.job("job_1", Duration::ZERO).await.unwrap();
    assert_eq!(
        (m.last().path.as_str(), m.last().query.as_deref()),
        ("/v1/cloudflare/jobs/job_1", None)
    );
    c.job("a/b c", Duration::from_secs(25)).await.unwrap();
    assert_eq!(
        (m.last().path.as_str(), m.last().query.as_deref()),
        ("/v1/cloudflare/jobs/a%2Fb%20c", Some("wait=25"))
    );
}

#[tokio::test]
async fn a_call_ends_with_its_timeout() {
    let m = serve(|_, _| -> Reply { slow(Duration::from_secs(10)) }).await;
    let c = cloudflare::Client::builder("key")
        .endpoint(&m.url)
        .timeout(Duration::from_millis(100))
        .build()
        .unwrap();
    let start = Instant::now();
    let err = c.job("job_1", Duration::ZERO).await.unwrap_err();
    assert!(err.is_timeout(), "{err:?}");
    assert!(start.elapsed() < Duration::from_secs(3));
}

#[test]
fn clearance_from_cookies_when_the_payload_is_other() {
    let r = Solution {
        payload: "x".into(),
        cookies: vec![Cookie {
            name: "cf_clearance".into(),
            value: "from-cookie".into(),
            ..Default::default()
        }],
        ..Default::default()
    };
    assert_eq!(r.clearance(), Some("from-cookie"));
    assert_eq!(Solution::default().clearance(), None);
    assert_eq!(Solution::default().token(), "");
}

/// A predicate of the error.
type Check = fn(&Error) -> bool;
