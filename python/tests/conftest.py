"""A stand-in for the API on httpx.MockTransport, and a fixture that runs each test against the sync and the async client."""

from __future__ import annotations

import asyncio
import gzip
import inspect
import json
from dataclasses import dataclass
from typing import Any, Callable, Dict, Iterator, List, Optional, Union

import httpx
import pytest

import john_antibot as ja
from john_antibot import _jobs
from john_antibot.datadome import AsyncSession

ENDPOINT = "http://api.test"

Reply = Union[Dict[str, Any], str, tuple, Exception, Callable[[httpx.Request], Any]]


@dataclass
class Seen:
    method: str
    path: str
    raw_path: bytes
    query: Dict[str, str]
    headers: httpx.Headers
    body: Any  # the decoded JSON body, None without one
    size: int  # bytes on the wire
    timeout: Dict[str, Any]


class API:
    """Routes map "METHOD /path" to a reply, or a list of replies served in turn (the last one repeats).

    A reply is a JSON object (200), a (status, body) tuple, an exception to raise, or a callable of the request returning one of those.
    """

    def __init__(self, routes: Optional[Dict[str, Union[Reply, List[Reply]]]] = None) -> None:
        self.routes = routes or {}
        self.seen: List[Seen] = []

    def __call__(self, request: httpx.Request) -> httpx.Response:
        raw = request.read()
        size = len(raw)
        if request.headers.get("content-encoding") == "gzip":
            raw = gzip.decompress(raw)
        self.seen.append(
            Seen(
                method=request.method,
                path=request.url.path,
                raw_path=request.url.raw_path,
                query=dict(request.url.params),
                headers=request.headers,
                body=json.loads(raw) if raw else None,
                size=size,
                timeout=request.extensions.get("timeout", {}),
            )
        )
        key = f"{request.method} {request.url.path}"
        assert key in self.routes, f"unexpected call {key}"
        reply = self.routes[key]
        if isinstance(reply, list):
            reply = reply.pop(0) if len(reply) > 1 else reply[0]
        if callable(reply):
            reply = reply(request)
        if isinstance(reply, Exception):
            raise reply
        status, payload = reply if isinstance(reply, tuple) else (200, reply)
        content = payload if isinstance(payload, (str, bytes)) else json.dumps(payload)
        return httpx.Response(status, content=content, headers={"content-type": "application/json"})

    @property
    def last(self) -> Seen:
        return self.seen[-1]


class Blocking:
    """An async client whose calls run to completion on one loop, so a test drives it like the sync one."""

    def __init__(self, target: Any, loop: asyncio.AbstractEventLoop) -> None:
        object.__setattr__(self, "_target", target)
        object.__setattr__(self, "_loop", loop)

    def __getattr__(self, name: str) -> Any:
        value = getattr(self._target, name)
        if not callable(value):
            return value

        def call(*args: Any, **kwargs: Any) -> Any:
            out = value(*args, **kwargs)
            if inspect.isawaitable(out):
                out = self._loop.run_until_complete(out)
            return Blocking(out, self._loop) if isinstance(out, AsyncSession) else out

        return call

    def __setattr__(self, name: str, value: Any) -> None:
        setattr(self._target, name, value)


ASYNC = {ja.DataDome: ja.AsyncDataDome, ja.Cloudflare: ja.AsyncCloudflare, ja.Recaptcha: ja.AsyncRecaptcha, ja.Account: ja.AsyncAccount}


@pytest.fixture(params=["sync", "async"])
def make(request: pytest.FixtureRequest) -> Iterator[Callable[..., Any]]:
    """make(Cloudflare, api, **kwargs): the client (sync, or async behind Blocking) on the stand-in, with key "key" at ENDPOINT."""
    loop = asyncio.new_event_loop() if request.param == "async" else None

    def build(cls: Any, api: API, **kwargs: Any) -> Any:
        kwargs.setdefault("api_key", "key")
        kwargs.setdefault("endpoint", ENDPOINT)
        if loop is None:
            return cls(http_client=httpx.Client(transport=httpx.MockTransport(api)), **kwargs)
        return Blocking(ASYNC[cls](http_client=httpx.AsyncClient(transport=httpx.MockTransport(api)), **kwargs), loop)

    yield build
    if loop is not None:
        loop.close()


@pytest.fixture(autouse=True)
def no_retry_pause(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(_jobs, "RETRY_DELAY", 0.0)
