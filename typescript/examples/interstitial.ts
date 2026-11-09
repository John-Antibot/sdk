// node examples/interstitial.ts <page URL> <interstitial URL you loaded> <its HTML file> [its script file]
import { readFileSync } from 'node:fs'
import { DataDome } from '@john-antibot/sdk'

const [referer, deviceLink, htmlFile, scriptFile] = process.argv.slice(2)
const html = readFileSync(htmlFile, 'utf8')
const script = scriptFile ? readFileSync(scriptFile, 'utf8') : undefined // the source of challengeScriptUrl(html)
const session = new DataDome({ apiKey: process.env.JOHNANTIBOT_API_KEY }).session({ referer })

const inter = await session.generateInterstitial({ html, deviceLink, script })
console.log(inter.formBody) // POST it to https://geo.captcha-delivery.com/interstitial/, with session.userAgent
