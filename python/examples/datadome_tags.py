"""Load a page, then post the ch and le tags to the site's DataDome /js/ endpoint.

    PROXY=http://user:pass@host:port FONTS="Inter,Inter Fallback" python examples/datadome_tags.py https://www.example.com/ <site key> [js url]

FONTS: the @font-face families the page loads.
"""

import os
import sys
from urllib.parse import urlsplit

import httpx

from john_antibot import DataDome
from john_antibot.datadome import sec_ch_ua, sec_ch_ua_platform, validate_geo

page_url, site_key = sys.argv[1], sys.argv[2]
js_url = sys.argv[3] if len(sys.argv) > 3 else "https://api-js.datadome.co/js/"
domain = "." + (urlsplit(page_url).hostname or "").removeprefix("www.")
fonts = [f for f in os.environ.get("FONTS", "").split(",") if f]

datadome = DataDome(api_key=os.environ["JOHNANTIBOT_API_KEY"])
session = datadome.session(referer=page_url)
ua = session.user_agent  # send it on every request of the session
browser = {"user-agent": ua, "sec-ch-ua": sec_ch_ua(ua), "sec-ch-ua-mobile": "?0", "sec-ch-ua-platform": sec_ch_ua_platform(ua)}

with datadome, httpx.Client(proxy=os.environ.get("PROXY"), headers=browser, follow_redirects=True) as web:
    print("page", web.get(page_url).status_code)
    for event in ("ch", "le"):
        tags = session.generate_tags(event=event, cookie=web.cookies.get("datadome") or "", site_key=site_key, loaded_font_families=fonts)
        reply = web.post(js_url, content=tags.payload, headers={"content-type": "application/x-www-form-urlencoded", "referer": page_url})
        cookie = validate_geo(reply.content).cookie
        if cookie:
            web.cookies.set("datadome", cookie, domain=domain)
        print("tags", event, "->", reply.status_code)
