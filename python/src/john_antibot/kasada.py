"""Kasada: a binary /tl payload from the ips.js your client fetched."""

from __future__ import annotations

import base64
import hashlib
import json
import os
import random
import time
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional

from ._http import Call, Op, _Async, _Sync

__all__ = ["AsyncKasada", "CDParams", "Kasada", "PayloadResult", "VersionResult"]

SERVICE = "kasada"
PATH = "/v1/kasada/payload"


def _sha256(text: str) -> str:
    return hashlib.sha256(text.encode()).hexdigest()


def _cd_answers(first: str, difficulty: int, rounds: int) -> List[int]:
    """p.js's proof of work: per round, the first nonce n whose sha256("<n>, <previous hash>") scores at least
    difficulty/rounds (score = 2^52 over the first 13 hex digits, plus one); the passing hash starts the next round."""
    target = difficulty / rounds
    digest = _sha256(first)
    answers: List[int] = []
    while len(answers) < rounds:
        n = 1
        while True:
            nxt = _sha256(f"{n}, {digest}")
            if (1 << 52) / (int(nxt[:13], 16) + 1) >= target:
                answers.append(n)
                digest = nxt
                break
            n += 1
    return answers


@dataclass
class CDParams:
    """What x-kpsdk-cd is made from besides the session. They change only when the site's p.js does."""

    difficulty: int
    subchallenge_count: int
    crypto_customer_key: str = ""
    seed_suffix: str = ""
    label: str = "tp-v2-input"
    ct_prefix: bool = True

    def header(self, ct: str, st: int) -> str:
        """x-kpsdk-cd for one protected request. Computed here: about ten SHA-256 hashes, no API call, no charge.

        ``ct`` is the session's current x-kpsdk-ct (the one sent on this very request; each reply rotates it) and
        ``st`` the x-kpsdk-st header /tl answered with, in milliseconds. Make a new header for every request."""
        if self.difficulty < 1 or self.subchallenge_count < 1:
            raise ValueError("kasada: difficulty and subchallenge_count must be positive")
        if st <= 0:
            raise ValueError("kasada: st must be the x-kpsdk-st value /tl answered with")
        label = self.label or "tp-v2-input"
        if self.ct_prefix:
            if len(ct) < 16:
                raise ValueError("kasada: ct must be the x-kpsdk-ct value (at least 16 characters)")
            label += ct[:16]
        ident = os.urandom(16).hex()
        work_time = int(time.time() * 1000) - 10 - random.randrange(41)
        first = f"{label}, {work_time}, {ident}"
        if self.crypto_customer_key:
            first += f", {self.crypto_customer_key}"
        if self.seed_suffix:
            first += f", {self.seed_suffix}"
        # a real page's clock reads st a little before or after the server's, the same for the whole session
        d = hashlib.sha256(str(st).encode()).digest()[0] * 90 // 255 - 10
        return json.dumps(
            {
                "workTime": work_time, "id": ident, "answers": _cd_answers(first, self.difficulty, self.subchallenge_count),
                "duration": (200 + random.randrange(551)) / 10, "d": d, "st": st, "rst": st + d,
            },
            separators=(",", ":"),
        )


@dataclass
class VersionResult:
    """The x-kpsdk-v value p.js stamps on /fp, and what x-kpsdk-cd is made from (see :meth:`cd_header`)."""

    version: str
    payload: str = ""
    session: str = ""
    user_agent: str = ""
    cd: Optional[CDParams] = None

    def cd_header(self, ct: str, st: int) -> str:
        """x-kpsdk-cd for one protected request, computed locally (no API call, no charge); see :meth:`CDParams.header`."""
        if self.cd is None:
            raise ValueError("kasada: this version has no cd parameters (p.js's proof of work could not be read)")
        return self.cd.header(ct, st)


@dataclass
class PayloadResult:
    """The binary /tl artifact and the browser it was made as."""

    payload: bytes
    payload_base64: str = ""
    headers: Dict[str, str] = field(default_factory=dict)
    tl_url: str = ""
    session: str = ""
    user_agent: str = ""


def _text(data: Dict[str, Any], key: str) -> str:
    value = data.get(key)
    return value if isinstance(value, str) else ""


def _body(
    *,
    script: str,
    page_url: str,
    user_agent: str,
    ip: str,
    accept_language: str,
    html: str = "",
    ips_link: str = "",
    session: str = "",
) -> Dict[str, Any]:
    if not script or not page_url or not user_agent or not ip or not accept_language or not (html or ips_link):
        raise ValueError("kasada: a payload needs script, html or ips_link, page_url, user_agent, ip and accept_language")
    body: Dict[str, Any] = {
        "script": script,
        "pageUrl": page_url,
        "userAgent": user_agent,
        "ip": ip,
        "acceptLanguage": accept_language,
    }
    if html:
        body["html"] = html
    if ips_link:
        body["ipsLink"] = ips_link
    if session:
        body["session"] = session
    return body


