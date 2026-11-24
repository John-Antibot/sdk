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
