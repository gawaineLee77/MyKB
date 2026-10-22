// Real browser DOM, KaTeX MathML and createAnswerWord; synthetic fixtures only.
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs'
import { createRequire } from 'node:module'
import { resolve } from 'node:path'
import assert from 'node:assert/strict'

const root = process.cwd(), build = resolve(process.env.WORD_EXPORT_BUILD || '.local/word-arrow-20261022'), ui = `${build}/ui`
const { chromium } = createRequire(import.meta.url)('playwright')
const { createServer } = await import(`${ui}/node_modules/vite/dist/node/index.js`)
mkdirSync(`${build}/screenshots`, { recursive: true })
const originalExport = readFileSync(`${ui}/src/mindcreek/word-export.ts`, 'utf8')
assert.ok(originalExport.includes("from './word-math'"), 'Expected unmodified export entry import')
writeFileSync(`${ui}/src/mindcreek/word-arrow-before-math.ts`, readFileSync(`${build}/before-word-math.ts`, 'utf8'))
writeFileSync(`${ui}/src/mindcreek/word-arrow-before-export.ts`, originalExport.replace("from './word-math'", "from './word-arrow-before-math'"))
writeFileSync(`${ui}/word-arrow-check.html`, `<!doctype html><html lang="zh-CN"><head><meta charset="UTF-8"><title>带标注箭头 · Word 导出验证</title><style>
body{font:17px/1.65 system-ui,sans-serif;margin:48px auto;max-width:1120px;color:#17352c;background:#fafcfb}h1{font-size:27px}p{color:#50645b}.panel{background:white;border:1px solid #d6e3db;border-radius:12px;padding:24px;margin:20px 0}pre{white-space:pre-wrap;word-break:break-word;font:15px/1.7 monospace}.status{border-left:5px solid #159965}.status.error{border-color:#c14a4a}#preview .katex-display{margin:1.3em 0}.hint{font-size:14px;color:#62796d}</style></head><body>
<h1>带中文标注箭头 · Word 导出验证</h1><p>本机浏览器使用真实导出函数；以下均为合成测试内容。</p>
<section class="panel"><h2 id="mode"></h2><div id="preview"></div><pre id="latex"></pre></section>
<section id="status" class="panel status"><h2 id="result-title">等待验证</h2><pre id="result"></pre></section><p class="hint">截图展示浏览器导出结果；Word 内排版另行验证。</p>
<script type="module">import {createAnswerWord as before} from '/src/mindcreek/word-arrow-before-export.ts';import {createAnswerWord as after} from '/src/mindcreek/word-export.ts';import JSZip from 'jszip';import katex from 'katex';import 'katex/dist/katex.min.css';window.arrowCheck={before,after,JSZip,katex};</script></body></html>`)
const server = await createServer({ configFile: false, root: ui, optimizeDeps: { entries: ['word-arrow-check.html'] }, server: { host: '127.0.0.1', port: 0, fs: { allow: [root] } } })
await server.listen()
const origin = `http://127.0.0.1:${server.httpServer.address().port}`
const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const page = await browser.newPage({ viewport: { width: 1280, height: 1000 } })
const requests = [], errors = [], report = { status: 'running', checks: [], screenshots: [], node: process.version }
page.on('pageerror', error => errors.push(error.message))
await page.route('**/*', route => {
  const url = route.request().url(); requests.push(url)
  return url.startsWith(origin + '/') ? route.continue() : route.abort()
})
const sample = readFileSync(`${root}/testdata/word-export/annotated-arrows.md`, 'utf8')
const sampleLatex = String.raw`\sigma \xrightarrow{\text{范特西-泽尼克}} \gamma(\Delta x) \xrightarrow{\text{Gamo矩阵}} \{\lambda\}`
const dualLatex = String.raw`\sigma \xrightarrow[\text{这是更长的下方条件标签}]{\text{上方}} \gamma`
const show = (mode, formulas, title, result, error = false) => page.evaluate(({ mode, formulas, title, result, error }) => {
  document.getElementById('mode').textContent = mode
  document.getElementById('latex').textContent = formulas.join('\n\n')
  document.getElementById('preview').innerHTML = formulas.map(latex => window.arrowCheck.katex.renderToString(latex, { displayMode: true, throwOnError: true, trust: false })).join('')
  document.getElementById('result-title').textContent = title
  document.getElementById('result').textContent = result
  document.getElementById('status').classList.toggle('error', error)
}, { mode, formulas, title, result, error })
const screenshot = async name => { await page.screenshot({ path: `${build}/screenshots/${name}.png`, fullPage: true, animations: 'disabled' }); report.screenshots.push(`${name}.png`) }

