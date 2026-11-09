from __future__ import annotations

import hashlib
import json
from datetime import datetime, timezone

import httpx
import pytest

import john_antibot as ja
from john_antibot import datadome as dd
from john_antibot._http import COMPRESS_ABOVE, Call

from .conftest import ENDPOINT, API

INTERSTITIAL_HTML = """<html><head><meta name="dd-page-size" content="10510">
<script>var ddm = { cid: 'CID~x', hash: 'HASH', userEnv: 'UE', seed: 'SEED', env: 'ENV', s: 43624, b: 1265040, e: 'E1' };</script>
<script src="https://ct.captcha-delivery.com/interstitial.1.0.js"></script></head></html>"""
LINK = "https://geo.captcha-delivery.com/interstitial/?x=1"
CAPTCHA = "https://geo.captcha-delivery.com/captcha/?x=1"


def sha256(s: str) -> str:
    return hashlib.sha256(s.encode()).hexdigest()


def test_interstitial_wire_and_form(make):
    form = (
        "cid=CID~x&hash=HASH&referer=https%3A%2F%2Fwww.example.com%2F&url=https%3A%2F%2Fwww.example.com%2F&s=43624&e=E1&env=ENV"
        "&userEnv=UE&seed=SEED&b=1265040&dm=cd&ddMessageFormat=&payload=PLV2BLOB&plv3=PLV3TOK&ps=10510"
    )
    api = API({"POST /v1/datadome/interstitial": {"payload": form, "session": "sess-1", "userAgent": "UA-FROM-PROFILE"}})
    client = make(ja.DataDome, api, api_key="ja_live_x", org_key="ja_org_y")
    session = client.session(session_key="sess-1", timezone="America/New_York", timezone_offset=240)
    out = session.generate_interstitial(
        html=INTERSTITIAL_HTML,
        device_link=LINK,
        referer="https://www.example.com/",
        fetched_at=datetime.fromtimestamp(1792000000, timezone.utc),
        script="/*script*/",
    )
    seen = api.last
    assert (seen.method, seen.path, seen.query) == ("POST", "/v1/datadome/interstitial", {})
    assert seen.headers["x-api-key"] == "ja_live_x" and seen.headers["x-org-key"] == "ja_org_y"
    assert seen.headers["content-type"] == "application/json" and "content-encoding" not in seen.headers
    # the documented fields and nothing else: the API reads what the page carries from html
    assert seen.body == {
        "html": INTERSTITIAL_HTML,
        "deviceUrl": LINK,
        "scriptHash": sha256("/*script*/"),
        "referer": "https://www.example.com/",
        "session": "sess-1",
        "sessionKey": "sess-1",
        "timezone": "America/New_York",
        "timezoneOffset": 240,
        "userAgent": dd.LATEST_USER_AGENT,
        "challengeFetchedAtMs": 1792000000000,
    }
    # the session speaks the latest Chrome by default and keeps it
    assert session.session == "sess-1" and session.user_agent == dd.LATEST_USER_AGENT
    assert isinstance(out, dd.InterstitialResult)
    assert out.session == "sess-1" and out.user_agent == "UA-FROM-PROFILE"
    assert out.payload == form and out.form_body == form
    assert out.form_body_for("https://www.example.com/") == form
    assert out.form_body_for("") == form
    moved = out.form_body_for("https://www.example.com/a b~c")
    assert "referer=https%3A%2F%2Fwww.example.com%2Fa+b~c&url=https%3A%2F%2Fwww.example.com%2Fa+b~c&" in moved
    assert moved.startswith("cid=CID~x&hash=HASH&") and "%7E" not in moved


