import { after, test } from 'node:test'
import assert from 'node:assert/strict'
import {
  ApiError,
  Cloudflare,
  JobError,
  isBadRequest,
  isDeclined,
  isInsufficientBalance,
  isNoChallenge,
  isRateLimited,
  isRotatingProxy,
  isSiteNotApproved,
} from '../src/index.ts'
import { closeAll, serve, type Seen } from './server.ts'

after(closeAll)

const doneJob = {
  id: 'job_1',
  state: 'done',
  task: 'challenge',
  createdAt: '2026-10-23T10:00:00Z',
  proxy: '8.8.8.8:80',
  result: {
    payload: 'cf_clearance=abc123',
    userAgent: 'UA/1',
    session: 'sess_x',
    exitIp: '9.9.9.9',
    ms: 7000,
    cookies: [
      { name: 'cf_clearance', value: 'abc123', domain: 'example.com', path: '/' },
      { name: '__cf_bm', value: 'bm', domain: 'example.com', path: '/' },
    ],
  },
}

// The API: a submit answers 202 with job_1, the first poll says running, the next says terminal.
async function api(terminal: unknown) {
  let polls = 0
  const srv = await serve((req: Seen) => {
    if (req.method === 'POST') return { status: 202, body: { id: 'job_1', state: 'queued' } }
    if (req.path === '/v1/cloudflare/jobs/job_1') return { body: ++polls === 1 ? { id: 'job_1', state: 'running' } : terminal }
    return { status: 404, body: { error: 'not found' } }
  })
  return { srv, cf: new Cloudflare({ endpoint: srv.url + '/', apiKey: 'key', orgKey: 'org' }) }
}

test('solveChallenge submits, then polls until done', async () => {
  const { srv, cf } = await api(doneJob)
  const res = await cf.solveChallenge({ url: 'https://example.com/', proxy: 'u:p@8.8.8.8:80', timezone: 'America/New_York', timeoutMs: 90_000 })
  assert.deepEqual(res, { ...doneJob.result, clearance: 'abc123', token: 'cf_clearance=abc123' })
  const [submit, poll1, poll2] = srv.seen
  assert.equal(srv.seen.length, 3)
  assert.equal(submit.method, 'POST')
  assert.equal(submit.path, '/v1/cloudflare/challenge')
  assert.equal(submit.headers['x-api-key'], 'key')
  assert.equal(submit.headers['x-org-key'], 'org')
  assert.equal(submit.headers['content-type'], 'application/json')
  // the fields given, none of the ones left empty
  assert.deepEqual(submit.body, { url: 'https://example.com/', proxy: 'u:p@8.8.8.8:80', timezone: 'America/New_York', timeoutMs: 90000 })
  // the polls ask the server to hold them 25 s, with the same keys and no body
  for (const p of [poll1, poll2]) {
    assert.equal(p.method, 'GET')
    assert.equal(p.path + p.query, '/v1/cloudflare/jobs/job_1?wait=25')
    assert.equal(p.headers['x-api-key'], 'key')
    assert.equal(p.headers['x-org-key'], 'org')
    assert.equal(p.headers['content-type'], undefined)
    assert.equal(p.body, undefined)
  }
})

test('solveTurnstile', async () => {
  const { srv, cf } = await api({ id: 'job_1', state: 'done', task: 'turnstile', result: { payload: '0.token', userAgent: 'UA/1' } })
  const res = await cf.solveTurnstile({ pageUrl: 'https://example.com/login', siteKey: '0x4AAA', proxy: '8.8.8.8:80', action: 'login' })
  assert.equal(res.token, '0.token')
  assert.equal(res.clearance, '')
  assert.deepEqual(res.cookies, [])
  assert.equal(srv.seen[0].path, '/v1/cloudflare/turnstile')
  assert.deepEqual(srv.seen[0].body, { pageUrl: 'https://example.com/login', sitekey: '0x4AAA', proxy: '8.8.8.8:80', action: 'login' })

  await cf.submitTurnstile({
    pageUrl: 'https://example.com/login', siteKey: 'k', proxy: 'p', action: 'a', cData: 'c', chlPageData: 'd', userAgent: 'UA', timezone: 'Europe/Berlin', session: 's', timeoutMs: 30_000,
  })
  assert.deepEqual(srv.seen.at(-1)?.body, {
    pageUrl: 'https://example.com/login', sitekey: 'k', proxy: 'p', action: 'a', cData: 'c', chlPageData: 'd', userAgent: 'UA', timezone: 'Europe/Berlin', session: 's', timeoutMs: 30000,
  })
})

