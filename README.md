# John Antibot Go SDK

Go client for the [John Antibot](https://antibot.johnavenue.com) DataDome and Cloudflare APIs.

```bash
go get github.com/John-Avenue/john-antibot-sdk@latest
```

## Usage

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

- `tags.Payload`: form body for the site's `/js/`.
- `inter.FormBody`: form body for `https://geo.captcha-delivery.com/interstitial/`.
- `slider.CheckURL`: GET it.

Use one `Session` per proxy and cookie jar, and send `session.UserAgent` on every request.

Software you distribute under an organization sets `OrgKey` (sent as `x-org-key`) next to each user's own `APIKey`.

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
// res.Clearance(): the cf_clearance value. res.UserAgent: send it in each request to the site, from the same proxy.

// A Turnstile widget: the token.
res, err = cf.SolveTurnstile(ctx, &cloudflare.TurnstileInput{PageURL: "https://www.example.com/login", SiteKey: "0x4AAAAAAA…", Proxy: proxy})
// res.Token()
```

- A solve takes about 7 seconds. `ctx` bounds the wait, not the job. `SubmitChallenge`, `SubmitTurnstile`, `Job` and `Wait` are the same in steps.
- The price is charged when the job is sent and returned if it fails or is declined. `cloudflare.IsDeclined(err)`: the site turned this attempt down (a few in a hundred); send another.
- `IsInsufficientBalance`, `IsSiteNotApproved`, `IsRateLimited` (50 open jobs per account), `IsBadRequest` and `IsNoChallenge` name the other refusals.
- `UserAgent` is optional. Set it to your client's user agent to get a clearance for it (the exact user agent of a Chrome build on macOS that the API runs; anything else is refused), or leave it empty and use `res.UserAgent`.
- If your proxy admits only listed source addresses, admit ours: `GET /api/catalog`, `egressIps` of `cloudflare`.

## HTTP/1.1

`geo.captcha-delivery.com` uses HTTP/1.1 only. Header sets are in HTTP/2 form. When your transport sends a request over HTTP/1.1, send `req.HTTP1()` in its order. It adds `Host` and `Connection: keep-alive` first and `Content-Length` on a POST. It writes names as Chrome does (`User-Agent`, lowercase `sec-ch-ua`) and removes `priority`. Both example transports do this for `geo.captcha-delivery.com` and for each host that replied over HTTP/1.1.

## Flows

`flow.Solve` takes a 403 body and runs the whole challenge on your transport. `flow.SendTags` posts tags. `examples/` has httpcloak and tls-client transports.

```bash
cd examples && go run ./datadome_interstitial -transport httpcloak -proxy http://user:pass@host:port
```
