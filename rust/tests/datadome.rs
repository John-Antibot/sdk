mod common;

use std::time::{Duration, UNIX_EPOCH};

use common::{hang_up, keys, reply, serve, slow};
use john_antibot::datadome::{
    self, Event, InterstitialInput, LATEST_USER_AGENT, SessionOptions, SliderInput, TagsInput,
    script_hash,
};
use john_antibot::{DEFAULT_ENDPOINT, Error};
use serde_json::json;

const INTERSTITIAL_HTML: &str = r#"<html><head><meta name="dd-page-size" content="10510">
<script>var ddm = { cid: 'CID~x', hash: 'HASH', userEnv: 'UE', seed: 'SEED', env: 'ENV', s: 43624, b: 1265040, e: 'E1' };</script>
<script src="https://ct.captcha-delivery.com/interstitial.1.0.js"></script></head></html>"#;

const DEVICE_LINK: &str = "https://geo.captcha-delivery.com/interstitial/?x=1";

/// A tags input with its required site key.
fn tags() -> TagsInput {
    TagsInput {
        site_key: "KEY".into(),
        ..Default::default()
    }
}

fn client(url: &str) -> datadome::Client {
    datadome::Client::builder("key")
        .endpoint(url)
        .build()
        .unwrap()
}

#[tokio::test]
async fn interstitial_wire_and_form() {
    let form = "cid=CID~x&hash=HASH&referer=https%3A%2F%2Fwww.example.com%2F&url=https%3A%2F%2Fwww.example.com%2F&s=43624&e=E1&env=ENV&userEnv=UE&seed=SEED&b=1265040&dm=cd&ddMessageFormat=&payload=PLV2BLOB&plv3=PLV3TOK&ps=10510";
    let body =
        json!({"payload": form, "session": "sess-1", "userAgent": "UA-FROM-PROFILE"}).to_string();
    let m = serve(move |_, _| reply(200, &body)).await;
    let mut session = client(&m.url).session(SessionOptions {
        session: Some("sess-1".into()),
        timezone: Some("America/New_York".into()),
        timezone_offset: Some(240),
        ..Default::default()
    });
    let out = session
        .generate_interstitial(InterstitialInput {
            html: INTERSTITIAL_HTML.into(),
            device_link: DEVICE_LINK.into(),
            referer: Some("https://www.example.com/".into()),
            fetched_at: Some(UNIX_EPOCH + Duration::from_millis(1_792_000_000_000)),
            script: Some("/*script*/".into()),
        })
        .await
        .unwrap();

    let req = m.last();
    assert_eq!(
        (req.method.as_str(), req.path.as_str(), req.query.as_deref()),
        ("POST", "/v1/datadome/interstitial", None)
    );
    assert_eq!(req.header("x-api-key"), Some("key"));
    assert_eq!(req.header("content-type"), Some("application/json"));
    assert_eq!(req.header("x-org-key"), None);
    assert_eq!(req.header("content-encoding"), None);
    // only the documented fields: the service reads what the page carries from html
    assert_eq!(
        req.keys(),
        keys(&[
            "challengeFetchedAtMs",
            "deviceUrl",
            "html",
            "referer",
            "scriptHash",
            "session",
            "sessionKey",
            "timezone",
            "timezoneOffset",
            "userAgent"
        ])
    );
    let got = req.json();
    assert_eq!(got["html"], INTERSTITIAL_HTML);
    assert_eq!(got["deviceUrl"], DEVICE_LINK);
    assert_eq!(got["scriptHash"], script_hash("/*script*/"));
    assert_eq!(got["referer"], "https://www.example.com/");
    assert_eq!(got["session"], "sess-1");
    assert_eq!(got["sessionKey"], "sess-1");
    assert_eq!(got["timezone"], "America/New_York");
    assert_eq!(got["timezoneOffset"], 240);
    assert_eq!(got["challengeFetchedAtMs"], 1_792_000_000_000u64);
    // the session speaks the latest Chrome by default and keeps it
    assert_eq!(got["userAgent"], LATEST_USER_AGENT);
    assert_eq!(
        (session.token(), session.user_agent()),
        (Some("sess-1"), LATEST_USER_AGENT)
    );

    assert_eq!(out.session, "sess-1");
    assert_eq!(out.user_agent, "UA-FROM-PROFILE");
    assert_eq!(out.form_body, form);
    assert_eq!(out.form_body_for("https://www.example.com/"), form);
    assert!(!out.form_body.contains("%7E") && out.form_body.starts_with("cid="));
}

