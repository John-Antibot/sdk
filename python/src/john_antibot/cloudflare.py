"""Cloudflare: clear a challenge page, earn a JSD clearance or solve a Turnstile widget, as a job through your proxy."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional, Tuple

from . import _jobs
from ._http import _Async, _Sync
from ._jobs import Job

__all__ = ["AsyncCloudflare", "Cloudflare", "Cookie", "Job", "Result"]

SERVICE = "cloudflare"


@dataclass
class Cookie:
    name: str
    value: str
    domain: str = ""
    path: str = ""


@dataclass
class Result:
    """What a finished job holds."""

    payload: str  # "cf_clearance=<value>" for a challenge or jsd, the token for Turnstile
    user_agent: str = ""  # send it in each request to the site, from the same proxy
    session: str = ""  # the device the solve ran as: send it as session in the next solve for the same site session
    cookies: List[Cookie] = field(default_factory=list)  # the cookies a browser on the page would send now
    exit_ip: str = ""  # the address the site saw
    ms: int = 0  # how long the solve took

    @property
    def clearance(self) -> str:
        """The cf_clearance value ("" when there is none)."""
        if self.payload.startswith("cf_clearance="):
            return self.payload[len("cf_clearance=") :]
        return next((c.value for c in self.cookies if c.name == "cf_clearance"), "")

    @property
    def token(self) -> str:
        """The Turnstile token, for the form field cf-turnstile-response."""
        return self.payload


def _result(data: Dict[str, Any]) -> Result:
    t = _jobs.text
    cookies = data.get("cookies")
    return Result(
        payload=t(data, "payload"),
        user_agent=t(data, "userAgent"),
        session=t(data, "session"),
        cookies=[
            Cookie(name=t(c, "name"), value=t(c, "value"), domain=t(c, "domain"), path=t(c, "path"))
            for c in (cookies if isinstance(cookies, list) else [])
            if isinstance(c, dict)
        ],
        exit_ip=t(data, "exitIp"),
        ms=_jobs.number(data, "ms"),
    )


def _body(proxy: str, user_agent: str, timezone: str, session: str, timeout: Optional[float], **page: str) -> Dict[str, Any]:
    body: Dict[str, Any] = {**page, "proxy": proxy, "userAgent": user_agent, "timezone": timezone, "session": session}
    body["timeoutMs"] = _jobs.seconds_to_ms(timeout)
    return {k: v for k, v in body.items() if v not in ("", 0)}  # an empty optional is left out


def _page(task: str, url: str, proxy: str, user_agent: str, timezone: str, session: str, timeout: Optional[float]) -> Tuple[str, Dict[str, Any]]:
    if not url or not proxy:
        raise ValueError(f"cloudflare: a {task} needs a URL and a proxy")
    return f"/v1/cloudflare/{task}", _body(proxy, user_agent, timezone, session, timeout, url=url)


def _turnstile(
    page_url: str, site_key: str, proxy: str, action: str, c_data: str, chl_page_data: str,
    user_agent: str, timezone: str, session: str, timeout: Optional[float],
) -> Tuple[str, Dict[str, Any]]:  # fmt: skip
    if not page_url or not site_key or not proxy:
        raise ValueError("cloudflare: a turnstile solve needs a page URL, a site key and a proxy")
    body = _body(proxy, user_agent, timezone, session, timeout, pageUrl=page_url, sitekey=site_key, action=action, cData=c_data, chlPageData=chl_page_data)
    return "/v1/cloudflare/turnstile", body


class Cloudflare(_Sync):
    """Cloudflare client. A solve is a job: ``solve_*`` submits it and waits; ``submit_*``, ``job`` and ``wait`` do it in steps."""

    _service = SERVICE
    _default_timeout = 40.0

    def solve_challenge(
        self, *, url: str, proxy: str, user_agent: str = "", timezone: str = "", session: str = "",
        timeout: Optional[float] = None, max_wait: Optional[float] = None,
    ) -> Result:  # fmt: skip
        """Clear the challenge page at url and wait for the clearance."""
        return self._run(_jobs.solve(SERVICE, *_page("challenge", url, proxy, user_agent, timezone, session, timeout), max_wait, _result))

    def solve_jsd(
        self, *, url: str, proxy: str, user_agent: str = "", timezone: str = "", session: str = "",
        timeout: Optional[float] = None, max_wait: Optional[float] = None,
    ) -> Result:  # fmt: skip
        """Earn the clearance of the page's JavaScript detections at url and wait for it."""
        return self._run(_jobs.solve(SERVICE, *_page("jsd", url, proxy, user_agent, timezone, session, timeout), max_wait, _result))

    def solve_turnstile(
        self, *, page_url: str, site_key: str, proxy: str, action: str = "", c_data: str = "", chl_page_data: str = "",
        user_agent: str = "", timezone: str = "", session: str = "", timeout: Optional[float] = None, max_wait: Optional[float] = None,
    ) -> Result:  # fmt: skip
        """Solve the Turnstile widget on page_url and wait for the token."""
        call = _turnstile(page_url, site_key, proxy, action, c_data, chl_page_data, user_agent, timezone, session, timeout)
        return self._run(_jobs.solve(SERVICE, *call, max_wait, _result))

    def submit_challenge(
        self, *, url: str, proxy: str, user_agent: str = "", timezone: str = "", session: str = "", timeout: Optional[float] = None,
    ) -> Job[Result]:  # fmt: skip
        """Send a challenge job; it comes back queued."""
        return self._run(_jobs.submit(SERVICE, *_page("challenge", url, proxy, user_agent, timezone, session, timeout), _result))

    def submit_jsd(
        self, *, url: str, proxy: str, user_agent: str = "", timezone: str = "", session: str = "", timeout: Optional[float] = None,
    ) -> Job[Result]:  # fmt: skip
        """Send a jsd job; it comes back queued."""
        return self._run(_jobs.submit(SERVICE, *_page("jsd", url, proxy, user_agent, timezone, session, timeout), _result))

    def submit_turnstile(
        self, *, page_url: str, site_key: str, proxy: str, action: str = "", c_data: str = "", chl_page_data: str = "",
        user_agent: str = "", timezone: str = "", session: str = "", timeout: Optional[float] = None,
    ) -> Job[Result]:  # fmt: skip
        """Send a Turnstile job; it comes back queued."""
        call = _turnstile(page_url, site_key, proxy, action, c_data, chl_page_data, user_agent, timezone, session, timeout)
        return self._run(_jobs.submit(SERVICE, *call, _result))

    def job(self, id: str, wait: float = 0) -> Job[Result]:
        """Read a job. With wait (seconds, up to 25), the API answers as soon as the job ends, or after wait with the state it has."""
        return self._run(_jobs.get(SERVICE, id, wait, _result))

    def wait(self, job: Job[Result], *, max_wait: Optional[float] = None) -> Result:
        """Follow a job until it ends. failed or declined is a JobError; past max_wait seconds, TimeoutError (the job runs on)."""
        return self._run(_jobs.wait(SERVICE, job, max_wait, _result))


