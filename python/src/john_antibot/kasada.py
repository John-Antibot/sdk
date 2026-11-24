"""Kasada: a binary /tl payload from the ips.js your client fetched."""

from __future__ import annotations

import base64
from dataclasses import dataclass, field
from typing import Any, Dict

from ._http import Call, Op, _Async, _Sync

__all__ = ["AsyncKasada", "Kasada", "PayloadResult", "VersionResult"]

SERVICE = "kasada"
PATH = "/v1/kasada/payload"


@dataclass
class VersionResult:
    """The x-kpsdk-v value p.js stamps on /fp."""

    version: str
    payload: str = ""
    session: str = ""
    user_agent: str = ""


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
    return VersionResult(
        version=version,
        payload=payload if isinstance(payload, str) else version,
        session=session if isinstance(session, str) else "",
        user_agent=user_agent if isinstance(user_agent, str) else "",
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
