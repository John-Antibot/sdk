// node examples/slider.ts <page URL> <captcha URL you loaded> <its HTML file> [its script file]
import { readFileSync } from 'node:fs'
import { DataDome, isHardblock, isPuzzleUnsupported } from '@john-antibot/sdk'

const [referer, captchaUrl, htmlFile, scriptFile] = process.argv.slice(2)
const html = readFileSync(htmlFile, 'utf8')
const script = scriptFile ? readFileSync(scriptFile, 'utf8') : undefined
const session = new DataDome({ apiKey: process.env.JOHNANTIBOT_API_KEY }).session({ referer })

try {
  const slider = await session.generateSlider({ html, captchaUrl, script })
  console.log(slider.checkUrl) // GET it, with session.userAgent
} catch (err) {
  if (isHardblock(err)) console.log('hardblock: change the proxy and the session')
  else if (isPuzzleUnsupported(err)) console.log('a puzzle captcha, not a slider')
  else throw err
}
