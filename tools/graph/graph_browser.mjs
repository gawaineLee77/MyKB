// Real native UI + gateway + Neo4j. Only loopback network access is allowed.
import { createRequire } from 'node:module'
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs'
import { createHash } from 'node:crypto'
const root = process.env.GRAPH_ROOT, origin = process.env.GRAPH_ORIGIN
const { chromium } = createRequire(`${root}/tools/redesign/r4_browser.mjs`)('playwright')
const fixture = JSON.parse(readFileSync(process.env.GRAPH_FIXTURE, 'utf8'))
const out = `${root}/docs/assets/graph`
mkdirSync(out, { recursive: true })
const report = { status: 'running', scope: 'Unchanged native R4 UI, real gateway/native API/Neo4j; synthetic identity and model.',
  ui: JSON.parse(readFileSync(`${root}/docs/plans/evidence/r4-ui-build.json`, 'utf8')),
  node: process.version, requests: [], screenshots: [], checks: [],
  probe_sha256: createHash('sha256').update(readFileSync(import.meta.filename)).digest('hex') }
const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
let page
async function context(role) {
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1050 }, locale: 'zh-CN' })
  await ctx.route('**/*', route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort())
  await ctx.addInitScript(({ token, tenant, role }) => {
    localStorage.setItem('weknora_token', token)
    localStorage.setItem('mindcreek_auth_kind', role === 'admin' ? 'admin' : 'corporate')
    localStorage.setItem('weknora_selected_tenant_id', String(tenant))
    localStorage.setItem('locale', 'zh-CN')
    localStorage.setItem('weknora:new-user-guide-done:v1', '1')
    for (const key of ['kb-list:v2','kb-create:v3','tenant-models:v1','kb-detail:v1','chat:v1','agent-list:v1','agent-create:v1']) localStorage.setItem('weknora:contextual-guide-' + key, '1')
  }, { token: fixture[role], tenant: fixture.tenant, role })
  page = await ctx.newPage()
  page.setDefaultTimeout(45000)
  page.on('response', r => {
    const path = new URL(r.url()).pathname
    if (path.startsWith('/api/')) report.requests.push({ role, path, method: r.request().method(), status: r.status() })
  })
  return ctx
}
async function shot(name) {
  await page.waitForLoadState('networkidle')
  await page.screenshot({ path: `${out}/${name}.png`, fullPage: true, animations: 'disabled' })
  report.screenshots.push(`docs/assets/graph/${name}.png`)
}
const waitAPI = (path, method) => page.waitForResponse(r => new URL(r.url()).pathname === path && r.request().method() === method)
try {
  const ctx = await context('admin')
  const headers = { Authorization: `Bearer ${fixture.admin}`, 'X-Tenant-ID': String(fixture.tenant) }
  // A new empty KB shows the real before/after switch, without altering the
  // graph-only KB used to prove that answers come from Neo4j.
  const created = await ctx.request.post(origin + '/api/v1/knowledge-bases', { headers, data: {
    name: '知识图谱配置 · 合成验证', type: 'document',
    indexing_strategy: { vector_enabled: true, keyword_enabled: false, graph_enabled: false, wiki_enabled: false }
  } })
  if (!created.ok()) throw new Error('Browser fixture KB creation failed')
  const kb = (await created.json()).data.id
  await page.goto(origin + `/platform/knowledge-bases/${kb}`)
  await page.locator('.kb-settings-button').click()
  await page.locator('[data-guide="kb-editor-nav-graph"]').click()
  await page.getByText('启用实体关系提取', { exact: true }).waitFor()
  await shot('before-extraction-enabled')
  await page.locator('.graph-settings .t-switch').click()
  const tags = page.locator('.tags-control-group input').last()
  await tags.fill('DEPENDS_ON')
  await page.locator('.t-select-option:visible').filter({ hasText: 'DEPENDS_ON' }).click()
  await page.getByPlaceholder('输入一段包含实体和关系的文本...').fill('Astra depends on Helios.')
  const [preview] = await Promise.all([waitAPI('/api/v1/initialization/extract/text-relation', 'POST'), page.getByRole('button', { name: '开始提取', exact: true }).click()])
  const graph = (await preview.json()).data
  if (!preview.ok() || graph.nodes.length !== 2 || graph.relations.length !== 1) throw new Error(`Native graph preview failed: HTTP ${preview.status()}`)
  await page.locator('.node-name-input input').first().waitFor()
  await shot('after-extraction-preview')
  const [saved] = await Promise.all([waitAPI(`/api/v1/initialization/config/${kb}`, 'PUT'), page.locator('[data-guide="kb-create-submit"]').click()])
  if (!saved.ok()) throw new Error('Native graph config save failed')
  const persisted = (await (await ctx.request.get(origin + `/api/v1/knowledge-bases/${kb}`, { headers })).json()).data
  if (!persisted.extract_config.enabled || !persisted.indexing_strategy.graph_enabled) throw new Error('Graph settings not persisted')
  await page.reload()
  await page.locator('.kb-settings-button').click()
  await page.locator('[data-guide="kb-editor-nav-graph"]').click()
  await page.locator('.node-name-input input').first().waitFor()
  await shot('after-saved-graph-settings')
  report.checks.push('native_toggle_preview_save_reload')
  await ctx.close()
  const viewer = await context('viewer')
  await page.goto(origin + '/platform/creatChat')
  await page.waitForLoadState('networkidle')
  await page.locator('.agent-mode-btn').click()
  await page.locator('.agent-option').filter({ hasText: 'Graph assistant' }).click()
  await page.locator('.agent-mode-btn').filter({ hasText: 'Graph assistant' }).waitFor()
  await page.locator('[data-guide="chat-input"] textarea').fill('What does Astra depend on?')
  const [answer] = await Promise.all([page.waitForResponse(r => /\/api\/v1\/(knowledge-chat|agent-chat)\//.test(new URL(r.url()).pathname)), page.locator('[data-guide="chat-send"]').click()])
  if (!answer.ok()) throw new Error('Employee graph conversation failed')
  if (answer.request().postDataJSON().agent_id !== fixture.agent) throw new Error('Browser did not select the graph Agent')
  const wire = await answer.text()
  if (!wire.includes('"knowledge_id"') || !wire.includes(fixture.kb)) throw new Error('Graph source evidence missing from the browser stream')
  report.graph_stream_source_verified = true
  await page.locator('.bot_msg .markdown-content').filter({ hasText: 'MindCreek synthetic answer grounded in the retrieved knowledge.' }).first().waitFor()
  await shot('after-employee-graph-answer')
  report.checks.push('viewer_graph_agent_stream')
  await viewer.close()
  report.status = 'passed'
} catch (error) {
  report.status = 'failed'; report.error = String(error)
  if (page && !page.isClosed()) {
    await shot('failure')
    console.error((await page.locator('body').innerText()).slice(0, 3500))
  }
  throw error
} finally {
  writeFileSync(`${root}/docs/plans/evidence/graph-browser.json`, JSON.stringify(report, null, 2) + '\n')
  await browser.close()
}
