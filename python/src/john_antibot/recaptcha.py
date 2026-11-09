"""reCAPTCHA v3: a token for a page, classic or Enterprise, made as a job through your proxy."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Any, Dict, Optional

from . import _jobs
from ._http import _Async, _Sync
from ._jobs import Job

__all__ = ["AsyncRecaptcha", "Job", "Recaptcha", "Result"]

SERVICE = "recaptcha"


@dataclass
class Result:
    """What a finished job holds: the token, and the browser it was made as."""

    token: str  # the g-recaptcha-response value, or what the page's execute call would have returned
    expires_at: Optional[datetime] = None  # when Google stops accepting the token; None when the API gave none
    user_agent: str = ""
    accept_language: str = ""
    sec_ua: str = ""
    sec_ua_mobile: str = ""
    sec_ua_platform: str = ""

    @property
    def headers(self) -> Dict[str, str]:
        """user-agent, accept-language and the client hints, for the request that carries the token; empty ones left out."""
        pairs = {
            "user-agent": self.user_agent,
            "accept-language": self.accept_language,
            "sec-ch-ua": self.sec_ua,
            "sec-ch-ua-mobile": self.sec_ua_mobile,
            "sec-ch-ua-platform": self.sec_ua_platform,
        }
        return {name: value for name, value in pairs.items() if value}


def _result(data: Dict[str, Any]) -> Result:
    t = _jobs.text
    return Result(
        token=t(data, "token"),
        expires_at=_jobs.parse_time(data.get("expiresAt")),
        user_agent=t(data, "userAgent"),
        accept_language=t(data, "acceptLanguage"),
        sec_ua=t(data, "secUa"),
        sec_ua_mobile=t(data, "secUaMobile"),
        sec_ua_platform=t(data, "secUaPlatform"),
    )


def _body(url: str, site_key: str, proxy: str, action: str, timeout: Optional[float]) -> Dict[str, Any]:
    if not url or not site_key or not proxy:
        raise ValueError("recaptcha: a solve needs a URL, a site key and a proxy")
    body: Dict[str, Any] = {"url": url, "siteKey": site_key, "proxy": proxy}
    if action:
        body["action"] = action
    if _jobs.seconds_to_ms(timeout):
        body["timeoutMs"] = _jobs.seconds_to_ms(timeout)
    return body


V3 = "/v1/recaptcha/v3"
ENTERPRISE = "/v1/recaptcha/v3-enterprise"


class Recaptcha(_Sync):
    """reCAPTCHA client. ``solve_*`` submits a job and waits; ``submit_*``, ``job`` and ``wait`` do it in steps."""

    _service = SERVICE
    _default_timeout = 40.0

    def solve_v3(
        self, *, url: str, site_key: str, proxy: str, action: str = "", timeout: Optional[float] = None, max_wait: Optional[float] = None
    ) -> Result:
        """A token for a page that loads the classic script (api.js)."""
        return self._run(_jobs.solve(SERVICE, V3, _body(url, site_key, proxy, action, timeout), max_wait, _result))

    def solve_v3_enterprise(
        self, *, url: str, site_key: str, proxy: str, action: str = "", timeout: Optional[float] = None, max_wait: Optional[float] = None
    ) -> Result:
        """A token for a page that loads reCAPTCHA Enterprise (enterprise.js)."""
        return self._run(_jobs.solve(SERVICE, ENTERPRISE, _body(url, site_key, proxy, action, timeout), max_wait, _result))

    def submit_v3(self, *, url: str, site_key: str, proxy: str, action: str = "", timeout: Optional[float] = None) -> Job[Result]:
        return self._run(_jobs.submit(SERVICE, V3, _body(url, site_key, proxy, action, timeout), _result))

    def submit_v3_enterprise(self, *, url: str, site_key: str, proxy: str, action: str = "", timeout: Optional[float] = None) -> Job[Result]:
        return self._run(_jobs.submit(SERVICE, ENTERPRISE, _body(url, site_key, proxy, action, timeout), _result))

    def job(self, id: str, wait: float = 0) -> Job[Result]:
        """Read a job. With wait (seconds, up to 25), the API answers as soon as the job ends, or after wait with the state it has."""
        return self._run(_jobs.get(SERVICE, id, wait, _result))

    def wait(self, job: Job[Result], *, max_wait: Optional[float] = None) -> Result:
        """Follow a job until it ends. failed or declined is a JobError; past max_wait seconds, TimeoutError (the job runs on)."""
        return self._run(_jobs.wait(SERVICE, job, max_wait, _result))


class AsyncRecaptcha(_Async):
    """reCAPTCHA client for asyncio: the methods of Recaptcha, awaited."""

    _service = SERVICE
    _default_timeout = 40.0

    async def solve_v3(
        self, *, url: str, site_key: str, proxy: str, action: str = "", timeout: Optional[float] = None, max_wait: Optional[float] = None
    ) -> Result:
        return await self._run(_jobs.solve(SERVICE, V3, _body(url, site_key, proxy, action, timeout), max_wait, _result))

    async def solve_v3_enterprise(
        self, *, url: str, site_key: str, proxy: str, action: str = "", timeout: Optional[float] = None, max_wait: Optional[float] = None
    ) -> Result:
        return await self._run(_jobs.solve(SERVICE, ENTERPRISE, _body(url, site_key, proxy, action, timeout), max_wait, _result))

    async def submit_v3(self, *, url: str, site_key: str, proxy: str, action: str = "", timeout: Optional[float] = None) -> Job[Result]:
        return await self._run(_jobs.submit(SERVICE, V3, _body(url, site_key, proxy, action, timeout), _result))

    async def submit_v3_enterprise(
        self, *, url: str, site_key: str, proxy: str, action: str = "", timeout: Optional[float] = None
    ) -> Job[Result]:
        return await self._run(_jobs.submit(SERVICE, ENTERPRISE, _body(url, site_key, proxy, action, timeout), _result))

    async def job(self, id: str, wait: float = 0) -> Job[Result]:
        return await self._run(_jobs.get(SERVICE, id, wait, _result))

    async def wait(self, job: Job[Result], *, max_wait: Optional[float] = None) -> Result:
        return await self._run(_jobs.wait(SERVICE, job, max_wait, _result))