test('solveJsd', async () => {
  const { srv, cf } = await api({ ...doneJob, task: 'jsd' })
  const res = await cf.solveJsd({ url: 'https://example.com/event', proxy: '8.8.8.8:80' })
  assert.equal(res.clearance, 'abc123')
  assert.equal(srv.seen[0].path, '/v1/cloudflare/jsd')
  assert.deepEqual(srv.seen[0].body, { url: 'https://example.com/event', proxy: '8.8.8.8:80' })
  await cf.submitJsd({ url: 'https://example.com/', proxy: 'p', userAgent: 'UA', timezone: 'America/Chicago', session: 's', timeoutMs: 20_000 })
  assert.deepEqual(srv.seen.at(-1)?.body, { url: 'https://example.com/', proxy: 'p', userAgent: 'UA', timezone: 'America/Chicago', session: 's', timeoutMs: 20000 })
})

test('submit, job and wait in steps', async () => {
  const { srv, cf } = await api(doneJob)
  const job = await cf.submitChallenge({ url: 'https://example.com/', proxy: 'p' })
  assert.deepEqual(job, { id: 'job_1', state: 'queued' })
  assert.equal((await cf.job(job.id)).state, 'running')
  assert.equal(srv.seen.at(-1)?.query, '')
  const done = await cf.job(job.id, { waitSeconds: 1.5 })
  assert.equal(srv.seen.at(-1)?.query, '?wait=2')
  assert.equal(done.state, 'done')
  assert.equal(done.createdAt, '2026-10-23T10:00:00Z')
  // a job that has ended is not polled again
  const before = srv.seen.length
  assert.equal((await cf.wait(done)).clearance, 'abc123')
  assert.equal(srv.seen.length, before)
  await cf.job('a/b', { waitSeconds: 25 }).catch(() => {})
  assert.equal(srv.seen.at(-1)?.path, '/v1/cloudflare/jobs/a%2Fb')
  await assert.rejects(cf.job(''), /a job id is required/)
})

test('a job that ends without a result is a JobError', async () => {
  const cases: [string, object, (e: unknown) => boolean, boolean][] = [
    ['declined', { id: 'job_1', state: 'declined', error: 'the challenge was not passed', retry: true }, isDeclined, true],
    ['failed', { id: 'job_1', state: 'failed', error: 'the proxy is refused: x is not a public address' }, (e) => !isDeclined(e), false],
    ['no wall', { id: 'job_1', state: 'failed', error: 'the page serves no challenge (status 200)' }, isNoChallenge, false],
    [
      'rotating proxy',
      { id: 'job_1', state: 'failed', error: 'the proxy exits from a different address on each connection (1.1.1.1, then 2.2.2.2): a clearance is for one address' },
      isRotatingProxy,
      false,
    ],
    ['done without a result', { id: 'job_1', state: 'done' }, (e) => (e as Error).message.includes('without a result'), false],
    ['result expired', { id: 'job_1', state: 'done', resultExpired: true }, (e) => (e as Error).message.includes('no longer holds its result'), false],
  ]
  for (const [name, terminal, check, retry] of cases) {
    const { cf } = await api(terminal)
    const err = await cf.solveChallenge({ url: 'https://example.com/', proxy: '8.8.8.8:80' }).catch((e) => e)
    assert.ok(err instanceof JobError, name)
    assert.ok(check(err), name)
    assert.equal(err.id, 'job_1', name)
    assert.equal(err.state, (terminal as { state: string }).state, name)
    assert.equal(err.retry, retry, name)
    assert.ok(err.message, name)
    assert.ok(!(err instanceof ApiError), name)
  }
})

test('refusals are ApiErrors', async () => {
  const cases: [number, (e: unknown) => boolean][] = [
    [402, isInsufficientBalance],
    [403, isSiteNotApproved],
    [429, isRateLimited],
    [400, isBadRequest],
  ]
  for (const [status, check] of cases) {
    const srv = await serve(() => ({ status, body: { error: 'gateway: nope' } }))
    const err = await new Cloudflare({ endpoint: srv.url }).solveChallenge({ url: 'https://example.com/', proxy: '8.8.8.8:80' }).catch((e) => e)
    assert.ok(err instanceof ApiError)
    assert.equal(err.status, status)
    assert.equal(err.message, 'gateway: nope')
    assert.ok(check(err))
    assert.equal(srv.seen.length, 1, 'a refusal is not a job: nothing was waited for')
    for (const [, other] of cases) if (other !== check) assert.ok(!other(err))
  }
})

