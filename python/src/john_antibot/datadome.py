"""DataDome: payloads for tags, the interstitial and the slider, and the helpers a flow needs around them."""

from __future__ import annotations

import hashlib
import json
import re
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, Optional, Sequence, Tuple, Union
from urllib.parse import parse_qs, quote_plus, urlsplit
from zoneinfo import ZoneInfo

from ._http import Call, Op, _Async, _Sync
from ._jobs import number, text
from .errors import JohnAntibotError

__all__ = [
    "DATADOME_COOKIE",
    "GEO_ORIGIN",
    "LATEST_CHROME_MAJOR",
    "LATEST_USER_AGENT",
    "AsyncDataDome",
    "AsyncSession",
    "DDObject",
    "DataDome",
    "GeoAnswer",
    "InterstitialResult",
    "Session",
    "SliderResult",
    "TagsResult",
    "build_captcha_url",
    "build_interstitial_url",
    "challenge_link",
    "challenge_script_url",
    "chrome_major",
    "chrome_user_agent",
    "is_hardblock_page",
    "js_offset_minutes",
    "parse_dd",
    "sec_ch_ua",
    "sec_ch_ua_platform",
    "validate_geo",
]

# The origin of the interstitial and captcha pages.
GEO_ORIGIN = "https://geo.captcha-delivery.com"
# The name of the DataDome session cookie.
DATADOME_COOKIE = "datadome"

# ---- Chrome identity


def chrome_user_agent(major: int) -> str:
    """Chrome's reduced macOS User-Agent for a major version."""
    return f"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/{major}.0.0.0 Safari/537.36"


# The Chrome major version a new session speaks.
LATEST_CHROME_MAJOR = 156
LATEST_USER_AGENT = chrome_user_agent(LATEST_CHROME_MAJOR)

_CHROME_MAJOR = re.compile(r"Chrome/(\d+)\.")


def chrome_major(user_agent: str) -> int:
    """The Chrome major version in user_agent, or 0 if it is not Chrome."""
    m = _CHROME_MAJOR.search(user_agent)
    return int(m.group(1)) if m else 0


def sec_ch_ua(user_agent: str) -> str:
    """The sec-ch-ua header Chrome sends with user_agent, or "" if it is not Chrome."""
    major = chrome_major(user_agent)
    if major == 0:
        return ""
    chars = [" ", "(", ":", "-", ".", "/", ")", ";", "=", "?", "_"]
    grease = f'"Not{chars[major % 11]}A{chars[(major + 1) % 11]}Brand";v="{["8", "99", "24"][major % 3]}"'
    order = [(0, 1, 2), (0, 2, 1), (1, 0, 2), (1, 2, 0), (2, 0, 1), (2, 1, 0)][major % 6]
    brands = ["", "", ""]
    brands[order[0]] = grease
    brands[order[1]] = f'"Chromium";v="{major}"'
    brands[order[2]] = f'"Google Chrome";v="{major}"'
    return ", ".join(brands)


def sec_ch_ua_platform(user_agent: str) -> str:
    """The quoted sec-ch-ua-platform for user_agent."""
    for marker, platform in (("Windows NT", '"Windows"'), ("Macintosh", '"macOS"'), ("Android", '"Android"'), ("CrOS", '"Chrome OS"'), ("Linux", '"Linux"')):
        if marker in user_agent:
            return platform
    return ""


def js_offset_minutes(zone: str, at: Optional[datetime] = None) -> int:
    """JavaScript's Date.getTimezoneOffset() for the IANA zone at a time (default now): the session's timezone_offset."""
    offset = (at or datetime.now(timezone.utc)).astimezone(ZoneInfo(zone)).utcoffset()
    return -int(offset.total_seconds() / 60) if offset else 0


# ---- Pages


@dataclass
class DDObject:
    """The ``var dd = {...}`` block of a blocked page."""

    rt: str = ""  # "i": interstitial, "c": captcha
    cid: str = ""
    hash: str = ""
    t: str = ""  # "": solvable, "fe": slider escalation, "bv": hard block
    s: str = ""
    e: str = ""
    b: str = ""
    cookie: str = ""

    @property
    def hard_blocked(self) -> bool:
        """The block is a hard block (t=bv): change the proxy and the session."""
        return self.t == "bv"


