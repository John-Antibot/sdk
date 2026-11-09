# John Antibot Python SDK

Python client for the [John Antibot](https://antibot.johnavenue.com) DataDome, Cloudflare and reCAPTCHA APIs and your account's balance.

```bash
pip install "git+https://github.com/John-Antibot/sdk.git@v0.8.0#subdirectory=python"
```

Python 3.9+. One dependency, `httpx`. Typed.

| Client | |
|---|---|
| `DataDome` | Payloads for tags, the interstitial and the slider. |
| `Cloudflare` | Challenge, JSD and Turnstile solves, as jobs on our side through your proxy. |
| `Recaptcha` | reCAPTCHA v3 tokens, classic and Enterprise, as jobs on our side through your proxy. |
| `Account` | The balance of your key's account. |

Each client takes `api_key`, and optionally `endpoint` (default `https://antibot.johnavenue.com`), `timeout` (seconds per HTTP call) and `http_client` (your own `httpx.Client`). Each is a context manager. Software you distribute under an organization also passes `org_key` (sent as `x-org-key`) next to each user's own `api_key`.

## DataDome

```python
import os
from john_antibot import DataDome

datadome = DataDome(api_key=os.environ["JOHNANTIBOT_API_KEY"])
session = datadome.session(referer="https://www.example.com/")

# Tags: ch, then le. loaded_font_families: the @font-face families the page loaded (document.fonts).
tags = session.generate_tags(event="ch", cookie=cid, site_key=ddk, loaded_font_families=["Inter", "Inter Fallback"])

# Interstitial: the page, the URL you loaded, the script it loads (only its SHA-256 is sent).
inter = session.generate_interstitial(html=html, device_link=link, script=script)

# Slider: same inputs.
slider = session.generate_slider(html=html, captcha_url=link, script=script)
```

| Call | Takes |
|---|---|
| `datadome.session` | `referer`, `session` (to resume one), `device`, `user_agent` (default `LATEST_USER_AGENT`), `timezone`, `timezone_offset`. All optional. |
| `generate_tags` | `event` (`"ch"` or `"le"`), `cookie` (the `datadome` cookie), `site_key` (the page's DataDome key); optional `loaded_font_families` (required by the API for most sites), `tags_script_url` (the site's tags.js when it is not `https://datadome.<site>/tags.js`), `version`, `script` (the tags.js source), `referer` (default: the session's). |
| `generate_interstitial` | `html`, `device_link`; `script` when the page loads one; optional `referer`, `fetched_at` (a `datetime`). |
| `generate_slider` | `html`, `captcha_url`; `script` when the page loads one; optional `parent_url`, `fetched_at`. |

| Result | Use |
|---|---|
| `tags.payload` | Form body for the site's `/js/`. |
| `inter.form_body` | Form body for `https://geo.captcha-delivery.com/interstitial/`. `inter.form_body_for(page_url)` sets its page. |
| `slider.check_url` | GET it. |

Each result also has `session` and `user_agent`; the session keeps both. Use one session per proxy and cookie jar, and send `session.user_agent` on every request.

| Helper (`john_antibot.datadome`) | |
|---|---|
| `challenge_link(body, page_url, cookie)` | The challenge URL in a 403 body and whether it is a slider: `(link, captcha)`, or `None`. |
| `challenge_script_url(html)` | The script a challenge page loads, `""` when inline. Load it right after the page. |
| `parse_dd(html)` | The `var dd` block of a blocked page; `.hard_blocked` is `t=bv`. |
| `build_interstitial_url(dd, referer)`, `build_captcha_url(dd, referer)` | The challenge URL of a `dd` block. |
| `is_hardblock_page(html)` | A captcha page with nothing to solve. |
| `validate_geo(body)` | The reply to the interstitial POST or the slider check: `.cleared`, `.cookie`, `.cookie_max_age`, `.view`, `.url`, `.hard_block`. |
| `LATEST_USER_AGENT`, `chrome_user_agent(major)`, `chrome_major(ua)`, `sec_ch_ua(ua)`, `sec_ch_ua_platform(ua)` | The session's browser headers. |
| `js_offset_minutes(zone)` | `timezone_offset` for an IANA zone. |
| `GEO_ORIGIN` | `https://geo.captcha-delivery.com` |

### Billing and errors

The price is charged when the request is sent and returned if it fails. A non-2xx reply is an `ApiError` with `status` and `message`.

| Check | Meaning |
|---|---|
| `is_hardblock(err)` | The site blocked the session outright: DataDome served its hardblock page, with no slider to solve (`422`, `err.hardblock`). Change the proxy and the session. Free. |
| `is_puzzle_unsupported(err)` | A puzzle captcha, not a slider. Free. |
| `is_rate_limited(err)` | `429`: wait, then try again. |

## Cloudflare

A Cloudflare solve is a job that runs on our side, through your proxy. You send the page and the proxy; the clearance is for the address that proxy exits from.

```python
from john_antibot import Cloudflare

cf = Cloudflare(api_key=os.environ["JOHNANTIBOT_API_KEY"])

# A challenge page: the cf_clearance cookie.
res = cf.solve_challenge(
    url="https://www.example.com/",
    proxy="http://user:pass@host:port",  # any form a provider prints; a public address
    timezone="America/New_York",         # the zone of the proxy's location
)

# JSD: the cf_clearance the page's JavaScript detections grant. The page must load.
res = cf.solve_jsd(url="https://www.example.com/", proxy=proxy)

# A Turnstile widget: the token.
res = cf.solve_turnstile(page_url="https://www.example.com/login", site_key="0x4AAAAAAA…", proxy=proxy)
```

| Call | Takes |
|---|---|
| `solve_challenge`, `solve_jsd` | `url`, `proxy`; optional `user_agent`, `timezone`, `session`, `timeout`, `max_wait`. |
| `solve_turnstile` | `page_url`, `site_key`, `proxy`; optional `action`, `c_data`, `chl_page_data` (the widget's `data-action`, `data-cdata`, `chlPageData`), `user_agent`, `timezone`, `session`, `timeout`, `max_wait`. |

`timeout` is the job's time in seconds (10 to 90; default 60). `max_wait` bounds the wait in seconds (default: until the job ends).

| Result | Use |
|---|---|
| `res.clearance` | The `cf_clearance` value. |
| `res.cookies` | The cookies (`name`, `value`, `domain`, `path`) a browser on the page would send now. |
| `res.token` | The Turnstile token, for the form field `cf-turnstile-response`. |
| `res.user_agent` | Send it in each request to the site, from the same proxy. |
| `res.exit_ip` | The address the site saw. |
| `res.session` | Send it as `session` in the next solve for the same site session. |
| `res.ms` | How long the solve took. |

A solve takes about 7 seconds. `submit_challenge`, `submit_jsd`, `submit_turnstile` (the same inputs, without `max_wait`), `job(id, wait=25)` and `wait(job, max_wait=...)` do the same in steps. A `Job` has `id`, `state` (`queued`, `running`, `done`, `failed`, `declined`), `terminal`, `result`, `error`, `retry`.

### Proxy

- The proxy must keep its address from one connection to the next, because you spend the clearance from a connection of your own. Use a sticky session (the provider's session id in the username) or a static proxy. A proxy that changes its address on each connection is refused and the price is returned.
- Forms: `http://user:pass@host:port`, `https://…`, `socks5://…`, `socks5h://…`, `host:port`, `host:port:user:pass`, `user:pass@host:port`.
- If your proxy admits only listed source addresses, admit ours: `GET /api/catalog`, `egressIps` of `cloudflare`.
- `user_agent` is optional. Set it to your client's user agent to get a clearance for it (the exact user agent of a Chrome build on macOS that the API runs; anything else is refused), or leave it empty and use `res.user_agent`.

### Billing and errors

The price is charged when the job is sent and returned if it fails or is declined.

| Check | Meaning |
|---|---|
| `is_declined(err)` | The site turned this attempt down (a few in a hundred). Send another. |
| `is_rotating_proxy(err)` | The proxy changes its address on each connection. |
| `is_no_challenge(err)` | The page serves no challenge. |
| `is_insufficient_balance(err)` | `402`: the balance is below the price. |
| `is_site_not_approved(err)` | `403`: the site is not approved for your key. |
| `is_rate_limited(err)` | `429`: wait, then try again. |
| `is_bad_request(err)` | `400`: the input was refused. |

## reCAPTCHA

A reCAPTCHA v3 token is a job that runs on our side, through your proxy. The token is for the address that proxy exits from.

```python
from john_antibot import Recaptcha

rc = Recaptcha(api_key=os.environ["JOHNANTIBOT_API_KEY"])

# A page that loads the classic script (api.js).
res = rc.solve_v3(
    url="https://www.example.com/login",  # the page that asks for the token (approved for your key)
    site_key="6Lc…",                      # the site's reCAPTCHA key
    action="login",                       # what the page passes to execute; empty is "submit"
    proxy="http://user:pass@host:port",   # any form a provider prints; a public address
)

# A page that loads reCAPTCHA Enterprise (enterprise.js): the same inputs.
res = rc.solve_v3_enterprise(url="https://www.example.com/login", site_key="6Lc…", proxy=proxy)
```

Optional: `action`, `timeout` (the job's time in seconds, 10 to 60; default 30), `max_wait`.

| Result | Use |
|---|---|
| `res.token` | The `g-recaptcha-response` value, or what the page's `execute` call would have returned. |
| `res.expires_at` | When Google stops accepting it (a `datetime`, or `None`). Send the token before. |
| `res.headers` | `user-agent`, `accept-language` and the client hints of the browser the token was made as. Send them in the request that carries the token. |

Also `res.user_agent`, `res.accept_language`, `res.sec_ua`, `res.sec_ua_mobile`, `res.sec_ua_platform`. `submit_v3`, `submit_v3_enterprise`, `job(id, wait=25)` and `wait(job)` do the same in steps.

- The proxy must have a public address. Send the request that carries the token from the same address.
- If your proxy admits only listed source addresses, admit ours: `GET /api/catalog`, `egressIps` of `recaptcha`.

The price is charged when the job is sent and returned if it fails. Checks: `is_insufficient_balance`, `is_site_not_approved`, `is_rate_limited`, `is_bad_request`, `is_declined`, as for Cloudflare.

## Account

Each paid call takes its price from your balance when it starts, and gets a `402` when the balance is less. `balance()` reads it first. It is free.

```python
from john_antibot import Account

balance = Account(api_key=os.environ["JOHNANTIBOT_API_KEY"]).balance()
# balance.balance_micros is in millionths of a dollar, the unit of a price in the catalog: 2000 is $0.002. balance.balance_usd is for display.
```

A key that is not metered has no account: `balance.metered` is `False` and `balance_micros` is `0`. A missing, unknown or revoked key, or the key of a disabled account, is `is_bad_key(err)`. `Account` takes no `org_key`: software under an organization reads the balance of each user's own key.

## Errors

| Error | |
|---|---|
| `ApiError` | A non-2xx reply: `status`, `message`, `hardblock`. |
| `JobError` | A job that ended `failed` or `declined`: `id`, `state`, `message`, `retry`. |
| `JohnAntibotError` | Base of both; raised as is for a reply the client cannot read. |
| `ValueError` | A required input is missing. Raised before any call. |
| `TimeoutError` | `max_wait` passed. The job runs on: `job(id)` reads it. |
| `httpx.HTTPError` | The network, or `timeout` of one HTTP call. |

Every check (`is_hardblock`, `is_puzzle_unsupported`, `is_rate_limited`, `is_declined`, `is_rotating_proxy`, `is_no_challenge`, `is_insufficient_balance`, `is_site_not_approved`, `is_bad_request`, `is_bad_key`) is imported from `john_antibot` and takes the exception.

## Async

```python
import asyncio
from john_antibot import AsyncCloudflare

async def main():
    async with AsyncCloudflare(api_key=os.environ["JOHNANTIBOT_API_KEY"]) as cf:
        res = await cf.solve_challenge(url="https://www.example.com/", proxy=proxy)

asyncio.run(main())
```

`AsyncDataDome`, `AsyncCloudflare`, `AsyncRecaptcha` and `AsyncAccount` have the same methods, awaited; `http_client` is an `httpx.AsyncClient`. `AsyncDataDome.session()` is not awaited. `asyncio.wait_for` or cancelling the task ends a wait.

## Examples

Set `JOHNANTIBOT_API_KEY` and `PROXY=http://user:pass@host:port`, then:

| Program | Does |
|---|---|
| `FONTS="Inter,Inter Fallback" python examples/datadome_tags.py <page> <site key> [js url]` | Loads a page and posts the `ch` and `le` tags. |
| `python examples/datadome_interstitial.py <page>` | Solves the interstitial and reloads the page. |
| `python examples/datadome_slider.py <page>` | Solves the slider and reloads the page. |
| `python examples/cloudflare_challenge.py <page>` | Clears the challenge, then loads the page with the clearance and the user agent of the result. |
| `python examples/cloudflare_jsd.py <page>` | Earns the JSD clearance, then loads the page with it. |
| `python examples/cloudflare_turnstile.py <page> <site key>` | Solves a widget and prints the token. |
| `python examples/recaptcha_v3.py <page> <site key> [action]` | Makes a token (`ENTERPRISE=1` for Enterprise pages) and prints it with its headers. |
| `python examples/account_balance.py` | Prints the balance in dollars. |
| `python examples/async_cloudflare.py <page>…` | Reads the balance, then clears several pages at once. |

## Tests

```bash
uv run pytest
JOHNANTIBOT_LIVE=1 JOHNANTIBOT_API_KEY=… uv run pytest tests/test_live.py   # against the API
```
