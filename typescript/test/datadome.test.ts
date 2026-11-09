import { after, test } from 'node:test'
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import {
  ApiError,
  DataDome,
  DEFAULT_ENDPOINT,
  GEO_ORIGIN,
  LATEST_CHROME_MAJOR,
  LATEST_USER_AGENT,
  buildCaptchaUrl,
  buildInterstitialUrl,
  challengeLink,
  challengeScriptUrl,
  chromeMajor,
  chromeUserAgent,
  formBodyFor,
  isHardblock,
  isHardblockPage,
  isPuzzleUnsupported,
  isRateLimited,
  jsOffsetMinutes,
  parseDD,
  secChUa,
  secChUaPlatform,
  validateGeo,
} from '../src/index.ts'
import { closeAll, keys, serve } from './server.ts'

after(closeAll)

const sha256 = (s: string) => createHash('sha256').update(s).digest('hex')

const interstitialHTML = `<html><head><meta name="dd-page-size" content="10510">
<script>var ddm = { cid: 'CID~x', hash: 'HASH', userEnv: 'UE', seed: 'SEED', env: 'ENV', s: 43624, b: 1265040, e: 'E1' };</script>
<script src="https://ct.captcha-delivery.com/interstitial.1.0.js"></script></head></html>`
const deviceLink = 'https://geo.captcha-delivery.com/interstitial/?x=1'

test('interstitial: the wire and the form', async () => {
  const formBody =
    'cid=CID~x&hash=HASH&referer=https%3A%2F%2Fwww.example.com%2F&url=https%3A%2F%2Fwww.example.com%2F&s=43624&e=E1&env=ENV&userEnv=UE&seed=SEED&b=1265040&dm=cd&ddMessageFormat=&payload=PLV2BLOB&plv3=PLV3TOK&ps=10510'
  const srv = await serve(() => ({ body: { payload: formBody, session: 'sess-1', userAgent: 'UA-FROM-PROFILE' } }))
  const datadome = new DataDome({ endpoint: srv.url + '/', apiKey: 'key', orgKey: 'org' })
  const session = datadome.session({ sessionKey: 'sess-1', timezone: 'America/New_York', timezoneOffset: 240 })
  const out = await session.generateInterstitial({
    html: interstitialHTML,
    deviceLink,
    referer: 'https://www.example.com/',
    fetchedAt: new Date(1792000000000),
    script: '/*script*/',
  })
  const [req] = srv.seen
  assert.equal(req.method, 'POST')
  assert.equal(req.path, '/v1/datadome/interstitial')
  assert.equal(req.query, '')
  assert.equal(req.headers['content-type'], 'application/json')
  assert.equal(req.headers['x-api-key'], 'key')
  assert.equal(req.headers['x-org-key'], 'org')
  assert.equal(req.headers['content-encoding'], undefined)
  // the documented fields only: the service reads what the page carries from html, and the script is never sent
  assert.deepEqual(req.body, {
    html: interstitialHTML,
    deviceUrl: deviceLink,
    scriptHash: sha256('/*script*/'),
    referer: 'https://www.example.com/',
    challengeFetchedAtMs: 1792000000000,
    session: 'sess-1',
    sessionKey: 'sess-1',
    timezone: 'America/New_York',
    timezoneOffset: 240,
    userAgent: LATEST_USER_AGENT,
  })
  // the session speaks the latest Chrome by default and keeps it
  assert.equal(session.session, 'sess-1')
  assert.equal(session.userAgent, LATEST_USER_AGENT)
  assert.deepEqual(out, { formBody, payload: formBody, session: 'sess-1', userAgent: 'UA-FROM-PROFILE' })
  assert.equal(formBodyFor(out, 'https://www.example.com/'), out.formBody)
  assert.equal(formBodyFor(out, ''), out.formBody)
  assert.match(formBodyFor(out, 'https://www.example.com/a b'), /referer=https%3A%2F%2Fwww\.example\.com%2Fa\+b&url=https%3A%2F%2Fwww\.example\.com%2Fa\+b&/)
  assert.ok(!out.formBody.includes('%7E'))
})

