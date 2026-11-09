# John Antibot TypeScript SDK

TypeScript client for the [John Antibot](https://antibot.johnavenue.com) DataDome, Cloudflare and reCAPTCHA APIs and your account's balance. No dependencies: Node 18.17+ and Bun.

```bash
npm install https://github.com/John-Antibot/sdk/releases/download/v0.8.1/john-antibot-sdk-0.8.1.tgz
```

| Class | |
|---|---|
| `DataDome` | Generators for tags, interstitial and slider payloads. |
| `Cloudflare` | Cloudflare challenge, JSD and Turnstile solves, as jobs on our side through your proxy. |
| `Recaptcha` | reCAPTCHA v3 tokens, classic and Enterprise, as jobs on our side through your proxy. |
| `Account` | The balance of your key's account. |

Each client takes `{ apiKey, orgKey?, endpoint?, timeoutMs?, fetch? }`. Software you distribute under an organization also sets `orgKey` (sent as `x-org-key`) next to each user's own `apiKey`. `endpoint` is the API base URL, `https://antibot.johnavenue.com` by default. `timeoutMs` bounds one HTTP call (DataDome 60 s, Cloudflare and reCAPTCHA 40 s, Account 20 s). Every call takes a `signal` (an `AbortSignal`) in its input or options.

## DataDome

```ts
import { DataDome } from '@john-antibot/sdk'

const datadome = new DataDome({ apiKey: process.env.JOHNANTIBOT_API_KEY })
const session = datadome.session({ referer: 'https://www.example.com/' })

// Tags: ch, then le. loadedFontFamilies: the @font-face families the page loaded (document.fonts).
const tags = await session.generateTags({ event: 'ch', cookie: cid, siteKey: ddk, loadedFontFamilies: ['…'] })

// Interstitial: the page, the URL you loaded, the script it loads (only its SHA-256 is sent).
const inter = await session.generateInterstitial({ html, deviceLink, script })

// Slider: same inputs.
const slider = await session.generateSlider({ html, captchaUrl, script })
```

| Result | Use |
|---|---|
| `tags.payload` | Form body for the site's `/js/`. |
| `inter.formBody` | Form body for `https://geo.captcha-delivery.com/interstitial/`. |
| `slider.checkUrl` | GET it. |
| `session` | Each result's session token; the session keeps it. |

Use one session per proxy and cookie jar, and send `session.userAgent` on every request. `challengeScriptUrl(html)` is the script to load right after the page and pass as `script` (`""`: the page inlines it).

| Input | Fields |
|---|---|
| `datadome.session(…)` | `referer`, `session`, `device`, `userAgent`, `timezone`, `timezoneOffset`; all optional |
| `generateTags` | `event` (`'ch'` or `'le'`), `cookie`, `siteKey`, `loadedFontFamilies?`, `tagsScriptUrl?` (when not `https://datadome.<site>/tags.js`), `version?`, `script?`, `referer?` |
| `generateInterstitial` | `html`, `deviceLink`, `script?`, `referer?`, `fetchedAt?` |
| `generateSlider` | `html`, `captchaUrl`, `script?`, `parentUrl?`, `fetchedAt?` |

Helpers: `challengeLink(body, pageUrl, cookie?)` (the challenge URL in a 403 body, `{ link, captcha }`), `parseDD(html)`, `buildInterstitialUrl(dd, referer)`, `buildCaptchaUrl(dd, referer)`, `isHardblockPage(html)`, `validateGeo(body)` (the reply to the form or the check: `cleared`, `cookie`, `hardBlock`), `formBodyFor(inter, pageUrl)`, `jsOffsetMinutes(zone, at?)`, `secChUa(userAgent)`, `secChUaPlatform(userAgent)`, `chromeMajor(userAgent)`, `chromeUserAgent(major)`, `LATEST_USER_AGENT`, `LATEST_CHROME_MAJOR`, `GEO_ORIGIN`.

### Billing and errors

The price is charged when the request is sent and returned if it fails. A non-2xx reply is an `ApiError` with `status` and `message`.

| Check | Meaning |
|---|---|
| `isHardblock(err)` | The site blocked the session outright, with no slider to solve (`422`, `hardblock: true`). Change the proxy and the session. Free. |
| `isPuzzleUnsupported(err)` | A puzzle captcha, not a slider. Free. |
| `isRateLimited(err)` | `429`: wait, then try again. |

## Cloudflare

A Cloudflare solve is a job that runs on our side, through your proxy. You send the page and the proxy; the clearance is for the address that proxy exits from.

```ts
import { Cloudflare } from '@john-antibot/sdk'

const cloudflare = new Cloudflare({ apiKey: process.env.JOHNANTIBOT_API_KEY })

// A challenge page: the cf_clearance cookie.
const res = await cloudflare.solveChallenge({
  url: 'https://www.example.com/',
  proxy: 'http://user:pass@host:port', // any form a provider prints; a public address
  timezone: 'America/New_York', // the zone of the proxy's location
})

// A page that loads the JavaScript detections: the cf_clearance their report is granted.
const jsd = await cloudflare.solveJsd({ url: 'https://www.example.com/', proxy })

// A Turnstile widget: the token.
const ts = await cloudflare.solveTurnstile({ pageUrl: 'https://www.example.com/login', siteKey: '0x4AAAAAAA…', proxy })
```

| Result | Use |
|---|---|
| `res.clearance` | The `cf_clearance` value. |
| `res.cookies` | The cookies a browser on the page would send now: `name`, `value`, `domain`, `path`. |
| `ts.token` | The Turnstile token, for the form field `cf-turnstile-response`. |
| `res.userAgent` | Send it in each request to the site, from the same proxy. |
| `res.exitIp` | The address the site saw. |
| `res.session` | Send it as `session` in the next solve for the same site session. |
| `res.payload`, `res.ms` | `cf_clearance=…` (or the token), and how long the solve took. |

| Input | Fields |
|---|---|
| `solveChallenge`, `solveJsd` | `url`, `proxy`, `userAgent?`, `timezone?`, `session?`, `timeoutMs?` |
| `solveTurnstile` | `pageUrl`, `siteKey`, `proxy`, `action?`, `cData?`, `chlPageData?`, `userAgent?`, `timezone?`, `session?`, `timeoutMs?` |

`timeoutMs` is the job's time. `signal` bounds the wait, not the job. `submitChallenge`, `submitJsd`, `submitTurnstile`, `job` and `wait` do the same in steps:

```ts
const job = await cloudflare.submitChallenge({ url, proxy }) // { id, state: 'queued' }
const now = await cloudflare.job(job.id, { waitSeconds: 25 }) // when it ends, or after 25 s with its state
const done = await cloudflare.wait(job) // until it ends
```

Job states: `queued`, `running`, `done`, `failed`, `declined`.

### Proxy

- The proxy must keep its address from one connection to the next, because you spend the clearance from a connection of your own. Use a sticky session (the provider's session id in the username) or a static proxy. A proxy that changes its address on each connection is refused and the price is returned.
- If your proxy admits only listed source addresses, admit ours: `GET /api/catalog`, `egressIps` of `cloudflare`.
- `userAgent` is optional. Set it to your client's user agent to get a clearance for it (the exact user agent of a Chrome build on macOS that the API runs; anything else is refused), or leave it out and use `res.userAgent`.

### Billing and errors

The price is charged when the job is sent and returned if it fails or is declined. A refusal is an `ApiError` with `status` and `message`; a job that ends without a result is a `JobError` with `id`, `state`, `message` and `retry`.

| Check | Meaning |
|---|---|
| `isDeclined(err)` | The site turned this attempt down. Send another. |
| `isRotatingProxy(err)` | The proxy changes its address on each connection. |
| `isNoChallenge(err)` | The page serves no challenge. |
| `isInsufficientBalance(err)` | `402`: the balance is below the price. |
| `isSiteNotApproved(err)` | `403`: the site is not approved for your key. |
| `isRateLimited(err)` | `429`: wait, then try again. |
| `isBadRequest(err)` | `400`: the input was refused. |

## reCAPTCHA

A reCAPTCHA v3 token is a job that runs on our side, through your proxy, so the token is for the address that proxy exits from.

```ts
import { Recaptcha } from '@john-antibot/sdk'

const recaptcha = new Recaptcha({ apiKey: process.env.JOHNANTIBOT_API_KEY })

// A page that loads the classic script (api.js).
const res = await recaptcha.solveV3({
  url: 'https://www.example.com/login', // the page that asks for the token
  siteKey: '6Lc…',
  action: 'login', // what the page passes to execute; omitted: "submit"
  proxy: 'http://user:pass@host:port',
})

// A page that loads reCAPTCHA Enterprise (enterprise.js): the same input.
const ent = await recaptcha.solveV3Enterprise({ url: 'https://www.example.com/login', siteKey: '6Lc…', proxy })
```

| Result | Use |
|---|---|
| `res.token` | The `g-recaptcha-response` value, or what the page's `execute` call would have returned. |
| `res.expiresAt` | When Google stops accepting it (a `Date`, or `null`). Send the token before. |
| `res.headers` | `user-agent`, `accept-language` and the client hints of the browser the token was made as. Send them in the request that carries the token. |
| `res.userAgent`, `res.acceptLanguage`, `res.secUa`, `res.secUaMobile`, `res.secUaPlatform` | The same values, one by one. |

Input: `url`, `siteKey`, `proxy`, `action?`, `timeoutMs?` (the job's time: from 10,000 to the most the API allows, 60,000 on the hosted API; omitted: 30,000). `submitV3`, `submitV3Enterprise`, `job` and `wait` do the same in steps. If your proxy admits only listed source addresses, admit `egressIps` of `recaptcha` in `GET /api/catalog`.

The price is charged when the job is sent and returned if it fails. The checks are Cloudflare's: `isInsufficientBalance`, `isSiteNotApproved`, `isRateLimited`, `isBadRequest`, `isDeclined`.

## Account

Each paid call takes its price from your balance when it starts, and gets a `402` when the balance is less. `balance()` reads it first. It is free.

```ts
import { Account } from '@john-antibot/sdk'

const account = new Account({ apiKey: process.env.JOHNANTIBOT_API_KEY })
const { metered, balanceMicros, balanceUsd } = await account.balance()
```

`balanceMicros` is in millionths of a dollar, the unit of a price in the catalog: 2000 is $0.002. `balanceUsd` is for display. A key that is not metered has `metered: false` and no balance. A missing, unknown or revoked key, or the key of a disabled account, is `isBadKey(err)` (`401`).

## Examples

```bash
npm install && npm run build
export JOHNANTIBOT_API_KEY=…
node examples/challenge.ts https://www.example.com/ http://user:pass@host:port
```

| Program | Does |
|---|---|
| `tags.ts <page> <cookie> <siteKey> <fonts>` | Prints the `ch` tags payload. |
| `interstitial.ts <page> <link> <html file> [script file]` | Prints the interstitial form body. |
| `slider.ts <page> <link> <html file> [script file]` | Prints the slider check URL. |
| `challenge.ts <url> <proxy>` | Clears a challenge page (a declined job is sent again) and prints the clearance. |
| `jsd.ts <url> <proxy>` | Prints the JSD clearance. |
| `turnstile.ts <pageUrl> <siteKey> <proxy>` | Prints the Turnstile token. |
| `recaptcha-v3.ts <url> <siteKey> <proxy> [action]` | Prints a reCAPTCHA v3 token and its headers. |
| `balance.ts` | Prints the balance in dollars. |

## API

```ts
new DataDome({ apiKey?, orgKey?, endpoint?, timeoutMs?, fetch?, defaults? })
  .session(options?) → Session { session, device, userAgent, referer, timezone, timezoneOffset }
    .generateTags(input) → { payload, session, userAgent, generatedAt }
    .generateInterstitial(input) → { formBody, payload, session, userAgent }
    .generateSlider(input) → { checkUrl, payload, session, userAgent }
new Cloudflare(options)
  .solveChallenge(input) / .solveJsd(input) / .solveTurnstile(input) → { payload, clearance, token, userAgent, session?, cookies, exitIp?, ms? }
  .submitChallenge(input) / .submitJsd(input) / .submitTurnstile(input) → Job
  .job(id, { waitSeconds?, signal? }) → Job { id, state, task?, error?, retry?, result?, resultExpired?, createdAt?, startedAt?, finishedAt?, proxy? }
  .wait(job, { signal? }) → result
new Recaptcha(options)
  .solveV3(input) / .solveV3Enterprise(input) → { token, expiresAt, userAgent, acceptLanguage, secUa?, secUaMobile?, secUaPlatform?, headers }
  .submitV3(input) / .submitV3Enterprise(input) → Job; .job(id, options?); .wait(job, options?)
new Account(options).balance({ signal? }) → { metered, balanceMicros, balanceUsd }
ApiError { status, message, hardblock }; JobError { id, state, message, retry }
isHardblock, isPuzzleUnsupported, isRateLimited, isDeclined, isRotatingProxy, isNoChallenge,
isInsufficientBalance, isSiteNotApproved, isBadRequest, isBadKey (err) → boolean
```

## Development

```bash
npm run build && npm run typecheck && npm test   # bun test runs the same tests
JOHNANTIBOT_LIVE=1 JOHNANTIBOT_API_KEY=… npm test # against the hosted API
```