def parse_dd(html: str) -> DDObject:
    """The dd object of a blocked page body."""

    def get(key: str) -> str:
        m = re.search(r"""['"]""" + re.escape(key) + r"""['"]\s*:\s*(?:['"]([^'"]*)['"]|(-?[0-9]+))""", html)
        return (m.group(1) or m.group(2) or "") if m else ""

    return DDObject(rt=get("rt"), cid=get("cid"), hash=get("hsh"), t=get("t"), s=get("s"), e=get("e"), b=get("b"), cookie=get("cookie"))


def _q(value: str) -> str:
    return quote_plus(value, safe="")


def _query(pairs: Tuple[Tuple[str, str], ...]) -> str:
    return "&".join(f"{_q(k)}={_q(v)}" for k, v in pairs)


def build_interstitial_url(dd: DDObject, referer: str) -> str:
    """The interstitial URL for dd, blocked on the page referer."""
    q = _query((("initialCid", dd.cid), ("hash", dd.hash), ("cid", dd.cookie), ("referer", referer), ("s", dd.s)))
    if dd.e:
        q += "&e=" + _q(dd.e)
    return f"{GEO_ORIGIN}/interstitial/?{q}&b={_q(dd.b)}&dm=cd"


def build_captcha_url(dd: DDObject, referer: str) -> str:
    """The captcha URL for dd, blocked on the page referer."""
    q = _query((("initialCid", dd.cid), ("hash", dd.hash), ("cid", dd.cookie), ("t", dd.t), ("referer", referer), ("s", dd.s)))
    if dd.e:
        q += "&e=" + _q(dd.e)
    return f"{GEO_ORIGIN}/captcha/?{q}&dm=cd"


_CHALLENGE_SCRIPT = re.compile(r'<script[^>]+src="(https://ct\.captcha-delivery\.com/(?:interstitial|captcha)[^"]*\.js)"')
_CHALLENGE_LINK = re.compile(r"""https://geo\.captcha-delivery\.com/(?:captcha|interstitial)/\?[^"'\\\s<]+""")
_HARDBLOCK_PAGE = re.compile(r"""responseType\s*=\s*['"]hardblock['"]""")


def challenge_script_url(html: str) -> str:
    """The script an interstitial or captcha page loads, or "" when it is inline. Load it right after the page."""
    m = _CHALLENGE_SCRIPT.search(html)
    return m.group(1) if m else ""


def challenge_link(body: str, page_url: str = "", cookie: str = "") -> Optional[Tuple[str, bool]]:
    """The challenge URL in a 403 body and whether it is a captcha (slider), or None when there is none.

    cookie is the current datadome cookie, used when the block page carries none.
    """
    body = body.replace("\\/", "/")
    m = _CHALLENGE_LINK.search(body)
    if m:
        link = m.group(0).replace("&amp;", "&")
        if "&dm=" not in link:
            link += "&dm=" + ("jd" if body.strip().startswith("{") else "cd")
        return link, "/captcha/" in link
    dd = parse_dd(body)
    if not dd.cid or not dd.hash:
        return None
    dd.cookie = dd.cookie or cookie
    if dd.rt == "c":
        return build_captcha_url(dd, page_url), True
    if dd.rt == "i":
        return build_interstitial_url(dd, page_url), False
    return None


def is_hardblock_page(html: str) -> bool:
    """A captcha page is the hardblock variant, with nothing to solve."""
    return bool(_HARDBLOCK_PAGE.search(html))


# ---- Replies of geo.captcha-delivery.com


