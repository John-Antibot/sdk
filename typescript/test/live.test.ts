// Against the hosted API. Opt in: JOHNANTIBOT_LIVE=1 and JOHNANTIBOT_API_KEY (JOHNANTIBOT_ENDPOINT for another API).
// JOHNANTIBOT_LIVE_REFERER (a page of a site approved for the key) adds one tags call, which is charged.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { Account, DataDome } from '../src/index.ts'

const { JOHNANTIBOT_LIVE, JOHNANTIBOT_API_KEY: apiKey, JOHNANTIBOT_ENDPOINT: endpoint, JOHNANTIBOT_LIVE_REFERER: referer } = process.env
const skip = JOHNANTIBOT_LIVE === '1' && apiKey ? false : 'set JOHNANTIBOT_LIVE=1 and JOHNANTIBOT_API_KEY'

test('live: balance', { skip }, async () => {
  const b = await new Account({ apiKey, endpoint }).balance()
  assert.equal(typeof b.metered, 'boolean')
  assert.equal(typeof b.balanceMicros, 'number')
})

test('live: tags', { skip: skip || (!referer && 'set JOHNANTIBOT_LIVE_REFERER') }, async () => {
  const session = new DataDome({ apiKey, endpoint }).session({ referer })
  const tags = await session.generateTags({ event: 'ch', cookie: 'DUMMYCOOKIE', siteKey: 'KEY' })
  assert.ok(tags.payload.length > 0)
  assert.ok(session.session)
})
