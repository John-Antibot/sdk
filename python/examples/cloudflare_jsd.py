"""Earn the JSD cf_clearance of a page through your proxy, then load it with the clearance.

    PROXY=http://user:pass@host:port python examples/cloudflare_jsd.py https://www.example.com/
"""

import os
import sys

import httpx

from john_antibot import Cloudflare

url, proxy = sys.argv[1], os.environ["PROXY"]

with Cloudflare(api_key=os.environ["JOHNANTIBOT_API_KEY"]) as cf:
    res = cf.solve_jsd(url=url, proxy=proxy, max_wait=120)
print("cf_clearance", res.clearance)

with httpx.Client(proxy=proxy, headers={"user-agent": res.user_agent}, cookies={c.name: c.value for c in res.cookies}) as web:
    print("page with clearance", web.get(url).status_code)
