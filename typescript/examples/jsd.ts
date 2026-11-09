// node examples/jsd.ts <URL> <proxy>
import { Cloudflare } from '@john-antibot/sdk'

const [url, proxy] = process.argv.slice(2)
const cloudflare = new Cloudflare({ apiKey: process.env.JOHNANTIBOT_API_KEY })

const res = await cloudflare.solveJsd({ url, proxy, signal: AbortSignal.timeout(90_000) })
console.log('cf_clearance', res.clearance, 'from', res.exitIp, 'for', res.userAgent)