def test_tags_wire_and_learning(make):
    api = API({"POST /v1/datadome/tags": [{"payload": "FORM", "session": "sess-new", "userAgent": "UA", "generatedAt": 5}, {"payload": "FORM2"}]})
    session = make(ja.DataDome, api).session(referer="https://www.example.com/")
    out = session.generate_tags(event="ch", cookie="cookieval", site_key="KEY")
    assert api.last.path == "/v1/datadome/tags"
    assert api.last.body == {"cid": "cookieval", "jsType": "ch", "ddk": "KEY", "referer": "https://www.example.com/", "userAgent": dd.LATEST_USER_AGENT}
    assert out == dd.TagsResult(payload="FORM", session="sess-new", user_agent="UA", generated_at=5)
    assert session.session == "sess-new" and session.session_key == "sess-new"
    # the next call carries the session it learned, and the hash of the script, never the script
    session.generate_tags(event="le", cookie="c", site_key="KEY", version="5.1.0", script="/*tags*/", referer="https://www.example.com/p")
    assert api.last.body == {
        "session": "sess-new",
        "sessionKey": "sess-new",
        "cid": "c",
        "jsType": "le",
        "ddk": "KEY",
        "version": "5.1.0",
        "scriptHash": sha256("/*tags*/"),
        "referer": "https://www.example.com/p",
        "userAgent": dd.LATEST_USER_AGENT,
    }


def test_tags_page_fonts_and_script_url(make):
    api = API({"POST /v1/datadome/tags": {"payload": "F"}})
    s = make(ja.DataDome, api).session(referer="https://www.example.com/")
    s.generate_tags(
        event="ch", cookie="c", site_key="KEY", loaded_font_families=("Roobert", "Roobert Fallback"), tags_script_url="https://js.datadome.co/tags.js"
    )
    assert api.last.body == {
        "cid": "c",
        "jsType": "ch",
        "ddk": "KEY",
        "loadedFontFamilies": ["Roobert", "Roobert Fallback"],
        "tagsScriptUrl": "https://js.datadome.co/tags.js",
        "referer": "https://www.example.com/",
        "userAgent": dd.LATEST_USER_AGENT,
    }
    # empty ones are left out
    s.generate_tags(event="le", cookie="c", site_key="KEY", loaded_font_families=[], tags_script_url="")
    assert "loadedFontFamilies" not in api.last.body and "tagsScriptUrl" not in api.last.body


def test_session_identity(make):
    api = API({"POST /v1/datadome/tags": {"payload": "F"}})
    client = make(ja.DataDome, api)
    s = client.session(session="tok", session_key="other", device="mac-156", user_agent="UA/9", referer="https://www.example.com/")
    assert (s.session, s.session_key, s.device, s.user_agent) == ("tok", "tok", "mac-156", "UA/9")
    s.generate_tags(event="ch", cookie="c", site_key="KEY")
    assert api.last.body == {"session": "tok", "sessionKey": "tok", "device": "mac-156", "userAgent": "UA/9", "cid": "c", "jsType": "ch", "ddk": "KEY", "referer": "https://www.example.com/"}
    # a reply without a session keeps the one the session has
    assert s.session == "tok"
    assert client.session(session_key="k").session == "k"


def test_slider_wire_and_check_url(make):
    replies = [{"payload": "?cid=C&plv3=TOK", "session": "s2"}, {"payload": "https://geo.captcha-delivery.com/captcha/check?cid=C"}]
    api = API({"POST /v1/datadome/slider": replies})
    session = make(ja.DataDome, api).session()
    out = session.generate_slider(html="<html></html>", captcha_url=CAPTCHA, parent_url="https://www.example.com/", fetched_at=datetime(2026, 10, 15, tzinfo=timezone.utc))
    assert api.last.body == {
        "html": "<html></html>",
        "captchaUrl": CAPTCHA,
        "parentUrl": "https://www.example.com/",
        "challengeFetchedAtMs": 1792022400000,
        "userAgent": dd.LATEST_USER_AGENT,
    }
    # a query alone is a check URL on the captcha origin
    assert out.check_url == out.payload == dd.GEO_ORIGIN + "/captcha/check?cid=C&plv3=TOK"
    assert session.session == "s2"
    assert session.generate_slider(html="<html></html>", captcha_url=CAPTCHA).check_url == "https://geo.captcha-delivery.com/captcha/check?cid=C"