@dataclass
class GeoAnswer:
    """The reply to an interstitial POST or a slider check GET."""

    is_json: bool = False
    view: str = ""  # "redirect" when cleared, "captcha" on a slider handoff
    url: str = ""  # the redirect target, or the slider handoff URL
    t: str = ""  # the t parameter of url
    ir: str = ""  # the ir parameter of url
    raw_cookie: str = ""  # the whole Set-Cookie string
    cookie: str = ""  # the datadome value in it
    cookie_max_age: int = 0  # its lifetime in seconds (-1: delete it)
    hard_block: bool = False  # t is bv

    @property
    def cleared(self) -> bool:
        """The session was cleared, with a cookie to keep."""
        return self.view == "redirect" and bool(self.cookie)


def validate_geo(body: Union[str, bytes]) -> GeoAnswer:
    """Parse a geo.captcha-delivery.com reply body."""
    try:
        data = json.loads(body)
    except ValueError:
        return GeoAnswer()
    if not isinstance(data, dict) or any(data.get(k) is not None and not isinstance(data[k], str) for k in ("cookie", "view", "url")):
        return GeoAnswer()
    a = GeoAnswer(is_json=True, view=text(data, "view").strip().lower(), url=text(data, "url"), raw_cookie=text(data, "cookie"))
    a.cookie, a.cookie_max_age = _read_set_cookie(a.raw_cookie)
    if a.url:
        try:
            query = parse_qs(urlsplit(a.url).query)
        except ValueError:
            return a
        a.t = query.get("t", [""])[0].lower()
        a.ir = query.get("ir", [""])[0]
        a.hard_block = a.t == "bv"
    return a


def _read_set_cookie(line: str) -> Tuple[str, int]:
    first, *attrs = line.split(";")
    name, eq, value = first.partition("=")
    if not eq or name.strip() != DATADOME_COOKIE:
        return "", 0
    value = value.strip()
    if len(value) > 1 and value[0] == value[-1] == '"':
        value = value[1:-1]
    max_age = 0
    for attr in attrs:
        key, _, v = attr.partition("=")
        if key.strip().lower() == "max-age":
            try:
                max_age = int(v.strip())
            except ValueError:
                continue
            max_age = max_age if max_age > 0 else -1
    return value, max_age


def _replace_form_fields(form: str, values: Dict[str, str]) -> str:
    parts = form.split("&") if form else []
    for i, part in enumerate(parts):
        key = part.partition("=")[0]
        if key in values:
            parts[i] = _q(key) + "=" + _q(values[key]).replace("%7E", "~")
    return "&".join(parts)


# ---- Results


@dataclass
class TagsResult:
    payload: str  # the form body for the site's /js/
    session: str = ""
    user_agent: str = ""
    generated_at: int = 0  # Unix milliseconds


@dataclass
class InterstitialResult:
    payload: str
    form_body: str  # = payload: the form body for GEO_ORIGIN + "/interstitial/"
    session: str = ""
    user_agent: str = ""

    def form_body_for(self, page_url: str) -> str:
        """The form body with its referer and url fields set to page_url ("" leaves it as is)."""
        body = self.form_body or self.payload
        return _replace_form_fields(body, {"referer": page_url, "url": page_url}) if page_url else body


@dataclass
class SliderResult:
    payload: str
    check_url: str  # = payload: the /captcha/check URL to GET
    session: str = ""
    user_agent: str = ""


# ---- Sessions


def _script_hash(script: str) -> str:
    return hashlib.sha256(script.encode()).hexdigest()


def _require_script(html: str, script: str) -> None:
    url = challenge_script_url(html)
    if url and not script:
        raise ValueError(f"datadome: script is required: load {url} right after the page and pass its source")


_EPOCH = datetime(1970, 1, 1, tzinfo=timezone.utc)


def _unix_ms(at: datetime) -> int:
    return ((at if at.tzinfo else at.astimezone()) - _EPOCH) // timedelta(milliseconds=1)


