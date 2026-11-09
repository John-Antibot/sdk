mod common;

use std::time::{Duration, Instant};

use common::{Mock, Request, keys, reply, serve};
use john_antibot::Error;
use john_antibot::recaptcha::{self, Input, Job, Solution};

const DONE_JOB: &str = r#"{"id":"job_1","state":"done","task":"v3","createdAt":"2026-10-23T10:00:00Z","proxy":"8.8.8.8:80","result":{"token":"03AFcWeA","expiresAt":"2026-10-23T10:01:50.000Z","userAgent":"UA/1","acceptLanguage":"en-US,en;q=0.9","secUa":"\"Chromium\";v=\"156\"","secUaMobile":"?0","secUaPlatform":"\"macOS\""}}"#;

/// The API: a submit answers 202 with job_1, the first poll says running, the next says `terminal`.
async fn api(terminal: &'static str) -> Mock {
    serve(
        move |r: &Request, n| match (r.method.as_str(), r.path.as_str()) {
            ("POST", _) => reply(202, r#"{"id":"job_1","state":"queued"}"#),
            ("GET", "/v1/recaptcha/jobs/job_1") if n == 1 => {
                reply(200, r#"{"id":"job_1","state":"running"}"#)
            }
            ("GET", "/v1/recaptcha/jobs/job_1") => reply(200, terminal),
            _ => reply(404, r#"{"error":"not found"}"#),
        },
    )
    .await
}

fn client(m: &Mock) -> recaptcha::Client {
    recaptcha::Client::builder("key")
        .org_key("org")
        .endpoint(format!("{}/", m.url))
        .build()
        .unwrap()
}

fn input() -> Input {
    Input {
        url: "https://www.example.com/".into(),
        site_key: "k".into(),
        proxy: "8.8.8.8:80".into(),
        ..Default::default()
    }
}

#[tokio::test]
async fn solve_v3_submits_and_waits() {
    let m = api(DONE_JOB).await;
    let res = client(&m)
        .solve_v3(Input {
            url: "https://www.example.com/login".into(),
            site_key: "6Lc-key_1".into(),
            action: Some("login".into()),
            proxy: "u:p@8.8.8.8:80".into(),
            timeout: Some(Duration::from_secs(45)),
        })
        .await
        .unwrap();
    assert_eq!(
        (
            res.token.as_str(),
            res.user_agent.as_str(),
            res.accept_language.as_str()
        ),
        ("03AFcWeA", "UA/1", "en-US,en;q=0.9")
    );
    assert_eq!(
        (res.sec_ua_mobile.as_str(), res.sec_ua_platform.as_str()),
        ("?0", r#""macOS""#)
    );
    assert_eq!(res.expires_at.as_deref(), Some("2026-10-23T10:01:50.000Z"));
    let reqs = m.requests();
    assert_eq!(reqs.len(), 3);
    let submit = &reqs[0];
    assert_eq!(
        (
            submit.method.as_str(),
            submit.path.as_str(),
            submit.query.as_deref()
        ),
        ("POST", "/v1/recaptcha/v3", None)
    );
    assert_eq!(
        (submit.header("x-api-key"), submit.header("x-org-key")),
        (Some("key"), Some("org"))
    );
    assert_eq!(submit.header("content-type"), Some("application/json"));
    assert_eq!(
        submit.keys(),
        keys(&["action", "proxy", "siteKey", "timeoutMs", "url"])
    );
    let got = submit.json();
    assert_eq!(
        (
            got["url"].as_str(),
            got["siteKey"].as_str(),
            got["action"].as_str()
        ),
        (
            Some("https://www.example.com/login"),
            Some("6Lc-key_1"),
            Some("login")
        )
    );
    assert_eq!(
        (got["proxy"].as_str(), got["timeoutMs"].as_u64()),
        (Some("u:p@8.8.8.8:80"), Some(45000))
    );
    for poll in &reqs[1..] {
        assert_eq!(
            (
                poll.method.as_str(),
                poll.path.as_str(),
                poll.query.as_deref()
            ),
            ("GET", "/v1/recaptcha/jobs/job_1", Some("wait=25"))
        );
    }
}

#[tokio::test]
async fn enterprise_goes_to_its_route_and_leaves_empty_optionals_out() {
    let m = api(DONE_JOB).await;
    let res = client(&m)
        .solve_v3_enterprise(Input {
            action: Some(String::new()),
            timeout: Some(Duration::ZERO),
            ..input()
        })
        .await
        .unwrap();
    assert_eq!(res.token, "03AFcWeA");
    let submit = &m.requests()[0];
    assert_eq!(submit.path, "/v1/recaptcha/v3-enterprise");
    assert_eq!(submit.keys(), keys(&["proxy", "siteKey", "url"]));
}

#[tokio::test]
async fn ended_jobs_are_errors() {
    for (terminal, message, declined, retry) in [
        (
            r#"{"id":"job_1","state":"declined","error":"the attempt was turned down","retry":true}"#,
            "the attempt was turned down",
            true,
            true,
        ),
        (
            r#"{"id":"job_1","state":"failed","error":"the proxy is refused: x is not a public address"}"#,
            "the proxy is refused: x is not a public address",
            false,
            false,
        ),
        (
            r#"{"id":"job_1","state":"done","resultExpired":true}"#,
            "no longer holds its result",
            false,
            false,
        ),
        (
            r#"{"id":"job_1","state":"done"}"#,
            "without a result",
            false,
            false,
        ),
    ] {
        let m = api(terminal).await;
        let err = client(&m).solve_v3(input()).await.unwrap_err();
        assert_eq!(err.is_declined(), declined, "{err}");
        assert!(
            matches!(&err, Error::Job { id, message: m, retry: r, .. } if id == "job_1" && m.contains(message) && *r == retry),
            "{err:?}"
        );
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
        let err = client(&m).solve_v3(input()).await.unwrap_err();
        assert!(
            matches!(&err, Error::Api { status: s, message, .. } if *s == status && message == "gateway: nope"),
            "{err:?}"
        );
        assert!(check(&err), "{status}");
        assert_eq!(
            predicates.iter().filter(|(_, p)| p(&err)).count(),
            1,
            "{status}"
        );
    }
    // a solve with a field missing is refused before any call
    let m = serve(|_, _| reply(202, r#"{"id":"job_1","state":"queued"}"#)).await;
    let c = client(&m);
    for (name, bad) in [
        (
            "no proxy",
            Input {
                proxy: String::new(),
                ..input()
            },
        ),
        (
            "no site key",
            Input {
                site_key: String::new(),
                ..input()
            },
        ),
        (
            "no url",
            Input {
                url: String::new(),
                ..input()
            },
        ),
    ] {
        let err = c.submit_v3(bad).await.unwrap_err();
        assert!(
            matches!(&err, Error::Input(m) if m == "recaptcha: a solve needs a URL, a site key and a proxy"),
            "{name}: {err}"
        );
    }
    assert_eq!(m.count(), 0);
}

#[tokio::test]
async fn wait_survives_a_bad_poll_and_stops_when_dropped() {
    let m = serve(|_, n| {
        if n == 0 {
            reply(502, "")
        } else {
            reply(200, DONE_JOB)
        }
    })
    .await;
    let res = client(&m)
        .wait(&Job {
            id: "job_1".into(),
            state: "queued".into(),
            ..Default::default()
        })
        .await
        .unwrap();
    assert_eq!((res.token.as_str(), m.count()), ("03AFcWeA", 2));

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
    assert!(matches!(&err, Error::Api { status: 404, .. }), "{err}");
    assert_eq!(m.count(), 1);
}

#[tokio::test]
async fn job_reads_one_job() {
    let m = serve(|_, _| reply(200, DONE_JOB)).await;
    let job = client(&m)
        .job("job_1", Duration::from_secs(5))
        .await
        .unwrap();
    assert_eq!(
        (
            job.state.as_str(),
            job.task.as_str(),
            m.last().query.as_deref()
        ),
        ("done", "v3", Some("wait=5"))
    );
    assert_eq!(job.result.unwrap().token, "03AFcWeA");
}

#[test]
fn headers_are_the_browser_the_token_was_made_as() {
    let job: Job = serde_json::from_str(DONE_JOB).unwrap();
    let h = job.result.unwrap().headers();
    assert_eq!(h.len(), 5);
    assert_eq!(h["user-agent"], "UA/1");
    assert_eq!(h["accept-language"], "en-US,en;q=0.9");
    assert_eq!(h["sec-ch-ua"], r#""Chromium";v="156""#);
    assert_eq!(h["sec-ch-ua-mobile"], "?0");
    assert_eq!(h["sec-ch-ua-platform"], r#""macOS""#);
    // a result without client hints leaves them out instead of sending empty ones
    let bare = Solution {
        user_agent: "UA/2".into(),
        accept_language: "de".into(),
        ..Default::default()
    }
    .headers();
    assert_eq!(bare.len(), 2);
    assert!(bare.get("sec-ch-ua").is_none());
    assert!(Solution::default().headers().is_empty());
    // a result with no expiry says so
    let open: Solution = serde_json::from_str(r#"{"token":"t","expiresAt":null}"#).unwrap();
    assert_eq!(open.expires_at, None);
}

/// A predicate of the error.
type Check = fn(&Error) -> bool;
