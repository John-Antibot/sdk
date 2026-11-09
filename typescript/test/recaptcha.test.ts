import { after, test } from 'node:test'
import assert from 'node:assert/strict'
import { ApiError, JobError, Recaptcha, isBadRequest, isDeclined, isInsufficientBalance, isRateLimited, isSiteNotApproved } from '../src/index.ts'
import { closeAll, serve, type Seen } from './server.ts'

after(closeAll)

const result = {
  token: '03AFcWeA',
  expiresAt: '2026-10-23T10:01:50.000Z',
  userAgent: 'UA/1',
  acceptLanguage: 'en-US,en;q=0.9',
  secUa: '"Chromium";v="156"',
  secUaMobile: '?0',
  secUaPlatform: '"macOS"',
}
const doneJob = { id: 'job_1', state: 'done', task: 'v3', createdAt: '2026-10-23T10:00:00Z', proxy: '8.8.8.8:80', result }

async function api(terminal: unknown) {
  let polls = 0
  const srv = await serve((req: Seen) => {
    if (req.method === 'POST') return { status: 202, body: { id: 'job_1', state: 'queued' } }
    if (req.path === '/v1/recaptcha/jobs/job_1') return { body: ++polls === 1 ? { id: 'job_1', state: 'running' } : terminal }
    return { status: 404, body: { error: 'not found' } }
  })
  return { srv, rc: new Recaptcha({ endpoint: srv.url + '/', apiKey: 'key', orgKey: 'org' }) }
}

test('solveV3 submits, then polls until done', async () => {
  const { srv, rc } = await api(doneJob)
  const res = await rc.solveV3({ url: 'https://example.com/login', siteKey: '6Lc-key_1', action: 'login', proxy: 'u:p@8.8.8.8:80', timeoutMs: 45_000 })
  assert.equal(res.token, '03AFcWeA')
  assert.equal(res.userAgent, 'UA/1')
  assert.equal(res.acceptLanguage, 'en-US,en;q=0.9')
  assert.equal(res.secUaMobile, '?0')
  assert.equal(res.secUaPlatform, '"macOS"')
  assert.deepEqual(res.expiresAt, new Date(Date.UTC(2026, 9, 23, 10, 1, 50)))
  const [submit, ...polls] = srv.seen
  assert.equal(submit.method, 'POST')
  assert.equal(submit.path, '/v1/recaptcha/v3')
  assert.equal(submit.headers['x-api-key'], 'key')
  assert.equal(submit.headers['x-org-key'], 'org')
  assert.equal(submit.headers['content-type'], 'application/json')
  assert.deepEqual(submit.body, { url: 'https://example.com/login', siteKey: '6Lc-key_1', action: 'login', proxy: 'u:p@8.8.8.8:80', timeoutMs: 45000 })
  assert.deepEqual(polls.map((p) => `${p.method} ${p.path}${p.query} ${p.headers['x-api-key']} ${p.headers['x-org-key']}`), [
    'GET /v1/recaptcha/jobs/job_1?wait=25 key org',
    'GET /v1/recaptcha/jobs/job_1?wait=25 key org',
  ])
})

test('Enterprise goes to its route and leaves empty optionals out', async () => {
  const { srv, rc } = await api({ ...doneJob, task: 'v3-enterprise' })
  const res = await rc.solveV3Enterprise({ url: 'https://example.com/', siteKey: '6Lc-key_1', proxy: '8.8.8.8:80', action: '' })
  assert.equal(res.token, '03AFcWeA')
  assert.equal(srv.seen[0].path, '/v1/recaptcha/v3-enterprise')
  assert.deepEqual(srv.seen[0].body, { url: 'https://example.com/', siteKey: '6Lc-key_1', proxy: '8.8.8.8:80' })
  assert.deepEqual(await rc.submitV3Enterprise({ url: 'https://example.com/', siteKey: 'k', proxy: 'p' }), { id: 'job_1', state: 'queued' })
  assert.equal((await rc.job('job_1', { waitSeconds: 10 })).state, 'done')
  assert.equal(srv.seen.at(-1)?.query, '?wait=10')
})

