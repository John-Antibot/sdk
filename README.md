# John Antibot Go SDK

Go client for the [John Antibot](https://antibot.johnavenue.com) DataDome and Cloudflare APIs.

```bash
go get github.com/John-Avenue/john-antibot-sdk@latest
```

| Package | |
|---|---|
| `datadome` | Generators for tags, interstitial and slider payloads. |
| `cloudflare` | Cloudflare challenge and Turnstile solves, as jobs on our side through your proxy. |
| `flow` | Complete DataDome flows over your own HTTP transport. |
| `examples` | Runnable programs for each of the above. |

Each client takes `APIKey`. Software you distribute under an organization also sets `OrgKey` (sent as `x-org-key`) next to each user's own `APIKey`.

## DataDome

```go
client, _ := datadome.New(datadome.Config{APIKey: os.Getenv("JOHNANTIBOT_API_KEY")})
session := client.Session(datadome.Session{Referer: "https://seatgeek.com/"})

// Tags: ch, then le.
tags, err := session.GenerateTags(ctx, &datadome.TagsInput{Event: datadome.EventCH, Cookie: cid, SiteKey: ddk})

// Interstitial: the page, the URL you loaded, the script it loads.
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

## Examples

`examples` is its own module. Set `JOHNANTIBOT_API_KEY`, then:

```bash
cd examples
export CF_PROXY=http://user:pass@host:port   # a sticky session or a static proxy

go run ./cloudflare_challenge -url https://www.example.com/
go run ./cloudflare_turnstile -url https://www.example.com/login -sitekey 0x4AAAAAAA…
go run ./datadome_interstitial -transport httpcloak -proxy http://user:pass@host:port
```

| Program | Does |
|---|---|
| `cloudflare_challenge` | Loads a page (403), clears it through the proxy, loads it again with the clearance and the user agent of the result. |
| `cloudflare_turnstile` | Solves a widget and prints the token. |
| `datadome_tags` | Loads a page and posts the `ch` and `le` tags. |
| `datadome_interstitial` | Solves the interstitial and reloads the page. |
| `datadome_slider` | Solves an interstitial or slider until the page loads. |
