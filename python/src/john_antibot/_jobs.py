"""Jobs, as Cloudflare and reCAPTCHA share them: submit, read, wait."""

from __future__ import annotations

import math
import re
import time
from dataclasses import dataclass
from datetime import datetime
from typing import Any, Callable, Dict, Generic, Optional, TypeVar
from urllib.parse import quote

import httpx

from ._http import Call, Op
from .errors import ApiError, JobError, JohnAntibotError

# How long a poll asks the API to hold it for a job that has not ended (the API's most is 25 seconds).
POLL_WAIT = 25
# How many polls in a row may fail (the network, a 5xx) before a wait gives up, and the pause after each.
POLL_RETRIES = 3
RETRY_DELAY = 1.0

R = TypeVar("R")
Parse = Callable[[Dict[str, Any]], R]


@dataclass
class Job(Generic[R]):
    """A job as the API reports it. state is ``queued``, ``running``, ``done``, ``failed`` or ``declined``."""

    id: str
    state: str
    task: str = ""
    error: str = ""  # when failed or declined
    retry: bool = False  # when declined: send a new job
    result: Optional[R] = None  # when done
    result_expired: bool = False  # when done: the API no longer holds the result
    proxy: str = ""  # the proxy's label, without its credentials
    created_at: Optional[datetime] = None
    started_at: Optional[datetime] = None
    finished_at: Optional[datetime] = None

    @property
    def terminal(self) -> bool:
        """The job has ended."""
        return self.state not in ("queued", "running")


def text(data: Dict[str, Any], key: str) -> str:
    value = data.get(key)
    return value if isinstance(value, str) else ""


def number(data: Dict[str, Any], key: str) -> int:
    value = data.get(key)
    return int(value) if isinstance(value, (int, float)) and not isinstance(value, bool) else 0


_FRACTION = re.compile(r"\.(\d+)")


def parse_time(value: Any) -> Optional[datetime]:
    """An RFC 3339 time; None when absent or unreadable."""
    if not isinstance(value, str) or not value:
        return None
    s = value[:-1] + "+00:00" if value[-1] in "Zz" else value
    s = _FRACTION.sub(lambda m: "." + (m.group(1) + "000000")[:6], s, count=1)  # fromisoformat before 3.11 takes 3 or 6 digits
    try:
        return datetime.fromisoformat(s)
    except ValueError:
        return None


def _job(data: Dict[str, Any], parse: Parse[R]) -> Job[R]:
    result = data.get("result")
    return Job(
        id=text(data, "id"),
        state=text(data, "state"),
        task=text(data, "task"),
        error=text(data, "error"),
        retry=data.get("retry") is True,
        result=parse(result) if isinstance(result, dict) else None,
        result_expired=data.get("resultExpired") is True,
        proxy=text(data, "proxy"),
        created_at=parse_time(data.get("createdAt")),
        started_at=parse_time(data.get("startedAt")),
        finished_at=parse_time(data.get("finishedAt")),
    )


def submit(service: str, path: str, body: Dict[str, Any], parse: Parse[R]) -> Op[Job[R]]:
    job = _job((yield Call("POST", path, body)), parse)
    if not job.id:
        raise JohnAntibotError(f"{service}: the API answered without a job id")
    return job


def get(service: str, id: str, wait: float, parse: Parse[R]) -> Op[Job[R]]:
    if not id:
        raise ValueError(f"{service}: a job id is required")
    path = f"/v1/{service}/jobs/{quote(id, safe='')}"
    if wait > 0:
        path += f"?wait={math.ceil(wait)}"
    return _job((yield Call("GET", path)), parse)


def wait(service: str, job: Job[R], max_wait: Optional[float], parse: Parse[R]) -> Op[R]:
    deadline = None if max_wait is None else time.monotonic() + max_wait
    failures = 0
    while not job.terminal:
        left = POLL_WAIT if deadline is None else deadline - time.monotonic()
        if left <= 0:
            raise TimeoutError(f"{service}: job {job.id} is still {job.state} after {max_wait} s")
        try:
            job = yield from get(service, job.id, min(POLL_WAIT, left), parse)
            failures = 0
        except (JohnAntibotError, httpx.HTTPError) as err:
            # (a poll that failed for a reason that may be gone next time: the network, a 5xx, an unreadable reply)
            if isinstance(err, ApiError) and err.status < 500 or failures >= POLL_RETRIES:
                raise
            failures += 1
            yield RETRY_DELAY if deadline is None else min(RETRY_DELAY, max(0.0, deadline - time.monotonic()))
    return outcome(service, job)


def solve(service: str, path: str, body: Dict[str, Any], max_wait: Optional[float], parse: Parse[R]) -> Op[R]:
    job = yield from submit(service, path, body, parse)
    return (yield from wait(service, job, max_wait, parse))


def outcome(service: str, job: Job[R]) -> R:
    if job.state != "done":
        raise JobError(job.id, job.state, job.error, retry=job.retry, service=service)
    if job.result is not None:
        return job.result
    message = "the job is done but the API no longer holds its result" if job.result_expired else "the job is done without a result"
    raise JobError(job.id, job.state, message, service=service)


def seconds_to_ms(timeout: Optional[float]) -> int:
    return int(timeout * 1000) if timeout and timeout > 0 else 0
