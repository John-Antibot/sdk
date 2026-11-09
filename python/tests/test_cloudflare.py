from __future__ import annotations

import asyncio
import time
from datetime import datetime, timezone

import httpx
import pytest

import john_antibot as ja
from john_antibot import _jobs
from john_antibot.cloudflare import Cookie, Job, Result

from .conftest import ENDPOINT, API

RESULT = {
    "payload": "cf_clearance=abc123",
    "userAgent": "UA/1",
    "session": "sess_x",
    "exitIp": "9.9.9.9",
    "ms": 7000,
    "cookies": [
        {"name": "cf_clearance", "value": "abc123", "domain": "example.com", "path": "/"},
        {"name": "__cf_bm", "value": "bm", "domain": "example.com", "path": "/"},
    ],
}
DONE = {"id": "job_1", "state": "done", "task": "challenge", "createdAt": "2026-10-23T10:00:00Z", "proxy": "8.8.8.8:80", "result": RESULT}
QUEUED = (202, {"id": "job_1", "state": "queued"})
RUNNING = {"id": "job_1", "state": "running"}


def stand_in(terminal, task="challenge"):
    """Submit answers 202 with job_1; the first poll says running, the next one terminal."""
    return API({f"POST /v1/cloudflare/{task}": QUEUED, "GET /v1/cloudflare/jobs/job_1": [RUNNING, terminal]})


def test_solve_challenge_submits_and_waits(make):
    api = stand_in(DONE)
    cf = make(ja.Cloudflare, api, org_key="org", endpoint=ENDPOINT + "/")
    res = cf.solve_challenge(url="https://example.com/", proxy="u:p@8.8.8.8:80", timezone="America/New_York", timeout=90)
    assert res == Result(
        payload="cf_clearance=abc123",
        user_agent="UA/1",
        session="sess_x",
        exit_ip="9.9.9.9",
        ms=7000,
        cookies=[Cookie("cf_clearance", "abc123", "example.com", "/"), Cookie("__cf_bm", "bm", "example.com", "/")],
    )
    assert res.clearance == "abc123"
    submit, *polls = api.seen
    assert (submit.method, submit.path, submit.query) == ("POST", "/v1/cloudflare/challenge", {})
    # the fields given, none of the ones left empty
    assert submit.body == {"url": "https://example.com/", "proxy": "u:p@8.8.8.8:80", "timezone": "America/New_York", "timeoutMs": 90000}
    for seen in api.seen:
        assert seen.headers["x-api-key"] == "key" and seen.headers["x-org-key"] == "org" and "content-encoding" not in seen.headers
    # it polled with the API's wait, twice (running, then done)
    assert [(p.method, p.path, p.query) for p in polls] == [("GET", "/v1/cloudflare/jobs/job_1", {"wait": "25"})] * 2
    assert polls[0].body is None and "content-type" not in polls[0].headers


def test_every_challenge_field(make):
    api = stand_in(DONE)
    make(ja.Cloudflare, api).submit_challenge(
        url="https://example.com/", proxy="8.8.8.8:80", user_agent=ja.datadome.LATEST_USER_AGENT, timezone="Europe/Paris", session="s-1", timeout=12.5
    )
    assert api.last.body == {
        "url": "https://example.com/",
        "proxy": "8.8.8.8:80",
        "userAgent": ja.datadome.LATEST_USER_AGENT,
        "timezone": "Europe/Paris",
        "session": "s-1",
        "timeoutMs": 12500,
    }


def test_solve_turnstile(make):
    api = stand_in({"id": "job_1", "state": "done", "task": "turnstile", "result": {"payload": "0.token", "userAgent": "UA/1"}}, "turnstile")
    cf = make(ja.Cloudflare, api)
    res = cf.solve_turnstile(page_url="https://example.com/login", site_key="0x4AAA", proxy="8.8.8.8:80", action="login")
    assert res.token == "0.token" and res.clearance == "" and res.cookies == []
    assert api.seen[0].path == "/v1/cloudflare/turnstile"
    assert api.seen[0].body == {"pageUrl": "https://example.com/login", "sitekey": "0x4AAA", "proxy": "8.8.8.8:80", "action": "login"}
    cf.submit_turnstile(
        page_url="https://example.com/login", site_key="0x4AAA", proxy="8.8.8.8:80", c_data="cd", chl_page_data="pd", user_agent="UA", session="s", timezone="UTC", timeout=30
    )
    assert api.last.body == {
        "pageUrl": "https://example.com/login",
        "sitekey": "0x4AAA",
        "proxy": "8.8.8.8:80",
        "cData": "cd",
        "chlPageData": "pd",
        "userAgent": "UA",
        "session": "s",
        "timezone": "UTC",
        "timeoutMs": 30000,
    }


