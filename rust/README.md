# John Antibot Rust SDK

Rust client for the [John Antibot](https://antibot.johnavenue.com) DataDome, Cloudflare and reCAPTCHA APIs and your account's balance.

```toml
[dependencies]
john-antibot = "0.1"
tokio = { version = "1", features = ["macros", "rt-multi-thread"] }
```

Async on tokio, TLS by rustls. Rust 1.88 or later.

| Module | |
|---|---|
| `datadome` | Tags, interstitial and slider payloads. |
| `cloudflare` | Cloudflare challenge, Turnstile and JSD solves, as jobs through your proxy. |
| `recaptcha` | reCAPTCHA v3 tokens, classic and Enterprise, as jobs through your proxy. |
| `account` | The balance of your key's account. |

Each module has a `Client`: `Client::new(&key)`, or `Client::builder(&key)` with `.org_key(..)`, `.endpoint(..)` (the API root, default `https://antibot.johnavenue.com`), `.timeout(..)` (one HTTP call) and `.http_client(reqwest::Client)`, then `.build()?`. A client is cheap to clone and `Send + Sync`. Inputs are structs: set what you need and end with `..Default::default()`.

Software you distribute under an organization also sets `.org_key(..)` (sent as `x-org-key`) next to each user's own key.

## DataDome

```rust
use john_antibot::datadome;

let key = std::env::var("JOHNANTIBOT_API_KEY")?;
let client = datadome::Client::new(&key);
let mut session = client.session(datadome::SessionOptions {
    referer: Some("https://www.example.com/".into()),
    ..Default::default()
});

// Tags: ch, then le.
let tags = session
    .generate_tags(datadome::TagsInput {
        event: datadome::Event::Ch,
        cookie: cid,
        site_key: ddk,
        loaded_font_families: vec!["Inter".into()], // the @font-face families the page loaded (document.fonts)
        ..Default::default()
    })
    .await?;

// Interstitial: the page, the URL you loaded, the script it loads (only its SHA-256 is sent).
let inter = session
    .generate_interstitial(datadome::InterstitialInput { html, device_link, script, ..Default::default() })
    .await?;

// Slider: same inputs.
let slider = session
    .generate_slider(datadome::SliderInput { html, captcha_url, script, ..Default::default() })
    .await?;
```

| Result | Use |
|---|---|
| `tags.payload` | Form body for the site's `/js/`. |
| `inter.form_body` | Form body for `https://geo.captcha-delivery.com/interstitial/`. |
| `slider.check_url` | GET it. |

Use one `Session` per proxy and cookie jar, and send `session.user_agent()` on every request.

| Field | |
|---|---|
| `SessionOptions` | `session`, `device`, `user_agent`, `referer`, `timezone`, `timezone_offset` |
| `TagsInput` | `event`, `cookie`, `site_key`, `version`, `script`, `referer`, `loaded_font_families`, `tags_script_url` |
| `InterstitialInput` | `html`, `device_link`, `referer`, `fetched_at`, `script` |
| `SliderInput` | `html`, `captcha_url`, `parent_url`, `fetched_at`, `script` |

Helpers: `challenge_link(body, page_url, cookie)` (the challenge a 403 points to), `challenge_script_url(html)` (the script to load, `None` when inline), `parse_dd`, `build_interstitial_url`, `build_captcha_url`, `validate_geo(body)` (the geo reply: `cleared()`, `cookie`, `hard_block`), `inter.form_body_for(page_url)`, `script_hash`, `LATEST_USER_AGENT`, `chrome_user_agent`, `sec_ch_ua`, `sec_ch_ua_platform`, `GEO_ORIGIN`.

The price is charged when the request is sent and returned if it fails.

| Check | Meaning |
|---|---|
| `err.is_hardblock()` | The site blocked the session outright: no slider to solve (`422`, `"hardblock": true`). Change the proxy and the session. Free. |
| `err.is_puzzle_unsupported()` | A puzzle captcha, not a slider. Free. |
| `err.is_rate_limited()` | `429`: wait, then try again. |

## Cloudflare

A Cloudflare solve is a job that runs on our side, through your proxy. The clearance is for the address that proxy exits from.

```rust
use john_antibot::cloudflare;

let cf = cloudflare::Client::new(&key);

// A challenge page: the cf_clearance cookie.
let res = cf
    .solve_challenge(cloudflare::ChallengeInput {
        url: "https://www.example.com/".into(),
        proxy: "http://user:pass@host:port".into(), // any form a provider prints; a public address
        timezone: Some("America/New_York".into()), // the zone of the proxy's location
        ..Default::default()
    })
    .await?;

// A Turnstile widget: the token.
let res = cf
    .solve_turnstile(cloudflare::TurnstileInput {
        page_url: "https://www.example.com/login".into(),
        site_key: "0x4AAAAAAA...".into(),
        proxy: proxy.clone(),
        ..Default::default()
    })
    .await?;

// JSD: the cf_clearance a page's JavaScript detections grant.
let res = cf.solve_jsd(cloudflare::JsdInput { url: "https://www.example.com/".into(), proxy, ..Default::default() }).await?;
```

| Result | Use |
|---|---|
| `res.clearance()` | The `cf_clearance` value (`Option<&str>`). |
| `res.cookies` | The cookies a browser on the page would send now (`name`, `value`, `domain`, `path`). |
| `res.token()` | The Turnstile token, for the form field `cf-turnstile-response`. |
| `res.user_agent` | Send it in each request to the site, from the same proxy. |
| `res.exit_ip` | The address the site saw. |
| `res.session` | Send it as `session` in the next solve for the same site session. |

| Field | |
|---|---|
| `ChallengeInput`, `JsdInput` | `url`, `proxy`, `user_agent`, `timezone`, `session`, `timeout` |
| `TurnstileInput` | `page_url`, `site_key`, `proxy`, `action`, `c_data`, `chl_page_data`, `user_agent`, `timezone`, `session`, `timeout` |

A solve takes about 7 seconds. Wrap it in `tokio::time::timeout` to bound the wait; dropping the future ends the wait, not the job. `submit_challenge`, `submit_turnstile`, `submit_jsd`, `job(id, wait)` and `wait(&job)` do the same in steps:

```rust
let job = cf.submit_challenge(input).await?;           // job.id, job.state: queued
let job = cf.job(&job.id, Duration::from_secs(25)).await?; // returns when the job ends, or after up to 25 s
let res = cf.wait(&job).await?;
```

- The proxy must keep its address from one connection to the next. Use a sticky session (the provider's session id in the username) or a static proxy. A proxy that changes its address on each connection is refused and the price is returned.
- If your proxy admits only listed source addresses, admit ours: `GET /api/catalog`, `egressIps` of `cloudflare`.
- `user_agent` is optional. Set it to your client's user agent to get a clearance for it (the exact user agent of a Chrome build on macOS that the API runs; anything else is refused), or leave it out and use `res.user_agent`.

The price is charged when the job is sent and returned if it fails or is declined.

| Check | Meaning |
|---|---|
| `err.is_declined()` | The site turned this attempt down (a few in a hundred). Send another. |
| `err.is_rotating_proxy()` | The proxy changes its address on each connection. |
| `err.is_no_challenge()` | The page serves no challenge. |
| `err.is_insufficient_balance()` | `402`: the balance is below the price. |
| `err.is_site_not_approved()` | `403`: the site is not approved for your key. |
| `err.is_rate_limited()` | `429`: wait, then try again. |
| `err.is_bad_request()` | `400`: the input was refused. |

## reCAPTCHA

A reCAPTCHA v3 token is a job that runs on our side, through your proxy. The token is for the address that proxy exits from.

```rust
use john_antibot::recaptcha;

let rc = recaptcha::Client::new(&key);
let input = recaptcha::Input {
    url: "https://www.example.com/login".into(), // the page that asks for the token (approved for your key)
    site_key: "6Lc...".into(),                    // the site's reCAPTCHA key
    action: Some("login".into()),                 // what the page passes to execute; absent is "submit"
    proxy: "http://user:pass@host:port".into(),   // any form a provider prints; a public address
    ..Default::default()
};

// A page that loads the classic script (api.js).
let res = rc.solve_v3(input.clone()).await?;

// A page that loads reCAPTCHA Enterprise (enterprise.js): the same input.
let res = rc.solve_v3_enterprise(input).await?;
```

| Result | Use |
|---|---|
| `res.token` | The `g-recaptcha-response` value, or what the page's `execute` call would have returned. |
| `res.expires_at` | When Google stops accepting it (RFC 3339). Send the token before. |
| `res.headers()` | `user-agent`, `accept-language` and the client hints of the browser the token was made as (a `reqwest::header::HeaderMap`). Send them in the request that carries the token. |

`Input` fields: `url`, `site_key`, `action`, `proxy`, `timeout` (the job's wall: from 10 seconds to 60; absent is 30). `submit_v3`, `submit_v3_enterprise`, `job(id, wait)` and `wait(&job)` do the same in steps. Send the request that carries the token from the proxy's address. If your proxy admits only listed source addresses, admit ours: `GET /api/catalog`, `egressIps` of `recaptcha`.

The price is charged when the job is sent and returned if it fails.

| Check | Meaning |
|---|---|
| `err.is_insufficient_balance()` | The balance is below the price. |
| `err.is_site_not_approved()` | The site is not approved for your key. |
| `err.is_rate_limited()` | `429`: wait, then try again. |
| `err.is_bad_request()` | The input was refused: the proxy, the site key, a field. |
| `err.is_declined()` | The attempt was turned down and not charged. Send another. |

## Account

Each paid call takes its price from your balance when it starts, and gets a `402` when the balance is less. `balance()` reads it first. It is free.

```rust
let balance = john_antibot::account::Client::new(&key).balance().await?;
// balance.balance_micros is in millionths of a dollar (2000 is $0.002); balance.balance_usd() is for display.
```

A key that is not metered has no account: `balance.metered` is `false` and `balance_micros` is zero. A missing, unknown or revoked key, or the key of a disabled account, is `err.is_bad_key()`.

## Errors

Every call returns `john_antibot::Result<T>`; the error is `john_antibot::Error`.

| Variant | |
|---|---|
| `Error::Api { status, message, hardblock }` | A non-2xx reply: `message` is the API's text. |
| `Error::Job { id, state, message, retry }` | A job that ended `failed` or `declined`. |
| `Error::Input(String)` | The input was refused before any request was sent. |
| `Error::Transport(reqwest::Error)` | The request did not complete; `err.is_timeout()` for the per-call timeout. |
| `Error::Decode(String)` | The reply could not be read. |

## Examples

Set `JOHNANTIBOT_API_KEY`, then:

```bash
cargo run --example challenge -- https://www.example.com/ http://user:pass@host:port America/New_York
cargo run --example turnstile -- https://www.example.com/login 0x4AAAAAAA... http://user:pass@host:port
cargo run --example jsd -- https://www.example.com/ http://user:pass@host:port
cargo run --example recaptcha_v3 -- https://www.example.com/login 6Lc... http://user:pass@host:port login
cargo run --example balance
cargo run --example tags -- https://www.example.com/ <datadome cookie> <site key> Inter
cargo run --example interstitial -- page.html 'https://geo.captcha-delivery.com/interstitial/?...' script.js
cargo run --example slider -- page.html 'https://geo.captcha-delivery.com/captcha/?...' script.js
```

| Program | Does |
|---|---|
| `challenge` | Clears a challenge page and prints the clearance, the user agent, the exit address and the cookies. |
| `turnstile` | Solves a widget and prints the token. |
| `jsd` | Earns a page's JSD clearance and prints it. |
| `recaptcha_v3` | Makes a token (`--enterprise` for Enterprise pages) and prints it with the headers of the browser it was made as. |
| `balance` | Prints the balance in dollars. |
| `tags` | Prints the `ch` tags payload for a page. |
| `interstitial` | Prints the interstitial form for a page you loaded. |
| `slider` | Prints the check URL for a captcha page you loaded. |

## License

MIT
