// Focused browser regression of the production protected-image renderer.
// Synthetic image/API fixtures only; this does not certify backend authorization.
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { createRequire } from 'node:module'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'

const require = createRequire(import.meta.url)
const { chromium } = require('playwright')
const { build } = require('../../upstream/weknora/frontend/node_modules/esbuild')
const base = resolve(process.env.IMAGE_ACCESS_UI || '.local/image-access-ui')
const before = resolve(process.env.IMAGE_ACCESS_BEFORE || '.local/product-details-ui/after')
const after = resolve(process.env.IMAGE_ACCESS_AFTER || `${base}/after`)
mkdirSync(`${base}/screenshots`, { recursive: true })
const report = { scope: 'Production protected-image helper bundled for a focused browser harness; synthetic API fixtures, not deployed backend acceptance.', checks: [], screenshots: [], pageErrors: [] }
const html = `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>历史图片兼容修复 · 合成验收</title><style>
body{font:16px/1.7 system-ui,sans-serif;background:#f2f5f4;color:#213e34;margin:0;padding:48px}main{max-width:1080px;margin:auto}h1{font-size:28px}small{color:#61756c}.panels{display:grid;grid-template-columns:1fr 1fr;gap:24px}section{padding:28px;background:white;border-radius:16px;min-height:290px;border:1px solid #d9e5df}img{display:block;width:100%;max-width:420px;border-radius:8px}img[hidden]{display:none}[role=status]{display:block;color:#925a16;background:#fff5df;padding:16px;border-radius:8px}#extra{margin-top:24px}</style>
<main><small>MindCreek · 本地合成浏览器验收（非生产数据）</small><h1>历史图片与回答图片</h1><p>以下两处使用产品实际的受保护图片渲染函数；接口响应由验收脚本提供。</p><div class="panels"><section><h2>知识库文档中的图片</h2><div id="kb"></div></section><section><h2>问答内容中的图片</h2><div id="message"></div></section></div><div id="extra"></div></main><script type="module" src="/renderer.js"></script></html>`