#[tokio::test]
async fn tags_wire_and_session_learning() {
    let m = serve(|_, _| {
        reply(
            200,
            r#"{"payload":"FORM","session":"sess-new","userAgent":"UA","generatedAt":5}"#,
        )
    })
    .await;
    let mut session = client(&m.url).session(SessionOptions {
        referer: Some("https://www.example.com/".into()),
        ..Default::default()
    });
    let out = session
        .generate_tags(TagsInput {
            event: Event::Ch,
            cookie: "cookieval".into(),
            site_key: "KEY".into(),
            ..Default::default()
        })
        .await
        .unwrap();
    assert_eq!(
        (
            out.payload.as_str(),
            out.session.as_str(),
            out.user_agent.as_str(),
            out.generated_at
        ),
        ("FORM", "sess-new", "UA", 5)
    );
    let first = m.last();
    assert_eq!(
        (first.method.as_str(), first.path.as_str()),
        ("POST", "/v1/datadome/tags")
    );
    // the first call carries no session, and no scriptHash without a script
    assert_eq!(
        first.keys(),
        keys(&["cid", "ddk", "jsType", "referer", "userAgent"])
    );
    let got = first.json();
    assert_eq!(
        (
            got["jsType"].as_str(),
            got["cid"].as_str(),
            got["ddk"].as_str()
        ),
        (Some("ch"), Some("cookieval"), Some("KEY"))
    );
    assert_eq!(got["referer"], "https://www.example.com/");
    assert_eq!(session.token(), Some("sess-new"));

    session
        .generate_tags(TagsInput {
            event: Event::Le,
            cookie: "c".into(),
            site_key: "KEY".into(),
            version: Some("5.1.2".into()),
            script: Some("/*tags*/".into()),
            referer: Some("https://www.example.com/cart".into()),
            loaded_font_families: vec!["Inter".into(), "Inter Fallback".into()],
            tags_script_url: Some("https://js.datadome.co/tags.js".into()),
        })
        .await
        .unwrap();
    let got = m.last().json();
    assert_eq!(got["jsType"], "le");
    assert_eq!(got["session"], "sess-new");
    assert_eq!(got["sessionKey"], "sess-new");
    assert_eq!(got["version"], "5.1.2");
    assert_eq!(got["referer"], "https://www.example.com/cart");
    assert_eq!(got["scriptHash"], script_hash("/*tags*/"));
    assert!(!got.contains_key("script"));
    assert_eq!(script_hash("/*tags*/").len(), 64);
    assert_eq!(
        got["loadedFontFamilies"],
        json!(["Inter", "Inter Fallback"])
    );
    assert_eq!(got["tagsScriptUrl"], "https://js.datadome.co/tags.js");
    assert_eq!(
        m.last().keys(),
        keys(&[
            "cid",
            "ddk",
            "jsType",
            "loadedFontFamilies",
            "referer",
            "scriptHash",
            "session",
            "sessionKey",
            "tagsScriptUrl",
            "userAgent",
            "version"
        ])
    );
}

