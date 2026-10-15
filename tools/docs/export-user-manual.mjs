// Export the repository manual as a standalone, offline HTML reader.
// Node 24; uses the pinned frontend Marked dependency. No network requests.
import { readFileSync, writeFileSync, mkdirSync, existsSync, copyFileSync } from 'node:fs'
import { resolve, dirname, relative, basename } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const input = resolve(root, 'docs/guides/MINDCREEK_USER_MANUAL_ZH.md')
const output = resolve(process.argv[2] || resolve(root, 'images/archives/mindcreek-user-manual-20261010'))
const { Marked } = await import(pathToFileURL(resolve(root, 'upstream/weknora/frontend/node_modules/marked/lib/marked.esm.js')).href)
const source = readFileSync(input, 'utf8')
const sourceCommit = /对应源码 `([a-f0-9]+)`/.exec(source)?.[1]
if (!sourceCommit) throw new Error('Missing versioned source reference')
const upstreamCommit = /160000 commit ([a-f0-9]+)/.exec(execFileSync('git', ['ls-tree', sourceCommit, 'upstream/weknora'], { cwd: root, encoding: 'utf8' }))?.[1]
if (!upstreamCommit) throw new Error('Missing pinned upstream reference')
const escape = value => String(value).replace(/[&<>"']/g, ch => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]))
const files = new Set([input])
const toc = []
const counts = new Map()
function localFile(href) {
  const path = resolve(dirname(input), decodeURIComponent(href.split('#')[0]))
  if (!path.startsWith(root + '/') || !existsSync(path)) throw new Error('Missing local reference: ' + href)
  files.add(path)
  return path
}
const marked = new Marked({ gfm: true, renderer: {
  heading({ tokens, depth }) {
    const text = this.parser.parseInline(tokens)
    const slug = text.replace(/<[^>]*>/g, '').toLowerCase().replace(/[^\p{L}\p{N}\s_-]/gu, '').trim().replace(/\s/g, '-')
    const n = counts.get(slug) || 0
    counts.set(slug, n + 1)
    const id = slug + (n ? `-${n}` : '')
    if (depth === 2) toc.push({ id, text })
    return `<h${depth} id="${escape(id)}">${text}</h${depth}>\n`
  },
  image({ href, text }) {
    const path = localFile(href)
    if (!path.endsWith('.png')) throw new Error('Expected an existing PNG screenshot')
    const encoded = readFileSync(path).toString('base64')
    return `<figure><button class="image-open" type="button" aria-label="放大${escape(text)}"><img src="data:image/png;base64,${encoded}" alt="${escape(text)}" loading="lazy"></button></figure>`
  },
  link({ href, tokens }) {
    const label = this.parser.parseInline(tokens)
    if (/^https:\/\//.test(href)) return `<a href="${escape(href)}" target="_blank" rel="noreferrer">${label}</a>`
    if (href.startsWith('#')) return `<a href="${escape(href)}">${label}</a>`
    const path = localFile(href)
    if (relative(root, path).startsWith('docs/examples/user-manual/')) {
      return `<a download="${escape(basename(path))}" href="data:application/octet-stream;base64,${readFileSync(path).toString('base64')}">${label} ↓</a>`
    }
    const relativePath = relative(root, path)
    const isUpstream = relativePath.startsWith('upstream/weknora/')
    const remoteRoot = isUpstream
      ? 'https://github.com/Tencent/WeKnora/blob/' + upstreamCommit + '/'
      : 'https://github.com/gawaineLee77/MyKB/blob/' + sourceCommit + '/'
    const remotePath = isUpstream ? relativePath.slice('upstream/weknora/'.length) : relativePath
    const remote = remoteRoot + remotePath.split('/').map(encodeURIComponent).join('/')
    return `<a href="${remote}" target="_blank" rel="noreferrer">${label}</a>`
  },
  code({ text, lang }) {
    return `<div class="codebox"><div class="codebar"><span>${escape(lang || 'text')}</span><button class="copy" type="button">复制</button></div><pre><code>${escape(text)}</code></pre></div>\n`
  }
} })
let body = marked.parse(source).replace(/<p>(<figure>[\s\S]*?<\/figure>)<\/p>/g, '$1')
const tocHTML = toc.map(({ id, text }) => `<a href="#${escape(id)}">${text}</a>`).join('\n')
const html = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>MindCreek 使用手册</title>
<style>
:root{color-scheme:light;--ink:#172e2b;--muted:#576d66;--line:#dae5df;--green:#0b7458;--paper:#fff;--wash:#f3f7f4}*{box-sizing:border-box}
html{scroll-behavior:smooth;scroll-padding-top:28px}body{margin:0;background:var(--wash);color:var(--ink);font:16px/1.85 -apple-system,BlinkMacSystemFont,"PingFang SC","Microsoft YaHei",sans-serif}
aside{position:fixed;inset:0 auto 0 0;width:260px;padding:32px 22px;overflow:auto;background:#e9f2ed;border-right:1px solid var(--line)}.brand{font-size:25px;font-weight:750;letter-spacing:-.5px}.kicker{font-size:12px;color:var(--green);letter-spacing:2px;margin:8px 0 24px}.toc a{display:block;text-decoration:none;color:#36534a;font-size:14px;line-height:1.6;padding:8px 10px;border-radius:6px}.toc a:hover,.toc a:focus{background:#d5e7dd;color:#034b38}.tools{display:flex;gap:7px;margin:24px 0}.tools button,.copy,dialog button{font:inherit;font-size:12px;cursor:pointer;border:1px solid #bed2c7;border-radius:5px;background:white;color:var(--green);padding:5px 10px}.tip{font-size:12px;color:var(--muted);line-height:1.6}
main{margin-left:260px;padding:48px 6vw 80px;max-width:1350px}article{background:var(--paper);padding:50px 58px;box-shadow:0 6px 36px #16362909;border:1px solid var(--line);border-radius:12px}.meta-label{font-size:12px;letter-spacing:2px;color:var(--green);font-weight:700}h1{font-size:42px;line-height:1.2;letter-spacing:-1px;margin:15px 0 24px}h2{font-size:27px;margin:58px 0 24px;padding-top:24px;border-top:2px solid var(--green);line-height:1.4}h3{font-size:21px;line-height:1.5;margin:34px 0 14px}p{margin:12px 0}a{color:var(--green);text-underline-offset:3px}li{padding-left:3px;margin:6px 0}ol,ul{padding-left:26px}strong{font-weight:650}code{font:13px/1.6 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;background:#eef4f0;border-radius:3px;padding:2px 4px;overflow-wrap:anywhere}
.table-wrap{overflow-x:auto;margin:20px 0;border:1px solid var(--line);border-radius:7px}table{width:100%;border-collapse:collapse;font-size:14px;line-height:1.65}th{background:#eaf3ed;text-align:left;font-weight:650}td,th{padding:11px 13px;vertical-align:top;border-bottom:1px solid var(--line);min-width:85px}tr:last-child td{border-bottom:0}tr:nth-child(even) td{background:#fafcfb}figure{margin:24px 0 12px}figure img{display:block;width:100%;height:auto}.image-open{display:block;padding:0;width:100%;background:white;border:1px solid var(--line);border-radius:7px;overflow:hidden;cursor:zoom-in}.codebox{margin:18px 0;border:1px solid #cdded4;border-radius:7px;overflow:hidden}.codebar{background:#e8f0eb;display:flex;align-items:center;justify-content:space-between;padding:5px 12px;font-size:12px;color:#496558}pre{background:#f7faf8;margin:0;padding:17px 18px;overflow-x:auto;line-height:1.6}pre code{background:none;padding:0;overflow-wrap:normal;font-size:12px}blockquote{border-left:3px solid var(--green);padding:8px 20px;background:var(--wash);margin:18px 0}dialog{border:0;border-radius:10px;padding:16px;max-width:96vw;max-height:95vh}dialog::backdrop{background:#102b23bd}dialog img{display:block;max-width:91vw;max-height:82vh;object-fit:contain}dialog .close{float:right;margin-bottom:8px}.footer{font-size:12px;color:var(--muted);margin-top:35px;border-top:1px solid var(--line);padding-top:18px}
@media(max-width:1000px){aside{width:220px}main{margin-left:220px;padding:25px}article{padding:35px 30px}}
@media(max-width:720px){aside{position:static;width:auto;padding:22px}.brand{font-size:22px}.kicker{margin-bottom:10px}.toc{display:grid;grid-template-columns:1fr 1fr}.toc a{font-size:12px;padding:5px}.tools{margin:12px 0}.tip{display:none}main{margin:0;padding:12px}article{padding:24px 18px}h1{font-size:32px}h2{font-size:24px}h3{font-size:19px}body{font-size:15px}td,th{min-width:110px;padding:9px}}
@media print{@page{size:A4;margin:16mm 14mm}body{background:white;font-size:10pt;line-height:1.6}aside,dialog,.codebar button,.meta-label{display:none!important}main{margin:0;padding:0;max-width:none}article{padding:0;border:0;box-shadow:none}h1{font-size:28pt}h2{font-size:18pt;break-before:page;margin:0 0 15px;padding-top:12px}h3{font-size:13pt;break-after:avoid}p,li{orphans:3;widows:3}table{font-size:8pt}td,th{min-width:0;padding:7px}thead{display:table-header-group}tr{break-inside:avoid}.table-wrap{overflow:visible}pre{white-space:pre-wrap;overflow-wrap:anywhere;overflow:visible}pre code{font-size:8pt}figure{break-inside:avoid}figure img{max-height:155mm;object-fit:contain}a{color:inherit}a::after{content:none}.codebox{break-inside:avoid}.footer{margin-top:16px}}
</style></head><body>
<aside><div class="brand">MindCreek</div><div class="kicker">使用手册 · 中文 1.0</div><nav class="toc" aria-label="章节目录">${tocHTML}</nav><div class="tools"><button id="print" type="button">打印 / 保存 PDF</button></div><p class="tip">正文和 9 张示例图可离线阅读。点击截图可放大。配套脚本可直接下载。扩展参考链接需访问仓库或官方站点。</p></aside>
<main><article><div class="meta-label">企业知识工作指南</div>${body}<p class="footer">本手册采用 STE 写作原则的中文表达。合成示例不适用于真实设备操作。文档中的 Key 和令牌均为占位说明。</p></article></main>
<dialog id="zoom"><button class="close" type="button">关闭</button><img alt="放大的示例截图"></dialog>
<script>
document.querySelectorAll('table').forEach(table=>{const wrap=document.createElement('div');wrap.className='table-wrap';table.replaceWith(wrap);wrap.append(table)});
document.querySelector('#print').addEventListener('click',()=>window.print());
document.querySelectorAll('.copy').forEach(button=>button.addEventListener('click',async()=>{const code=button.closest('.codebox').querySelector('code');try{await navigator.clipboard.writeText(code.textContent);button.textContent='已复制';setTimeout(()=>button.textContent='复制',1800)}catch{const range=document.createRange();range.selectNodeContents(code);const selection=window.getSelection();selection.removeAllRanges();selection.addRange(range);button.textContent='已选中，请复制'}}));
const dialog=document.querySelector('#zoom');document.querySelectorAll('.image-open').forEach(button=>button.addEventListener('click',()=>{dialog.querySelector('img').src=button.querySelector('img').src;dialog.querySelector('img').alt=button.querySelector('img').alt;dialog.showModal()}));dialog.querySelector('.close').addEventListener('click',()=>dialog.close());dialog.addEventListener('click',event=>{if(event.target===dialog)dialog.close()});
</script></body></html>`
mkdirSync(output, { recursive: true })
writeFileSync(resolve(output, 'MindCreek-使用手册.html'), html)
mkdirSync(resolve(output, 'examples'), { recursive: true })
for (const name of ['mcp.sh', 'api.sh', 'pump-maintenance.md']) copyFileSync(resolve(root, 'docs/examples/user-manual', name), resolve(output, 'examples', name))
writeFileSync(resolve(output, 'README.txt'), '打开 MindCreek-使用手册.html 阅读图文手册。正文和截图无需联网。\nexamples 目录包含 MCP、API 示例脚本和合成练习资料。先阅读第 8 节再执行。\n脚本仅供明确配置后调用企业服务，不在打开手册时自动运行。\n扩展参考指向对应版本的仓库或标准官方网站，需要网络访问。\n')
const digest = p => createHash('sha256').update(readFileSync(p)).digest('hex')
const manifest = { date: '2026-10-10', title: 'MindCreek 使用手册', source_commit: sourceCommit, upstream_commit: upstreamCommit, screenshots: [...files].filter(p => p.endsWith('.png')).length,
  source_sha256: digest(input), html_sha256: digest(resolve(output, 'MindCreek-使用手册.html')),
  inputs: Object.fromEntries([...files].map(p => [relative(root, p), digest(p)])) }
writeFileSync(resolve(output, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n')
console.log(JSON.stringify({ output, screenshots: manifest.screenshots, sections: toc.length, bytes: Buffer.byteLength(html) }))
