// node examples/tags.ts <page URL> <datadome cookie> <site key> <font families, comma-separated>
import { DataDome } from '@john-antibot/sdk'

const [referer, cookie, siteKey, fonts = ''] = process.argv.slice(2)
const datadome = new DataDome({ apiKey: process.env.JOHNANTIBOT_API_KEY })
const session = datadome.session({ referer })

const tags = await session.generateTags({ event: 'ch', cookie, siteKey, loadedFontFamilies: fonts.split(',').filter(Boolean) })
console.log(tags.payload) // POST it to the site's /js/, with session.userAgent; then the same with event 'le'