#[tokio::test]
async fn slider_wire_and_check_url() {
    let m = serve(|_, n| match n {
        0 => reply(200, r#"{"payload":"?cid=C&icid=IC&ccid=&userEnv=UE&dm=dc_ir&ddCaptchaChallenge=ECH&plv3=TOK&ir=503003","session":"s2","userAgent":"UA"}"#),
        1 => reply(200, r#"{"payload":"https://geo.captcha-delivery.com/captcha/check?cid=C","session":"s3"}"#),
        _ => reply(200, r#"{"payload":"PLV2BLOB","session":"s4"}"#),
    })
    .await;
    let mut session = client(&m.url).session(SessionOptions {
        referer: Some("https://www.example.com/".into()),
        ..Default::default()
    });
    let input = SliderInput {
        html: "<html></html>".into(),
        captcha_url: "https://geo.captcha-delivery.com/captcha/?x=1".into(),
        parent_url: Some("https://www.example.com/p".into()),
        ..Default::default()
    };
    let out = session.generate_slider(input.clone()).await.unwrap();
    assert_eq!(
        out.check_url,
        "https://geo.captcha-delivery.com/captcha/check?cid=C&icid=IC&ccid=&userEnv=UE&dm=dc_ir&ddCaptchaChallenge=ECH&plv3=TOK&ir=503003"
    );
    let req = m.last();
    assert_eq!(req.path, "/v1/datadome/slider");
    // a slider sends no referer: the service reads the page from html
    assert_eq!(
        req.keys(),
        keys(&["captchaUrl", "html", "parentUrl", "userAgent"])
    );
    assert_eq!(
        req.json()["captchaUrl"],
        "https://geo.captcha-delivery.com/captcha/?x=1"
    );
    assert_eq!(session.token(), Some("s2"));

    let out = session.generate_slider(input.clone()).await.unwrap();
    assert_eq!(
        out.check_url,
        "https://geo.captcha-delivery.com/captcha/check?cid=C"
    );
    assert_eq!(m.last().json()["session"], "s2");

    // a payload that is not a URL is no check URL, and the session keeps its token
    let err = session.generate_slider(input).await.unwrap_err();
    assert!(
        matches!(err, Error::Decode(ref m) if m.contains("no check URL")),
        "{err}"
    );
    assert_eq!(session.token(), Some("s3"));
}

#[tokio::test]
async fn interstitial_without_a_form_is_an_error() {
    let m = serve(|_, _| reply(200, r#"{"payload":"PLV2BLOB","session":"s"}"#)).await;
    let mut session = client(&m.url).session(SessionOptions::default());
    let err = session
        .generate_interstitial(InterstitialInput {
            html: "<html></html>".into(),
            device_link: DEVICE_LINK.into(),
            ..Default::default()
        })
        .await
        .unwrap_err();
    assert!(
        matches!(err, Error::Decode(ref m) if m.contains("no ready-to-post form")),
        "{err}"
    );
    assert_eq!(session.token(), None);
}

#[tokio::test]
async fn interstitial_sends_the_hash_of_the_client_script() {
    let m = serve(|_, _| reply(200, r#"{"payload":"cid=x","session":"s"}"#)).await;
    client(&m.url)
        .session(SessionOptions {
            session: Some("s".into()),
            ..Default::default()
        })
        .generate_interstitial(InterstitialInput {
            html: INTERSTITIAL_HTML.into(),
            device_link: DEVICE_LINK.into(),
            script: Some("/*challenge script*/".into()),
            ..Default::default()
        })
        .await
        .unwrap();
    let got = m.last().json();
    assert_eq!(got["scriptHash"], script_hash("/*challenge script*/"));
    assert!(!got.contains_key("script") && !got.contains_key("referer"));
}

#[tokio::test]
async fn api_errors_are_classified() {
    for (status, body, hardblock, puzzle, message) in [
        (
            422,
            r#"{"error":"captcha page is hardblock (no solvable slider)","session":"s","hardblock":true}"#,
            true,
            false,
            "captcha page is hardblock (no solvable slider)",
        ),
        (
            500,
            r#"{"error":"captcha page is hardblock (no solvable slider)"}"#,
            true,
            false,
            "captcha page is hardblock (no solvable slider)",
        ),
        (
            500,
            r#"{"error":"generation failed"}"#,
            false,
            false,
            "generation failed",
        ),
        (
            422,
            r#"{"error":"puzzle slider unsupported"}"#,
            false,
            true,
            "puzzle slider unsupported",
        ),
        (502, "  upstream down \n", false, false, "upstream down"),
        (503, "", false, false, "503 Service Unavailable"),
    ] {
        let m = serve(move |_, _| reply(status, body)).await;
        let err = client(&m.url)
            .session(SessionOptions::default())
            .generate_slider(SliderInput {
                html: "<html></html>".into(),
                captcha_url: "https://geo.captcha-delivery.com/captcha/?x=1".into(),
                ..Default::default()
            })
            .await
            .unwrap_err();
        match &err {
            Error::Api {
                status: s,
                message: msg,
                ..
            } => assert_eq!((*s, msg.as_str()), (status, message)),
            other => panic!("{other:?}"),
        }
        assert_eq!(err.is_hardblock(), hardblock, "{err}");
        assert_eq!(err.is_puzzle_unsupported(), puzzle, "{err}");
        assert!(!err.is_rate_limited() && !err.is_declined());
    }
    let m = serve(|_, _| reply(429, r#"{"error":"slow down"}"#)).await;
    let err = client(&m.url)
        .session(SessionOptions {
            referer: Some("https://www.example.com/".into()),
            ..Default::default()
        })
        .generate_tags(tags())
        .await
        .unwrap_err();
    assert!(err.is_rate_limited() && !err.is_hardblock(), "{err}");
    assert_eq!(err.to_string(), "HTTP 429: slow down");
}

#[tokio::test]
async fn required_inputs_fail_before_the_network() {
    let m = serve(|_, _| reply(200, "{}")).await;
    let mut s = client(&m.url).session(SessionOptions {
        session: Some("s".into()),
        ..Default::default()
    });
    let err = s
        .generate_interstitial(InterstitialInput {
            html: INTERSTITIAL_HTML.into(),
            ..Default::default()
        })
        .await
        .unwrap_err();
    assert!(
        matches!(&err, Error::Input(m) if m.contains("device_link is required")),
        "{err}"
    );
    let err = s
        .generate_interstitial(InterstitialInput {
            html: INTERSTITIAL_HTML.into(),
            device_link: DEVICE_LINK.into(),
            ..Default::default()
        })
        .await
        .unwrap_err();
    assert!(
        err.to_string().contains(
            "script is required: load https://ct.captcha-delivery.com/interstitial.1.0.js"
        ),
        "{err}"
    );
    let err = s
        .generate_slider(SliderInput {
            html: "<html></html>".into(),
            ..Default::default()
        })
        .await
        .unwrap_err();
    assert!(
        matches!(&err, Error::Input(m) if m.contains("captcha_url is required")),
        "{err}"
    );
    let err = s.generate_tags(TagsInput::default()).await.unwrap_err();
    assert!(
        matches!(&err, Error::Input(m) if m.contains("site_key is required")),
        "{err}"
    );
    let err = s.generate_tags(tags()).await.unwrap_err();
    assert!(
        matches!(&err, Error::Input(m) if m.contains("referer")),
        "{err}"
    );
    assert_eq!(m.count(), 0);
}

#[tokio::test]
async fn keys_are_sent_as_headers() {
    let m = serve(|_, _| reply(200, r#"{"payload":"F"}"#)).await;
    let c = datadome::Client::builder("ja_live_x")
        .org_key("ja_org_y")
        .endpoint(&m.url)
        .build()
        .unwrap();
    let tags = TagsInput {
        referer: Some("https://www.example.com/".into()),
        ..tags()
    };
    c.session(SessionOptions::default())
        .generate_tags(tags.clone())
        .await
        .unwrap();
    let req = m.last();
    assert_eq!(
        (req.header("x-api-key"), req.header("x-org-key")),
        (Some("ja_live_x"), Some("ja_org_y"))
    );
    // no key, no header
    let c = datadome::Client::builder("")
        .endpoint(&m.url)
        .build()
        .unwrap();
    c.session(SessionOptions::default())
        .generate_tags(tags)
        .await
        .unwrap();
    let req = m.last();
    assert_eq!(
        (req.header("x-api-key"), req.header("x-org-key")),
        (None, None)
    );
}

#[tokio::test]
async fn large_bodies_are_gzipped() {
    let m = serve(|_, _| reply(200, r#"{"payload":"cid=x"}"#)).await;
    let mut s = client(&m.url).session(SessionOptions::default());
    let html = format!("<html>{}</html>", "a".repeat(100 << 10));
    s.generate_interstitial(InterstitialInput {
        html: html.clone(),
        device_link: DEVICE_LINK.into(),
        ..Default::default()
    })
    .await
    .unwrap();
    let req = m.last();
    assert_eq!(req.header("content-encoding"), Some("gzip"));
    assert!(req.body.len() < 10 << 10, "sent {} bytes", req.body.len());
    assert_eq!(req.json()["html"], html.as_str());
    // a small body is sent as is
    s.generate_interstitial(InterstitialInput {
        html: "<html></html>".into(),
        device_link: DEVICE_LINK.into(),
        ..Default::default()
    })
    .await
    .unwrap();
    assert_eq!(m.last().header("content-encoding"), None);
}

#[tokio::test]
async fn a_call_ends_with_its_timeout() {
    let m = serve(|_, _| slow(Duration::from_secs(10))).await;
    let c = datadome::Client::builder("key")
        .endpoint(&m.url)
        .timeout(Duration::from_millis(100))
        .build()
        .unwrap();
    let start = std::time::Instant::now();
    let err = c
        .session(SessionOptions {
            referer: Some("https://www.example.com/".into()),
            ..Default::default()
        })
        .generate_tags(tags())
        .await
        .unwrap_err();
    assert!(err.is_timeout(), "{err:?}");
    assert!(start.elapsed() < Duration::from_secs(3));
}

#[tokio::test]
async fn a_dropped_connection_is_a_transport_error() {
    let m = serve(|_, _| hang_up()).await;
    let err = client(&m.url)
        .session(SessionOptions {
            referer: Some("https://www.example.com/".into()),
            ..Default::default()
        })
        .generate_tags(tags())
        .await
        .unwrap_err();
    assert!(
        matches!(err, Error::Transport(_)) && !err.is_timeout(),
        "{err:?}"
    );
}

#[tokio::test]
async fn endpoints_and_http_clients() {
    assert_eq!(datadome::Client::new("k").endpoint(), DEFAULT_ENDPOINT);
    assert_eq!(DEFAULT_ENDPOINT, "https://antibot.johnavenue.com");
    let m = serve(|_, _| reply(200, r#"{"payload":"F"}"#)).await;
    // a trailing slash is dropped; a caller's own client carries the calls
    let http = reqwest::Client::builder()
        .user_agent("my-app/1")
        .build()
        .unwrap();
    let c = datadome::Client::builder("k")
        .endpoint(format!("{}/", m.url))
        .http_client(http)
        .build()
        .unwrap();
    assert_eq!(c.endpoint(), m.url);
    c.session(SessionOptions {
        referer: Some("https://www.example.com/".into()),
        ..Default::default()
    })
    .generate_tags(tags())
    .await
    .unwrap();
    let req = m.last();
    assert_eq!(
        (req.path.as_str(), req.header("user-agent")),
        ("/v1/datadome/tags", Some("my-app/1"))
    );
    let err = datadome::Client::builder("k")
        .endpoint("antibot.johnavenue.com")
        .build()
        .unwrap_err();
    assert!(matches!(err, Error::Input(_)), "{err}");
}

#[test]
fn clients_and_sessions_are_shareable() {
    fn shareable<T: Clone + Send + Sync + 'static>() {}
    shareable::<datadome::Client>();
    shareable::<datadome::Session>();
    shareable::<john_antibot::cloudflare::Client>();
    shareable::<john_antibot::recaptcha::Client>();
    shareable::<john_antibot::account::Client>();
    shareable::<john_antibot::cloudflare::Job>();
    fn send<T: Send>(_: T) {}
    let mut s = datadome::Client::new("k").session(SessionOptions::default());
    send(s.generate_tags(TagsInput::default()));
    let _: &(dyn std::error::Error + Send + Sync) = &Error::Input("x".into());
}
