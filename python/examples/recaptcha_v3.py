"""Make a reCAPTCHA v3 token through your proxy and print it with the headers of the browser it was made as.

    PROXY=http://user:pass@host:port python examples/recaptcha_v3.py https://www.example.com/login 6Lc... [action]
    ENTERPRISE=1 ...   for a page that loads enterprise.js
"""

import os
import sys

from john_antibot import Recaptcha

page_url, site_key = sys.argv[1], sys.argv[2]
action = sys.argv[3] if len(sys.argv) > 3 else ""  # empty is "submit"

with Recaptcha(api_key=os.environ["JOHNANTIBOT_API_KEY"]) as rc:
    solve = rc.solve_v3_enterprise if os.environ.get("ENTERPRISE") == "1" else rc.solve_v3
    res = solve(url=page_url, site_key=site_key, action=action, proxy=os.environ["PROXY"], max_wait=90)

# Send the token from the same proxy, with these headers, before it expires.
print(f"token {len(res.token)} bytes, valid until {res.expires_at}")
for name, value in res.headers.items():
    print(f"{name}: {value}")
