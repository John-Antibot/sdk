from __future__ import annotations

from datetime import datetime, timezone

import httpx
import pytest

import john_antibot as ja
from john_antibot.recaptcha import Job, Result

from .conftest import ENDPOINT, API

RESULT = {
    "token": "03AFcWeA",
    "expiresAt": "2026-10-23T10:01:50.000Z",
    "userAgent": "UA/1",
    "acceptLanguage": "en-US,en;q=0.9",
    "secUa": '"Chromium";v="156"',
    "secUaMobile": "?0",
    "secUaPlatform": '"macOS"',
}
DONE = {"id": "job_1", "state": "done", "task": "v3", "createdAt": "2026-10-23T10:00:00Z", "proxy": "8.8.8.8:80", "result": RESULT}
RUNNING = {"id": "job_1", "state": "running"}


def stand_in(terminal):
    queued = (202, {"id": "job_1", "state": "queued"})
    return API({"POST /v1/recaptcha/v3": queued, "POST /v1/recaptcha/v3-enterprise": queued, "GET /v1/recaptcha/jobs/job_1": [RUNNING, terminal]})


def test_solve_v3_submits_and_waits(make):
    api = stand_in(DONE)
    rc = make(ja.Recaptcha, api, org_key="org", endpoint=ENDPOINT + "/")
    res = rc.solve_v3(url="https://example.com/login", site_key="6Lc-key_1", action="login", proxy="u:p@8.8.8.8:80", timeout=45)
    assert res == Result(
        token="03AFcWeA",
        expires_at=datetime(2026, 10, 23, 10, 1, 50, tzinfo=timezone.utc),
        user_agent="UA/1",
        accept_language="en-US,en;q=0.9",
        sec_ua='"Chromium";v="156"',
        sec_ua_mobile="?0",
        sec_ua_platform='"macOS"',
    )
    submit, *polls = api.seen
    assert (submit.method, submit.path) == ("POST", "/v1/recaptcha/v3")
    assert submit.body == {"url": "https://example.com/login", "siteKey": "6Lc-key_1", "action": "login", "proxy": "u:p@8.8.8.8:80", "timeoutMs": 45000}
    for seen in api.seen:
        assert seen.headers["x-api-key"] == "key" and seen.headers["x-org-key"] == "org"
    assert [(p.method, p.path, p.query) for p in polls] == [("GET", "/v1/recaptcha/jobs/job_1", {"wait": "25"})] * 2


def test_enterprise_goes_to_its_route_and_leaves_empty_optionals_out(make):
    api = stand_in(dict(DONE, task="v3-enterprise"))
    rc = make(ja.Recaptcha, api)
    assert rc.solve_v3_enterprise(url="https://example.com/", site_key="6Lc-key_1", proxy="8.8.8.8:80").token == "03AFcWeA"
    assert api.seen[0].path == "/v1/recaptcha/v3-enterprise"
    assert api.seen[0].body == {"url": "https://example.com/", "siteKey": "6Lc-key_1", "proxy": "8.8.8.8:80"}
    job = rc.submit_v3_enterprise(url="https://example.com/", site_key="k", proxy="8.8.8.8:80", action="a")
    assert (api.last.path, api.last.body["action"], job) == ("/v1/recaptcha/v3-enterprise", "a", Job(id="job_1", state="queued"))
    rc.submit_v3(url="https://example.com/", site_key="k", proxy="8.8.8.8:80")
    assert api.last.path == "/v1/recaptcha/v3"
    job = rc.job("job_1", wait=10)
    assert api.last.query == {"wait": "10"} and job.state == "done" and job.task == "v3-enterprise"


@pytest.mark.parametrize(
    "terminal,message,declined,retry",
    [
        ({"id": "job_1", "state": "declined", "error": "the attempt was turned down", "retry": True}, "the attempt was turned down", True, True),
        ({"id": "job_1", "state": "failed", "error": "the proxy is refused: x is not a public address"}, "the proxy is refused: x is not a public address", False, False),
        ({"id": "job_1", "state": "done", "resultExpired": True}, "no longer holds its result", False, False),
        ({"id": "job_1", "state": "done"}, "without a result", False, False),
    ],
)
def test_ended_jobs_are_errors(make, terminal, message, declined, retry):
    with pytest.raises(ja.JobError) as err:
        make(ja.Recaptcha, stand_in(terminal)).solve_v3(url="https://example.com/", site_key="k", proxy="8.8.8.8:80")
    e = err.value
    assert ja.is_declined(e) is declined and e.retry is retry and e.id == "job_1" and message in e.message
    assert str(e).startswith("recaptcha: job job_1 ")