@pytest.mark.parametrize("route,call", [("interstitial", "generate_interstitial"), ("slider", "generate_slider")])
def test_replies_that_are_not_the_wire(make, route, call):
    api = API({f"POST /v1/datadome/{route}": {"payload": "jsData=x", "session": "s"}})
    session = make(ja.DataDome, api).session()
    kwargs = {"device_link": LINK} if route == "interstitial" else {"captcha_url": CAPTCHA}
    with pytest.raises(ja.JohnAntibotError) as err:
        getattr(session, call)(html="<html></html>", **kwargs)
    assert not isinstance(err.value, ja.ApiError) and "datadome: the API returned no" in str(err.value)
    assert session.session == ""  # nothing learned from a reply it refused


def test_interstitial_sends_the_hash_of_the_script(make):
    api = API({"POST /v1/datadome/interstitial": {"payload": "cid=x", "session": "s"}})
    make(ja.DataDome, api).session(session_key="s").generate_interstitial(html=INTERSTITIAL_HTML, script="/*challenge script*/", device_link=LINK)
    assert api.last.body["scriptHash"] == sha256("/*challenge script*/") and "script" not in api.last.body
    assert "referer" not in api.last.body and "challengeFetchedAtMs" not in api.last.body


def test_required_inputs_fail_before_the_network(make):
    api = API()  # any call would fail the test
    s = make(ja.DataDome, api).session(session_key="s")
    with pytest.raises(ValueError, match="device_link is required"):
        s.generate_interstitial(html=INTERSTITIAL_HTML, device_link="")
    with pytest.raises(ValueError, match="script is required: load https://ct.captcha-delivery.com/interstitial.1.0.js"):
        s.generate_interstitial(html=INTERSTITIAL_HTML, device_link=LINK)
    with pytest.raises(ValueError, match="captcha_url is required"):
        s.generate_slider(html="<html></html>", captcha_url="")
    with pytest.raises(ValueError, match="site_key is required: the site key from the page's DataDome config"):
        s.generate_tags(event="ch", cookie="c", site_key="", referer="https://www.example.com/")
    with pytest.raises(ValueError, match=r"referer \(or the session's referer\) is required"):
        s.generate_tags(event="ch", cookie="c", site_key="KEY")
    assert api.seen == []


@pytest.mark.parametrize(
    "status,body,hardblock,puzzle,limited",
    [
        (422, '{"error":"captcha page is hardblock (no solvable slider)","session":"s","hardblock":true}', True, False, False),
        (500, '{"error":"captcha page is hardblock (no solvable slider)"}', True, False, False),  # an earlier gateway's 500
        (500, '{"error":"generation failed"}', False, False, False),
        (422, '{"error":"puzzle slider unsupported"}', False, True, False),
        (429, '{"error":"gateway: busy"}', False, False, True),
    ],
)
def test_api_error_classification(make, status, body, hardblock, puzzle, limited):
    api = API({"POST /v1/datadome/slider": (status, body)})
    with pytest.raises(ja.ApiError) as err:
        make(ja.DataDome, api).session().generate_slider(html="<html></html>", captcha_url=CAPTCHA)
    e = err.value
    assert e.status == status and e.message == json.loads(body)["error"] and e.hardblock is ('"hardblock":true' in body)
    assert str(e) == f"datadome: HTTP {status}: {e.message}"
    assert (ja.is_hardblock(e), ja.is_puzzle_unsupported(e), ja.is_rate_limited(e)) == (hardblock, puzzle, limited)


def test_error_message_falls_back_to_the_body_then_the_status(make):
    api = API({"POST /v1/datadome/tags": [(502, "  upstream down \n"), (503, ""), (500, '{"error": 7}')]})
    s = make(ja.DataDome, api).session(referer="https://www.example.com/")
    messages = []
    for _ in range(3):
        with pytest.raises(ja.ApiError) as err:
            s.generate_tags(event="ch", cookie="c", site_key="KEY")
        messages.append(err.value.message)
    assert messages == ["upstream down", "503 Service Unavailable", '{"error": 7}']


