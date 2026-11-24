mod common;

use common::{Mock, reply, serve};
use john_antibot::Error;
use john_antibot::kasada::{self, PayloadInput};

fn client(m: &Mock) -> kasada::Client {
    kasada::Client::builder("key")
        .org_key("org")
        .endpoint(format!("{}/", m.url))
        .build()
        .unwrap()
}

fn input() -> PayloadInput {
    PayloadInput {
        script: "ips".into(),
        ips_link: Some("/a/b/ips.js".into()),
        page_url: "https://www.example.com/".into(),
        user_agent: "UA/1".into(),
        ip: "203.0.113.1".into(),
        accept_language: "en-US".into(),
        ..Default::default()
    }
}

#[tokio::test]
async fn generate_payload_sends_script_and_decodes_artifact() {
    let m = serve(|_, _| {
        reply(
            200,
            r#"{"payloadBase64":"AAEC","headers":{"x-kpsdk-ct":"ct"},"tlUrl":"https://www.example.com/tl","session":"s1","userAgent":"UA/1"}"#,
        )
    })
    .await;
    let out = client(&m).generate_payload(input()).await.unwrap();
    assert_eq!(out.payload, [0, 1, 2]);
    assert_eq!(out.headers["x-kpsdk-ct"], "ct");
    let req = m.last();
    assert_eq!(
        (req.method.as_str(), req.path.as_str()),
        ("POST", "/v1/kasada/payload")
    );
    assert_eq!(
        (req.header("x-api-key"), req.header("x-org-key")),
        (Some("key"), Some("org"))
    );
    assert_eq!(req.json()["ip"].as_str(), Some("203.0.113.1"));
}

#[tokio::test]
async fn refusals() {
    for (status, check) in [
        (402, Error::is_insufficient_balance as fn(&Error) -> bool),
        (400, Error::is_bad_request as fn(&Error) -> bool),
    ] {
        let m = serve(move |_, _| reply(status, r#"{"error":"gateway: nope"}"#)).await;
        let err = client(&m).generate_payload(input()).await.unwrap_err();
        assert!(check(&err), "{status}: {err}");
    }
}

#[tokio::test]
async fn version_sends_pjs_to_its_route() {
    let m = serve(|_, _| {
        reply(
            200,
            r#"{"version":"j-1.2.1020","payload":"j-1.2.1020","session":"s1","userAgent":"UA/1"}"#,
        )
    })
    .await;
    let out = client(&m)
        .version(kasada::VersionInput {
            script: "pjs".into(),
            page_url: "https://www.example.com/".into(),
            ..Default::default()
        })
        .await
        .unwrap();
    assert_eq!(out.version, "j-1.2.1020");
    assert_eq!(m.last().path, "/v1/kasada/version");
}

#[tokio::test]
async fn generate_payload_without_a_user_agent_sends_none() {
    let m = serve(|_, _| {
        reply(
            200,
            r#"{"payloadBase64":"AAEC","headers":{"x-kpsdk-ct":"ct"},"tlUrl":"https://www.example.com/tl","session":"s1","userAgent":"Chrome/158 picked"}"#,
        )
    })
    .await;
    let mut no_ua = input();
    no_ua.user_agent = String::new();
    let out = client(&m).generate_payload(no_ua).await.unwrap();
    assert_eq!(out.user_agent, "Chrome/158 picked");
    assert!(m.last().json().get("userAgent").is_none());
}
