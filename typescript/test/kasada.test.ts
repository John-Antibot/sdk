import { after, test } from 'node:test'
import assert from 'node:assert/strict'
import { ApiError, Kasada, isBadRequest, isInsufficientBalance } from '../src/index.ts'
import { closeAll, serve } from './server.ts'

after(closeAll)

test('generatePayload sends the script and decodes the binary artifact', async () => {
  const srv = await serve(() => ({
    body: {
      payloadBase64: 'AAEC', headers: { 'x-kpsdk-ct': 'ct', 'x-kpsdk-v': 'j-1' },
      tlUrl: 'https://example.com/a/b/tl', session: 's1', userAgent: 'UA/1',
    },
  }))
  const out = await new Kasada({ endpoint: srv.url, apiKey: 'key', orgKey: 'org' }).generatePayload({
    script: 'ips', ipsLink: '/a/b/ips.js', pageUrl: 'https://example.com/', userAgent: 'UA/1', ip: '203.0.113.1', acceptLanguage: 'en-US',
  })
  assert.deepEqual([...out.payload], [0, 1, 2])
  assert.equal(out.headers['x-kpsdk-ct'], 'ct')
  assert.equal(srv.seen[0].path, '/v1/kasada/payload')
  assert.equal(srv.seen[0].headers['x-api-key'], 'key')
  assert.equal(srv.seen[0].headers['x-org-key'], 'org')
  assert.equal(srv.seen[0].body.ip, '203.0.113.1')
})

test('version sends p.js to its route', async () => {
  const srv = await serve((req) => req.path === '/v1/kasada/version'
    ? { body: { version: 'j-1.2.1020', payload: 'j-1.2.1020', session: 's1', userAgent: 'UA/1' } }
    : { status: 404, body: { error: 'not found' } })
  const out = await new Kasada({ endpoint: srv.url }).version({ script: 'pjs', pageUrl: 'https://example.com/' })
  assert.equal(out.version, 'j-1.2.1020')
  assert.equal(srv.seen[0].path, '/v1/kasada/version')
})

test('kasada refusals', async () => {
  for (const [status, check] of [[402, isInsufficientBalance], [400, isBadRequest]] as const) {
    const srv = await serve(() => ({ status, body: { error: 'gateway: nope' } }))
    const err = await new Kasada({ endpoint: srv.url }).generatePayload({
      script: 'ips', ipsLink: '/x', pageUrl: 'https://e/', userAgent: 'UA', ip: '1.2.3.4', acceptLanguage: 'en',
    }).catch((e) => e)
    assert.ok(err instanceof ApiError && check(err), String(status))
  }
})
