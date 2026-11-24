import { Transport, type CallOptions, type ClientOptions } from './http.ts'

/** One Kasada fingerprint solve: the ips.js the site served and the identity that will post to `/tl`. */
export interface KasadaPayloadInput extends CallOptions {
  /** The ips.js body, exactly as fetched. */
  script: string
  /** The block or /fp page that named ips.js. Send this or `ipsLink`. */
  html?: string
  /** The ips.js path and query from that page. Send this or `html`. */
  ipsLink?: string
  /** The URL of the block or /fp page. The site must be approved for your key. */
  pageUrl: string
  userAgent: string
  /** The address the resulting /tl request will be posted from. */
  ip: string
  acceptLanguage: string
  /** Continue a browser identity from an earlier result. */
  session?: string
}

/** The p.js whose x-kpsdk-v value is wanted. */
export interface KasadaVersionInput extends CallOptions {
  script: string
  pageUrl: string
  userAgent?: string
  acceptLanguage?: string
  session?: string
}

/** The x-kpsdk-v value p.js stamps on /fp. */
export interface KasadaVersionResult {
  version: string
  payload: string
  session: string
  userAgent: string
}

/** The binary /tl artifact and the browser it was made as. */
export interface KasadaPayloadResult {
  /** The decoded binary /tl request body. Post it unchanged. */
  payload: Uint8Array
  payloadBase64: string
  /** The `x-kpsdk-*` headers for the /tl POST. */
  headers: Record<string, string>
  tlUrl: string
  session: string
  userAgent: string
}

type Wire = Omit<KasadaPayloadResult, 'payload'>

function decodeBase64(value: string): Uint8Array {
  if (typeof Buffer !== 'undefined') return new Uint8Array(Buffer.from(value, 'base64'))
  const binary = atob(value)
  return Uint8Array.from(binary, (ch) => ch.charCodeAt(0))
}

/** Kasada payloads: your client fetches and posts; this computes the /tl artifact. */
export class Kasada {
  readonly #http: Transport

  constructor(options: ClientOptions = {}) {
    this.#http = new Transport('kasada', options, 60_000)
  }

  get endpoint(): string {
    return this.#http.endpoint
  }

  async version(input: KasadaVersionInput): Promise<KasadaVersionResult> {
    const { script, pageUrl, userAgent, acceptLanguage, session, signal } = input ?? ({} as never)
    if (!script || !pageUrl) throw new Error('kasada: a version needs script and pageUrl')
    return this.#http.call<KasadaVersionResult>('POST', '/v1/kasada/version', {
      script, pageUrl, userAgent, acceptLanguage, session,
    }, signal)
  }

  async generatePayload(input: KasadaPayloadInput): Promise<KasadaPayloadResult> {
    const { script, html, ipsLink, pageUrl, userAgent, ip, acceptLanguage, session, signal } = input ?? ({} as never)
    if (!script || !pageUrl || !userAgent || !ip || !acceptLanguage || (!html && !ipsLink)) {
      throw new Error('kasada: a payload needs script, html or ipsLink, pageUrl, userAgent, ip and acceptLanguage')
    }
    const raw = await this.#http.call<Wire>('POST', '/v1/kasada/payload', {
      script, html, ipsLink, pageUrl, userAgent, ip, acceptLanguage, session,
    }, signal)
    return { ...raw, payload: decodeBase64(raw.payloadBase64) }
  }
}
