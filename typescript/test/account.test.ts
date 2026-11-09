import { after, test } from 'node:test'
import assert from 'node:assert/strict'
import { Account, ApiError, DEFAULT_ENDPOINT, isBadKey } from '../src/index.ts'
import { closeAll, serve } from './server.ts'

after(closeAll)

test('balance reads the account of the key', async () => {
  const srv = await serve(() => ({ body: { metered: true, balanceMicros: 4821000 } }))
  const b = await new Account({ endpoint: srv.url + '/', apiKey: 'key', orgKey: 'org' }).balance()
  assert.deepEqual(b, { metered: true, balanceMicros: 4821000, balanceUsd: 4.821 })
  const [req] = srv.seen
  assert.equal(`${req.method} ${req.path}${req.query}`, 'GET /v1/balance')
  assert.equal(req.headers['x-api-key'], 'key')
  assert.equal(req.headers['x-org-key'], 'org')
  assert.equal(req.headers['content-type'], undefined)
  assert.equal(req.body, undefined)
})

test('a key that is not metered has no balance', async () => {
  const srv = await serve(() => ({ body: { metered: false } }))
  assert.deepEqual(await new Account({ endpoint: srv.url }).balance(), { metered: false, balanceMicros: 0, balanceUsd: 0 })
})

test('refusals are typed and keep the API’s text', async () => {
  const srv = await serve(() => ({ status: 401, body: { error: 'gateway: missing or bad x-api-key' } }))
  const err = await new Account({ endpoint: srv.url }).balance().catch((e) => e)
  assert.ok(err instanceof ApiError && err.status === 401 && err.message === 'gateway: missing or bad x-api-key' && isBadKey(err))
  const down = await serve(() => ({ status: 503, body: 'We cannot load the billing data.' }))
  const other = await new Account({ endpoint: down.url }).balance().catch((e) => e)
  assert.ok(other instanceof ApiError && other.message === 'We cannot load the billing data.' && !isBadKey(other))
})

test('a default endpoint, a timeout and a signal', async () => {
  assert.equal(new Account().endpoint, DEFAULT_ENDPOINT)
  const hang = await serve(() => 'hang')
  const start = Date.now()
  await assert.rejects(new Account({ endpoint: hang.url, timeoutMs: 100 }).balance(), { name: 'TimeoutError' })
  assert.ok(Date.now() - start < 3000)
  await assert.rejects(new Account({ endpoint: hang.url }).balance({ signal: AbortSignal.timeout(100) }), { name: 'TimeoutError' })
})