test('a job that ends without a result is a JobError', async () => {
  const cases: [string, object, string, boolean, boolean][] = [
    ['declined', { id: 'job_1', state: 'declined', error: 'the attempt was turned down', retry: true }, 'the attempt was turned down', true, true],
    ['failed', { id: 'job_1', state: 'failed', error: 'the proxy is refused: x is not a public address' }, 'the proxy is refused: x is not a public address', false, false],
    ['result expired', { id: 'job_1', state: 'done', resultExpired: true }, 'no longer holds its result', false, false],
    ['done without a result', { id: 'job_1', state: 'done' }, 'without a result', false, false],
  ]
  for (const [name, terminal, message, declined, retry] of cases) {
    const { rc } = await api(terminal)
    const err = await rc.solveV3({ url: 'https://example.com/', siteKey: 'k', proxy: '8.8.8.8:80' }).catch((e) => e)
    assert.ok(err instanceof JobError, name)
    assert.equal(isDeclined(err), declined, name)
    assert.equal(err.retry, retry, name)
    assert.equal(err.id, 'job_1', name)
    assert.ok(err.message.includes(message), name)
  }
})

test('refusals, and inputs checked before any call', async () => {
  const cases: [number, (e: unknown) => boolean][] = [
    [402, isInsufficientBalance],
    [403, isSiteNotApproved],
    [429, isRateLimited],
    [400, isBadRequest],
  ]
  for (const [status, check] of cases) {
    const srv = await serve(() => ({ status, body: { error: 'gateway: nope' } }))
    const err = await new Recaptcha({ endpoint: srv.url }).solveV3({ url: 'https://example.com/', siteKey: 'k', proxy: '8.8.8.8:80' }).catch((e) => e)
    assert.ok(err instanceof ApiError && err.status === status && err.message === 'gateway: nope' && check(err), String(status))
  }
  const rc = new Recaptcha({ endpoint: 'http://127.0.0.1:1' })
  for (const input of [
    { url: 'https://example.com/', siteKey: 'k', proxy: '' },
    { url: 'https://example.com/', siteKey: '', proxy: '8.8.8.8:80' },
    { url: '', siteKey: 'k', proxy: '8.8.8.8:80' },
    undefined as never,
  ]) {
    await assert.rejects(rc.submitV3(input), /recaptcha: a solve needs a URL, a site key and a proxy/)
  }
})

test('wait survives a bad poll and stops with its signal', async () => {
  const flaky = await serve((_, n) => (n === 1 ? { status: 502, body: '' } : { body: doneJob }))
  assert.equal((await new Recaptcha({ endpoint: flaky.url }).wait({ id: 'job_1', state: 'queued' })).token, '03AFcWeA')
  assert.equal(flaky.seen.length, 2)

  const running = await serve(() => ({ body: { id: 'job_1', state: 'running' } }))
  const start = Date.now()
  await assert.rejects(new Recaptcha({ endpoint: running.url }).wait({ id: 'job_1', state: 'running' }, { signal: AbortSignal.timeout(150) }), {
    name: 'TimeoutError',
  })
  assert.ok(Date.now() - start < 3000)
})

test('a not found is not retried', async () => {
  const srv = await serve(() => ({ status: 404, body: { error: 'not found' } }))
  const err = await new Recaptcha({ endpoint: srv.url }).wait({ id: 'job_x', state: 'running' }).catch((e) => e)
  assert.ok(err instanceof ApiError && err.status === 404)
  assert.equal(srv.seen.length, 1)
})

test('the headers are the browser the token was made as', async () => {
  const { rc } = await api(doneJob)
  const res = await rc.solveV3({ url: 'https://example.com/', siteKey: 'k', proxy: 'p' })
  assert.deepEqual(res.headers, {
    'user-agent': 'UA/1',
    'accept-language': 'en-US,en;q=0.9',
    'sec-ch-ua': '"Chromium";v="156"',
    'sec-ch-ua-mobile': '?0',
    'sec-ch-ua-platform': '"macOS"',
  })
  // a browser that sends no client hints leaves them out; no expiry is null
  const bare = await (await api({ id: 'job_1', state: 'done', result: { token: 't', expiresAt: null, userAgent: 'UA/2', acceptLanguage: 'de' } })).rc.solveV3({
    url: 'https://example.com/',
    siteKey: 'k',
    proxy: 'p',
  })
  assert.deepEqual(bare.headers, { 'user-agent': 'UA/2', 'accept-language': 'de' })
  assert.equal(bare.expiresAt, null)
})
