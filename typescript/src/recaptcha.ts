import { JobClient, type CallOptions, type ClientOptions, type Job } from './http.ts'

/** A page that uses reCAPTCHA v3, and the proxy the token is made through. */
export interface RecaptchaInput extends CallOptions {
  /** The page that asks for the token. The site must be approved for your key. */
  url: string
  /** The site's reCAPTCHA key. */
  siteKey: string
  /** Any form a provider prints; a public address. Send the token from the same address. */
  proxy: string
  /** What the page passes to `execute`, for example "login"; omitted: "submit". */
  action?: string
  /** The job's time, in milliseconds, from 10,000 (the API's default is 30,000). */
  timeoutMs?: number
}

/** The token, and the browser it was made as. */
export interface RecaptchaResult {
  /** The `g-recaptcha-response` value, or what the page's `execute` call would have returned. */
  token: string
  /** When Google stops accepting the token; null when the API gave none. */
  expiresAt: Date | null
  userAgent: string
  acceptLanguage: string
  secUa?: string
  secUaMobile?: string
  secUaPlatform?: string
  /** `user-agent`, `accept-language` and the client hints the result has: send them in the request that carries the token. */
  headers: Record<string, string>
}

type Wire = Omit<RecaptchaResult, 'expiresAt' | 'headers'> & { expiresAt?: string | null }

export type RecaptchaJob = Job<Wire>

/** reCAPTCHA v3 tokens, classic and Enterprise, as jobs run through your proxy. */
export class Recaptcha extends JobClient<Wire, RecaptchaResult> {
  constructor(options: ClientOptions = {}) {
    super('recaptcha', options)
  }

  /** A token for a page that loads the classic script (api.js). */
  async solveV3(input: RecaptchaInput): Promise<RecaptchaResult> {
    return this.wait(await this.submitV3(input), input)
  }

  /** A token for a page that loads reCAPTCHA Enterprise (enterprise.js). */
  async solveV3Enterprise(input: RecaptchaInput): Promise<RecaptchaResult> {
    return this.wait(await this.submitV3Enterprise(input), input)
  }

  /** Sends a classic v3 job and returns it queued. The price is charged now and returned if the job fails. */
  submitV3(input: RecaptchaInput): Promise<RecaptchaJob> {
    return this.#send('v3', input)
  }

  /** Sends an Enterprise job and returns it queued. */
  submitV3Enterprise(input: RecaptchaInput): Promise<RecaptchaJob> {
    return this.#send('v3-enterprise', input)
  }

  async #send(task: string, input: RecaptchaInput): Promise<RecaptchaJob> {
    const { url, siteKey, proxy, action, timeoutMs, signal } = input ?? ({} as never)
    if (!url || !siteKey || !proxy) throw new Error('recaptcha: a solve needs a URL, a site key and a proxy')
    return this.submit(task, { url, siteKey, proxy, action, timeoutMs }, signal)
  }

  protected result(raw: Wire): RecaptchaResult {
    const headers: Record<string, string> = {}
    for (const [name, value] of [
      ['user-agent', raw.userAgent],
      ['accept-language', raw.acceptLanguage],
      ['sec-ch-ua', raw.secUa],
      ['sec-ch-ua-mobile', raw.secUaMobile],
      ['sec-ch-ua-platform', raw.secUaPlatform],
    ]) {
      if (name && value) headers[name] = value
    }
    return { ...raw, expiresAt: raw.expiresAt ? new Date(raw.expiresAt) : null, headers }
  }
}
