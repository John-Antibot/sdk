import { Transport, type CallOptions, type ClientOptions } from './http.ts'
import { sha256Hex } from './sha256.ts'

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
  /**
   * Chrome 156, 157 or 158 on macOS. Optional: left out, the service picks one of them and the result's `userAgent` says which; use it for
   * every request of the session.
   */
  userAgent?: string
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

/** What x-kpsdk-cd is made from besides the session. They change only when the site's p.js does. */
export interface KasadaCdParams {
  difficulty: number
  subchallengeCount: number
  cryptoCustomerKey: string
  seedSuffix: string
  /** What p.js hashes first, for example `tp-v2-input`. */
  label: string
  /** Whether the label is followed by the first 16 characters of `ct` (a desktop browser's is). */
  ctPrefix: boolean
}

/** The x-kpsdk-v value p.js stamps on /fp, and what x-kpsdk-cd is made from. */
export interface KasadaVersionResult {
  version: string
  payload: string
  session: string
  userAgent: string
  /** Absent when p.js's proof of work could not be read. */
  cd?: KasadaCdParams
  /**
   * `x-kpsdk-cd` for one protected request, computed here: about ten SHA-256 hashes, no API call, no charge.
   * `ct` is the session's current `x-kpsdk-ct` (the one sent on this very request; each reply rotates it) and `st` the
   * `x-kpsdk-st` header /tl answered with, in milliseconds. Make a new one for every request.
   */
  cdHeader(ct: string, st: number): string
}

/** p.js's proof of work: per round, the first nonce n whose sha256("<n>, <previous hash>") scores at least difficulty/rounds
 * (score = 2^52 over the first 13 hex digits, plus one); the passing hash starts the next round. */
export function cdAnswers(first: string, difficulty: number, rounds: number): number[] {
  const target = difficulty / rounds
  let hash = sha256Hex(first)
  const answers: number[] = []
  while (answers.length < rounds) {
    for (let n = 1; ; n++) {
      const next = sha256Hex(`${n}, ${hash}`)
      if (2 ** 52 / (parseInt(next.slice(0, 13), 16) + 1) >= target) {
        answers.push(n)
        hash = next
        break
      }
    }
  }
  return answers
}

/** `x-kpsdk-cd` from the version's `cd` parameters (see {@link KasadaVersionResult.cdHeader}). */
export function cdHeader(params: KasadaCdParams | undefined, ct: string, st: number): string {
  if (!params) throw new Error("kasada: this version has no cd parameters (p.js's proof of work could not be read)")
  if (!(params.difficulty >= 1) || !(params.subchallengeCount >= 1)) throw new Error('kasada: difficulty and subchallengeCount must be positive')
  if (!(st > 0)) throw new Error('kasada: st must be the x-kpsdk-st value /tl answered with')
  let label = params.label || 'tp-v2-input'
  if (params.ctPrefix) {
    if (ct.length < 16) throw new Error('kasada: ct must be the x-kpsdk-ct value (at least 16 characters)')
    label += ct.slice(0, 16)
  }
  const raw = new Uint8Array(16)
  for (let i = 0; i < 16; i++) raw[i] = Math.floor(Math.random() * 256)
  const id = Array.from(raw, (b) => b.toString(16).padStart(2, '0')).join('')
  const workTime = Date.now() - 10 - Math.floor(Math.random() * 41)
  let first = `${label}, ${workTime}, ${id}`
  if (params.cryptoCustomerKey) first += `, ${params.cryptoCustomerKey}`
  if (params.seedSuffix) first += `, ${params.seedSuffix}`
  // a real page's clock reads st a little before or after the server's, the same for the whole session
  const d = Math.floor((parseInt(sha256Hex(String(st)).slice(0, 2), 16) * 90) / 255) - 10
  return JSON.stringify({
    workTime, id, answers: cdAnswers(first, params.difficulty, params.subchallengeCount),
    duration: (200 + Math.floor(Math.random() * 551)) / 10, d, st, rst: st + d,
  })
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
    const raw = await this.#http.call<Omit<KasadaVersionResult, 'cdHeader'>>('POST', '/v1/kasada/version', {
      script, pageUrl, userAgent, acceptLanguage, session,
    }, signal)
    const cd = raw.cd
    // a method, not a field: the result stays what the API sent when it is logged or serialized
    return Object.defineProperty({ ...raw }, 'cdHeader', { value: (ct: string, st: number) => cdHeader(cd, ct, st) }) as KasadaVersionResult
  }

  async generatePayload(input: KasadaPayloadInput): Promise<KasadaPayloadResult> {
    const { script, html, ipsLink, pageUrl, userAgent, ip, acceptLanguage, session, signal } = input ?? ({} as never)
    if (!script || !pageUrl || !ip || !acceptLanguage || (!html && !ipsLink)) {
      throw new Error('kasada: a payload needs script, html or ipsLink, pageUrl, ip and acceptLanguage')
    }
    const raw = await this.#http.call<Wire>('POST', '/v1/kasada/payload', {
      script, html, ipsLink, pageUrl, userAgent, ip, acceptLanguage, session,
    }, signal)
    return { ...raw, payload: decodeBase64(raw.payloadBase64) }
  }
}
