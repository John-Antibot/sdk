// node examples/challenge.ts <URL> <proxy>
import { Cloudflare, isDeclined } from '@john-antibot/sdk'

const [url, proxy] = process.argv.slice(2)
const cloudflare = new Cloudflare({ apiKey: process.env.JOHNANTIBOT_API_KEY })

for (let attempt = 1; ; attempt++) {
  try {
    const res = await cloudflare.solveChallenge({ url, proxy, timezone: 'America/New_York', signal: AbortSignal.timeout(90_000) })
    console.log('cf_clearance', res.clearance, 'from', res.exitIp)
    console.log('user-agent', res.userAgent) // send it, and res.cookies, from the same proxy
    break
  } catch (err) {
    if (!isDeclined(err) || attempt === 3) throw err
  }
}