@pytest.mark.parametrize("status,check", [(402, ja.is_insufficient_balance), (403, ja.is_site_not_approved), (429, ja.is_rate_limited), (400, ja.is_bad_request)])
def test_refusals(make, status, check):
    api = API({"POST /v1/recaptcha/v3": (status, {"error": "gateway: nope"})})
    with pytest.raises(ja.ApiError) as err:
        make(ja.Recaptcha, api).solve_v3(url="https://example.com/", site_key="k", proxy="8.8.8.8:80")
    assert err.value.status == status and err.value.message == "gateway: nope" and check(err.value) and len(api.seen) == 1


@pytest.mark.parametrize(
    "kwargs",
    [
        {"url": "https://example.com/", "site_key": "k", "proxy": ""},
        {"url": "https://example.com/", "site_key": "", "proxy": "8.8.8.8:80"},
        {"url": "", "site_key": "k", "proxy": "8.8.8.8:80"},
    ],
)
def test_a_field_missing_is_refused_before_any_call(make, kwargs):
    api = API()
    with pytest.raises(ValueError, match="recaptcha: a solve needs a URL, a site key and a proxy"):
        make(ja.Recaptcha, api).submit_v3(**kwargs)
    assert api.seen == []


def test_wait_survives_a_bad_poll_and_is_bounded(make):
    api = API({"GET /v1/recaptcha/jobs/job_1": [(502, ""), httpx.ReadError("eof"), DONE]})
    rc = make(ja.Recaptcha, api)
    assert rc.wait(Job(id="job_1", state="queued")).token == "03AFcWeA" and len(api.seen) == 3
    hang = API({"GET /v1/recaptcha/jobs/job_1": RUNNING})
    with pytest.raises(TimeoutError):
        make(ja.Recaptcha, hang).wait(Job(id="job_1", state="running"), max_wait=0.1)


def test_not_found_is_not_retried(make):
    api = API({"GET /v1/recaptcha/jobs/job_x": (404, {"error": "not found"})})
    with pytest.raises(ja.ApiError) as err:
        make(ja.Recaptcha, api).wait(Job(id="job_x", state="running"))
    assert err.value.status == 404 and len(api.seen) == 1


def test_headers_are_the_browser_the_token_was_made_as():
    res = Result(**{"token": "t", "user_agent": "UA/1", "accept_language": "en-US,en;q=0.9", "sec_ua": '"Chromium";v="156"', "sec_ua_mobile": "?0", "sec_ua_platform": '"macOS"'})
    assert res.headers == {
        "user-agent": "UA/1",
        "accept-language": "en-US,en;q=0.9",
        "sec-ch-ua": '"Chromium";v="156"',
        "sec-ch-ua-mobile": "?0",
        "sec-ch-ua-platform": '"macOS"',
    }
    # a result without client hints leaves them out instead of sending empty ones
    assert Result(token="t", user_agent="UA/2", accept_language="de").headers == {"user-agent": "UA/2", "accept-language": "de"}


def test_expires_at(make):
    for raw, want in [
        (None, None),
        ("", None),
        ("soon", None),
        ("2026-10-23T10:01:50Z", datetime(2026, 10, 23, 10, 1, 50, tzinfo=timezone.utc)),
        ("2026-10-23T10:01:50.123456789Z", datetime(2026, 10, 23, 10, 1, 50, 123456, tzinfo=timezone.utc)),
        ("2026-10-23T12:01:50.5+02:00", datetime(2026, 10, 23, 10, 1, 50, 500000, tzinfo=timezone.utc)),
    ]:
        api = API({"GET /v1/recaptcha/jobs/j": {"id": "j", "state": "done", "result": {"token": "t", "expiresAt": raw}}})
        job = make(ja.Recaptcha, api).job("j")
        assert job.result is not None and job.result.expires_at == want, raw
    assert ja.Recaptcha("k").timeout == 40