class AsyncCloudflare(_Async):
    """Cloudflare client for asyncio: the methods of Cloudflare, awaited."""

    _service = SERVICE
    _default_timeout = 40.0

    async def solve_challenge(
        self, *, url: str, proxy: str, user_agent: str = "", timezone: str = "", session: str = "",
        timeout: Optional[float] = None, max_wait: Optional[float] = None,
    ) -> Result:  # fmt: skip
        return await self._run(_jobs.solve(SERVICE, *_page("challenge", url, proxy, user_agent, timezone, session, timeout), max_wait, _result))

    async def solve_jsd(
        self, *, url: str, proxy: str, user_agent: str = "", timezone: str = "", session: str = "",
        timeout: Optional[float] = None, max_wait: Optional[float] = None,
    ) -> Result:  # fmt: skip
        return await self._run(_jobs.solve(SERVICE, *_page("jsd", url, proxy, user_agent, timezone, session, timeout), max_wait, _result))

    async def solve_turnstile(
        self, *, page_url: str, site_key: str, proxy: str, action: str = "", c_data: str = "", chl_page_data: str = "",
        user_agent: str = "", timezone: str = "", session: str = "", timeout: Optional[float] = None, max_wait: Optional[float] = None,
    ) -> Result:  # fmt: skip
        call = _turnstile(page_url, site_key, proxy, action, c_data, chl_page_data, user_agent, timezone, session, timeout)
        return await self._run(_jobs.solve(SERVICE, *call, max_wait, _result))

    async def submit_challenge(
        self, *, url: str, proxy: str, user_agent: str = "", timezone: str = "", session: str = "", timeout: Optional[float] = None,
    ) -> Job[Result]:  # fmt: skip
        return await self._run(_jobs.submit(SERVICE, *_page("challenge", url, proxy, user_agent, timezone, session, timeout), _result))

    async def submit_jsd(
        self, *, url: str, proxy: str, user_agent: str = "", timezone: str = "", session: str = "", timeout: Optional[float] = None,
    ) -> Job[Result]:  # fmt: skip
        return await self._run(_jobs.submit(SERVICE, *_page("jsd", url, proxy, user_agent, timezone, session, timeout), _result))

    async def submit_turnstile(
        self, *, page_url: str, site_key: str, proxy: str, action: str = "", c_data: str = "", chl_page_data: str = "",
        user_agent: str = "", timezone: str = "", session: str = "", timeout: Optional[float] = None,
    ) -> Job[Result]:  # fmt: skip
        call = _turnstile(page_url, site_key, proxy, action, c_data, chl_page_data, user_agent, timezone, session, timeout)
        return await self._run(_jobs.submit(SERVICE, *call, _result))

    async def job(self, id: str, wait: float = 0) -> Job[Result]:
        return await self._run(_jobs.get(SERVICE, id, wait, _result))

    async def wait(self, job: Job[Result], *, max_wait: Optional[float] = None) -> Result:
        return await self._run(_jobs.wait(SERVICE, job, max_wait, _result))
