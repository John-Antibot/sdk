from __future__ import annotations

import httpx
import pytest

import john_antibot as ja
from john_antibot.account import Balance

from .conftest import ENDPOINT, API


def test_balance_reads_the_account_of_the_key(make):
    api = API({"GET /v1/balance": {"metered": True, "balanceMicros": 4821000}})
    b = make(ja.Account, api, endpoint=ENDPOINT + "/").balance()
    assert b == Balance(metered=True, balance_micros=4821000) and b.balance_usd == 4.821
    seen = api.last
    assert (seen.method, seen.path, seen.query, seen.body) == ("GET", "/v1/balance", {}, None)
    assert seen.headers["x-api-key"] == "key" and "x-org-key" not in seen.headers and "content-type" not in seen.headers


def test_a_key_that_is_not_metered_has_no_balance(make):
    b = make(ja.Account, API({"GET /v1/balance": {"metered": False}})).balance()
    assert b == Balance(metered=False, balance_micros=0) and b.balance_usd == 0


def test_refusals_are_typed_and_keep_the_apis_text(make):
    with pytest.raises(ja.ApiError) as err:
        make(ja.Account, API({"GET /v1/balance": (401, {"error": "gateway: missing or bad x-api-key"})})).balance()
    assert err.value.status == 401 and err.value.message == "gateway: missing or bad x-api-key" and ja.is_bad_key(err.value)
    assert str(err.value) == "account: HTTP 401: gateway: missing or bad x-api-key"
    # other failures are not a bad key
    with pytest.raises(ja.ApiError) as err:
        make(ja.Account, API({"GET /v1/balance": (503, "We cannot load the billing data.")})).balance()
    assert not ja.is_bad_key(err.value) and err.value.message == "We cannot load the billing data."


def test_a_default_endpoint_and_a_timeout(make):
    assert ja.Account("k").endpoint == ja.DEFAULT_ENDPOINT == "https://antibot.johnavenue.com" and ja.Account("k").timeout == 20
    api = API({"GET /v1/balance": httpx.ReadTimeout("slow")})
    with pytest.raises(httpx.TimeoutException):
        make(ja.Account, api, timeout=0.1).balance()
    assert api.last.timeout["read"] == 0.1
