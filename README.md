# John Antibot Go SDK

Go client for the [John Antibot](https://antibot.johnavenue.com) DataDome API.

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

## Flows

`flow.Solve` takes a 403 body and runs the whole challenge on your transport. `flow.SendTags` posts tags. `examples/` has httpcloak and tls-client transports.

```bash
cd examples && go run ./datadome_interstitial -transport httpcloak -proxy http://user:pass@host:port
```