def test_solve_jsd(make):
    api = stand_in({"id": "job_1", "state": "done", "task": "jsd", "result": {"payload": "cf_clearance=jsd1", "userAgent": "UA/1"}}, "jsd")
    cf = make(ja.Cloudflare, api)
    res = cf.solve_jsd(url="https://example.com/event", proxy="user:pass@8.8.8.8:1080")
    assert res.clearance == "jsd1"
    assert (api.seen[0].path, api.seen[0].body) == ("/v1/cloudflare/jsd", {"url": "https://example.com/event", "proxy": "user:pass@8.8.8.8:1080"})
    job = cf.submit_jsd(url="https://example.com/event", proxy="8.8.8.8:80", session="s", timeout=20)
    assert api.last.body == {"url": "https://example.com/event", "proxy": "8.8.8.8:80", "session": "s", "timeoutMs": 20000}
    assert job == Job(id="job_1", state="queued") and not job.terminal


@pytest.mark.parametrize(
    "terminal,check,retry,message",
    [
        ({"id": "job_1", "state": "declined", "error": "the challenge was not passed", "retry": True}, ja.is_declined, True, "the challenge was not passed"),
        ({"id": "job_1", "state": "failed", "error": "the proxy is refused: x is not a public address"}, lambda e: not ja.is_declined(e), False, "not a public address"),
        ({"id": "job_1", "state": "failed", "error": "the page serves no challenge (status 200)"}, ja.is_no_challenge, False, "serves no challenge"),
        (
            {"id": "job_1", "state": "failed", "error": "the proxy exits from a different address on each connection (1.1.1.1, then 2.2.2.2): a clearance is for one address"},
            ja.is_rotating_proxy,
            False,
            "different address",
        ),
        ({"id": "job_1", "state": "done", "resultExpired": True}, lambda e: not ja.is_declined(e), False, "no longer holds its result"),
        ({"id": "job_1", "state": "done"}, lambda e: not ja.is_declined(e), False, "without a result"),
    ],
)
def test_ended_jobs_are_errors(make, terminal, check, retry, message):
    with pytest.raises(ja.JobError) as err:
        make(ja.Cloudflare, stand_in(terminal)).solve_challenge(url="https://example.com/", proxy="8.8.8.8:80")
    e = err.value
    assert check(e) and e.retry is retry and e.id == "job_1" and e.state == terminal["state"] and message in e.message
    assert str(e).startswith(f"cloudflare: job job_1 {terminal['state']}: ")
    assert not ja.is_rate_limited(e) and not ja.is_bad_request(e)


@pytest.mark.parametrize(
    "status,check",
    [(402, ja.is_insufficient_balance), (403, ja.is_site_not_approved), (429, ja.is_rate_limited), (400, ja.is_bad_request), (401, ja.is_bad_key)],
)
def test_refusals(make, status, check):
    api = API({"POST /v1/cloudflare/challenge": (status, {"error": "gateway: nope"})})
    with pytest.raises(ja.ApiError) as err:
        make(ja.Cloudflare, api).solve_challenge(url="https://example.com/", proxy="8.8.8.8:80")
    assert err.value.status == status and err.value.message == "gateway: nope" and check(err.value)
    assert str(err.value) == f"cloudflare: HTTP {status}: gateway: nope"
    assert len(api.seen) == 1  # a refusal is not a job: nothing was waited for
    others = [p for p in (ja.is_insufficient_balance, ja.is_site_not_approved, ja.is_rate_limited, ja.is_bad_request, ja.is_bad_key, ja.is_declined) if p is not check]
    assert not any(p(err.value) for p in others)


def test_inputs_are_refused_before_any_call(make):
    api = API()
    cf = make(ja.Cloudflare, api)
    with pytest.raises(ValueError, match="cloudflare: a challenge needs a URL and a proxy"):
        cf.submit_challenge(url="https://example.com/", proxy="")
    with pytest.raises(ValueError, match="a jsd needs a URL and a proxy"):
        cf.solve_jsd(url="", proxy="8.8.8.8:80")
    with pytest.raises(ValueError, match="a turnstile solve needs a page URL, a site key and a proxy"):
        cf.submit_turnstile(page_url="https://example.com/", site_key="", proxy="8.8.8.8:80")
    with pytest.raises(ValueError, match="a job id is required"):
        cf.job("")
    assert api.seen == []


def test_a_submit_without_a_job_id(make):
    api = API({"POST /v1/cloudflare/challenge": (202, {"state": "queued"})})
    with pytest.raises(ja.JohnAntibotError, match="the API answered without a job id"):
        make(ja.Cloudflare, api).submit_challenge(url="https://example.com/", proxy="8.8.8.8:80")


