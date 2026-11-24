import { after, test } from 'node:test'
import assert from 'node:assert/strict'
import { ApiError, Kasada, cdAnswers, isBadRequest, isInsufficientBalance } from '../src/index.ts'
import { sha256Hex } from '../src/sha256.ts'
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

const NIKE_KEY = '1760fa26d0fa116e2f26e90dc8b1379c3d9e946f650852b7f9925457ab77a14a'

test('sha256 agrees with the standard vectors', () => {
  assert.equal(sha256Hex(''), 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855')
  assert.equal(sha256Hex('abc'), 'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad')
  assert.equal(sha256Hex('a'.repeat(1000)), '41edece42d63e8d9bf515a9ba6932e1c20cbc9f5a5d134645adb5db1b9737ea3')
  assert.equal(sha256Hex('é☃'), '85336da7a98ef7c782aec8005e1de5f7488ec5911969cc570a4a83bb339f97ed')
})

// Two cds real Chrome stamped on nike's cart writes (2026-11-24), with the x-kpsdk-ct each request carried.
test('the proof of work reproduces what real Chrome answered', () => {
  assert.deepEqual(cdAnswers(`tp-v2-input045g90GkupSRHwee, 1795536575353, ce1a0639146179bcf6ab456ac1925359, ${NIKE_KEY}`, 10, 2), [13, 5])
  assert.deepEqual(cdAnswers(`tp-v2-input01pDYjEopgLXL7X4, 1795536579918, 2d455b029dc31363685cb443dce15ea4, ${NIKE_KEY}`, 10, 2), [5, 1])
})

test('version carries the cd parameters and makes x-kpsdk-cd locally', async () => {
  const srv = await serve(() => ({
    body: {
      version: 'j-1.2.1020', payload: 'j-1.2.1020', session: 's1', userAgent: 'UA/1',
      cd: { difficulty: 10, subchallengeCount: 2, cryptoCustomerKey: NIKE_KEY, seedSuffix: '', label: 'tp-v2-input', ctPrefix: true },
    },
  }))
  const out = await new Kasada({ endpoint: srv.url }).version({ script: 'pjs', pageUrl: 'https://example.com/' })
  const ct = '045g90GkupSRHweeAkVbaHmX2luTJgqAGpoWX3q5D4o'
  const st = Date.now() - 300
  const offsets = new Set<number>()
  for (let i = 0; i < 10; i++) {
    const cd = JSON.parse(out.cdHeader(ct, st))
    assert.deepEqual(Object.keys(cd), ['workTime', 'id', 'answers', 'duration', 'd', 'st', 'rst'])
    assert.deepEqual(cd.answers, cdAnswers(`tp-v2-input${ct.slice(0, 16)}, ${cd.workTime}, ${cd.id}, ${NIKE_KEY}`, 10, 2))
    assert.equal(cd.rst - cd.st, cd.d)
    assert.ok(cd.d >= -10 && cd.d <= 80 && cd.duration >= 20 && cd.duration <= 75)
    offsets.add(cd.d)
  }
  assert.equal(offsets.size, 1)
  assert.equal(JSON.parse(JSON.stringify(out)).cdHeader, undefined)
  const bare = await new Kasada({ endpoint: (await serve(() => ({ body: { version: 'j', payload: 'j', session: '', userAgent: '' } }))).url }).version({ script: 'pjs', pageUrl: 'https://example.com/' })
  assert.throws(() => bare.cdHeader(ct, st), /no cd parameters/)
})

test('generatePayload without a userAgent sends none and returns the one the service picked', async () => {
  const srv = await serve(() => ({
    body: { payloadBase64: 'AAEC', headers: { 'x-kpsdk-ct': 'ct' }, tlUrl: 'https://example.com/a/b/tl', session: 's1', userAgent: 'Chrome/158 picked' },
  }))
  const out = await new Kasada({ endpoint: srv.url }).generatePayload({
    script: 'ips', ipsLink: '/a/b/ips.js', pageUrl: 'https://example.com/', ip: '203.0.113.1', acceptLanguage: 'en-US',
  })
  assert.equal(out.userAgent, 'Chrome/158 picked')
  assert.equal('userAgent' in srv.seen[0].body, false)
})