test('tags: the wire, then the session it learned', async () => {
  const srv = await serve((_, n) => ({ body: { payload: 'FORM', session: n === 1 ? 'sess-new' : 'sess-2', userAgent: 'UA', generatedAt: 5 } }))
  const session = new DataDome({ endpoint: srv.url }).session({ referer: 'https://www.example.com/' })
  const out = await session.generateTags({ event: 'ch', cookie: 'cookieval', siteKey: 'KEY' })
  assert.deepEqual(out, { payload: 'FORM', session: 'sess-new', userAgent: 'UA', generatedAt: 5 })
  const first = srv.seen[0]
  assert.equal(first.path, '/v1/datadome/tags')
  assert.equal(first.headers['x-api-key'], undefined)
  assert.equal(first.headers['x-org-key'], undefined)
  // the first call carries no session, and no script, no scriptHash
  assert.deepEqual(first.body, { cid: 'cookieval', jsType: 'ch', ddk: 'KEY', referer: 'https://www.example.com/', userAgent: LATEST_USER_AGENT })
  assert.equal(session.session, 'sess-new')

  await session.generateTags({
    event: 'le',
    cookie: 'c',
    siteKey: 'KEY',
    version: '5.1.0',
    script: '/*tags*/',
    referer: 'https://www.example.com/x',
    loadedFontFamilies: ['Inter', 'Inter Fallback'],
    tagsScriptUrl: 'https://js.datadome.co/tags.js',
  })
  assert.deepEqual(srv.seen[1].body, {
    cid: 'c',
    jsType: 'le',
    ddk: 'KEY',
    version: '5.1.0',
    scriptHash: sha256('/*tags*/'),
    referer: 'https://www.example.com/x',
    loadedFontFamilies: ['Inter', 'Inter Fallback'],
    tagsScriptUrl: 'https://js.datadome.co/tags.js',
    session: 'sess-new',
    sessionKey: 'sess-new',
    userAgent: LATEST_USER_AGENT,
  })
  assert.equal(session.session, 'sess-2')

  // an empty list and an empty URL are left out
  await session.generateTags({ event: 'ch', cookie: 'c', siteKey: 'KEY', loadedFontFamilies: [], tagsScriptUrl: '' })
  assert.equal('loadedFontFamilies' in srv.seen[2].body || 'tagsScriptUrl' in srv.seen[2].body, false)
})

test('slider: the wire and the check URL', async () => {
  const payloads = ['?cid=C&plv3=TOK&ir=503003', 'https://geo.captcha-delivery.com/captcha/check?cid=C', 'not a url']
  const srv = await serve((_, n) => ({ body: { payload: payloads[n - 1], session: 's' } }))
  const session = new DataDome({ endpoint: srv.url, defaults: { device: 'dev-1', referer: 'https://www.example.com/' } }).session({ session: 'resume' })
  const captchaUrl = 'https://geo.captcha-delivery.com/captcha/?x=1'
  const out = await session.generateSlider({ html: '<html></html>', captchaUrl, parentUrl: 'https://www.example.com/p', fetchedAt: 1792000000000 })
  assert.equal(srv.seen[0].path, '/v1/datadome/slider')
  assert.deepEqual(srv.seen[0].body, {
    html: '<html></html>',
    captchaUrl,
    parentUrl: 'https://www.example.com/p',
    challengeFetchedAtMs: 1792000000000,
    session: 'resume',
    sessionKey: 'resume',
    device: 'dev-1',
    userAgent: LATEST_USER_AGENT,
  })
  // a query is completed to the check URL
  assert.equal(out.checkUrl, GEO_ORIGIN + '/captcha/check?cid=C&plv3=TOK&ir=503003')
  assert.equal(out.payload, out.checkUrl)
  assert.equal(out.userAgent, '')
  // a whole URL is kept
  assert.equal((await session.generateSlider({ html: '', captchaUrl })).checkUrl, payloads[1])
  // anything else is not a check URL
  await assert.rejects(session.generateSlider({ html: '', captchaUrl }), /no check URL/)
})