test('a submit reply without an id is an error', async () => {
  const srv = await serve(() => ({ status: 202, body: { state: 'queued' } }))
  await assert.rejects(new Cloudflare({ endpoint: srv.url }).submitChallenge({ url: 'https://example.com/', proxy: 'p' }), /without a job id/)
})

test('inputs are checked before any call', async () => {
  const cf = new Cloudflare({ endpoint: 'http://127.0.0.1:1' })
  await assert.rejects(cf.submitChallenge({ url: 'https://example.com/', proxy: '' }), /a challenge needs a URL and a proxy/)
  await assert.rejects(cf.submitJsd({ url: '', proxy: 'p' }), /a jsd job needs a URL and a proxy/)
  await assert.rejects(cf.submitTurnstile({ pageUrl: 'https://example.com/', siteKey: '', proxy: '8.8.8.8:80' }), /needs a page URL, a site key and a proxy/)
  await assert.rejects(cf.solveChallenge(undefined as never), /needs a URL and a proxy/)
  await assert.rejects(cf.wait(undefined as never), /a job is required/)
})

test('wait survives a bad poll', async () => {
  const flaky = await serve((_, n) => (n === 1 ? { status: 502, body: '' } : { body: doneJob }))
  const res = await new Cloudflare({ endpoint: flaky.url }).wait({ id: 'job_1', state: 'queued' })
  assert.equal(res.clearance, 'abc123')
  assert.equal(flaky.seen.length, 2)
})

test('wait survives a network failure', async () => {
  let calls = 0
  const cf = new Cloudflare({
    fetch: async () => {
      if (++calls === 1) throw new TypeError('fetch failed')
      return new Response(JSON.stringify(doneJob))
    },
  })
  assert.equal((await cf.wait({ id: 'job_1', state: 'running' })).clearance, 'abc123')
  assert.equal(calls, 2)
})

test('wait gives up after four bad polls in a row', async () => {
  const down = await serve(() => ({ status: 503, body: { error: 'billing unavailable; retry shortly' } }))
  const err = await new Cloudflare({ endpoint: down.url }).wait({ id: 'job_1', state: 'running' }).catch((e) => e)
  assert.ok(err instanceof ApiError && err.status === 503)
  assert.equal(down.seen.length, 4)
})

test('a not found is not retried', async () => {
  const srv = await serve(() => ({ status: 404, body: { error: 'not found' } }))
  const err = await new Cloudflare({ endpoint: srv.url }).wait({ id: 'job_x', state: 'running' }).catch((e) => e)
  assert.ok(err instanceof ApiError && err.status === 404)
  assert.equal(srv.seen.length, 1)
})

test('the signal ends the wait, not the job', async () => {
  const running = await serve(() => ({ body: { id: 'job_1', state: 'running' } }))
  const cf = new Cloudflare({ endpoint: running.url })
  const start = Date.now()
  await assert.rejects(cf.wait({ id: 'job_1', state: 'running' }, { signal: AbortSignal.timeout(150) }), { name: 'TimeoutError' })
  assert.ok(Date.now() - start < 3000)

  // one that the server holds, and a retry's pause, end with it too
  const held = await serve(() => 'hang')
  await assert.rejects(new Cloudflare({ endpoint: held.url }).solveChallenge({ url: 'https://example.com/', proxy: 'p', signal: AbortSignal.timeout(150) }), { name: 'TimeoutError' })
  const flaky = await serve(() => ({ status: 502, body: '' }))
  const t0 = Date.now()
  await assert.rejects(new Cloudflare({ endpoint: flaky.url }).wait({ id: 'job_1', state: 'running' }, { signal: AbortSignal.timeout(300) }), { name: 'TimeoutError' })
  assert.ok(Date.now() - t0 < 900)
})

test('the clearance from the cookies when the payload is something else', async () => {
  const { cf } = await api({ id: 'job_1', state: 'done', result: { payload: 'x', userAgent: 'UA', cookies: [{ name: 'cf_clearance', value: 'from-cookie' }] } })
  assert.equal((await cf.solveChallenge({ url: 'https://example.com/', proxy: 'p' })).clearance, 'from-cookie')
})