class _SessionState:
    def __init__(
        self, *, session: str, session_key: str, device: str, user_agent: str, referer: str, timezone: str, timezone_offset: int
    ) -> None:
        self.session = session or session_key  # the API's session token: set it only to resume one
        self.session_key = self.session  # an alias of session
        self.device = device  # pins a device profile by name
        self.user_agent = user_agent or LATEST_USER_AGENT  # send it on every request of the session
        self.referer = referer  # the default page URL of the calls
        self.timezone = timezone  # IANA name
        self.timezone_offset = timezone_offset  # see js_offset_minutes

    def _learn(self, session: str, user_agent: str) -> None:
        if session:
            self.session = self.session_key = session
        if not self.user_agent and user_agent:
            self.user_agent = user_agent

    def _identity(self) -> Dict[str, Any]:
        m: Dict[str, Any] = {}
        token = self.session or self.session_key
        if token:
            m["session"] = m["sessionKey"] = token
        if self.device:
            m["device"] = self.device
        if self.user_agent:
            m["userAgent"] = self.user_agent
        if self.timezone:
            m["timezone"] = self.timezone
        if self.timezone_offset:
            m["timezoneOffset"] = self.timezone_offset
        return m

    def _tags(
        self, event: str, cookie: str, site_key: str, version: str, script: str, referer: str,
        fonts: Optional[Sequence[str]], tags_script_url: Optional[str],
    ) -> Op[TagsResult]:  # fmt: skip
        if not site_key:
            raise ValueError("datadome: site_key is required: the site key from the page's DataDome config")
        req = self._identity()
        req["cid"] = cookie
        req["jsType"] = event
        req["ddk"] = site_key
        if version:
            req["version"] = version
        if script:
            req["scriptHash"] = _script_hash(script)
        if fonts:
            req["loadedFontFamilies"] = list(fonts)
        if tags_script_url:
            req["tagsScriptUrl"] = tags_script_url
        req["referer"] = referer or self.referer
        if not req["referer"]:
            raise ValueError("datadome: referer (or the session's referer) is required: the page the tags run on")
        data = yield Call("POST", "/v1/datadome/tags", req)
        out = TagsResult(payload=text(data, "payload"), session=text(data, "session"), user_agent=text(data, "userAgent"), generated_at=number(data, "generatedAt"))
        self._learn(out.session, out.user_agent)
        return out

    def _interstitial(self, html: str, device_link: str, script: str, referer: str, fetched_at: Optional[datetime]) -> Op[InterstitialResult]:
        if not device_link:
            raise ValueError("datadome: device_link is required: the interstitial URL you loaded")
        _require_script(html, script)
        req = self._identity()
        req["html"] = html
        req["deviceUrl"] = device_link
        if referer or self.referer:
            req["referer"] = referer or self.referer
        if fetched_at is not None:
            req["challengeFetchedAtMs"] = _unix_ms(fetched_at)
        if script:
            req["scriptHash"] = _script_hash(script)
        data = yield Call("POST", "/v1/datadome/interstitial", req)
        payload = text(data, "payload")
        if not payload.startswith("cid="):
            raise JohnAntibotError("datadome: the API returned no ready-to-post form")
        out = InterstitialResult(payload=payload, form_body=payload, session=text(data, "session"), user_agent=text(data, "userAgent"))
        self._learn(out.session, out.user_agent)
        return out

    def _slider(self, html: str, captcha_url: str, script: str, parent_url: str, fetched_at: Optional[datetime]) -> Op[SliderResult]:
        if not captcha_url:
            raise ValueError("datadome: captcha_url is required: the captcha URL you loaded")
        _require_script(html, script)
        req = self._identity()
        req["html"] = html
        req["captchaUrl"] = captcha_url
        if parent_url:
            req["parentUrl"] = parent_url
        if fetched_at is not None:
            req["challengeFetchedAtMs"] = _unix_ms(fetched_at)
        if script:
            req["scriptHash"] = _script_hash(script)
        data = yield Call("POST", "/v1/datadome/slider", req)
        payload = text(data, "payload")
        if payload.startswith(("http://", "https://")):
            url = payload
        elif payload.startswith("?"):
            url = GEO_ORIGIN + "/captcha/check" + payload
        else:
            raise JohnAntibotError("datadome: the API returned no check URL")
        out = SliderResult(payload=url, check_url=url, session=text(data, "session"), user_agent=text(data, "userAgent"))
        self._learn(out.session, out.user_agent)
        return out


