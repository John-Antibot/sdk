"""Against the real API. Opt in with JOHNANTIBOT_LIVE=1 and JOHNANTIBOT_API_KEY (JOHNANTIBOT_ENDPOINT overrides the URL).

The balance is free. A Cloudflare challenge runs, and is charged, only with JOHNANTIBOT_LIVE_CF_URL (an approved page) and
JOHNANTIBOT_LIVE_PROXY.
"""

from __future__ import annotations

import os

import pytest

import john_antibot as ja

pytestmark = pytest.mark.skipif(
    os.environ.get("JOHNANTIBOT_LIVE") != "1" or not os.environ.get("JOHNANTIBOT_API_KEY"),
    reason="set JOHNANTIBOT_LIVE=1 and JOHNANTIBOT_API_KEY to run against the API",
)


def client(cls):
    return cls(os.environ["JOHNANTIBOT_API_KEY"], endpoint=os.environ.get("JOHNANTIBOT_ENDPOINT", ja.DEFAULT_ENDPOINT))


def test_live_balance():
    with client(ja.Account) as account:
        b = account.balance()
    assert b.balance_micros >= 0 and (b.metered or b.balance_micros == 0)


def test_live_cloudflare_challenge():
    url, proxy = os.environ.get("JOHNANTIBOT_LIVE_CF_URL"), os.environ.get("JOHNANTIBOT_LIVE_PROXY")
    if not url or not proxy:
        pytest.skip("set JOHNANTIBOT_LIVE_CF_URL and JOHNANTIBOT_LIVE_PROXY")
    with client(ja.Cloudflare) as cf:
        res = cf.solve_challenge(url=url, proxy=proxy, max_wait=120)
    assert res.clearance and res.user_agent