test('interstitial: a reply that is not a form is refused', async () => {
  const srv = await serve(() => ({ body: { payload: 'PLV2BLOB', session: 'other' } }))
  const session = new DataDome({ endpoint: srv.url }).session({ session: 's' })
  await assert.rejects(session.generateInterstitial({ html: '', deviceLink }), /no ready-to-post form/)
  assert.equal(session.session, 's', 'a refused reply teaches nothing')
})

test('interstitial: the hash of the script the caller loaded, never the script', async () => {
  assert.equal(challengeScriptUrl(interstitialHTML), 'https://ct.captcha-delivery.com/interstitial.1.0.js')
  assert.equal(challengeScriptUrl('<script>var ddm = {};</script>'), '')
  const srv = await serve(() => ({ body: { payload: 'cid=x', session: 's' } }))
  await new DataDome({ endpoint: srv.url }).session({ sessionKey: 's' }).generateInterstitial({ html: interstitialHTML, script: '/*challenge script*/', deviceLink })
  assert.equal(srv.seen[0].body.scriptHash, sha256('/*challenge script*/'))
  assert.equal(srv.seen[0].body.script, undefined)
})

test('required inputs fail before the network', async () => {
  const s = new DataDome({ endpoint: 'http://127.0.0.1:1' }).session({ sessionKey: 's' }) // nothing listens there
  await assert.rejects(s.generateInterstitial({ html: interstitialHTML, deviceLink: '' }), /deviceLink is required/)
  await assert.rejects(
    s.generateInterstitial({ html: interstitialHTML, deviceLink }),
    /script is required: load https:\/\/ct\.captcha-delivery\.com\/interstitial\.1\.0\.js/,
  )
  await assert.rejects(s.generateSlider({ html: '<html></html>', captchaUrl: '' }), /captchaUrl is required/)
  await assert.rejects(s.generateTags({ event: 'ch', cookie: 'c', siteKey: 'k' }), /referer \(or Session\.referer\) is required/)
})

test('API errors and their predicates', async () => {
  const cases: [string, number, string, (e: unknown) => boolean][] = [
    ['422 hardblock', 422, '{"error":"captcha page is hardblock (no solvable slider)","session":"s","hardblock":true}', isHardblock],
    ['an earlier gateway’s 500', 500, '{"error":"captcha page is hardblock (no solvable slider)"}', isHardblock],
    ['a puzzle', 422, '{"error":"puzzle slider unsupported"}', isPuzzleUnsupported],
    ['429', 429, '{"error":"slow down"}', isRateLimited],
  ]
  for (const [name, status, body, check] of cases) {
    const srv = await serve(() => ({ status, body }))
    const err = await new DataDome({ endpoint: srv.url }).session().generateSlider({ html: '<html></html>', captchaUrl: 'https://geo.captcha-delivery.com/captcha/?x=1' }).catch((e) => e)
    assert.ok(err instanceof ApiError, name)
    assert.equal(err.status, status, name)
    assert.equal(err.message, JSON.parse(body).error, name)
    assert.ok(check(err), name)
    assert.equal(isPuzzleUnsupported(err), check === isPuzzleUnsupported, name)
  }
  const srv = await serve(() => ({ status: 500, body: '{"error":"generation failed"}' }))
  const err = await new DataDome({ endpoint: srv.url }).session({ referer: 'https://www.example.com/' }).generateTags({ event: 'ch', cookie: 'c', siteKey: 'k' }).catch((e) => e)
  assert.equal(err.hardblock, false)
  assert.ok(!isHardblock(err) && !isRateLimited(err) && !isHardblock(new Error('hardblock')))
})

test('an error without JSON keeps the text, else the status', async () => {
  for (const [body, want] of [['  upstream down \n', 'upstream down'], ['', '503 Service Unavailable'], ['{"error":""}', '{"error":""}']]) {
    const srv = await serve(() => ({ status: 503, body }))
    const err = await new DataDome({ endpoint: srv.url }).session({ referer: 'https://www.example.com/' }).generateTags({ event: 'ch', cookie: 'c', siteKey: 'k' }).catch((e) => e)
    assert.ok(err instanceof ApiError)
    assert.equal(err.message, want)
  }
})

