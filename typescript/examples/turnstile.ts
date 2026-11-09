// node examples/turnstile.ts <page URL> <site key> <proxy>
import { Cloudflare } from '@john-antibot/sdk'

const [pageUrl, siteKey, proxy] = process.argv.slice(2)
const cloudflare = new Cloudflare({ apiKey: process.env.JOHNANTIBOT_API_KEY })

const res = await cloudflare.solveTurnstile({ pageUrl, siteKey, proxy, signal: AbortSignal.timeout(90_000) })
console.log(res.token) // the form field cf-turnstile-response, sent from the same proxy with res.userAgent
