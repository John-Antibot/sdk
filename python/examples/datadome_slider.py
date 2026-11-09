"""Solve a DataDome slider, then load the page again.

    PROXY=http://user:pass@host:port python examples/datadome_slider.py https://www.example.com/
"""

import os
import sys
from datetime import datetime, timezone
from urllib.parse import urlsplit

import httpx

from john_antibot import ApiError, DataDome, is_hardblock, is_puzzle_unsupported
from john_antibot.datadome import challenge_link, challenge_script_url, is_hardblock_page, sec_ch_ua, sec_ch_ua_platform, validate_geo

page_url = sys.argv[1]
domain = "." + (urlsplit(page_url).hostname or "").removeprefix("www.")

datadome = DataDome(api_key=os.environ["JOHNANTIBOT_API_KEY"])
session = datadome.session(referer=page_url)
ua = session.user_agent
browser = {"user-agent": ua, "sec-ch-ua": sec_ch_ua(ua), "sec-ch-ua-mobile": "?0", "sec-ch-ua-platform": sec_ch_ua_platform(ua)}

with datadome, httpx.Client(proxy=os.environ.get("PROXY"), headers=browser) as web:
    page = web.get(page_url)
    found = challenge_link(page.text, page_url, web.cookies.get("datadome") or "")
    if page.status_code != 403 or found is None:
        sys.exit(f"no DataDome challenge (status {page.status_code})")
    link, captcha = found
    if not captcha:
        sys.exit("the page serves the interstitial: run datadome_interstitial.py")

    fetched_at = datetime.now(timezone.utc)
    html = web.get(link, headers={"referer": page_url}).text
    if is_hardblock_page(html):
        sys.exit("hard block: change the proxy and the session")
    script_url = challenge_script_url(html)
    script = web.get(script_url, headers={"referer": link}).text if script_url else ""

    try:
        slider = session.generate_slider(html=html, captcha_url=link, script=script, parent_url=page_url, fetched_at=fetched_at)
    except ApiError as err:
        if is_hardblock(err) or is_puzzle_unsupported(err):
            sys.exit(f"not a solvable slider (free): {err.message}")
        raise
    reply = validate_geo(web.get(slider.check_url, headers={"referer": link}).content)
    if not reply.cookie:
        sys.exit(f"the check set no cookie: view {reply.view!r}")
    web.cookies.set("datadome", reply.cookie, domain=domain)
    print("page again", web.get(page_url).status_code)
