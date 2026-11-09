//! Pure helpers for the pages and replies around a DataDome challenge.

use std::time::Duration;

use serde::Deserialize;

use super::InterstitialResponse;
use crate::http::escape;

/// Serves the interstitial and captcha pages.
pub const GEO_ORIGIN: &str = "https://geo.captcha-delivery.com";

/// The name of the DataDome session cookie.
pub const DATADOME_COOKIE: &str = "datadome";

/// `t=fe`: a solvable slider handoff.
pub const VERDICT_SOLVABLE: &str = "fe";

/// `t=bv`: a hard block.
pub const VERDICT_HARD_BLOCK: &str = "bv";

/// The Chrome major version default identities use.
pub const LATEST_CHROME_MAJOR: u32 = 156;

/// Chrome [`LATEST_CHROME_MAJOR`] on macOS: a session's user agent unless you set one.
pub const LATEST_USER_AGENT: &str = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36";

/// Chrome's reduced macOS user agent for a major version.
pub fn chrome_user_agent(major: u32) -> String {
    format!(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/{major}.0.0.0 Safari/537.36"
    )
}

/// The Chrome major version in `user_agent`; `None` when it is not Chrome.
pub fn chrome_major(user_agent: &str) -> Option<u32> {
    user_agent.match_indices("Chrome/").find_map(|(i, m)| {
        let rest = &user_agent[i + m.len()..];
        let digits = rest.bytes().take_while(u8::is_ascii_digit).count();
        (digits > 0 && rest[digits..].starts_with('.')).then(|| rest[..digits].parse().unwrap_or(0))
    })
}

