import { Transport, type CallOptions, type ClientOptions } from './http.ts'

/** Serves the interstitial and captcha pages. */
export const GEO_ORIGIN = 'https://geo.captcha-delivery.com'

/** The Chrome major version sessions use by default. */
export const LATEST_CHROME_MAJOR = 156

/** Chrome's reduced macOS user agent for a major version. */
export const chromeUserAgent = (major: number): string =>
  `Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/${major}.0.0.0 Safari/537.36`

/** Chrome `LATEST_CHROME_MAJOR` on macOS: the user agent of a new session. */
export const LATEST_USER_AGENT = chromeUserAgent(LATEST_CHROME_MAJOR)

/** The Chrome major version in a user agent, or 0. */
export function chromeMajor(userAgent: string): number {
  const m = /Chrome\/(\d+)\./.exec(userAgent)
  return m ? Number(m[1]) : 0
}

/** The `sec-ch-ua` header Chrome sends for a user agent, or "" if it is not Chrome. */
export function secChUa(userAgent: string): string {
  const major = chromeMajor(userAgent)
  if (!major) return ''
  const chars = [' ', '(', ':', '-', '.', '/', ')', ';', '=', '?', '_']
  const brands = [
    `"Not${chars[major % 11]}A${chars[(major + 1) % 11]}Brand";v="${['8', '99', '24'][major % 3]}"`,
    `"Chromium";v="${major}"`,
    `"Google Chrome";v="${major}"`,
  ]
  const order = [[0, 1, 2], [0, 2, 1], [1, 0, 2], [1, 2, 0], [2, 0, 1], [2, 1, 0]][major % 6]
  const list: string[] = []
  order.forEach((at, i) => (list[at] = brands[i]))
  return list.join(', ')
}

/** The quoted `sec-ch-ua-platform` for a user agent, or "". */
export function secChUaPlatform(userAgent: string): string {
  for (const [marker, platform] of [
    ['Windows NT', '"Windows"'],
    ['Macintosh', '"macOS"'],
    ['Android', '"Android"'],
    ['CrOS', '"Chrome OS"'],
    ['Linux', '"Linux"'],
  ]) {
    if (userAgent.includes(marker)) return platform
  }
  return ''
}

/** JavaScript's `Date.prototype.getTimezoneOffset()` for an IANA zone at a time (240 for New York in summer). */
export function jsOffsetMinutes(zone: string, at: Date = new Date()): number {
  const name = new Intl.DateTimeFormat('en-US', { timeZone: zone, timeZoneName: 'longOffset' })
    .formatToParts(at)
    .find((p) => p.type === 'timeZoneName')?.value
  const m = /GMT([+-])(\d{2}):?(\d{2})?/.exec(name ?? '')
  const minutes = m ? Number(m[2]) * 60 + Number(m[3] ?? 0) : 0
  return minutes === 0 ? 0 : m?.[1] === '+' ? -minutes : minutes
}

