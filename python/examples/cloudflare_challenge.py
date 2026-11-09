"""Clear a Cloudflare challenge page through your proxy, then load it with the clearance.

    PROXY=http://user:pass@host:port python examples/cloudflare_challenge.py https://www.example.com/
"""

import os
import sys

import httpx

from john_antibot import Cloudflare, JobError, is_declined

url, proxy = sys.argv[1], os.environ["PROXY"]  # a sticky session or a static proxy

with Cloudflare(api_key=os.environ["JOHNANTIBOT_API_KEY"]) as cf:
    for attempt in range(3):
        try:
            res = cf.solve_challenge(url=url, proxy=proxy, timezone="America/New_York", max_wait=120)
            break
        except JobError as err:
            if not is_declined(err) or attempt == 2:  # declined: send another
                raise
print(f"solved in {res.ms} ms from {res.exit_ip}")

# Spend the clearance from the same proxy, with its cookies and its user agent.
cookies = {c.name: c.value for c in res.cookies}
with httpx.Client(proxy=proxy, headers={"user-agent": res.user_agent}, cookies=cookies) as web:
    print("page with clearance", web.get(url).status_code)
