import { JobClient, type CallOptions, type ClientOptions, type Job } from './http.ts'

/** A page that serves a Cloudflare challenge (`challenge`), or a page the edge appends its detection script to (`jsd`). */
export interface ChallengeInput extends CallOptions {
  /** The page. The site must be approved for your key. */
  url: string
  /** Any form a provider prints: `http://user:pass@host:port`, `socks5://…`, `host:port`, `host:port:user:pass`, `user:pass@host:port`. A sticky session or a static proxy. */
  proxy: string
  /** Your client's user agent, the exact one of a Chrome build on macOS the API runs; omit it to get ours in the result. */
  userAgent?: string
  /** The IANA zone of the proxy's location, for example America/New_York. */
  timezone?: string
  /** The same session is the same device. */
  session?: string
  /** The job's time, in milliseconds (the API's default is 60,000). */
  timeoutMs?: number
}

export type JsdInput = ChallengeInput

/** A Turnstile widget on a page. */
export interface TurnstileInput extends CallOptions {
  /** The page with the widget. The site must be approved for your key. */
  pageUrl: string
  /** The widget's site key. */
  siteKey: string
  /** As in `ChallengeInput`. */
  proxy: string
  /** The widget's `data-action`. */
  action?: string
  /** The widget's `data-cdata`. */
  cData?: string
  /** The widget's `chlPageData`. */
  chlPageData?: string
  userAgent?: string
  timezone?: string
  session?: string
  timeoutMs?: number
}

export interface CloudflareCookie {
  name: string
  value: string
  domain?: string
  path?: string
}

/** What a finished Cloudflare job holds. */
export interface CloudflareResult {
  /** `cf_clearance=<value>` for a challenge or jsd, the token for Turnstile. */
  payload: string
  /** The `cf_clearance` value ("" for Turnstile). */
  clearance: string
  /** The Turnstile token, for the form field `cf-turnstile-response`. */
  token: string
  /** Send it in each request to the site, from the same proxy. */
  userAgent: string
  /** The device the solve ran as: send it as `session` in the next solve for the same site session. */
  session?: string
  /** The cookies a browser on the page would send now. */
  cookies: CloudflareCookie[]
  /** The address the site saw. */
  exitIp?: string
  /** How long the solve took. */
  ms?: number
}

type Wire = Omit<CloudflareResult, 'clearance' | 'token' | 'cookies'> & { cookies?: CloudflareCookie[] }

export type CloudflareJob = Job<Wire>

/** Cloudflare challenge, jsd and Turnstile solves, as jobs run through your proxy. */
export class Cloudflare extends JobClient<Wire, CloudflareResult> {
  constructor(options: ClientOptions = {}) {
    super('cloudflare', options)
  }

  /** Clears the challenge page at `url` and waits for the clearance. */
  async solveChallenge(input: ChallengeInput): Promise<CloudflareResult> {
    return this.wait(await this.submitChallenge(input), input)
  }

  /** Runs the detection script of the page at `url` and waits for the clearance its report was granted. */
  async solveJsd(input: JsdInput): Promise<CloudflareResult> {
    return this.wait(await this.submitJsd(input), input)
  }

  /** Solves the Turnstile widget on `pageUrl` and waits for the token. */
  async solveTurnstile(input: TurnstileInput): Promise<CloudflareResult> {
    return this.wait(await this.submitTurnstile(input), input)
  }

  /** Sends a challenge job and returns it queued. The price is charged now and returned if the job fails or is declined. */
  submitChallenge(input: ChallengeInput): Promise<CloudflareJob> {
    return this.#page('challenge', 'a challenge', input)
  }

  /** Sends a jsd job and returns it queued. */
  submitJsd(input: JsdInput): Promise<CloudflareJob> {
    return this.#page('jsd', 'a jsd job', input)
  }

  /** Sends a Turnstile job and returns it queued. */
  async submitTurnstile(input: TurnstileInput): Promise<CloudflareJob> {
    const { pageUrl, siteKey, proxy, action, cData, chlPageData, userAgent, timezone, session, timeoutMs, signal } = input ?? ({} as never)
    if (!pageUrl || !siteKey || !proxy) throw new Error('cloudflare: a turnstile solve needs a page URL, a site key and a proxy')
    return this.submit('turnstile', { pageUrl, sitekey: siteKey, proxy, action, cData, chlPageData, userAgent, timezone, session, timeoutMs }, signal)
  }

  async #page(task: string, what: string, input: ChallengeInput): Promise<CloudflareJob> {
    const { url, proxy, userAgent, timezone, session, timeoutMs, signal } = input ?? ({} as never)
    if (!url || !proxy) throw new Error(`cloudflare: ${what} needs a URL and a proxy`)
    return this.submit(task, { url, proxy, userAgent, timezone, session, timeoutMs }, signal)
  }

  protected result(raw: Wire): CloudflareResult {
    const cookies = raw.cookies ?? []
    const payload = raw.payload ?? ''
    const clearance = payload.startsWith('cf_clearance=')
      ? payload.slice('cf_clearance='.length)
      : (cookies.find((c) => c.name === 'cf_clearance')?.value ?? '')
    return { ...raw, payload, cookies, clearance, token: payload }
  }
}