// Go's url.QueryEscape: everything but letters, digits and -_.~ escaped, a space as +.
const queryEscape = (s: string) =>
  encodeURIComponent(s)
    .replace(/[!'()*]/g, (c) => '%' + c.charCodeAt(0).toString(16).toUpperCase())
    .replace(/%20/g, '+')

const orderedQuery = (pairs: [string, string][]) => pairs.map(([k, v]) => queryEscape(k) + '=' + queryEscape(v)).join('&')

/** The `var dd = {...}` block of a blocked page. */
export interface DDObject {
  /** "i" interstitial, "c" captcha. */
  rt: string
  cid: string
  hash: string
  /** "" solvable, "fe" slider escalation, "bv" hard block. */
  t: string
  s: string
  e: string
  b: string
  cookie: string
  /** `t` is "bv": change the proxy and the session. */
  hardBlocked: boolean
}

/** Reads the dd block of a blocked page. */
export function parseDD(html: string): DDObject {
  const get = (k: string) => {
    const m = new RegExp(`['"]${k}['"]\\s*:\\s*(?:['"]([^'"]*)['"]|(-?[0-9]+))`).exec(html)
    return m ? m[1] || m[2] || '' : ''
  }
  const t = get('t')
  return { rt: get('rt'), cid: get('cid'), hash: get('hsh'), t, s: get('s'), e: get('e'), b: get('b'), cookie: get('cookie'), hardBlocked: t === 'bv' }
}

/** The interstitial URL for a dd block on the page `referer`. */
export function buildInterstitialUrl(dd: DDObject, referer: string): string {
  let q = orderedQuery([['initialCid', dd.cid], ['hash', dd.hash], ['cid', dd.cookie], ['referer', referer], ['s', dd.s]])
  if (dd.e) q += '&e=' + queryEscape(dd.e)
  return `${GEO_ORIGIN}/interstitial/?${q}&b=${queryEscape(dd.b)}&dm=cd`
}

/** The captcha URL for a dd block on the page `referer`. */
export function buildCaptchaUrl(dd: DDObject, referer: string): string {
  let q = orderedQuery([['initialCid', dd.cid], ['hash', dd.hash], ['cid', dd.cookie], ['t', dd.t], ['referer', referer], ['s', dd.s]])
  if (dd.e) q += '&e=' + queryEscape(dd.e)
  return `${GEO_ORIGIN}/captcha/?${q}&dm=cd`
}

const challengeLinkRe = /https:\/\/geo\.captcha-delivery\.com\/(?:captcha|interstitial)\/\?[^"'\\\s<]+/

/**
 * The challenge URL in a 403 body: the link it carries, or one built from its dd block (with `cookie`, the jar's datadome
 * cookie, when the block has none). `captcha` is a slider. Undefined when the body has no challenge.
 */
export function challengeLink(body: string, pageUrl: string, cookie = ''): { link: string; captcha: boolean } | undefined {
  body = body.replaceAll('\\/', '/')
  let link = challengeLinkRe.exec(body)?.[0]
  if (link) {
    link = link.replaceAll('&amp;', '&')
    if (!link.includes('&dm=')) link += '&dm=' + (body.trimStart().startsWith('{') ? 'jd' : 'cd')
    return { link, captcha: link.includes('/captcha/') }
  }
  const dd = parseDD(body)
  if (!dd.cid || !dd.hash) return undefined
  dd.cookie ||= cookie
  if (dd.rt === 'c') return { link: buildCaptchaUrl(dd, pageUrl), captcha: true }
  if (dd.rt === 'i') return { link: buildInterstitialUrl(dd, pageUrl), captcha: false }
  return undefined
}

const challengeScriptRe = /<script[^>]+src="(https:\/\/ct\.captcha-delivery\.com\/(?:interstitial|captcha)[^"]*\.js)"/

/** The script a challenge page loads, or "" when the page inlines it. Load it right after the page and pass its source as `script`. */
export const challengeScriptUrl = (html: string): string => challengeScriptRe.exec(html)?.[1] ?? ''

/** A captcha page that is the unsolvable hardblock variant. */
export const isHardblockPage = (html: string): boolean => /responseType\s*=\s*['"]hardblock['"]/.test(html)

/** The reply to an interstitial POST or a slider check GET. */
export interface GeoAnswer {
  /** The body parsed as JSON. */
  isJson: boolean
  /** "redirect" when cleared, "captcha" on a slider handoff. */
  view: string
  /** The redirect target, or the slider handoff URL. */
  url: string
  /** The `t` parameter of `url`. */
  t: string
  /** The `ir` parameter of `url`. */
  ir: string
  /** The whole Set-Cookie line. */
  rawCookie: string
  /** The datadome cookie value. */
  cookie: string
  /** Its Max-Age in seconds (0: none, -1: delete). */
  cookieMaxAge: number
  /** `t` is "bv". */
  hardBlock: boolean
  /** Cleared with a usable cookie. */
  cleared: boolean
}

/** Reads a geo.captcha-delivery.com reply body. */
export function validateGeo(body: string): GeoAnswer {
  const a: GeoAnswer = { isJson: false, view: '', url: '', t: '', ir: '', rawCookie: '', cookie: '', cookieMaxAge: 0, hardBlock: false, cleared: false }
  let geo: Record<string, unknown> | null
  try {
    geo = JSON.parse(body)
  } catch {
    return a
  }
  if (typeof geo !== 'object' || Array.isArray(geo)) return a
  const str = (v: unknown) => (typeof v === 'string' ? v : '')
  a.isJson = true
  a.view = str(geo?.view).trim().toLowerCase()
  a.url = str(geo?.url)
  a.rawCookie = str(geo?.cookie)
  ;[a.cookie, a.cookieMaxAge] = readSetCookie(a.rawCookie)
  if (a.url) {
    try {
      const q = new URL(a.url, GEO_ORIGIN).searchParams
      a.t = (q.get('t') ?? '').toLowerCase()
      a.ir = q.get('ir') ?? ''
      a.hardBlock = a.t === 'bv'
    } catch {}
  }
  a.cleared = a.view === 'redirect' && a.cookie !== ''
  return a
}

function readSetCookie(line: string): [string, number] {
  const [pair = '', ...attrs] = line.split(';')
  const eq = pair.indexOf('=')
  if (eq < 0 || pair.slice(0, eq).trim() !== 'datadome') return ['', 0]
  let value = pair.slice(eq + 1).trim()
  if (value.length > 1 && value.startsWith('"') && value.endsWith('"')) value = value.slice(1, -1)
  let maxAge = 0
  for (const attr of attrs) {
    const [k = '', v = ''] = attr.split('=').map((s) => s.trim())
    if (k.toLowerCase() === 'max-age' && /^-?\d+$/.test(v)) maxAge = Number(v) > 0 ? Number(v) : -1
  }
  return [value, maxAge]
}

/** The interstitial form body with its `referer` and `url` fields set to `pageUrl` ("" leaves it as it is). */
export function formBodyFor(result: Pick<InterstitialResult, 'formBody'>, pageUrl: string): string {
  if (!pageUrl || !result.formBody) return result.formBody
  return result.formBody
    .split('&')
    .map((part) => {
      const key = part.split('=', 1)[0]
      return key === 'referer' || key === 'url' ? `${key}=${queryEscape(pageUrl)}` : part
    })
    .join('&')
}

/** One browser identity. Every field is optional. */
export interface SessionOptions {
  /** The service's session token: set it only to resume one. */
  session?: string
  /** An alias of `session`. */
  sessionKey?: string
  /** Pins a device profile by name. */
  device?: string
  /** The browser user agent; defaults to `LATEST_USER_AGENT`. */
  userAgent?: string
  /** The default page URL of calls. */
  referer?: string
  /** An IANA zone, for example America/New_York. */
  timezone?: string
  /** `getTimezoneOffset()` for that zone (see `jsOffsetMinutes`). */
  timezoneOffset?: number
}

export interface DataDomeOptions extends ClientOptions {
  /** Merged into every session the client creates. */
  defaults?: SessionOptions
}

export interface TagsInput extends CallOptions {
  /** "ch" first, then "le", on one session. */
  event: 'ch' | 'le'
  /** The current datadome cookie value. */
  cookie: string
  /** The site's DataDome key (ddjskey). */
  siteKey: string
  /** The tags.js version (ddv). */
  version?: string
  /** The source of the tags.js the page loaded: that build runs. Only its SHA-256 is sent. Empty: the current build. */
  script?: string
  /** The page URL; defaults to the session's referer. */
  referer?: string
  /** The @font-face families the page loaded (`document.fonts`). Required by the API for most sites. */
  loadedFontFamilies?: string[]
  /** The site's tags.js, when it is not `https://datadome.<site>/tags.js`. */
  tagsScriptUrl?: string
}

export interface TagsResult {
  /** The form body for the site's `/js/`. */
  payload: string
  session: string
  userAgent: string
  generatedAt: number
}

export interface InterstitialInput extends CallOptions {
  /** The interstitial page. */
  html: string
  /** The URL `html` was loaded from. */
  deviceLink: string
  /** The source of the page's script (`challengeScriptUrl`); omit it when the page inlines it. Only its SHA-256 is sent. */
  script?: string
  /** The blocked page; defaults to the session's referer. */
  referer?: string
  /** When `html` was loaded. */
  fetchedAt?: Date | number
}

export interface InterstitialResult {
  /** The form body for `https://geo.captcha-delivery.com/interstitial/`. */
  formBody: string
  /** The same as `formBody`. */
  payload: string
  session: string
  userAgent: string
}

export interface SliderInput extends CallOptions {
  /** The captcha page. */
  html: string
  /** The URL `html` was loaded from. */
  captchaUrl: string
  /** The page's script; see `InterstitialInput.script`. */
  script?: string
  /** The page that showed the captcha; omitted, it is read from `html`. */
  parentUrl?: string
  /** When `html` was loaded. */
  fetchedAt?: Date | number
}

export interface SliderResult {
  /** The `/captcha/check` URL: GET it. */
  checkUrl: string
  /** The same as `checkUrl`. */
  payload: string
  session: string
  userAgent: string
}

interface Reply {
  payload?: string
  session?: string
  userAgent?: string
}

let subtle: Promise<SubtleCrypto> | undefined
const nodeCrypto = 'node:crypto'

async function scriptHash(script: string): Promise<string> {
  subtle ??= globalThis.crypto?.subtle ? Promise.resolve(globalThis.crypto.subtle) : import(nodeCrypto).then((m) => m.webcrypto.subtle)
  const digest = await (await subtle).digest('SHA-256', new TextEncoder().encode(script))
  return Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, '0')).join('')
}

