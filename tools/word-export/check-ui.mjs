// Production Vue build, synthetic authenticated chat fixtures, real downloads.
import { createServer } from 'node:http'
import { readFileSync, writeFileSync, existsSync, statSync, mkdirSync } from 'node:fs'
import { resolve, extname } from 'node:path'
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'
const root = process.cwd(), build = `${root}/.local/word-export`, output = `${root}/docs/assets/word-export`
mkdirSync(output, { recursive: true })
const { chromium } = createRequire(import.meta.url)('playwright')
let directory = `${build}/ui/dist`
const server = createServer((req, res) => {
  let file = resolve(directory, '.' + new URL(req.url, 'http://localhost').pathname)
  if (!file.startsWith(directory + '/') || !existsSync(file) || !statSync(file).isFile()) file = `${directory}/index.html`
  res.setHeader('Content-Type', ({ '.html': 'text/html', '.js': 'application/javascript', '.css': 'text/css', '.json': 'application/json', '.png': 'image/png', '.svg': 'image/svg+xml', '.woff2': 'font/woff2' })[extname(file)] || 'application/octet-stream')
  res.end(readFileSync(file))
})
await new Promise(done => server.listen(0, '127.0.0.1', done))
const origin = `http://127.0.0.1:${server.address().port}`
const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const report = { status: 'running', node: process.version, checks: [], screenshots: [] }
const content = String.raw`### 方案 A 的评估结果

**F1 约为 0.847**，准确率和召回率较为均衡。

| 指标 | 公式 | 数值 |
| :--- | :---: | ---: |
| 准确率 | $P=\frac{TP}{TP+FP}$ | 0.90 |
| 召回率 | $R=\frac{TP}{TP+FN}$ | 0.80 |

$$F_1=\frac{2PR}{P+R}=0.847$$

以上为合成数据，仅用于验证导出。`
async function scenario(role, agent = false, before = false, invalid = false, legacy = false) {
  directory = before ? resolve(process.env.WORD_EXPORT_BEFORE_DIST || `${root}/.local/graph-view/ui/dist`) : `${build}/ui/dist`
  assert.ok(existsSync(`${directory}/index.html`), 'before/after production build exists')
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1050 }, acceptDownloads: true })
  const user = { id: 'fixture-user', username: '测试成员', email: 'fixture@example.invalid', tenant_id: 42, is_active: true, is_system_admin: role === 'owner', preferences: {} }
  const tenant = { id: 42, name: '产品研发空间', status: 'active' }, memberships = [{ tenant_id: 42, tenant_name: tenant.name, role }]
  await ctx.addInitScript(({ user, tenant, memberships, agent }) => {
    for (const [key, value] of Object.entries({ weknora_token: 'synthetic-ui-token', mindcreek_auth_kind: 'local-admin', weknora_selected_tenant_id: '42', locale: 'zh-CN', 'weknora:new-user-guide-done:v1': '1' })) localStorage.setItem(key, value)
    localStorage.setItem('weknora_user', JSON.stringify(user)); localStorage.setItem('weknora_tenant', JSON.stringify(tenant)); localStorage.setItem('weknora_memberships', JSON.stringify(memberships))
    localStorage.setItem('WeKnora_settings', JSON.stringify({ selectedAgentId: agent ? 'builtin-smart-reasoning' : 'builtin-quick-answer', isAgentEnabled: agent }))
    for (const key of ['kb-list:v2','kb-create:v3','tenant-models:v1','kb-detail:v1','chat:v1']) localStorage.setItem('weknora:contextual-guide-' + key, '1')
  }, { user, tenant, memberships, agent })
  const session = { id: 'fixture-session', title: '方案 A 的检索质量', tenant_id: 42, created_at: '2026-10-08T08:00:00Z', is_pinned: false, last_request_state: { agent_id: agent ? 'builtin-smart-reasoning' : 'builtin-quick-answer', agent_enabled: agent } }
  const answer = { id: 'fixture-answer', role: 'assistant', content: invalid ? '$\\unsupported{x}$' : content, is_completed: true, created_at: '2026-10-08T08:00:01Z', knowledge_references: [], agent_steps: agent && !legacy ? [{ iteration: 1, thought: '', timestamp: '2026-10-08T08:00:01Z' }] : [] }
  let apiCalls = 0
  await ctx.route('**/*', route => {
    const u = new URL(route.request().url()), path = u.pathname
    assert.equal(u.origin, origin, 'no external network request')
    const json = data => route.fulfill({ json: data })
    if (!path.startsWith('/api/')) return route.continue()
    apiCalls++
    if (path === '/api/v1/mindcreek/auth/config') return json({ enterprise_enabled: true })
    if (path === '/api/v1/auth/me') return json({ success: true, data: { user, tenant, memberships, active_tenant: tenant, capabilities: {}, mindcreek: { local_admin: true, installation_ready: true } } })
    if (path === '/api/v1/auth/oidc/config') return json({ success: true, data: { enabled: true } })
    if (path === '/api/v1/auth/validate') return json({ success: true, valid: true })
    if (path === '/api/v1/sessions') return json({ success: true, data: [session], total: 1 })
    if (path === '/api/v1/sessions/fixture-session') return json({ success: true, data: session })
    if (path === '/api/v1/messages/fixture-session/load') return json({ success: true, data: [{ id: 'fixture-question', role: 'user', content: '计算方案 A 的 F1 并解释结果', is_completed: true, created_at: '2026-10-08T08:00:00Z' }, answer] })
    if (path.includes('suggestions')) return json({ success: true, data: { status: 'ready', suggestions: [] } })
    if (path === '/api/v1/system/info') return json({ success: true, data: { version: 'v0.8.0', edition: 'standard' } })
    if (path === '/api/v1/system/deployment-capabilities') return json({ success: true, data: { edition: 'standard', capabilities: {} } })
    if (path === '/api/v1/models') return json({ success: true, data: [{ id: 'builtin-mindcreek-chat', name: '默认对话模型', type: 'KnowledgeQA', status: 'ready', is_builtin: true, parameters: {} }] })
    if (path.includes('invitations')) return json({ success: true, data: { invitations: [] } })
    return json({ success: true, data: [], total: 0 })
  })
  const page = await ctx.newPage(), errors = [], downloads = []
  page.setDefaultTimeout(20000); page.on('pageerror', e => errors.push(e.message)); page.on('download', d => downloads.push(d))
  try {
    await page.goto(origin + '/platform/chat/fixture-session')
    await page.locator('.answer-toolbar').first().waitFor(); await page.waitForLoadState('networkidle')
    assert.equal(await page.locator('.agent-stream-display').count(), legacy ? 0 : 1, 'the expected native answer renderer is active')
    assert.equal(await page.locator('.agent-stream-display.is-rag-mode').count(), agent ? 0 : 1, 'RAG and agent pipeline flags')
    async function shot(name) { await page.screenshot({ path: `${output}/${name}.png`, fullPage: true, animations: 'disabled' }); report.screenshots.push(`docs/assets/word-export/${name}.png`) }
    if (before) { assert.equal(await page.getByRole('button', { name: '导出 Word', exact: true }).count(), 0); await shot('before-chat') }
    else {
      const button = page.getByRole('button', { name: '导出 Word', exact: true })
      await button.scrollIntoViewIfNeeded()
      if (role === 'viewer' && !invalid && !legacy) await shot(agent ? 'after-agent' : 'after-chat')
      const calls = apiCalls
      if (invalid) {
        await button.click(); await page.getByText(/第 1 个公式无法导出/).waitFor(); assert.equal(downloads.length, 0); await shot('unsupported-formula')
      } else {
        const pending = page.waitForEvent('download'); await button.click(); const download = await pending
        assert.match(download.suggestedFilename(), /^MindCreek-.*\.docx$/)
        await download.saveAs(`${build}/${agent ? 'agent' : role}-answer.docx`)
        await page.getByText('Word 已导出，表格和支持的公式可编辑。', { exact: true }).waitFor()
      }
      assert.equal(apiCalls, calls, 'export makes no business API request')
    }
    assert.deepEqual(errors, [], 'Vue runtime errors')
    report.checks.push({ role, agent, before, invalid, legacy, status: 'passed' })
  } finally { await ctx.close() }
}
try {
  await scenario('viewer', false, true)
  for (const role of ['owner', 'admin', 'contributor', 'viewer']) await scenario(role)
  await scenario('viewer', true)
  await scenario('viewer', true, false, false, true)
  await scenario('viewer', false, false, true)
  report.status = 'passed'
} catch (error) { report.status = 'failed'; report.error = String(error); throw error }
finally { await browser.close(); server.close(); writeFileSync(`${build}/browser-ui.json`, JSON.stringify(report, null, 2) + '\n') }