test('bodies of 32 KiB or more are gzipped', async () => {
  const srv = await serve(() => ({ body: { payload: 'cid=x' } }))
  const session = new DataDome({ endpoint: srv.url }).session()
  const big = '<html>' + 'a'.repeat(100 << 10) + '</html>'
  await session.generateInterstitial({ html: big, deviceLink })
  await session.generateInterstitial({ html: 'small', deviceLink })
  assert.equal(srv.seen[0].headers['content-encoding'], 'gzip')
  assert.equal(srv.seen[0].body.html, big)
  assert.ok(srv.seen[0].wireBytes < 10 << 10)
  assert.equal(srv.seen[1].headers['content-encoding'], undefined)
})

test('a signal ends a call; so does the client timeout', async () => {
  const srv = await serve(() => 'hang')
  const session = new DataDome({ endpoint: srv.url }).session({ referer: 'https://www.example.com/' })
  const ctl = new AbortController()
  const pending = session.generateTags({ event: 'ch', cookie: 'c', siteKey: 'k', signal: ctl.signal })
  ctl.abort(new Error('caller left'))
  await assert.rejects(pending, /caller left/)
  await assert.rejects(session.generateTags({ event: 'ch', cookie: 'c', siteKey: 'k', signal: AbortSignal.abort() }), { name: 'AbortError' })
  const slow = new DataDome({ endpoint: srv.url, timeoutMs: 100 }).session({ referer: 'https://www.example.com/' })
  const start = Date.now()
  await assert.rejects(slow.generateTags({ event: 'ch', cookie: 'c', siteKey: 'k' }), { name: 'TimeoutError' })
  assert.ok(Date.now() - start < 3000)
})

test('a custom fetch and the default endpoint', async () => {
  const calls: string[] = []
  const datadome = new DataDome({
    fetch: async (input) => {
      calls.push(String(input))
      return new Response('{"payload":"P","session":"s","userAgent":"UA","generatedAt":1}')
    },
  })
  assert.equal(datadome.endpoint, DEFAULT_ENDPOINT)
  await datadome.session({ referer: 'https://www.example.com/' }).generateTags({ event: 'ch', cookie: 'c', siteKey: 'k' })
  assert.deepEqual(calls, ['https://antibot.johnavenue.com/v1/datadome/tags'])
  const bad = new DataDome({ fetch: async () => new Response('not json') })
  await assert.rejects(bad.session({ referer: 'https://www.example.com/' }).generateTags({ event: 'ch', cookie: 'c', siteKey: 'k' }), /datadome: decode response/)
})

test('parsers', () => {
  const dd = parseDD(`var dd = {'rt':'i','cid':'C1','hsh':'H1','t':'','s':123,'e':'E','b':456,'cookie':'CK'}`)
  assert.deepEqual(dd, { rt: 'i', cid: 'C1', hash: 'H1', t: '', s: '123', e: 'E', b: '456', cookie: 'CK', hardBlocked: false })
  const u = buildInterstitialUrl(dd, 'https://www.example.com/')
  assert.ok(u.startsWith(GEO_ORIGIN + '/interstitial/?initialCid=C1&hash=H1&cid=CK&referer=https%3A%2F%2Fwww.example.com%2F&s=123&e=E'), u)
  assert.ok(u.endsWith('&b=456&dm=cd'), u)
  assert.equal(buildCaptchaUrl({ ...dd, t: 'fe', e: '' }, 'https://x/'), GEO_ORIGIN + '/captcha/?initialCid=C1&hash=H1&cid=CK&t=fe&referer=https%3A%2F%2Fx%2F&s=123&dm=cd')
  assert.ok(parseDD(`var dd={"rt":"c","cid":"C","hsh":"H","t":"bv"}`).hardBlocked)
  assert.ok(isHardblockPage(`var responseType = 'hardblock';`))
  assert.ok(!isHardblockPage(`var responseType = 'captcha';`))
})

