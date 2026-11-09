// node examples/recaptcha-v3.ts <page URL> <site key> <proxy> [action]
import { Recaptcha } from '@john-antibot/sdk'

const [url, siteKey, proxy, action] = process.argv.slice(2)
const recaptcha = new Recaptcha({ apiKey: process.env.JOHNANTIBOT_API_KEY })

const res = await recaptcha.solveV3({ url, siteKey, proxy, action, signal: AbortSignal.timeout(90_000) })
console.log(res.token, 'until', res.expiresAt)
console.log(res.headers) // send them with the token, from the same proxy