def test_large_bodies_are_gzipped(make):
    api = API({"POST /v1/datadome/interstitial": {"payload": "cid=x"}})
    html = "<p>" * (40 << 10)
    make(ja.DataDome, api).session().generate_interstitial(html=html, device_link=LINK)
    assert api.last.headers["content-encoding"] == "gzip" and api.last.body["html"] == html and api.last.size < 10_000
    # the line is 32 KiB of JSON: one byte under is sent as it is
    client = ja.DataDome("k")
    for n, gzipped in ((COMPRESS_ABOVE - 8, True), (COMPRESS_ABOVE - 9, False)):
        req = client._request(httpx.Client(), Call("POST", "/x", {"a": "x" * n}))
        assert (req.headers.get("content-encoding") == "gzip") is gzipped
    # an empty key sends no header
    req = ja.DataDome("")._request(httpx.Client(), Call("GET", "/x"))
    assert "x-api-key" not in req.headers and "x-org-key" not in req.headers and "content-type" not in req.headers


def test_endpoint_and_timeout(make):
    api = API({"POST /v1/datadome/tags": {"payload": "F"}})
    make(ja.DataDome, api, endpoint=ENDPOINT + "/", timeout=7).session(referer="https://www.example.com/").generate_tags(event="ch", cookie="c", site_key="KEY")
    assert api.last.timeout["read"] == 7 and api.last.path == "/v1/datadome/tags"
    assert ja.DataDome("k").endpoint == "https://antibot.johnavenue.com" and ja.DataDome("k").timeout == 60
    assert ja.DataDome("k", endpoint="", timeout=0).endpoint == ja.DEFAULT_ENDPOINT


def test_a_call_that_times_out_raises_httpx_timeout(make):
    api = API({"POST /v1/datadome/tags": httpx.ReadTimeout("slow")})
    with pytest.raises(httpx.TimeoutException):
        make(ja.DataDome, api).session(referer="https://www.example.com/").generate_tags(event="ch", cookie="c", site_key="KEY")


def test_an_unreadable_2xx_is_an_error(make):
    api = API({"POST /v1/datadome/tags": "<html>"})
    with pytest.raises(ja.JohnAntibotError, match="decode response"):
        make(ja.DataDome, api).session(referer="https://www.example.com/").generate_tags(event="ch", cookie="c", site_key="KEY")


# ---- helpers


def test_sec_ch_ua_matches_chrome():
    assert dd.sec_ch_ua(dd.chrome_user_agent(131)) == '"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"'
    assert dd.sec_ch_ua(dd.chrome_user_agent(151)) == '"Not=A?Brand";v="99", "Google Chrome";v="151", "Chromium";v="151"'
    assert dd.sec_ch_ua(dd.chrome_user_agent(154)) == '"Chromium";v="154", "Google Chrome";v="154", "Not A(Brand";v="99"'
    assert dd.sec_ch_ua("Mozilla/5.0 (Macintosh) Version/18.0 Safari/605.1.15") == ""
    assert dd.chrome_major(dd.LATEST_USER_AGENT) == dd.LATEST_CHROME_MAJOR == 156
    assert dd.sec_ch_ua_platform(dd.LATEST_USER_AGENT) == '"macOS"' and dd.sec_ch_ua_platform("Windows NT 10.0") == '"Windows"'
    assert dd.sec_ch_ua_platform("Safari") == ""


def test_js_offset_minutes():
    assert dd.js_offset_minutes("America/New_York", datetime(2026, 10, 15, 12, tzinfo=timezone.utc)) == 240
    assert dd.js_offset_minutes("America/New_York", datetime(2026, 12, 15, 12, tzinfo=timezone.utc)) == 300
    assert dd.js_offset_minutes("Asia/Kolkata", datetime(2026, 1, 1, tzinfo=timezone.utc)) == -330


