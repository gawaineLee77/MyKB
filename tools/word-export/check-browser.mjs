// Browser-native DOM/math/package regression; synthetic content only.
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs'
import { createRequire } from 'node:module'
import { resolve } from 'node:path'
import assert from 'node:assert/strict'
const root = process.cwd(), build = resolve(process.env.WORD_EXPORT_BUILD || '.local/word-export'), ui = `${build}/ui`
const require = createRequire(import.meta.url), { chromium } = require('playwright')
const { createServer } = await import(`${ui}/node_modules/vite/dist/node/index.js`)
mkdirSync(build, { recursive: true })
writeFileSync(`${ui}/word-check.html`, '<html><head><meta charset="UTF-8"></head><body><script type="module">import {createAnswerWord} from "/src/mindcreek/word-export.ts";import JSZip from "jszip";window.wordCheck={createAnswerWord,JSZip}</script></body></html>')
const server = await createServer({ configFile: false, root: ui, optimizeDeps: { entries: ['word-check.html'] }, server: { host: '127.0.0.1', port: 0, fs: { allow: [root] } } })
await server.listen()
const origin = `http://127.0.0.1:${server.httpServer.address().port}`
const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const page = await browser.newPage()
const requests = []
await page.route('**/*', route => {
  const url = route.request().url(); requests.push(url)
  if (!url.startsWith(origin + '/')) return route.abort()
  return route.continue()
})
try {
  await page.goto(origin + '/word-check.html'); await page.waitForFunction(() => window.wordCheck)
  const report = await page.evaluate(async content => {
    const { createAnswerWord, JSZip } = window.wordCheck
    const M = 'http://schemas.openxmlformats.org/officeDocument/2006/math', W = 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'
    const checked = [], check = (condition, label) => { if (!condition) throw new Error(label); checked.push(label) }
    async function unpack(result) {
      const zip = await JSZip.loadAsync(result.bytes)
      const docs = {}
      for (const [name, file] of Object.entries(zip.files)) {
        if (file.dir) continue
        const xml = await file.async('string'), doc = new DOMParser().parseFromString(xml, 'application/xml')
        check(!doc.querySelector('parsererror'), 'valid XML: ' + name); docs[name] = doc
      }
      return docs
    }
    const result = await createAnswerWord({ title: '检索质量评估', content, references: [{ id: 'fixture-chunk', knowledge_title: '检索评估说明', chunk_index: 1 }] })
    const docs = await unpack(result), doc = docs['word/document.xml']
    check(result.tables === 1 && doc.getElementsByTagNameNS(W, 'tc').length === 12, 'native editable table')
    check(result.equations === 11 && doc.getElementsByTagNameNS(M, 'oMath').length === 11, 'all formulas are OMML')
    for (const tag of ['f', 'sSub', 'sSup', 'rad', 'nary', 'm', 'd', 'bar']) check(doc.getElementsByTagNameNS(M, tag).length > 0, 'math object: ' + tag)
    check(doc.getElementsByTagNameNS(W, 'drawing').length === 0, 'no raster equation fallback')
    check(result.warnings.length === 0, 'sample without loss warnings')
    for (const [latex, tag] of [
      ['x_i^2', 'sSubSup'], ['\\frac{1}{\\sqrt{2}}', 'f'], ['\\sqrt[3]{x}', 'rad'], ['\\hat{x}+\\vec{v}', 'acc'],
      ['\\lim_{x\\to 0} f(x)', 'limLow'], ['\\binom{n}{k}', 'f'],
      ['\\begin{bmatrix}1&0\\\\0&1\\end{bmatrix}', 'm'],
      ['\\begin{cases}x&x>0\\\\0&x\\le0\\end{cases}', 'm'],
      ['\\begin{aligned}a&=b+c\\\\d&=e\\end{aligned}', 'm'],
      ['\\prod_{i=1}^{n}x_i', 'nary'], ['\\overline{x}', 'bar'],
    ]) {
      const formula = await createAnswerWord({ content: '$$' + latex + '$$' })
      const xml = (await unpack(formula))['word/document.xml']
      check(xml.getElementsByTagNameNS(M, tag).length > 0, latex)
    }
    const escaped = await createAnswerWord({ content: '行内 \\(x+1\\)。\n\n\\[x^2\\]\n\n`$x$` 和 \\$20。' })
    check(escaped.equations === 2, 'escaped delimiters, literal currency and code')
    const entities = await createAnswerWord({ content: '&lt;script&gt; &amp; **安全**\n\n[说明](https://example.invalid/a?x=1&y=2)' })
    const entityDoc = (await unpack(entities))['word/document.xml']
    check(entityDoc.documentElement.textContent.includes('<script> & 安全'), 'text/entity escaping')
    const html = await createAnswerWord({ content: '<table><tr><th>项目</th><th>公式</th></tr><tr><td>A</td><td>$x^2$</td></tr></table>' })
    const htmlDoc = (await unpack(html))['word/document.xml']
    check(html.tables === 1 && html.equations === 1 && htmlDoc.getElementsByTagNameNS(W, 'tc').length === 4, 'HTML table with editable equation')
    for (const content of ['$\\badcommand{x}$', '$\\phantom{x}$', '$\\cancel{x}$', '$\\includegraphics{https://example.invalid/a}$', '$$x', '\\[x', '<table><tr><td colspan="2">merged</td></tr></table>']) {
      let rejected = false
      try { await createAnswerWord({ content }) } catch { rejected = true }
      check(rejected, 'reject without download: ' + content)
    }
    return { status: 'passed', checked, equations: result.equations, tables: result.tables, bytes: Array.from(result.bytes) }
  }, readFileSync(`${root}/testdata/word-export/answer.md`, 'utf8'))
  writeFileSync(`${build}/editable-answer.docx`, Buffer.from(report.bytes)); delete report.bytes
  assert.ok(requests.every(url => url.startsWith(origin + '/')), 'no external request')
  report.externalRequests = 0; report.node = process.version
  writeFileSync(`${build}/browser-core.json`, JSON.stringify(report, null, 2) + '\n')
  console.log(JSON.stringify({ status: report.status, checks: report.checked.length, equations: report.equations, tables: report.tables }))
} finally { await browser.close(); await server.close() }