try {
  await page.goto(origin + '/word-arrow-check.html'); await page.waitForFunction(() => window.arrowCheck)
  const beforeError = await page.evaluate(async content => {
    try { await window.arrowCheck.before({ content }); return '' } catch (error) { return error.message }
  }, sample)
  assert.match(beforeError, /第 10 个公式无法导出/)
  assert.match(beforeError, /mpadded/)
  report.beforeError = beforeError
  await show('修复前', [sampleLatex], '导出失败：已复现第十个公式', beforeError, true)
  await screenshot('before-annotated-arrow-error')

  const output = await page.evaluate(async ({ content, sampleLatex, dualLatex }) => {
    const { after: createAnswerWord, JSZip } = window.arrowCheck
    const M = 'http://schemas.openxmlformats.org/officeDocument/2006/math', W = 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'
    const checks = [], check = (ok, label) => { if (!ok) throw new Error(label); checks.push(label) }
    const elements = (node, tag) => [...node.getElementsByTagNameNS(M, tag)]
    const direct = (node, tag) => [...node.children].find(child => child.namespaceURI === M && child.localName === tag)
    const text = node => node ? elements(node, 't').map(item => item.textContent).join('') : ''
    const arrows = doc => [...elements(doc, 'chr').map(item => item.getAttributeNS(M, 'val') || ''), ...elements(doc, 't').map(item => item.textContent)].join('').match(/[→←↔]/g) || []
    const hasLabelAt = (doc, label, position) => elements(doc, position === 'above' ? 'limUpp' : 'limLow').some(node => text(direct(node, 'lim')).includes(label)) || elements(doc, 'groupChr').some(node => {
      const properties = direct(node, 'groupChrPr'), pos = properties && direct(properties, 'pos')?.getAttributeNS(M, 'val')
      return pos === (position === 'above' ? 'bot' : 'top') && text(direct(node, 'e')).includes(label)
    })
    async function unpack(result, label) {
      const zip = await JSZip.loadAsync(result.bytes), docs = {}
      check(!Object.keys(zip.files).some(name => /(?:^|\/)media\//.test(name) || /\.(?:png|jpe?g|gif|svg|webp)$/i.test(name)), `${label}: no images in DOCX package`)
      for (const [name, file] of Object.entries(zip.files)) {
        if (file.dir) continue
        const xml = await file.async('string'), doc = new DOMParser().parseFromString(xml, 'application/xml')
        check(!doc.querySelector('parsererror'), `${label}: valid XML ${name}`)
        docs[name] = doc
      }
      const doc = docs['word/document.xml']
      check(doc.getElementsByTagNameNS(W, 'drawing').length === 0 && doc.getElementsByTagNameNS(W, 'pict').length === 0, `${label}: editable math, no raster fallback`)
      check(![...docs['word/_rels/document.xml.rels'].documentElement.children].some(node => node.getAttribute('TargetMode') === 'External'), `${label}: no external relationship`)
      return doc
    }
    const result = await createAnswerWord({ title: '带中文标注箭头验证', content })
    const doc = await unpack(result, 'ten-formula sample')
    check(result.equations === 10 && elements(doc, 'oMath').length === 10, 'all ten formulas exported as native OMML')
    check(result.tables === 1 && doc.getElementsByTagNameNS(W, 'tbl').length === 1, 'sample retains editable table')
    const tenth = elements(doc, 'oMath')[9]
    check(text(tenth).includes('σ') && text(tenth).includes('γ') && text(tenth).includes('λ'), 'tenth formula retains sigma, gamma and lambda')
    check(hasLabelAt(tenth, '范特西-泽尼克', 'above') && hasLabelAt(tenth, 'Gamo矩阵', 'above'), 'long Chinese and mixed-language labels remain above their arrows')
    check(arrows(tenth).join('') === '→→', 'tenth formula keeps both right arrows')
    check(result.warnings.length === 0, 'ten-formula sample exports without loss warnings')

    const cases = []
    for (const [command, arrow] of [['xrightarrow', '→'], ['xleftarrow', '←'], ['xleftrightarrow', '↔']]) {
      cases.push({ name: command + '-above', latex: `a \\${command}{\\text{上方甲}} b`, arrow, above: '上方甲' })
      cases.push({ name: command + '-below', latex: `a \\${command}[\\text{下方乙}]{} b`, arrow, below: '下方乙' })
      cases.push({ name: command + '-both-long-below', latex: `a \\${command}[\\text{这是更长的下方条件标签}]{\\text{上方丙}} b`, arrow, above: '上方丙', below: '这是更长的下方条件标签' })
      cases.push({ name: command + '-empty', latex: `a \\${command}{} b`, arrow })
      cases.push({ name: command + '-empty-both', latex: `a \\${command}[]{} b`, arrow })
    }
    cases.push({ name: 'structured-labels', latex: String.raw`a \xrightarrow[x_i]{\frac{p}{q}} b`, arrow: '→', tags: ['f', 'sSub'] })
    cases.push({ name: 'nested-arrow-label', latex: String.raw`a \xrightarrow{b \xleftarrow{c} d} e`, arrow: '→←' })
    for (const item of cases) {
      const exported = await createAnswerWord({ content: '$$' + item.latex + '$$' }), xml = await unpack(exported, item.name)
      check(exported.equations === 1 && elements(xml, 'oMath').length === 1, `${item.name}: one editable formula`)
      check(arrows(xml).sort().join('') === [...item.arrow].sort().join(''), `${item.name}: preserves arrow directions and count`)
      if (item.above) check(hasLabelAt(xml, item.above, 'above'), `${item.name}: upper label position`)
      if (item.below) check(hasLabelAt(xml, item.below, 'below'), `${item.name}: lower label position`)
      for (const tag of item.tags || []) check(elements(xml, tag).length > 0, `${item.name}: retains editable ${tag}`)
      check(exported.warnings.length === 0, `${item.name}: no silent loss warning`)
    }
    const contexts = '行内：$a \\xrightarrow{\\text{行内标签}} b$。\n\n$$' + dualLatex + '$$\n\n| 场景 | 公式 |\n| --- | --- |\n| 表格 | $a \\xleftrightarrow[\\text{表格下方}]{\\text{表格上方}} b$ |\n'
    const contextResult = await createAnswerWord({ title: '行内、块与表格箭头', content: contexts }), contextDoc = await unpack(contextResult, 'inline-block-table')
    check(contextResult.equations === 3 && elements(contextDoc, 'oMath').length === 3, 'inline, display and table arrows all editable')
    check(elements(contextDoc, 'oMathPara').length === 1, 'display arrow retains block math wrapper')
    check([...contextDoc.getElementsByTagNameNS(W, 'tc')].some(cell => elements(cell, 'oMath').length === 1), 'table cell contains native editable arrow formula')
    check(hasLabelAt(contextDoc, '这是更长的下方条件标签', 'below') && hasLabelAt(contextDoc, '上方', 'above'), 'long lower label is not moved above the arrow')
    const rejected = []
    for (const latex of [String.raw`\mathllap{x}`, String.raw`\mathrlap{x}`, String.raw`\mathclap{x}`, String.raw`\raisebox{1em}{x}`, String.raw`\phantom{x}`, String.raw`a \xrightarrow{\mathllap{x}} b`, String.raw`a \xrightarrow{\raisebox{1em}{x}} b`, String.raw`\gama`, String.raw`\sigma \xrightarrow{\text{合法标签}} \gama`]) {
      let message = ''
      try { await createAnswerWord({ content: '$$' + latex + '$$' }) } catch (error) { message = error.message }
      check(message.includes('第 1 个公式无法导出'), 'unsupported formula rejected without DOCX: ' + latex)
      if (latex.includes('\\gama')) check(message.includes('gama'), 'unknown gama command is not silently corrected')
      rejected.push({ latex, message })
    }
    return { checks, cases: cases.map(({ name, latex }) => ({ name, latex })), rejected, equations: result.equations, tables: result.tables, sampleBytes: [...result.bytes], contextBytes: [...contextResult.bytes] }
  }, { content: sample, sampleLatex, dualLatex })
  writeFileSync(`${build}/annotated-arrows.docx`, Buffer.from(output.sampleBytes)); delete output.sampleBytes
  writeFileSync(`${build}/arrow-contexts.docx`, Buffer.from(output.contextBytes)); delete output.contextBytes
  Object.assign(report, output)
  await show('修复后', [sampleLatex, dualLatex], '导出成功：10 个可编辑公式、1 个表格', '已解包核对 OMML、箭头方向与上下标签。\n行内、块公式、表格和嵌套标注均通过。\n不支持的布局与错误命令仍明确拒绝。')
  await page.waitForLoadState('networkidle')
  await screenshot('after-annotated-arrow-export')
  assert.ok(requests.every(url => url.startsWith(origin + '/')), 'no external network request')
  assert.deepEqual(errors, [], 'no browser page errors')
  report.externalRequests = 0; report.pageErrors = errors; report.status = 'passed'
} catch (error) {
  report.status = 'failed'; report.error = error.stack; process.exitCode = 1
} finally {
  await browser.close(); await server.close()
  writeFileSync(`${build}/arrow-browser.json`, JSON.stringify(report, null, 2) + '\n')
  console.log(JSON.stringify({ status: report.status, checks: report.checks.length, equations: report.equations, tables: report.tables, error: report.error }, null, 2))
}
