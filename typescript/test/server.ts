// A stand-in for the API: it records each request (gzip decoded) and answers with what the handler returns.
import { createServer, type IncomingHttpHeaders } from 'node:http'
import type { AddressInfo } from 'node:net'
import { gunzipSync } from 'node:zlib'

export interface Seen {
  method: string
  path: string
  query: string
  headers: IncomingHttpHeaders
  /** The body as sent, before gzip was undone. */
  wireBytes: number
  body: any
}

/** A reply: an object is sent as JSON, a string as is; "hang" never answers. */
export type Reply = { status?: number; body?: unknown } | 'hang'

export interface Mock {
  url: string
  seen: Seen[]
  close(): Promise<void>
}

const servers: Mock[] = []

export async function serve(handler: (req: Seen, n: number) => Reply = () => ({ body: {} })): Promise<Mock> {
  const seen: Seen[] = []
  const server = createServer(async (req, res) => {
    const chunks: Buffer[] = []
    for await (const c of req) chunks.push(c as Buffer)
    let raw = Buffer.concat(chunks)
    const wireBytes = raw.length
    if (req.headers['content-encoding'] === 'gzip') raw = gunzipSync(raw)
    const u = new URL(req.url ?? '/', 'http://mock')
    const s: Seen = { method: req.method ?? '', path: u.pathname, query: u.search, headers: req.headers, wireBytes, body: raw.length ? JSON.parse(raw.toString()) : undefined }
    seen.push(s)
    const r = handler(s, seen.length)
    if (r === 'hang') return
    const body = r.body ?? {}
    res.writeHead(r.status ?? 200, { 'content-type': 'application/json' })
    res.end(typeof body === 'string' ? body : JSON.stringify(body))
  })
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
  const mock: Mock = {
    url: `http://127.0.0.1:${(server.address() as AddressInfo).port}`,
    seen,
    close: () => {
      server.closeAllConnections()
      return new Promise((resolve) => server.close(() => resolve()))
    },
  }
  servers.push(mock)
  return mock
}

/** Closes every server a test file started. */
export const closeAll = () => Promise.all(servers.splice(0).map((s) => s.close()))

/** The keys of an object, sorted. */
export const keys = (o: object) => Object.keys(o).sort()
