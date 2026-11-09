/** The hosted API, used when `endpoint` is not set. */
export const DEFAULT_ENDPOINT = 'https://antibot.johnavenue.com'

/** What every client takes. */
export interface ClientOptions {
  /** Your API key, sent as `x-api-key`. */
  apiKey?: string
  /** Software distributed under an organization sends its key, as `x-org-key`, next to each user's own `apiKey`. */
  orgKey?: string
  /** The API base URL; defaults to `DEFAULT_ENDPOINT`. */
  endpoint?: string
  /** The deadline of one HTTP call, in milliseconds (not of a solve: pass a `signal` for that). */
  timeoutMs?: number
  /** The fetch to call the API with; defaults to the global one. */
  fetch?: typeof fetch
}

/** Every call takes a `signal` that ends it. */
export interface CallOptions {
  signal?: AbortSignal
}

/** A non-2xx reply of the API. `message` is its error text. */
export class ApiError extends Error {
  readonly status: number
  /** DataDome: the site blocked the session outright. Change the proxy and the session. */
  readonly hardblock: boolean
  constructor(status: number, message: string, hardblock = false) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.hardblock = hardblock
  }
}

/** A job that ended without a result: `failed` (the price is returned) or `declined` (not charged: send another). */
export class JobError extends Error {
  readonly id: string
  readonly state: string
  readonly retry: boolean
  constructor(id: string, state: string, message: string, retry = false) {
    super(message)
    this.name = 'JobError'
    this.id = id
    this.state = state
    this.retry = retry
  }
}

const api = (err: unknown, status?: number): err is ApiError => err instanceof ApiError && (status === undefined || err.status === status)
const ended = (err: unknown, text: string) => err instanceof JobError && err.message.includes(text)

/** DataDome: the site blocked the session outright (`422`, `hardblock: true`). Free. */
export const isHardblock = (err: unknown): boolean => api(err) && (err.hardblock || err.message.includes('hardblock'))
/** DataDome: a puzzle captcha, not a slider. Free. */
export const isPuzzleUnsupported = (err: unknown): boolean => api(err) && err.message.includes('puzzle slider unsupported')
/** `429`. */
export const isRateLimited = (err: unknown): boolean => api(err, 429)
/** `402`: the balance is below the price. */
export const isInsufficientBalance = (err: unknown): boolean => api(err, 402)
/** `403`: the site is not approved for your key. */
export const isSiteNotApproved = (err: unknown): boolean => api(err, 403)
/** `400`: the input was refused. */
export const isBadRequest = (err: unknown): boolean => api(err, 400)
/** `401`: the key is missing, unknown or revoked, or its account is disabled. */
export const isBadKey = (err: unknown): boolean => api(err, 401)
/** A job the site turned down: send another. */
export const isDeclined = (err: unknown): boolean => err instanceof JobError && err.state === 'declined'
/** Cloudflare: the page serves no challenge. */
export const isNoChallenge = (err: unknown): boolean => ended(err, 'serves no challenge')
/** Cloudflare: the proxy changes its address on each connection. */
export const isRotatingProxy = (err: unknown): boolean => ended(err, 'different address on each connection')

const gzipFrom = 32 << 10

async function gzip(data: Uint8Array<ArrayBuffer>): Promise<Uint8Array<ArrayBuffer>> {
  const stream = new Blob([data]).stream().pipeThrough(new CompressionStream('gzip'))
  return new Uint8Array(await new Response(stream).arrayBuffer())
}

function apiError(status: number, statusText: string, text: string): ApiError {
  let error: unknown
  let hardblock = false
  try {
    const v = JSON.parse(text)
    error = v?.error
    hardblock = v?.hardblock === true
  } catch {}
  const message = (typeof error === 'string' && error) || text.trim() || `${status} ${statusText}`.trim()
  return new ApiError(status, message, hardblock)
}

/** @internal One service's calls to the API. */
export class Transport {
  readonly endpoint: string
  readonly service: string
  readonly #apiKey: string
  readonly #orgKey: string
  readonly #timeoutMs: number
  readonly #fetch: typeof fetch

  constructor(service: string, options: ClientOptions, defaultTimeoutMs: number) {
    this.service = service
    this.endpoint = (options.endpoint || DEFAULT_ENDPOINT).replace(/\/+$/, '')
    this.#apiKey = options.apiKey ?? ''
    this.#orgKey = options.orgKey ?? ''
    this.#timeoutMs = options.timeoutMs && options.timeoutMs > 0 ? options.timeoutMs : defaultTimeoutMs
    this.#fetch = options.fetch ?? ((input, init) => fetch(input, init))
  }