/// The `sec-ch-ua` header Chrome sends for `user_agent`; `None` when it is not Chrome.
pub fn sec_ch_ua(user_agent: &str) -> Option<String> {
    let major = chrome_major(user_agent).filter(|&m| m != 0)? as usize;
    const CHARS: [&str; 11] = [" ", "(", ":", "-", ".", "/", ")", ";", "=", "?", "_"];
    const ORDERS: [[usize; 3]; 6] = [
        [0, 1, 2],
        [0, 2, 1],
        [1, 0, 2],
        [1, 2, 0],
        [2, 0, 1],
        [2, 1, 0],
    ];
    let grease = format!(
        r#""Not{}A{}Brand";v="{}""#,
        CHARS[major % 11],
        CHARS[(major + 1) % 11],
        ["8", "99", "24"][major % 3]
    );
    let order = ORDERS[major % 6];
    let mut list = [String::new(), String::new(), String::new()];
    list[order[0]] = grease;
    list[order[1]] = format!(r#""Chromium";v="{major}""#);
    list[order[2]] = format!(r#""Google Chrome";v="{major}""#);
    Some(list.join(", "))
}

/// The quoted `sec-ch-ua-platform` for `user_agent`.
pub fn sec_ch_ua_platform(user_agent: &str) -> Option<&'static str> {
    [
        ("Windows NT", r#""Windows""#),
        ("Macintosh", r#""macOS""#),
        ("Android", r#""Android""#),
        ("CrOS", r#""Chrome OS""#),
        ("Linux", r#""Linux""#),
    ]
    .into_iter()
    .find(|(needle, _)| user_agent.contains(needle))
    .map(|(_, platform)| platform)
}

/// The `var dd = {...}` block of a blocked page.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct DdObject {
    /// `i` interstitial, `c` captcha.
    pub rt: String,
    /// `cid` (the initial cid).
    pub cid: String,
    /// `hsh`: the site key.
    pub hash: String,
    /// Empty: solvable; `fe`: slider escalation; `bv`: hard block.
    pub t: String,
    /// `s`.
    pub s: String,
    /// `e`.
    pub e: String,
    /// `b`.
    pub b: String,
    /// `cookie`.
    pub cookie: String,
}

impl DdObject {
    /// The block is a hard block (`t=bv`).
    pub fn hard_blocked(&self) -> bool {
        self.t == VERDICT_HARD_BLOCK
    }
}

fn is_space(b: u8) -> bool {
    matches!(b, b'\t' | b'\n' | b'\x0c' | b'\r' | b' ')
}

fn skip_space(s: &str) -> &str {
    s.trim_start_matches(|c: char| c.is_ascii() && is_space(c as u8))
}

fn is_quote(c: char) -> bool {
    c == '\'' || c == '"'
}

/// The value of `'key': 'value'` or `'key': 123` in `html`.
fn dd_value(html: &str, key: &str) -> String {
    for (i, _) in html.match_indices(key) {
        if i == 0 || !is_quote(html[..i].chars().next_back().unwrap_or(' ')) {
            continue;
        }
        let Some(rest) = html[i + key.len()..].strip_prefix(is_quote) else {
            continue;
        };
        let Some(rest) = skip_space(rest).strip_prefix(':') else {
            continue;
        };
        let rest = skip_space(rest);
        if let Some(v) = rest.strip_prefix(is_quote) {
            if let Some(end) = v.find(is_quote) {
                return v[..end].to_string();
            }
            continue;
        }
        let sign = usize::from(rest.starts_with('-'));
        let digits = rest[sign..].bytes().take_while(u8::is_ascii_digit).count();
        if digits > 0 {
            return rest[..sign + digits].to_string();
        }
    }
    String::new()
}

/// The `dd` object of a blocked page body.
pub fn parse_dd(html: &str) -> DdObject {
    DdObject {
        rt: dd_value(html, "rt"),
        cid: dd_value(html, "cid"),
        hash: dd_value(html, "hsh"),
        t: dd_value(html, "t"),
        s: dd_value(html, "s"),
        e: dd_value(html, "e"),
        b: dd_value(html, "b"),
        cookie: dd_value(html, "cookie"),
    }
}

fn ordered_query(pairs: &[(&str, &str)]) -> String {
    pairs
        .iter()
        .map(|(k, v)| format!("{}={}", escape(k, true), escape(v, true)))
        .collect::<Vec<_>>()
        .join("&")
}

/// The interstitial URL for `dd`, blocked on the page `referer`.
pub fn build_interstitial_url(dd: &DdObject, referer: &str) -> String {
    let mut q = ordered_query(&[
        ("initialCid", &dd.cid),
        ("hash", &dd.hash),
        ("cid", &dd.cookie),
        ("referer", referer),
        ("s", &dd.s),
    ]);
    if !dd.e.is_empty() {
        q += &format!("&e={}", escape(&dd.e, true));
    }
    q += &format!("&b={}&dm=cd", escape(&dd.b, true));
    format!("{GEO_ORIGIN}/interstitial/?{q}")
}

/// The captcha URL for `dd`, blocked on the page `referer`.
pub fn build_captcha_url(dd: &DdObject, referer: &str) -> String {
    let mut q = ordered_query(&[
        ("initialCid", &dd.cid),
        ("hash", &dd.hash),
        ("cid", &dd.cookie),
        ("t", &dd.t),
        ("referer", referer),
        ("s", &dd.s),
    ]);
    if !dd.e.is_empty() {
        q += &format!("&e={}", escape(&dd.e, true));
    }
    q += "&dm=cd";
    format!("{GEO_ORIGIN}/captcha/?{q}")
}

/// The challenge script URL a challenge page loads; `None` when it is inline.
pub fn challenge_script_url(html: &str) -> Option<String> {
    const SRC: &str = "src=\"https://ct.captcha-delivery.com/";
    for (i, _) in html.match_indices("<script") {
        let tag_start = i + "<script".len();
        let tag_end = html[tag_start..]
            .find('>')
            .map_or(html.len(), |n| tag_start + n);
        for (j, _) in html[tag_start..tag_end]
            .match_indices(SRC)
            .filter(|&(j, _)| j > 0)
        {
            let url = &html[tag_start + j + "src=\"".len()..];
            let Some(end) = url.find('"') else { continue };
            let url = &url[..end];
            let path = &url[SRC.len() - "src=\"".len()..];
            if (path.starts_with("interstitial") || path.starts_with("captcha"))
                && url.ends_with(".js")
            {
                return Some(url.to_string());
            }
        }
    }
    None
}

/// A captcha page is the unsolvable hardblock variant.
pub fn is_hardblock_page(html: &str) -> bool {
    html.match_indices("responseType").any(|(i, m)| {
        let rest = skip_space(&html[i + m.len()..]);
        let Some(rest) = rest.strip_prefix('=') else {
            return false;
        };
        let Some(rest) = skip_space(rest).strip_prefix(is_quote) else {
            return false;
        };
        rest.strip_prefix("hardblock")
            .is_some_and(|r| r.starts_with(is_quote))
    })
}

/// The challenge a 403 body points to.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ChallengeLink {
    /// The interstitial or captcha URL to load.
    pub url: String,
    /// `true` for a slider (`/captcha/`), `false` for the interstitial.
    pub captcha: bool,
}

/// The challenge URL in a 403 body from `page_url`; `cookie` is the jar's `datadome` value, used when the page has none.
pub fn challenge_link(body: &str, page_url: &str, cookie: &str) -> Option<ChallengeLink> {
    let body = body.replace("\\/", "/");
    if let Some(link) = find_geo_link(&body) {
        let mut url = link.replace("&amp;", "&");
        if !url.contains("&dm=") {
            url += if body.trim_start().starts_with('{') {
                "&dm=jd"
            } else {
                "&dm=cd"
            };
        }
        let captcha = url.contains("/captcha/");
        return Some(ChallengeLink { url, captcha });
    }
    let mut dd = parse_dd(&body);
    if dd.cid.is_empty() || dd.hash.is_empty() {
        return None;
    }
    if dd.cookie.is_empty() {
        dd.cookie = cookie.to_string();
    }
    match dd.rt.as_str() {
        "c" => Some(ChallengeLink {
            url: build_captcha_url(&dd, page_url),
            captcha: true,
        }),
        "i" => Some(ChallengeLink {
            url: build_interstitial_url(&dd, page_url),
            captcha: false,
        }),
        _ => None,
    }
}

fn find_geo_link(body: &str) -> Option<&str> {
    const ORIGIN: &str = "https://geo.captcha-delivery.com/";
    body.match_indices(ORIGIN).find_map(|(i, _)| {
        let rest = &body[i + ORIGIN.len()..];
        let path = ["captcha/?", "interstitial/?"]
            .into_iter()
            .find(|p| rest.starts_with(p))?;
        let query = &rest[path.len()..];
        let n = query.find(|c: char| {
            matches!(c, '"' | '\'' | '\\' | '<') || (c.is_ascii() && is_space(c as u8))
        });
        let n = n.unwrap_or(query.len());
        (n > 0).then(|| &body[i..i + ORIGIN.len() + path.len() + n])
    })
}

/// The reply to an interstitial POST or a slider check GET, read by [`validate_geo`].
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct GeoAnswer {
    /// The body is JSON.
    pub is_json: bool,
    /// `redirect` when cleared, `captcha` on a slider handoff.
    pub view: String,
    /// The redirect target or the slider handoff URL.
    pub url: String,
    /// The `t` parameter of `url`.
    pub t: String,
    /// The `ir` parameter of `url`.
    pub ir: String,
    /// The whole Set-Cookie string.
    pub raw_cookie: String,
    /// The `datadome` value in `raw_cookie`.
    pub cookie: String,
    /// The lifetime of `cookie` (`Max-Age`).
    pub cookie_max_age: Option<Duration>,
    /// `t` is [`VERDICT_HARD_BLOCK`].
    pub hard_block: bool,
}

impl GeoAnswer {
    /// The session was cleared with a usable cookie.
    pub fn cleared(&self) -> bool {
        self.view == "redirect" && !self.cookie.is_empty()
    }
}

/// Reads a `geo.captcha-delivery.com` reply body.
pub fn validate_geo(body: &[u8]) -> GeoAnswer {
    #[derive(Deserialize)]
    #[serde(default)]
    #[derive(Default)]
    struct Geo {
        cookie: String,
        view: String,
        url: String,
    }
    let Ok(geo) = serde_json::from_slice::<Geo>(body) else {
        return GeoAnswer::default();
    };
    let (cookie, cookie_max_age) = read_set_cookie(&geo.cookie);
    let mut a = GeoAnswer {
        is_json: true,
        view: geo.view.trim().to_lowercase(),
        url: geo.url,
        raw_cookie: geo.cookie,
        cookie,
        cookie_max_age,
        ..Default::default()
    };
    if !a.url.is_empty() {
        if let Ok(u) = reqwest::Url::parse(GEO_ORIGIN).and_then(|base| base.join(&a.url)) {
            for (k, v) in u.query_pairs() {
                match k.as_ref() {
                    "t" if a.t.is_empty() => a.t = v.to_lowercase(),
                    "ir" if a.ir.is_empty() => a.ir = v.into_owned(),
                    _ => {}
                }
            }
            a.hard_block = a.t == VERDICT_HARD_BLOCK;
        }
    }
    a
}

/// The `datadome` value of a Set-Cookie line and its `Max-Age`.
fn read_set_cookie(line: &str) -> (String, Option<Duration>) {
    let mut parts = line.split(';');
    let Some((name, value)) = parts.next().and_then(|p| p.trim().split_once('=')) else {
        return (String::new(), None);
    };
    if name.trim() != DATADOME_COOKIE {
        return (String::new(), None);
    }
    let value = value.trim();
    let value = value
        .strip_prefix('"')
        .and_then(|v| v.strip_suffix('"'))
        .unwrap_or(value);
    let max_age = parts.find_map(|attr| {
        let (k, v) = attr.trim().split_once('=')?;
        k.eq_ignore_ascii_case("max-age")
            .then(|| v.trim().parse::<u64>().ok().filter(|&s| s > 0))
            .flatten()
    });
    (value.to_string(), max_age.map(Duration::from_secs))
}

impl InterstitialResponse {
    /// The form body with its `referer` and `url` fields set to `page_url`; an empty `page_url` returns it unchanged.
    pub fn form_body_for(&self, page_url: &str) -> String {
        if page_url.is_empty() || self.form_body.is_empty() {
            return self.form_body.clone();
        }
        self.form_body
            .split('&')
            .map(|part| match part.split('=').next() {
                Some(key @ ("referer" | "url")) => format!("{key}={}", escape(page_url, true)),
                _ => part.to_string(),
            })
            .collect::<Vec<_>>()
            .join("&")
    }
}

/// A slider reply's payload as the URL to GET: absolute as is, a bare query under `GEO_ORIGIN/captcha/check`.
pub(crate) fn normalize_check_url(payload: &str) -> Option<String> {
    if payload.starts_with("http://") || payload.starts_with("https://") {
        Some(payload.to_string())
    } else if payload.starts_with('?') {
        Some(format!("{GEO_ORIGIN}/captcha/check{payload}"))
    } else {
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const INTERSTITIAL_HTML: &str = r#"<html><head><meta name="dd-page-size" content="10510">
<script>var ddm = { cid: 'CID~x', hash: 'HASH', userEnv: 'UE', seed: 'SEED', env: 'ENV', s: 43624, b: 1265040, e: 'E1' };</script>
<script src="https://ct.captcha-delivery.com/interstitial.1.0.js"></script></head></html>"#;

    #[test]
    fn parsers() {
        let dd = parse_dd(
            r#"var dd = {'rt':'i','cid':'C1','hsh':'H1','t':'','s':123,'e':'E','b':456,'cookie':'CK'}"#,
        );
        assert_eq!(
            dd,
            DdObject {
                rt: "i".into(),
                cid: "C1".into(),
                hash: "H1".into(),
                t: "".into(),
                s: "123".into(),
                e: "E".into(),
                b: "456".into(),
                cookie: "CK".into()
            }
        );
        let u = build_interstitial_url(&dd, "https://www.example.com/");
        assert!(
            u.starts_with(&format!(
                "{GEO_ORIGIN}/interstitial/?initialCid=C1&hash=H1&cid=CK&referer="
            )),
            "{u}"
        );
        assert!(u.ends_with("&b=456&dm=cd"), "{u}");
        assert!(
            u.contains("referer=https%3A%2F%2Fwww.example.com%2F&s=123&e=E"),
            "{u}"
        );
        let c = build_captcha_url(
            &DdObject {
                t: "fe".into(),
                ..dd.clone()
            },
            "https://www.example.com/",
        );
        assert!(
            c.starts_with(&format!(
                "{GEO_ORIGIN}/captcha/?initialCid=C1&hash=H1&cid=CK&t=fe&referer="
            )),
            "{c}"
        );
        assert!(c.ends_with("&e=E&dm=cd"), "{c}");
        assert!(is_hardblock_page("var responseType = 'hardblock';"));
        assert!(is_hardblock_page(r#"responseType="hardblock""#));
        assert!(!is_hardblock_page("var responseType = 'captcha';"));
        assert!(parse_dd("var dd = {'t':'bv','rt':'c'}").hard_blocked());
        // a number may be negative; a key must be quoted
        assert_eq!(parse_dd(r#"{"s": -12, cid: 'no', "cid" : "yes"}"#).s, "-12");
        assert_eq!(parse_dd(r#"{cid: 'no', "cid" : "yes"}"#).cid, "yes");
    }

    #[test]
    fn challenge_script() {
        assert_eq!(
            challenge_script_url(INTERSTITIAL_HTML).as_deref(),
            Some("https://ct.captcha-delivery.com/interstitial.1.0.js")
        );
        assert_eq!(
            challenge_script_url(r#"<script type="text/javascript" src="https://ct.captcha-delivery.com/captcha.3.1.js"></script>"#)
                .as_deref(),
            Some("https://ct.captcha-delivery.com/captcha.3.1.js")
        );
        assert_eq!(challenge_script_url("<script>var ddm = {};</script>"), None);
        assert_eq!(
            challenge_script_url(r#"<script src="https://ct.captcha-delivery.com/c.js"></script>"#),
            None
        );
        assert_eq!(
            challenge_script_url(
                r#"<script src="https://ct.captcha-delivery.com/captcha.js?x"></script>"#
            ),
            None
        );
        assert_eq!(
            challenge_script_url(
                r#"<script src="https://ct.captcha-delivery.com/tags.js"></script>"#
            ),
            None
        );
        assert_eq!(
            challenge_script_url(
                r#"<script src="https://ct.captcha-delivery.com/captcha.css"></script>"#
            ),
            None
        );
    }

    #[test]
    fn challenge_link_display_mode() {
        // API wall: JSON 403, the tag shows it with dm=jd.
        let l = challenge_link(
            r#"{"url":"https:\/\/geo.captcha-delivery.com\/interstitial\/?initialCid=a&cid=b&hash=h&t=it"}"#,
            "https://www.example.com/api/x",
            "",
        )
        .unwrap();
        assert!(!l.captcha && l.url.ends_with("&dm=jd"), "{l:?}");
        // Document 403 with an iframe: dm=cd; a link naming its mode is kept.
        let l = challenge_link(
            r#"<iframe src="https://geo.captcha-delivery.com/captcha/?cid=b&amp;t=fe"></iframe>"#,
            "",
            "",
        )
        .unwrap();
        assert_eq!(
            l,
            ChallengeLink {
                url: "https://geo.captcha-delivery.com/captcha/?cid=b&t=fe&dm=cd".into(),
                captcha: true
            }
        );
        let l = challenge_link(
            r#"{"url":"https://geo.captcha-delivery.com/interstitial/?cid=b&dm=cd"}"#,
            "",
            "",
        )
        .unwrap();
        assert_eq!(l.url.matches("dm=").count(), 1, "{l:?}");
        // var dd block page: built, with the jar's cookie when the page has none.
        let l = challenge_link(
            "var dd={'rt':'i','cid':'C','hsh':'H','b':1,'s':2}",
            "https://www.example.com/",
            "jar",
        )
        .unwrap();
        assert!(
            !l.captcha && l.url.contains("cid=jar") && l.url.ends_with("&dm=cd"),
            "{l:?}"
        );
        let l = challenge_link(
            "var dd={'rt':'c','cid':'C','hsh':'H','t':'fe','s':2}",
            "https://www.example.com/",
            "",
        )
        .unwrap();
        assert!(
            l.captcha && l.url.starts_with(&format!("{GEO_ORIGIN}/captcha/?")),
            "{l:?}"
        );
        assert_eq!(challenge_link("<html>no challenge</html>", "", ""), None);
    }

    #[test]
    fn chrome_headers_match_chrome() {
        for (major, want) in [
            (
                131,
                r#""Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24""#,
            ),
            (
                151,
                r#""Not=A?Brand";v="99", "Google Chrome";v="151", "Chromium";v="151""#,
            ),
            (
                154,
                r#""Chromium";v="154", "Google Chrome";v="154", "Not A(Brand";v="99""#,
            ),
        ] {
            assert_eq!(
                sec_ch_ua(&chrome_user_agent(major)).as_deref(),
                Some(want),
                "Chrome {major}"
            );
        }
        assert_eq!(
            sec_ch_ua("Mozilla/5.0 (Macintosh) Version/18.0 Safari/605.1.15"),
            None
        );
        assert_eq!(chrome_user_agent(LATEST_CHROME_MAJOR), LATEST_USER_AGENT);
        assert_eq!(chrome_major(LATEST_USER_AGENT), Some(LATEST_CHROME_MAJOR));
        assert_eq!(sec_ch_ua_platform(LATEST_USER_AGENT), Some(r#""macOS""#));
        assert_eq!(chrome_major("Chrome/x Chrome/12.1"), Some(12));
    }

    #[test]
    fn geo_answers() {
        let a = validate_geo(
            br#"{"cookie":"datadome=CK~1; Max-Age=31536000; Domain=.example.com; Path=/; Secure; SameSite=Lax","view":"Redirect","url":"https://www.example.com/"}"#,
        );
        assert!(
            a.is_json
                && a.cleared()
                && a.cookie == "CK~1"
                && a.cookie_max_age == Some(Duration::from_secs(31536000)),
            "{a:?}"
        );
        let a = validate_geo(br#"{"view":"captcha","url":"https://geo.captcha-delivery.com/captcha/?initialCid=x&t=BV&ir=503"}"#);
        assert!(
            a.hard_block && a.t == "bv" && a.ir == "503" && !a.cleared(),
            "{a:?}"
        );
        let a = validate_geo(br#"{"cookie":"other=1","view":"redirect"}"#);
        assert!(a.cookie.is_empty() && !a.cleared(), "{a:?}");
        assert!(!validate_geo(b"<html>").is_json);
    }

    #[test]
    fn form_body_for_sets_the_page() {
        let r = InterstitialResponse {
            form_body: "cid=CID~x&referer=old&url=old&s=1".into(),
            ..Default::default()
        };
        assert_eq!(
            r.form_body_for("https://www.example.com/a b~"),
            "cid=CID~x&referer=https%3A%2F%2Fwww.example.com%2Fa+b~&url=https%3A%2F%2Fwww.example.com%2Fa+b~&s=1"
        );
        assert_eq!(r.form_body_for(""), r.form_body);
    }

    #[test]
    fn check_urls() {
        assert_eq!(
            normalize_check_url("?cid=C&x=1").as_deref(),
            Some("https://geo.captcha-delivery.com/captcha/check?cid=C&x=1")
        );
        assert_eq!(
            normalize_check_url("https://geo.captcha-delivery.com/captcha/check?cid=C").as_deref(),
            Some("https://geo.captcha-delivery.com/captcha/check?cid=C")
        );
        assert_eq!(normalize_check_url("PAYLOADBLOB"), None);
        assert_eq!(normalize_check_url(""), None);
    }
}