def test_job_reads_with_wait(make):
    api = API({"GET /v1/cloudflare/jobs/job 1/x": DONE})
    cf = make(ja.Cloudflare, api)
    job = cf.job("job 1/x", wait=25)
    assert api.last.raw_path == b"/v1/cloudflare/jobs/job%201%2Fx?wait=25"
    cf.job("job 1/x", wait=0.2)
    assert api.last.query == {"wait": "1"}  # whole seconds, rounded up
    cf.job("job 1/x")
    assert api.last.query == {}
    assert job.terminal and job.task == "challenge" and job.proxy == "8.8.8.8:80" and job.result is not None and job.result.clearance == "abc123"
    assert job.created_at == datetime(2026, 10, 23, 10, tzinfo=timezone.utc) and job.started_at is None
    # a terminal job is waited for without a call
    n = len(api.seen)
    assert cf.wait(job).clearance == "abc123" and len(api.seen) == n


def test_wait_survives_bad_polls(make):
    api = API({"GET /v1/cloudflare/jobs/job_1": [(502, ""), httpx.ConnectError("reset"), "not json", DONE]})
    res = make(ja.Cloudflare, api).wait(Job(id="job_1", state="queued"))
    assert res.clearance == "abc123" and len(api.seen) == 4


def test_wait_gives_up_after_three_bad_polls_in_a_row(make):
    api = API({"GET /v1/cloudflare/jobs/job_1": [RUNNING, (502, ""), (502, ""), (502, ""), RUNNING, (503, ""), (503, ""), (503, ""), (504, "gone")]})
    with pytest.raises(ja.ApiError) as err:
        make(ja.Cloudflare, api).wait(Job(id="job_1", state="running"))
    assert err.value.status == 504 and len(api.seen) == 9  # a good poll in between resets the count


def test_not_found_is_not_retried(make):
    api = API({"GET /v1/cloudflare/jobs/job_x": (404, {"error": "not found"})})
    with pytest.raises(ja.ApiError) as err:
        make(ja.Cloudflare, api).wait(Job(id="job_x", state="running"))
    assert err.value.status == 404 and len(api.seen) == 1


def test_max_wait_ends_the_wait_not_the_job(make):
    api = API({"GET /v1/cloudflare/jobs/job_1": RUNNING})
    start = time.monotonic()
    with pytest.raises(TimeoutError, match="job job_1 is still running"):
        make(ja.Cloudflare, api).wait(Job(id="job_1", state="running"), max_wait=0.15)
    assert time.monotonic() - start < 3 and api.seen[0].query == {"wait": "1"}


def test_the_wait_of_a_solve_is_bounded(make):
    api = API({"POST /v1/cloudflare/challenge": QUEUED, "GET /v1/cloudflare/jobs/job_1": RUNNING})
    with pytest.raises(TimeoutError):
        make(ja.Cloudflare, api).solve_challenge(url="https://example.com/", proxy="8.8.8.8:80", max_wait=0.1)


def test_a_dropped_async_wait_stops_polling():
    polls = 0

    async def handler(request: httpx.Request) -> httpx.Response:
        nonlocal polls
        polls += 1
        await asyncio.sleep(0.02)
        return httpx.Response(200, json=RUNNING)

    async def main() -> None:
        async with ja.AsyncCloudflare("key", endpoint=ENDPOINT, http_client=httpx.AsyncClient(transport=httpx.MockTransport(handler))) as cf:
            with pytest.raises(asyncio.TimeoutError):
                await asyncio.wait_for(cf.wait(Job(id="job_1", state="queued")), 0.1)

    asyncio.run(main())
    seen = polls
    time.sleep(0.05)
    assert 1 <= seen == polls


def test_per_call_timeout(make):
    api = API({"POST /v1/cloudflare/challenge": QUEUED, "GET /v1/cloudflare/jobs/job_1": httpx.ReadTimeout("slow")})
    cf = make(ja.Cloudflare, api, timeout=3)
    job = cf.submit_challenge(url="https://example.com/", proxy="8.8.8.8:80")
    assert api.last.timeout == {"connect": 3, "read": 3, "write": 3, "pool": 3}
    with pytest.raises(httpx.ReadTimeout):
        cf.wait(job)  # a timed out poll is retried, then given up
    assert len(api.seen) == 1 + 1 + _jobs.POLL_RETRIES
    assert ja.Cloudflare("k").timeout == 40


def test_clearance_from_the_cookies_when_the_payload_is_other():
    assert Result(payload="x", cookies=[Cookie(name="cf_clearance", value="from-cookie")]).clearance == "from-cookie"
    assert Result(payload="").clearance == "" and Result(payload="t").token == "t"


def test_context_managers_close_their_own_client():
    with ja.Cloudflare("k") as cf:
        pass
    assert cf._http.is_closed
    mine = httpx.Client()
    with ja.Cloudflare("k", http_client=mine):
        pass
    assert not mine.is_closed

    async def main() -> None:
        async with ja.AsyncCloudflare("k") as acf:
            pass
        assert acf._http.is_closed
        theirs = httpx.AsyncClient()
        async with ja.AsyncCloudflare("k", http_client=theirs):
            pass
        assert not theirs.is_closed
        await theirs.aclose()

    asyncio.run(main())