def _version(body: Dict[str, Any]) -> Op[VersionResult]:
    data: Dict[str, Any] = yield Call("POST", "/v1/kasada/version", body)
    version = data.get("version")
    if not isinstance(version, str) or not version:
        raise ValueError("kasada: the API answered without a version")
    payload = data.get("payload")
    session = data.get("session")
    user_agent = data.get("userAgent")
    raw_cd = data.get("cd")
    cd: Optional[CDParams] = None
    if isinstance(raw_cd, dict) and isinstance(raw_cd.get("difficulty"), int) and isinstance(raw_cd.get("subchallengeCount"), int):
        cd = CDParams(
            difficulty=raw_cd["difficulty"],
            subchallenge_count=raw_cd["subchallengeCount"],
            crypto_customer_key=_text(raw_cd, "cryptoCustomerKey"),
            seed_suffix=_text(raw_cd, "seedSuffix"),
            label=_text(raw_cd, "label") or "tp-v2-input",
            ct_prefix=raw_cd.get("ctPrefix") is not False,
        )
    return VersionResult(
        version=version,
        payload=payload if isinstance(payload, str) else version,
        session=session if isinstance(session, str) else "",
        user_agent=user_agent if isinstance(user_agent, str) else "",
        cd=cd,
    )


def _generate(body: Dict[str, Any]) -> Op[PayloadResult]:
    data: Dict[str, Any] = yield Call("POST", PATH, body)
    encoded = _text(data, "payloadBase64")
    try:
        payload = base64.b64decode(encoded, validate=True)
    except ValueError as err:
        raise ValueError("kasada: payloadBase64 is not base64") from err
    raw_headers = data.get("headers")
    headers = {str(k): str(v) for k, v in raw_headers.items()} if isinstance(raw_headers, dict) else {}
    return PayloadResult(
        payload=payload,
        payload_base64=encoded,
        headers=headers,
        tl_url=_text(data, "tlUrl"),
        session=_text(data, "session"),
        user_agent=_text(data, "userAgent"),
    )


class Kasada(_Sync):
    """Kasada client: fetch and post with your client; this computes /tl."""

    _service = SERVICE
    _default_timeout = 60.0

    def version(
        self,
        *,
        script: str,
        page_url: str,
        user_agent: str = "",
        accept_language: str = "",
        session: str = "",
    ) -> VersionResult:
        if not script or not page_url:
            raise ValueError("kasada: a version needs script and page_url")
        body: Dict[str, Any] = {"script": script, "pageUrl": page_url}
        if user_agent:
            body["userAgent"] = user_agent
        if accept_language:
            body["acceptLanguage"] = accept_language
        if session:
            body["session"] = session
        return self._run(_version(body))

    def generate_payload(
        self,
        *,
        script: str,
        page_url: str,
        user_agent: str,
        ip: str,
        accept_language: str,
        html: str = "",
        ips_link: str = "",
        session: str = "",
    ) -> PayloadResult:
        body = _body(
            script=script, page_url=page_url, user_agent=user_agent, ip=ip,
            accept_language=accept_language, html=html, ips_link=ips_link, session=session,
        )
        return self._run(_generate(body))


class AsyncKasada(_Async):
    """Kasada client for asyncio."""

    _service = SERVICE
    _default_timeout = 60.0

    async def version(
        self,
        *,
        script: str,
        page_url: str,
        user_agent: str = "",
        accept_language: str = "",
        session: str = "",
    ) -> VersionResult:
        if not script or not page_url:
            raise ValueError("kasada: a version needs script and page_url")
        body: Dict[str, Any] = {"script": script, "pageUrl": page_url}
        if user_agent:
            body["userAgent"] = user_agent
        if accept_language:
            body["acceptLanguage"] = accept_language
        if session:
            body["session"] = session
        return await self._run(_version(body))

    async def generate_payload(
        self,
        *,
        script: str,
        page_url: str,
        user_agent: str,
        ip: str,
        accept_language: str,
        html: str = "",
        ips_link: str = "",
        session: str = "",
    ) -> PayloadResult:
        body = _body(
            script=script, page_url=page_url, user_agent=user_agent, ip=ip,
            accept_language=accept_language, html=html, ips_link=ips_link, session=session,
        )
        return await self._run(_generate(body))
