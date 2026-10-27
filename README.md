# John Antibot Go SDK

Go client for the [John Antibot](https://antibot.johnavenue.com) DataDome, Cloudflare and reCAPTCHA APIs and your account's balance.

```bash
go get github.com/John-Antibot/sdk@latest
```

Moved from `github.com/John-Avenue/john-antibot-sdk` in v0.6.0: change the import path to `github.com/John-Antibot/sdk`; v0.5.x stays at the old path.

| Package | |
|---|---|
| `datadome` | Generators for tags, interstitial and slider payloads. |
| `cloudflare` | Cloudflare challenge and Turnstile solves, as jobs on our side through your proxy. |
| `recaptcha` | reCAPTCHA v3 tokens, classic and Enterprise, as jobs on our side through your proxy. |
| `account` | The balance of your key's account. |
| `flow` | Complete DataDome flows over your own HTTP transport. |
| `examples` | Runnable programs for each of the above. |

Each client takes `APIKey`. Software you distribute under an organization also sets `OrgKey` (sent as `x-org-key`) next to each user's own `APIKey`.

## DataDome

```go
client, _ := datadome.New(datadome.Config{APIKey: os.Getenv("JOHNANTIBOT_API_KEY")})
session := client.Session(datadome.Session{Referer: "https://seatgeek.com/"})

// Tags: ch, then le.
tags, err := session.GenerateTags(ctx, &datadome.TagsInput{Event: datadome.EventCH, Cookie: cid, SiteKey: ddk})

// Interstitial: the page, the URL you loaded, the script it loads (only its SHA-256 is sent).
inter, err := session.GenerateInterstitial(ctx, &datadome.InterstitialInput{HTML: html, DeviceLink: link, Script: script})

// Slider: same inputs.
slider, err := session.GenerateSlider(ctx, &datadome.SliderInput{HTML: html, CaptchaURL: link, Script: script})
```

| Result | Use |
|---|---|
| `tags.Payload` | Form body for the site's `/js/`. |
| `inter.FormBody` | Form body for `https://geo.captcha-delivery.com/interstitial/`. |
| `slider.CheckURL` | GET it. |

Use one `Session` per proxy and cookie jar, and send `session.UserAgent` on every request.

### Billing and errors

The price is charged when the request is sent and returned if it fails. A non-200 reply is a `*datadome.APIError` with `StatusCode` and `Message`.

| Check | Meaning |
|---|---|
| `datadome.IsHardblock(err)` | The site blocked the session outright: DataDome served its hardblock page, with no slider to solve (`422`, `"hardblock": true`). Change the proxy and the session. Free, and not counted in your statistics. |
| `IsPuzzleUnsupported` | A puzzle captcha, not a slider. Free. |
| `IsRateLimited` | `429`: wait, then try again. |

`flow.Solve` refuses a hard block in the 403 itself (`t=bv`) before it sends anything.

### Flows

`flow.Solve` takes a 403 body and runs the whole challenge on your transport. `flow.SendTags` posts tags. The examples have httpcloak and tls-client transports.

### HTTP/1.1

`geo.captcha-delivery.com` uses HTTP/1.1 only. Header sets are in HTTP/2 form. When your transport sends a request over HTTP/1.1, send `req.HTTP1()` in its order. It adds `Host` and `Connection: keep-alive` first and `Content-Length` on a POST. It writes names as Chrome does (`User-Agent`, lowercase `sec-ch-ua`) and removes `priority`. Both example transports do this for `geo.captcha-delivery.com` and for each host that replied over HTTP/1.1.

## Cloudflare

A Cloudflare solve is a job that runs on our side, through your proxy. You send the page and the proxy; the clearance is for the address that proxy exits from.

```go
cf, _ := cloudflare.New(cloudflare.Config{APIKey: os.Getenv("JOHNANTIBOT_API_KEY")})

// A challenge page: the cf_clearance cookie.
res, err := cf.SolveChallenge(ctx, &cloudflare.ChallengeInput{
	URL:      "https://www.example.com/",
	Proxy:    "http://user:pass@host:port", // any form a provider prints; a public address
	Timezone: "America/New_York",           // the zone of the proxy's location
})

// A Turnstile widget: the token.
res, err = cf.SolveTurnstile(ctx, &cloudflare.TurnstileInput{PageURL: "https://www.example.com/login", SiteKey: "0x4AAAAAAA…", Proxy: proxy})
```

| Result | Use |
|---|---|
| `res.Clearance()` | The `cf_clearance` value. |
| `res.Cookies` | The cookies a browser on the page would send now. |
| `res.Token()` | The Turnstile token, for the form field `cf-turnstile-response`. |
| `res.UserAgent` | Send it in each request to the site, from the same proxy. |
| `res.ExitIP` | The address the site saw. |

A solve takes about 7 seconds. `ctx` bounds the wait, not the job. `SubmitChallenge`, `SubmitTurnstile`, `Job` and `Wait` do the same in steps.

### Proxy

- The proxy must keep its address from one connection to the next, because you spend the clearance from a connection of your own. Use a sticky session (the provider's session id in the username) or a static proxy. A proxy that changes its address on each connection is refused and the price is returned.
- If your proxy admits only listed source addresses, admit ours: `GET /api/catalog`, `egressIps` of `cloudflare`.
- `UserAgent` is optional. Set it to your client's user agent to get a clearance for it (the exact user agent of a Chrome build on macOS that the API runs; anything else is refused), or leave it empty and use `res.UserAgent`.

### Billing and errors

The price is charged when the job is sent and returned if it fails or is declined.

| Check | Meaning |
|---|---|
| `cloudflare.IsDeclined(err)` | The site turned this attempt down (a few in a hundred). Send another. |
| `IsRotatingProxy` | The proxy changes its address on each connection. |
| `IsNoChallenge` | The page serves no challenge. |
| `IsInsufficientBalance` | The balance is below the price. |
| `IsSiteNotApproved` | The site is not approved for your key. |
| `IsRateLimited` | 50 open jobs per account. |
| `IsBadRequest` | The input was refused. |

## reCAPTCHA

A reCAPTCHA v3 token is a job that runs on our side, through your proxy. You send the page, the site key and the proxy; we run the reCAPTCHA client of the site, so the token is for the address that proxy exits from.

```go
rc, _ := recaptcha.New(recaptcha.Config{APIKey: os.Getenv("JOHNANTIBOT_API_KEY")})

// A page that loads the classic script (api.js).
res, err := rc.SolveV3(ctx, &recaptcha.Input{
	URL:     "https://www.example.com/login", // the page that asks for the token (approved for your key)
	SiteKey: "6Lc…",                          // the site's reCAPTCHA key
	Action:  "login",                         // what the page passes to execute; empty is "submit"
	Proxy:   "http://user:pass@host:port",    // any form a provider prints; a public address
})

// A page that loads reCAPTCHA Enterprise (enterprise.js): the same input.
res, err = rc.SolveEnterprise(ctx, &recaptcha.Input{URL: "https://www.example.com/login", SiteKey: "6Lc…", Proxy: proxy})
```

| Result | Use |
|---|---|
| `res.Token` | The token: the `g-recaptcha-response` value, or what the page's `execute` call would have returned. |
| `res.ExpiresAt` | When Google stops accepting it. Send the token before. |
| `res.Headers()` | `user-agent`, `accept-language` and the client hints of the browser the token was made as. Send them in the request that carries the token. |

A solve takes a few seconds. `ctx` bounds the wait, not the job. `SubmitV3`, `SubmitEnterprise`, `Job` and `Wait` do the same in steps. `Timeout` is the job's wall: from 10 seconds to the most the API allows (60 on the hosted API); empty is 30.

- The proxy must have a public address. Send the request that carries the token from the same address.
- If your proxy admits only listed source addresses, admit ours: `GET /api/catalog`, `egressIps` of `recaptcha`.

### Billing and errors

The price is charged when the job is sent and returned if it fails. A job that ends without a token is a `*recaptcha.JobError` with the API's text; a refusal is a `*recaptcha.APIError` with `StatusCode` and `Message`.

| Check | Meaning |
|---|---|
| `recaptcha.IsInsufficientBalance(err)` | The balance is below the price. |
| `IsSiteNotApproved` | The site is not approved for your key. |
| `IsRateLimited` | The account has too many open jobs: wait for one to finish. |
| `IsBadRequest` | The input was refused: the proxy, the site key, a field. |
| `IsDeclined` | The API turned the attempt down and did not charge it. Send another. |

## Account

Each paid call takes its price from your balance when it starts, and gets a `402` when the balance is less. `Balance` reads it first. It is free.

```go
acct, _ := account.New(account.Config{APIKey: os.Getenv("JOHNANTIBOT_API_KEY")})
balance, err := acct.Balance(ctx)
// balance.Micros is in millionths of a dollar, the unit of a price in the catalog: 2000 is $0.002. balance.Dollars() is for display.
```

A key that is not metered (one of the service's own) has no account: `balance.Metered` is `false` and `Micros` is zero. A missing, unknown or revoked key, or the key of a disabled account, is `account.IsBadKey(err)`. Software under an organization reads the balance of each user's own key.

## Examples

`examples` is its own module. Set `JOHNANTIBOT_API_KEY`, then:

```bash
cd examples
export CF_PROXY=http://user:pass@host:port   # a sticky session or a static proxy

go run ./cloudflare_challenge -url https://www.example.com/
go run ./cloudflare_turnstile -url https://www.example.com/login -sitekey 0x4AAAAAAA…
go run ./recaptcha_v3 -url https://www.example.com/login -sitekey 6Lc… -proxy http://user:pass@host:port
go run ./account_balance
go run ./datadome_interstitial -transport httpcloak -proxy http://user:pass@host:port
```

| Program | Does |
|---|---|
| `cloudflare_challenge` | Loads a page (403), clears it through the proxy, loads it again with the clearance and the user agent of the result. |
| `cloudflare_turnstile` | Solves a widget and prints the token. |
| `recaptcha_v3` | Makes a reCAPTCHA v3 token (`-enterprise` for Enterprise pages) and prints it with the headers of the browser it was made as. |
| `account_balance` | Prints the balance of your key's account in dollars. |
| `datadome_tags` | Loads a page and posts the `ch` and `le` tags. |
| `datadome_interstitial` | Solves the interstitial and reloads the page. |
| `datadome_slider` | Solves an interstitial or slider until the page loads. |
