"""Solve a Turnstile widget through your proxy and print the token.

    PROXY=http://user:pass@host:port python examples/cloudflare_turnstile.py https://www.example.com/login 0x4AAAAAAA...
"""

import os
import sys

from john_antibot import Cloudflare

page_url, site_key = sys.argv[1], sys.argv[2]

with Cloudflare(api_key=os.environ["JOHNANTIBOT_API_KEY"]) as cf:
    res = cf.solve_turnstile(page_url=page_url, site_key=site_key, proxy=os.environ["PROXY"], timezone="America/New_York", max_wait=120)

# Send res.token as cf-turnstile-response with the site's form, from the same proxy, with res.user_agent.
print(f"token {len(res.token)} bytes in {res.ms} ms from {res.exit_ip}")
print("user agent:", res.user_agent)