def test_parsers():
    d = dd.parse_dd("var dd = {'rt':'i','cid':'C1','hsh':'H1','t':'','s':123,'e':'E','b':456,'cookie':'CK'}")
    assert d == dd.DDObject(rt="i", cid="C1", hash="H1", t="", s="123", e="E", b="456", cookie="CK") and not d.hard_blocked
    u = dd.build_interstitial_url(d, "https://www.example.com/")
    assert u == dd.GEO_ORIGIN + "/interstitial/?initialCid=C1&hash=H1&cid=CK&referer=https%3A%2F%2Fwww.example.com%2F&s=123&e=E&b=456&dm=cd"
    c = dd.build_captcha_url(dd.DDObject(rt="c", cid="C", hash="H", t="fe", s="1", cookie="K"), "https://www.example.com/")
    assert c == dd.GEO_ORIGIN + "/captcha/?initialCid=C&hash=H&cid=K&t=fe&referer=https%3A%2F%2Fwww.example.com%2F&s=1&dm=cd"
    assert dd.parse_dd("var dd={'t':'bv'}").hard_blocked
    assert dd.challenge_script_url(INTERSTITIAL_HTML) == "https://ct.captcha-delivery.com/interstitial.1.0.js"
    assert dd.challenge_script_url("<script>var ddm = {};</script>") == ""
    assert dd.is_hardblock_page("var responseType = 'hardblock';") and not dd.is_hardblock_page("<html></html>")


def test_challenge_link_display_mode():
    # an API wall: a JSON 403, shown with dm=jd
    link, captcha = dd.challenge_link(
        r'{"url":"https:\/\/geo.captcha-delivery.com\/interstitial\/?initialCid=a&cid=b&hash=h&t=it"}', "https://www.example.com/api/x"
    )
    assert not captcha and link.endswith("&dm=jd") and link.startswith("https://geo.captcha-delivery.com/interstitial/?initialCid=a")
    # a document 403 with an iframe: dm=cd; a link that names its mode keeps it
    assert dd.challenge_link('<iframe src="https://geo.captcha-delivery.com/captcha/?cid=b&amp;t=fe"></iframe>') == (
        "https://geo.captcha-delivery.com/captcha/?cid=b&t=fe&dm=cd",
        True,
    )
    kept = dd.challenge_link('{"url":"https://geo.captcha-delivery.com/interstitial/?cid=b&dm=cd"}')
    assert kept is not None and kept[0].count("dm=") == 1
    # a var dd page: built, with the jar's cookie when the page has none
    found = dd.challenge_link("var dd={'rt':'i','cid':'C','hsh':'H','b':1,'s':2}", "https://www.example.com/", "jar")
    assert found is not None and not found[1] and "cid=jar" in found[0] and found[0].endswith("&dm=cd")
    found = dd.challenge_link("var dd={'rt':'c','cid':'C','hsh':'H','t':'fe','cookie':'CK'}", "https://www.example.com/", "jar")
    assert found is not None and found[1] and "/captcha/?" in found[0] and "cid=CK" in found[0]
    assert dd.challenge_link("<html>no challenge</html>") is None
    assert dd.challenge_link("var dd={'rt':'x','cid':'C','hsh':'H'}") is None


def test_validate_geo():
    a = dd.validate_geo(
        b'{"view":" Redirect ","url":"https://www.example.com/?t=fe&ir=5","cookie":"datadome=AbC~1_x; Max-Age=31536000; Domain=.example.com; Path=/; Secure"}'
    )
    assert a.is_json and a.view == "redirect" and a.cookie == "AbC~1_x" and a.cookie_max_age == 31536000 and a.cleared
    assert (a.t, a.ir, a.hard_block) == ("fe", "5", False)
    assert a.raw_cookie.startswith("datadome=AbC~1_x;")
    blocked = dd.validate_geo('{"view":"captcha","url":"https://geo.captcha-delivery.com/captcha/?t=BV"}')
    assert blocked.hard_block and blocked.t == "bv" and not blocked.cleared
    assert dd.validate_geo('{"view":"redirect","cookie":"other=1"}').cookie == "" and not dd.validate_geo('{"view":"redirect","cookie":"other=1"}').cleared
    assert dd.validate_geo('{"cookie":"datadome=x; Max-Age=0"}').cookie_max_age == -1
    assert dd.validate_geo("not json") == dd.GeoAnswer() and dd.validate_geo('{"view":1}') == dd.GeoAnswer() and not dd.validate_geo("[]").is_json