function requireScript(html: string, script?: string): void {
  const u = challengeScriptUrl(html)
  if (u && !script) throw new Error(`datadome: script is required: load ${u} right after the page and pass its source`)
}

/** The DataDome payload generators. */
export class DataDome {
  readonly #http: Transport
  readonly #defaults: SessionOptions

  constructor(options: DataDomeOptions = {}) {
    this.#http = new Transport('datadome', options, 60_000)
    this.#defaults = options.defaults ?? {}
  }

  /** The API base URL. */
  get endpoint(): string {
    return this.#http.endpoint
  }

  /** A new browser identity: use one per proxy and cookie jar. */
  session(options: SessionOptions = {}): Session {
    return new Session(this.#http, options, this.#defaults)
  }
}

/** One browser identity, made by `DataDome.session`. Send `userAgent` on every request of the session. */
export class Session {
  /** The service's session token, learned from each reply. */
  session: string
  device: string
  userAgent: string
  referer: string
  timezone: string
  timezoneOffset: number
  readonly #http: Transport

  /** @internal Use `DataDome.session`. */
  constructor(http: Transport, o: SessionOptions, d: SessionOptions) {
    this.#http = http
    this.session = o.session || o.sessionKey || d.session || d.sessionKey || ''
    this.device = o.device || d.device || ''
    this.userAgent = o.userAgent || d.userAgent || LATEST_USER_AGENT
    this.referer = o.referer || d.referer || ''
    this.timezone = o.timezone || d.timezone || ''
    this.timezoneOffset = o.timezoneOffset || d.timezoneOffset || 0
  }

