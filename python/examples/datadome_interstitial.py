"""Solve a DataDome interstitial, then load the page again.

    PROXY=http://user:pass@host:port python examples/datadome_interstitial.py https://www.example.com/
"""

import os
import sys
from datetime import datetime, timezone
from urllib.parse import urlsplit

import httpx

from john_antibot import DataDome
from john_antibot.datadome import GEO_ORIGIN, challenge_link, challenge_script_url, sec_ch_ua, sec_ch_ua_platform, validate_geo

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
    if captcha:
        sys.exit("the page serves the slider: run datadome_slider.py")

    fetched_at = datetime.now(timezone.utc)
    html = web.get(link, headers={"referer": page_url}).text
    script_url = challenge_script_url(html)  # load it right after the page
    script = web.get(script_url, headers={"referer": link}).text if script_url else ""

    inter = session.generate_interstitial(html=html, device_link=link, script=script, referer=page_url, fetched_at=fetched_at)
    form = {"content-type": "application/x-www-form-urlencoded", "origin": GEO_ORIGIN, "referer": link}
    reply = validate_geo(web.post(GEO_ORIGIN + "/interstitial/", content=inter.form_body, headers=form).content)
    if not reply.cleared:
        sys.exit(f"not cleared: view {reply.view!r}")
    web.cookies.set("datadome", reply.cookie, domain=domain)
    print("page again", web.get(page_url).status_code)