  async call<T>(method: 'GET' | 'POST', path: string, payload?: object, signal?: AbortSignal): Promise<T> {
    signal?.throwIfAborted()
    const headers: Record<string, string> = {}
    if (this.#apiKey) headers['x-api-key'] = this.#apiKey
    if (this.#orgKey) headers['x-org-key'] = this.#orgKey
    let body: Uint8Array<ArrayBuffer> | undefined
    if (payload !== undefined) {
      headers['content-type'] = 'application/json'
      body = new TextEncoder().encode(JSON.stringify(payload))
      if (body.length >= gzipFrom) {
        body = await gzip(body)
        headers['content-encoding'] = 'gzip'
      }
    }
    const timeout = AbortSignal.timeout(this.#timeoutMs)
    const res = await this.#fetch(this.endpoint + path, { method, headers, body, signal: signal ? AbortSignal.any([signal, timeout]) : timeout })
    const text = await res.text()
    if (!res.ok) throw apiError(res.status, res.statusText, text)
    try {
      return JSON.parse(text) as T
    } catch (err) {
      throw new Error(`${this.service}: decode response: ${(err as Error).message}`, { cause: err })
    }
  }
}

/** A job's states. `queued` and `running` have not ended. */
export type JobState = 'queued' | 'running' | 'done' | 'failed' | 'declined'

/** A job as the API reports it. */
export interface Job<R = unknown> {
  id: string
  state: JobState
  task?: string
  /** When `failed` or `declined`: why. */
  error?: string
  /** When `declined`: send a new job. */
  retry?: boolean
  /** When `done`. */
  result?: R
  /** When `done`: the API no longer holds the result. */
  resultExpired?: boolean
  createdAt?: string
  startedAt?: string
  finishedAt?: string
  /** The proxy, without its credentials. */
  proxy?: string
}

export interface JobOptions extends CallOptions {
  /** Hold the call until the job ends, up to this many seconds (the API's most is 25). */
  waitSeconds?: number
}

const pollWait = 25
const pollRetries = 3

function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(resolve, ms)
    // (the listener stays: removing one stops a timeout signal from ever firing on some runtimes)
    signal?.addEventListener('abort', () => (clearTimeout(timer), reject(signal.reason)), { once: true })
  })
}

/** @internal The job calls Cloudflare and reCAPTCHA share. */
export abstract class JobClient<Raw, Result> {
  protected readonly http: Transport

  constructor(service: string, options: ClientOptions) {
    this.http = new Transport(service, options, 40_000)
  }

  /** The API base URL. */
  get endpoint(): string {
    return this.http.endpoint
  }

  protected abstract result(raw: Raw): Result

  protected async submit(task: string, payload: Record<string, unknown>, signal?: AbortSignal): Promise<Job<Raw>> {
    for (const [k, v] of Object.entries(payload)) if (v == null || v === '' || v === 0) delete payload[k] // an empty optional is left out
    const job = await this.http.call<Job<Raw>>('POST', `/v1/${this.http.service}/${task}`, payload, signal)
    if (!job?.id) throw new Error(`${this.http.service}: the API answered without a job id`)
    return job
  }

  /** Reads a job. With `waitSeconds`, the call returns once the job has ended, or after that long with the state it has. */
  job(id: string, options: JobOptions = {}): Promise<Job<Raw>> {
    if (!id) return Promise.reject(new Error(`${this.http.service}: a job id is required`))
    const wait = options.waitSeconds && options.waitSeconds > 0 ? `?wait=${Math.ceil(options.waitSeconds)}` : ''
    return this.http.call('GET', `/v1/${this.http.service}/jobs/${encodeURIComponent(id)}${wait}`, undefined, options.signal)
  }

  /** Follows a job until it ends. A job that failed or was declined is a `JobError`. The `signal` ends the wait, not the job. */
  async wait(job: Job<Raw>, options: CallOptions = {}): Promise<Result> {
    if (!job) throw new Error(`${this.http.service}: a job is required`)
    const { signal } = options
    for (let failures = 0; job.state === 'queued' || job.state === 'running'; ) {
      try {
        job = await this.job(job.id, { waitSeconds: pollWait, signal })
        failures = 0
      } catch (err) {
        if (signal?.aborted) throw signal.reason
        // (the network or a 5xx may not be there next time; the job runs on either way)
        if ((err instanceof ApiError && err.status < 500) || failures >= pollRetries) throw err
        failures++
        await sleep(1000, signal)
      }
    }
    if (job.state !== 'done') throw new JobError(job.id, job.state, job.error ?? '', job.retry === true)
    if (job.result) return this.result(job.result)
    throw new JobError(
      job.id,
      job.state,
      job.resultExpired ? 'the job is done but the API no longer holds its result' : 'the job is done without a result',
    )
  }
}