  #identity(): Record<string, unknown> {
    const m: Record<string, unknown> = {}
    if (this.session) m.session = m.sessionKey = this.session
    if (this.device) m.device = this.device
    if (this.userAgent) m.userAgent = this.userAgent
    if (this.timezone) m.timezone = this.timezone
    if (this.timezoneOffset) m.timezoneOffset = this.timezoneOffset
    return m
  }

  #post(route: string, req: Record<string, unknown>, signal?: AbortSignal): Promise<Reply> {
    return this.#http.call<Reply>('POST', '/v1/datadome/' + route, req, signal)
  }

  #learn(out: Reply): void {
    if (out.session) this.session = out.session
    if (!this.userAgent && out.userAgent) this.userAgent = out.userAgent
  }

  /** One tags payload: `ch`, then `le`. */
  async generateTags(input: TagsInput): Promise<TagsResult> {
    const referer = input.referer || this.referer
    if (!referer) throw new Error('datadome: TagsInput.referer (or Session.referer) is required: the page the tags run on')
    const req = this.#identity()
    req.cid = input.cookie
    req.jsType = input.event
    if (input.siteKey) req.ddk = input.siteKey
    if (input.version) req.version = input.version
    if (input.script) req.scriptHash = await scriptHash(input.script)
    req.referer = referer
    if (input.loadedFontFamilies?.length) req.loadedFontFamilies = input.loadedFontFamilies
    if (input.tagsScriptUrl) req.tagsScriptUrl = input.tagsScriptUrl
    const out = (await this.#post('tags', req, input.signal)) as TagsResult
    this.#learn(out)
    return out
  }

  /** The interstitial form to POST to `https://geo.captcha-delivery.com/interstitial/`. */
  async generateInterstitial(input: InterstitialInput): Promise<InterstitialResult> {
    if (!input.deviceLink) throw new Error('datadome: InterstitialInput.deviceLink is required: the interstitial URL you loaded')
    requireScript(input.html, input.script)
    const req = this.#identity()
    req.html = input.html
    req.deviceUrl = input.deviceLink
    const referer = input.referer || this.referer
    if (referer) req.referer = referer
    if (input.fetchedAt) req.challengeFetchedAtMs = Number(input.fetchedAt)
    if (input.script) req.scriptHash = await scriptHash(input.script)
    const out = await this.#post('interstitial', req, input.signal)
    if (typeof out.payload !== 'string' || !out.payload.startsWith('cid=')) {
      throw new Error('datadome: service returned no ready-to-post form; it predates the form-body wire')
    }
    this.#learn(out)
    return { formBody: out.payload, payload: out.payload, session: out.session ?? '', userAgent: out.userAgent ?? '' }
  }

  /** The `/captcha/check` URL to GET. */
  async generateSlider(input: SliderInput): Promise<SliderResult> {
    if (!input.captchaUrl) throw new Error('datadome: SliderInput.captchaUrl is required: the captcha URL you loaded')
    requireScript(input.html, input.script)
    const req = this.#identity()
    req.html = input.html
    req.captchaUrl = input.captchaUrl
    if (input.parentUrl) req.parentUrl = input.parentUrl
    if (input.fetchedAt) req.challengeFetchedAtMs = Number(input.fetchedAt)
    if (input.script) req.scriptHash = await scriptHash(input.script)
    const out = await this.#post('slider', req, input.signal)
    const p = typeof out.payload === 'string' ? out.payload : ''
    const u = /^https?:\/\//.test(p) ? p : p.startsWith('?') ? `${GEO_ORIGIN}/captcha/check${p}` : ''
    if (!u) throw new Error('datadome: service returned no check URL; it predates the check-URL wire')
    this.#learn(out)
    return { checkUrl: u, payload: u, session: out.session ?? '', userAgent: out.userAgent ?? '' }
  }
}