for (const [mode, source] of [['before', before], ['after', after]]) {
  await build({
    stdin: {
      contents: `import { hydrateProtectedFileImages, protectProviderImageSrcInHTML } from './src/utils/security.ts';
const ref = 'resource://AbCdEfGhIjKlMnOpQrStUv';
window.imageTest = {
  async render(id, access, source = ref) {
    const target = document.getElementById(id);
    target.innerHTML = protectProviderImageSrcInHTML('<p><img alt="合成知识库示意图" src="' + source + '"></p>');
    await hydrateProtectedFileImages(target, access);
  },
  async start() {
    await Promise.all([
      this.render('kb', { mode:'knowledgeBase', kbId:'synthetic-kb' }),
      this.render('message', { mode:'message', sessionId:'synthetic-session', messageId:'synthetic-message' }),
    ]);
  },
}; window.imageTest.ready = true;`,
      resolveDir: source, sourcefile: 'image-acceptance.ts', loader: 'ts',
    },
    bundle: true, platform: 'browser', format: 'esm', outfile: `${base}/${mode}-renderer.js`,
  })
}
let mode = 'before'
const server = createServer((req, res) => {
  if (req.url === '/renderer.js') { res.setHeader('Content-Type', 'application/javascript'); res.end(readFileSync(`${base}/${mode}-renderer.js`)); return }
  res.setHeader('Content-Type', 'text/html'); res.end(html)
})
await new Promise(done => server.listen(0, '127.0.0.1', done))
const origin = `http://127.0.0.1:${server.address().port}`
const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const image = `<svg xmlns="http://www.w3.org/2000/svg" width="480" height="240" viewBox="0 0 480 240"><rect width="480" height="240" rx="16" fill="#e4f4eb"/><rect x="30" y="70" width="110" height="90" rx="12" fill="#2b8b64"/><rect x="185" y="70" width="110" height="90" rx="12" fill="#4876a0"/><rect x="340" y="70" width="110" height="90" rx="12" fill="#8c699a"/><path d="M140 115h45m110 0h45" stroke="#436554" stroke-width="5"/><g fill="white" font-family="sans-serif" font-size="20" text-anchor="middle"><text x="85" y="123">Document</text><text x="240" y="123">Image</text><text x="395" y="123">Answer</text></g></svg>`
const calls = []
let responseStatus = 409, holdNext = false, heldRoute, releaseHeld
const context = await browser.newContext({ viewport: { width: 1240, height: 850 } })
await context.addInitScript(() => { localStorage.setItem('weknora_token', 'synthetic-token'); localStorage.setItem('weknora_selected_tenant_id', '42') })
await context.route('**/api/**', async route => {
  const request = route.request()
  calls.push({ url: request.url(), headers: request.headers() })
  if (holdNext) {
    holdNext = false; heldRoute = route
    await new Promise(done => { releaseHeld = done })
    return route.fulfill({ contentType: 'image/svg+xml', body: image })
  }
  if (responseStatus === 200) return route.fulfill({ contentType: 'image/svg+xml', body: image, headers: { 'Cache-Control': 'no-store' } })
  return route.fulfill({ status: responseStatus, json: { error: responseStatus === 409 ? 'kb_acl.resource_binding_unavailable' : 'kb_acl.not_found' } })
})
const page = await context.newPage()
page.on('pageerror', e => report.pageErrors.push(e.message))
const visibleImages = () => page.locator('img[src^="blob:"]:visible')
const screenshot = async name => { await page.screenshot({ path: `${base}/screenshots/${name}.png`, fullPage: true }); report.screenshots.push(name + '.png') }
try {
  await page.goto(origin)
  await page.waitForFunction(() => window.imageTest?.ready)
  await page.evaluate(() => window.imageTest.start())
  assert.equal(await page.getByRole('status').count(), 2)
  await screenshot('before-unverified-history-images')
  mode = 'after'
  await page.reload()
  await page.waitForFunction(() => window.imageTest?.ready)
  await page.evaluate(() => window.imageTest.start())
  assert.equal(await page.getByRole('status').count(), 2)
  responseStatus = 200
  await page.evaluate(() => window.dispatchEvent(new Event('mindcreek-kb-permissions-changed')))
  await page.waitForFunction(() => document.querySelectorAll('img[src^="blob:"]:not([hidden])').length === 2)
  await screenshot('after-restored-document-and-answer-images')
  assert.equal(await page.getByRole('status').count(), 0)
  assert.ok(calls.some(c => c.url.includes('/knowledge-bases/synthetic-kb/files')))
  assert.ok(calls.some(c => c.url.includes('/sessions/synthetic-session/messages/synthetic-message/files')))
  assert.ok(calls.every(c => c.headers.authorization === 'Bearer synthetic-token' && c.headers['x-tenant-id'] === '42'))
  report.checks.push('409 notices preserve image anchors; permission refresh restores document and answer images using distinct authenticated scopes')
  const beforeFocus = calls.length
  await page.evaluate(() => window.dispatchEvent(new Event('focus')))
  await page.waitForFunction(() => document.querySelectorAll('img[src^="blob:"]:not([hidden])').length === 2)
  assert.equal(calls.length, beforeFocus + 2)
  report.checks.push('focus revokes cached object URLs and revalidates both current resource scopes')
  responseStatus = 404
  await page.evaluate(() => window.dispatchEvent(new Event('mindcreek-kb-permissions-changed')))
  await page.waitForFunction(() => document.querySelectorAll('img[data-protected-src]').length === 0)
  assert.equal(await visibleImages().count(), 0)
  await screenshot('after-revoked-images-removed')
  report.checks.push('permission revocation removes previously rendered images after a denied fresh request')
  responseStatus = 200
  await page.evaluate(() => window.dispatchEvent(new Event('focus')))
  holdNext = true
  await page.evaluate(() => { void window.imageTest.render('kb', { mode:'knowledgeBase', kbId:'synthetic-kb' }) })
  await page.waitForFunction(() => document.querySelector('#kb img[data-protected-src]'))
  for (let i = 0; !heldRoute && i < 100; i++) await new Promise(done => setTimeout(done, 10))
  assert.ok(heldRoute)
  responseStatus = 404
  await page.evaluate(() => window.dispatchEvent(new Event('mindcreek-kb-permissions-changed')))
  await page.waitForFunction(() => !document.querySelector('#kb img[data-protected-src]'))
  releaseHeld()
  await page.waitForTimeout(100)
  assert.equal(await visibleImages().count(), 0)
  report.checks.push('a successful in-flight response from before revocation cannot restore revoked pixels')
  responseStatus = 200
  await page.evaluate(() => window.dispatchEvent(new Event('focus')))
  await page.evaluate(() => window.imageTest.render('message', { mode:'message', sessionId:'synthetic-session', messageId:'synthetic-message' }))
  const countBeforeCredentialChange = calls.length
  holdNext = true; heldRoute = undefined
  await page.evaluate(() => { localStorage.setItem('weknora_token', 'second-synthetic-token'); void window.imageTest.render('message', { mode:'message', sessionId:'synthetic-session', messageId:'synthetic-message' }) })
  for (let i = 0; !heldRoute && i < 100; i++) await new Promise(done => setTimeout(done, 10))
  assert.ok(heldRoute)
  assert.equal(await page.locator('#message img[src^="blob:"]').count(), 0, 'Do not briefly display a previous identity’s cached pixels before new authorization')
  releaseHeld()
  await page.waitForFunction(() => !!document.querySelector('#message img[src^="blob:"]'))
  assert.equal(calls.length, countBeforeCredentialChange + 1)
  assert.equal(calls.at(-1).headers.authorization, 'Bearer second-synthetic-token')
  report.checks.push('image request cache is separated by authentication headers; a new identity never receives cached pixels before fresh authorization')
  assert.deepEqual(report.pageErrors, [])
  report.success = true
} catch (error) { report.success = false; report.error = error.stack; process.exitCode = 1 }
finally { releaseHeld?.(); await browser.close(); server.close(); writeFileSync(`${base}/report.json`, JSON.stringify(report, null, 2)); console.log(JSON.stringify(report, null, 2)) }
