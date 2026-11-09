"""What every client shares: the request it builds, the reply it reads, and a sync and an async driver.

A call is an operation: a generator that yields a Call (or a sleep, in seconds), is sent the Call's JSON object, and has a failed Call
(ApiError, JohnAntibotError, httpx.HTTPError) thrown in where it yielded. The two drivers differ only in how they send and sleep.
"""

from __future__ import annotations

import asyncio
import gzip
import json
import time
from dataclasses import dataclass
from types import TracebackType
from typing import Any, Dict, Generator, Optional, Type, TypeVar, Union, cast

import httpx

from .errors import ApiError, JohnAntibotError

DEFAULT_ENDPOINT = "https://antibot.johnavenue.com"

# From this many bytes a request body is gzipped.
COMPRESS_ABOVE = 32 << 10

T = TypeVar("T")
S = TypeVar("S", bound="_Base")


@dataclass(frozen=True)
class Call:
    method: str
    path: str
    body: Optional[Dict[str, Any]] = None


Op = Generator[Union[Call, float], Any, T]


class _Base:
    _service = ""
    _default_timeout = 60.0

    def __init__(self, api_key: str, *, org_key: str = "", endpoint: str = DEFAULT_ENDPOINT, timeout: Optional[float] = None) -> None:
        self.api_key = api_key
        self.org_key = org_key
        self.endpoint = endpoint.rstrip("/") or DEFAULT_ENDPOINT
        # The bound of one HTTP call, in seconds.
        self.timeout = timeout if timeout is not None and timeout > 0 else self._default_timeout

    def _request(self, http: Union[httpx.Client, httpx.AsyncClient], call: Call) -> httpx.Request:
        headers: Dict[str, str] = {}
        content: Optional[bytes] = None
        if call.body is not None:
            content = json.dumps(call.body, separators=(",", ":"), ensure_ascii=False).encode()
            headers["content-type"] = "application/json"
            if len(content) >= COMPRESS_ABOVE:
                content = gzip.compress(content, compresslevel=1, mtime=0)
                headers["content-encoding"] = "gzip"
        if self.api_key:
            headers["x-api-key"] = self.api_key
        if self.org_key:
            headers["x-org-key"] = self.org_key
        return http.build_request(call.method, self.endpoint + call.path, content=content, headers=headers, timeout=self.timeout)

    def _reply(self, res: httpx.Response) -> Dict[str, Any]:
        try:
            data = res.json()
        except ValueError:
            data = None
        if not 200 <= res.status_code <= 299:
            failure: Dict[str, Any] = data if isinstance(data, dict) else {}
            error = failure.get("error")
            message = error if isinstance(error, str) else ""
            message = message or res.text.strip() or f"{res.status_code} {res.reason_phrase}".strip()
            raise ApiError(res.status_code, message, hardblock=failure.get("hardblock") is True, service=self._service)
        if not isinstance(data, dict):
            raise JohnAntibotError(f"{self._service}: decode response: the reply is not a JSON object")
        return data


class _Sync(_Base):
    def __init__(
        self,
        api_key: str,
        *,
        org_key: str = "",
        endpoint: str = DEFAULT_ENDPOINT,
        timeout: Optional[float] = None,
        http_client: Optional[httpx.Client] = None,
    ) -> None:
        super().__init__(api_key, org_key=org_key, endpoint=endpoint, timeout=timeout)
        self._owns_http = http_client is None
        self._http = http_client if http_client is not None else httpx.Client()

    def _run(self, op: Op[T]) -> T:
        value: Any = None
        error: Optional[BaseException] = None
        while True:
            try:
                step = op.send(value) if error is None else op.throw(error)
            except StopIteration as stop:
                return cast(T, stop.value)
            value, error = None, None
            if isinstance(step, Call):
                try:
                    value = self._reply(self._http.send(self._request(self._http, step)))
                except (JohnAntibotError, httpx.HTTPError) as err:
                    error = err
            else:
                time.sleep(step)

    def close(self) -> None:
        """Close the HTTP client, unless it was passed in."""
        if self._owns_http:
            self._http.close()

    def __enter__(self: S) -> S:
        return self

    def __exit__(self, t: Optional[Type[BaseException]], e: Optional[BaseException], tb: Optional[TracebackType]) -> None:
        self.close()


class _Async(_Base):
    def __init__(
        self,
        api_key: str,
        *,
        org_key: str = "",
        endpoint: str = DEFAULT_ENDPOINT,
        timeout: Optional[float] = None,
        http_client: Optional[httpx.AsyncClient] = None,
    ) -> None:
        super().__init__(api_key, org_key=org_key, endpoint=endpoint, timeout=timeout)
        self._owns_http = http_client is None
        self._http = http_client if http_client is not None else httpx.AsyncClient()

    async def _run(self, op: Op[T]) -> T:
        value: Any = None
        error: Optional[BaseException] = None
        while True:
            try:
                step = op.send(value) if error is None else op.throw(error)
            except StopIteration as stop:
                return cast(T, stop.value)
            value, error = None, None
            if isinstance(step, Call):
                try:
                    value = self._reply(await self._http.send(self._request(self._http, step)))
                except (JohnAntibotError, httpx.HTTPError) as err:
                    error = err
            else:
                await asyncio.sleep(step)

    async def aclose(self) -> None:
        """Close the HTTP client, unless it was passed in."""
        if self._owns_http:
            await self._http.aclose()

    async def __aenter__(self: S) -> S:
        return self

    async def __aexit__(self, t: Optional[Type[BaseException]], e: Optional[BaseException], tb: Optional[TracebackType]) -> None:
        await self.aclose()