test('challengeLink: the display mode', () => {
  // API wall: JSON 403, dm=jd
  const api = challengeLink(`{"url":"https:\\/\\/geo.captcha-delivery.com\\/interstitial\\/?initialCid=a&cid=b&hash=h&t=it"}`, 'https://www.example.com/api/x')
  assert.ok(api && !api.captcha && api.link.endsWith('&dm=jd'), JSON.stringify(api))
  // document 403 with an iframe: dm=cd; a link naming its mode is kept
  assert.deepEqual(challengeLink(`<iframe src="https://geo.captcha-delivery.com/captcha/?cid=b&amp;t=fe"></iframe>`, ''), {
    link: 'https://geo.captcha-delivery.com/captcha/?cid=b&t=fe&dm=cd',
    captcha: true,
  })
  assert.equal(challengeLink(`{"url":"https://geo.captcha-delivery.com/interstitial/?cid=b&dm=cd"}`, '')?.link.split('dm=').length, 2)
  // var dd block page: built, with the jar's cookie when the page has none
  const built = challengeLink(`var dd={'rt':'i','cid':'C','hsh':'H','b':1,'s':2}`, 'https://www.example.com/', 'jar')
  assert.ok(built && !built.captcha && built.link.includes('cid=jar') && built.link.endsWith('&dm=cd'), JSON.stringify(built))
  assert.equal(challengeLink('<html>no challenge</html>', ''), undefined)
})

test('validateGeo', () => {
  const ok = validateGeo('{"view":"Redirect","url":"https://www.example.com/","cookie":"datadome=VAL~1; Max-Age=31536000; Domain=.example.com; Path=/; Secure; SameSite=Lax"}')
  assert.deepEqual(ok, {
    isJson: true,
    view: 'redirect',
    url: 'https://www.example.com/',
    t: '',
    ir: '',
    rawCookie: 'datadome=VAL~1; Max-Age=31536000; Domain=.example.com; Path=/; Secure; SameSite=Lax',
    cookie: 'VAL~1',
    cookieMaxAge: 31536000,
    hardBlock: false,
    cleared: true,
  })
  const blocked = validateGeo('{"view":"captcha","url":"https://geo.captcha-delivery.com/captcha/?t=BV&ir=5","cookie":"other=x"}')
  assert.ok(blocked.hardBlock && blocked.t === 'bv' && blocked.ir === '5' && blocked.cookie === '' && !blocked.cleared)
  assert.equal(validateGeo('<html>').isJson, false)
  assert.equal(validateGeo('[1]').isJson, false)
})

test('jsOffsetMinutes', () => {
  assert.equal(jsOffsetMinutes('America/New_York', new Date(Date.UTC(2026, 9, 15, 12))), 240)
  assert.equal(jsOffsetMinutes('America/New_York', new Date(Date.UTC(2026, 11, 15, 12))), 300)
  assert.equal(jsOffsetMinutes('Asia/Kolkata', new Date(Date.UTC(2026, 9, 15, 12))), -330)
  assert.equal(jsOffsetMinutes('UTC'), 0)
  assert.throws(() => jsOffsetMinutes('Not/AZone'), RangeError)
})

test('sec-ch-ua matches Chrome', () => {
  // real Chrome headers (captures and published values)
  const want: Record<number, string> = {
    131: '"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"',
    151: '"Not=A?Brand";v="99", "Google Chrome";v="151", "Chromium";v="151"',
    154: '"Chromium";v="154", "Google Chrome";v="154", "Not A(Brand";v="99"',
  }
  for (const [major, header] of Object.entries(want)) assert.equal(secChUa(chromeUserAgent(Number(major))), header)
  assert.equal(secChUa('Mozilla/5.0 (Macintosh) Version/18.0 Safari/605.1.15'), '')
  assert.equal(chromeMajor(LATEST_USER_AGENT), LATEST_CHROME_MAJOR)
  assert.equal(secChUaPlatform(LATEST_USER_AGENT), '"macOS"')
  assert.equal(secChUaPlatform('Mozilla/5.0 (Windows NT 10.0; Win64; x64)'), '"Windows"')
})

test('the session keys', () => {
  assert.deepEqual(keys(new DataDome().session()), ['device', 'referer', 'session', 'timezone', 'timezoneOffset', 'userAgent'])
})
