from __future__ import annotations

import pytest

import john_antibot as ja

from .conftest import API, ENDPOINT


def test_generate_payload(make):
    api = API({
        "POST /v1/kasada/payload": {
            "payloadBase64": "AAEC", "headers": {"x-kpsdk-ct": "ct"},
            "tlUrl": "https://example.com/tl", "session": "s1", "userAgent": "UA/1",
        }
    })
    client = make(ja.Kasada, api, org_key="org", endpoint=ENDPOINT + "/")
    out = client.generate_payload(
        script="ips", ips_link="/a/b/ips.js", page_url="https://example.com/",
        user_agent="UA/1", ip="203.0.113.1", accept_language="en-US",
    )
    assert out.payload == b"\x00\x01\x02"
    assert out.headers == {"x-kpsdk-ct": "ct"}
    assert api.seen[0].path == "/v1/kasada/payload"
    assert api.seen[0].body["ip"] == "203.0.113.1"
    assert api.seen[0].headers["x-org-key"] == "org"


def test_version(make):
    api = API({"POST /v1/kasada/version": {"version": "j-1.2.1020", "payload": "j-1.2.1020", "session": "s1", "userAgent": "UA/1"}})
    out = make(ja.Kasada, api).version(script="pjs", page_url="https://example.com/")
    assert out.version == "j-1.2.1020"
    assert api.seen[0].path == "/v1/kasada/version"


@pytest.mark.parametrize("status,check", [(402, ja.is_insufficient_balance), (400, ja.is_bad_request)])
def test_refusals(make, status, check):
    api = API({"POST /v1/kasada/payload": (status, {"error": "gateway: nope"})})
    with pytest.raises(ja.ApiError) as err:
        make(ja.Kasada, api).generate_payload(
            script="ips", ips_link="/x", page_url="https://e/", user_agent="UA", ip="1.2.3.4", accept_language="en",
        )
    assert err.value.status == status and check(err.value)


NIKE_KEY = "1760fa26d0fa116e2f26e90dc8b1379c3d9e946f650852b7f9925457ab77a14a"


def test_cd_answers_match_real_chrome():
    # Two cds real Chrome stamped on nike's cart writes (2026-11-24), with the x-kpsdk-ct each request carried.
    from john_antibot.kasada import _cd_answers

    assert _cd_answers(f"tp-v2-input045g90GkupSRHwee, 1795536575353, ce1a0639146179bcf6ab456ac1925359, {NIKE_KEY}", 10, 2) == [13, 5]
    assert _cd_answers(f"tp-v2-input01pDYjEopgLXL7X4, 1795536579918, 2d455b029dc31363685cb443dce15ea4, {NIKE_KEY}", 10, 2) == [5, 1]


def test_cd_header_and_version_parse(make):
    import json
    import time

    from john_antibot.kasada import VersionResult, _cd_answers

    api = API({"POST /v1/kasada/version": {
        "version": "j-1.2.1020", "payload": "j-1.2.1020", "session": "s1", "userAgent": "UA/1",
        "cd": {"difficulty": 10, "subchallengeCount": 2, "cryptoCustomerKey": NIKE_KEY, "seedSuffix": "", "label": "tp-v2-input", "ctPrefix": True},
    }})
    version = make(ja.Kasada, api).version(script="pjs", page_url="https://example.com/")
    ct, st = "045g90GkupSRHweeAkVbaHmX2luTJgqAGpoWX3q5D4o", int(time.time() * 1000) - 300
    ds = set()
    for _ in range(10):
        raw = version.cd_header(ct, st)
        cd = json.loads(raw)
        assert list(cd) == ["workTime", "id", "answers", "duration", "d", "st", "rst"]
        assert cd["answers"] == _cd_answers(f"tp-v2-input{ct[:16]}, {cd['workTime']}, {cd['id']}, {NIKE_KEY}", 10, 2)
        assert cd["st"] == st and cd["rst"] - cd["st"] == cd["d"] and -10 <= cd["d"] <= 80 and 20 <= cd["duration"] <= 75
        ds.add(cd["d"])
    assert len(ds) == 1  # one session, one offset
    try:
        VersionResult(version="j").cd_header(ct, st)
    except ValueError:
        pass
    else:
        raise AssertionError("a version without cd parameters made a header")


def test_generate_payload_without_user_agent(make):
    api = API({
        "POST /v1/kasada/payload": {
            "payloadBase64": "AAEC", "headers": {"x-kpsdk-ct": "ct"},
            "tlUrl": "https://example.com/tl", "session": "s1", "userAgent": "Chrome/158 picked",
        }
    })
    out = make(ja.Kasada, api).generate_payload(
        script="ips", ips_link="/a/b/ips.js", page_url="https://example.com/", ip="203.0.113.1", accept_language="en-US",
    )
    assert out.user_agent == "Chrome/158 picked"
    assert "userAgent" not in api.seen[0].body