class Session(_SessionState):
    """One browser identity: one proxy and one cookie jar. Made by DataDome.session; not for concurrent use."""

    def __init__(self, client: DataDome, **identity: Any) -> None:
        super().__init__(**identity)
        self._client = client

    def generate_tags(
        self, *, event: str, cookie: str, site_key: str, version: str = "", script: str = "", referer: str = "",
        loaded_font_families: Optional[Sequence[str]] = None, tags_script_url: Optional[str] = None,
    ) -> TagsResult:  # fmt: skip
        """A tags payload: event "ch", then "le". script is the tags.js source the page loaded (only its SHA-256 is sent);
        loaded_font_families the @font-face families the page loaded; tags_script_url the site's tags.js when it is not
        https://datadome.<site>/tags.js."""
        return self._client._run(self._tags(event, cookie, site_key, version, script, referer, loaded_font_families, tags_script_url))

    def generate_interstitial(
        self, *, html: str, device_link: str, script: str = "", referer: str = "", fetched_at: Optional[datetime] = None
    ) -> InterstitialResult:
        """The interstitial form. device_link is the URL html came from; script the source of the script it loads."""
        return self._client._run(self._interstitial(html, device_link, script, referer, fetched_at))

    def generate_slider(
        self, *, html: str, captcha_url: str, script: str = "", parent_url: str = "", fetched_at: Optional[datetime] = None
    ) -> SliderResult:
        """The slider check URL. captcha_url is the URL html came from; script the source of the script it loads."""
        return self._client._run(self._slider(html, captcha_url, script, parent_url, fetched_at))


class AsyncSession(_SessionState):
    """Session for asyncio: the methods of Session, awaited."""

    def __init__(self, client: AsyncDataDome, **identity: Any) -> None:
        super().__init__(**identity)
        self._client = client

    async def generate_tags(
        self, *, event: str, cookie: str, site_key: str, version: str = "", script: str = "", referer: str = "",
        loaded_font_families: Optional[Sequence[str]] = None, tags_script_url: Optional[str] = None,
    ) -> TagsResult:  # fmt: skip
        return await self._client._run(self._tags(event, cookie, site_key, version, script, referer, loaded_font_families, tags_script_url))

    async def generate_interstitial(
        self, *, html: str, device_link: str, script: str = "", referer: str = "", fetched_at: Optional[datetime] = None
    ) -> InterstitialResult:
        return await self._client._run(self._interstitial(html, device_link, script, referer, fetched_at))

    async def generate_slider(
        self, *, html: str, captcha_url: str, script: str = "", parent_url: str = "", fetched_at: Optional[datetime] = None
    ) -> SliderResult:
        return await self._client._run(self._slider(html, captcha_url, script, parent_url, fetched_at))


class DataDome(_Sync):
    """DataDome client. Make one Session per proxy and cookie jar."""

    _service = "datadome"
    _default_timeout = 60.0

    def session(
        self, *, referer: str = "", session: str = "", session_key: str = "", device: str = "", user_agent: str = "",
        timezone: str = "", timezone_offset: int = 0,
    ) -> Session:  # fmt: skip
        return Session(
            self, session=session, session_key=session_key, device=device, user_agent=user_agent, referer=referer,
            timezone=timezone, timezone_offset=timezone_offset,
        )  # fmt: skip


class AsyncDataDome(_Async):
    """DataDome client for asyncio."""

    _service = "datadome"
    _default_timeout = 60.0

    def session(
        self, *, referer: str = "", session: str = "", session_key: str = "", device: str = "", user_agent: str = "",
        timezone: str = "", timezone_offset: int = 0,
    ) -> AsyncSession:  # fmt: skip
        return AsyncSession(
            self, session=session, session_key=session_key, device=device, user_agent=user_agent, referer=referer,
            timezone=timezone, timezone_offset=timezone_offset,
        )  # fmt: skip
