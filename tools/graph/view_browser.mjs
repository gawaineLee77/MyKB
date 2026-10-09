// Real compiled product UI with synthetic HTTP fixtures. Backend/Neo4j are
// independently tested by graphview Go tests; this does not test corporate SSO.
import { createServer } from 'node:http'
import { readFileSync, existsSync, statSync, mkdirSync, writeFileSync } from 'node:fs'
import { resolve, extname } from 'node:path'
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'
const { chromium } = createRequire(import.meta.url)('playwright')
const root = process.cwd(), build = `${root}/.local/graph-view`
const output = `${root}/docs/assets/graph-view`
mkdirSync(output, { recursive: true })
let directory = `${build}/ui/dist`
const report = { scope: 'Compiled Vue UI and synthetic API fixtures; no enterprise OAuth/model calls.', node: process.version, checks: [], screenshots: [] }
const server = createServer((req, res) => {
  let file = resolve(directory, '.' + new URL(req.url, 'http://localhost').pathname)
  if (!file.startsWith(directory + '/') || !existsSync(file) || !statSync(file).isFile()) file = `${directory}/index.html`
  res.setHeader('Content-Type', ({ '.html': 'text/html', '.js': 'application/javascript', '.css': 'text/css', '.json': 'application/json', '.png': 'image/png', '.svg': 'image/svg+xml', '.woff2': 'font/woff2' })[extname(file)] || 'application/octet-stream')
  res.end(readFileSync(file))
})
await new Promise(r => server.listen(0, '127.0.0.1', r))
const origin = `http://127.0.0.1:${server.address().port}`
const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const node = (id, name, attributes = []) => ({ id, name, knowledge_id: 'doc-a', attributes })
const graph = { state: 'ready', truncated: false, nodes: [node('1', '订单服务', ['负责订单创建与状态流转', '维护团队：交易平台组']), node('2', '库存服务', ['提供可用库存查询与预占']), node('3', '支付网关'), node('4', '交易数据库'), node('5', '消息队列'), node('6', '交易平台组')], edges: [
  { id: 'e1', source: '1', target: '2', type: '依赖于' }, { id: 'e2', source: '1', target: '3', type: '调用' },
  { id: 'e3', source: '1', target: '4', type: '存储于' }, { id: 'e4', source: '1', target: '5', type: '发布到' }, { id: 'e5', source: '6', target: '1', type: '负责' },
] }
let page
async function scenario(role, before = false) {
  directory = `${build}/${before ? 'before-ui' : 'ui'}/dist`
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: 'zh-CN' })
  const user = { id: 'synthetic-user', username: '测试成员', email: 'fixture@example.invalid', tenant_id: 42, is_active: true, is_system_admin: role === 'owner', preferences: {} }
  const tenant = { id: 42, name: '产品研发空间', status: 'active', storage_quota: 10737418240, storage_used: 0 }
  const memberships = [{ tenant_id: 42, tenant_name: tenant.name, role }]
  const kb = { id: 'synthetic-kb', name: '交易平台技术文档', tenant_id: 42, creator_id: user.id, type: 'document', summary_model_id: 'builtin-mindcreek-chat', embedding_model_id: 'builtin-mindcreek-embedding', chunking_config: {}, indexing_strategy: { vector_enabled: true, keyword_enabled: true, wiki_enabled: false, graph_enabled: true }, extract_config: { enabled: true }, vlm_config: { enabled: false }, storage_backend_id: 'local', storage_config: { provider: 'local' } }
  await ctx.addInitScript(({ user, tenant, memberships }) => {
    localStorage.setItem('weknora_token', 'synthetic-ui-token')
    localStorage.setItem('weknora_user', JSON.stringify(user)); localStorage.setItem('weknora_tenant', JSON.stringify(tenant)); localStorage.setItem('weknora_memberships', JSON.stringify(memberships))
    localStorage.setItem('mindcreek_auth_kind', 'local-admin'); localStorage.setItem('weknora_selected_tenant_id', '42'); localStorage.setItem('locale', 'zh-CN')
    localStorage.setItem('weknora:new-user-guide-done:v1', '1')
    for (const key of ['kb-list:v2','kb-create:v3','tenant-models:v1','kb-detail:v1','chat:v1']) localStorage.setItem('weknora:contextual-guide-' + key, '1')
  }, { user, tenant, memberships })
  let state = 'ready', graphCalls = 0, sourceCalls = 0, expansion = false
  await ctx.route('**/*', async route => {
    const u = new URL(route.request().url()), path = u.pathname
    if (u.origin !== origin) throw new Error('External browser request: ' + u.origin)
    const json = data => route.fulfill({ json: data })
    if (!path.startsWith('/api/')) return route.continue()
    if (path === '/api/v1/mindcreek/auth/config') return json({ enterprise_enabled: true })
    if (path === '/api/v1/auth/me') return json({ success: true, data: { user, tenant, memberships, active_tenant: tenant, capabilities: { can_create_tenant: role === 'owner' }, mindcreek: { local_admin: true, installation_ready: true } } })
    if (path === '/api/v1/auth/oidc/config') return json({ success: true, data: { enabled: true } })
    if (path === '/api/v1/auth/validate') return json({ success: true, valid: true, data: { valid: true } })
    if (path === '/api/v1/system/info') return json({ success: true, data: { version: 'v0.8.0', edition: 'standard', graph_database_engine: 'Neo4j' } })
    if (path === '/api/v1/system/deployment-capabilities') return json({ success: true, data: { edition: 'standard', capabilities: {} } })
    if (path === '/api/v1/models') return json({ success: true, data: [{ id: 'builtin-mindcreek-chat', name: '默认对话模型', type: 'KnowledgeQA', source: 'remote', is_builtin: true, status: 'ready', parameters: {} }, { id: 'builtin-mindcreek-embedding', name: '默认向量模型', type: 'Embedding', source: 'remote', is_builtin: true, status: 'ready', parameters: { dimension: 16 } }] })
    if (path === '/api/v1/knowledge-bases') return json({ success: true, data: [kb], total: 1 })
    if (path === '/api/v1/knowledge-bases/synthetic-kb') return json({ success: true, data: kb })
    if (path === '/api/v1/tenants/42') return json({ success: true, data: tenant })
    if (path === '/api/v1/mindcreek/graph/knowledge-bases/synthetic-kb') {
      graphCalls++
      assert.equal(route.request().headers()['authorization'], 'Bearer synthetic-ui-token')
      assert.equal(route.request().headers()['x-tenant-id'], '42')
      if (state === 'error') return route.fulfill({ status: 503, json: { error: { code: 'graph.unavailable' } } })
      if (state !== 'ready') return json({ success: true, data: { state, nodes: [], edges: [], truncated: false } })
      if (u.searchParams.has('node_id')) { expansion = true; return json({ success: true, data: { ...graph, nodes: [...graph.nodes, node('7', '运维平台')], edges: [...graph.edges, { id: 'e6', source: '7', target: '1', type: '监控' }] } }) }
      if (u.searchParams.has('q')) return json({ success: true, data: { ...graph, nodes: graph.nodes.filter(n => n.name.includes(u.searchParams.get('q'))), edges: [] } })
      if (u.searchParams.has('relation')) return json({ success: true, data: { ...graph, nodes: graph.nodes.slice(0,2), edges: [graph.edges[0]] } })
      return json({ success: true, data: graph })
    }
    if (path === '/api/v1/knowledge/doc-a') { sourceCalls++; return json({ success: true, data: { id: 'doc-a', knowledge_base_id: kb.id, title: '交易系统架构说明', file_type: 'md', type: 'manual', parse_status: 'completed', file_name: 'architecture.md' } }) }
    if (path.includes('invitations')) return json({ success: true, data: { invitations: [], total: 0 } })
    if (path.includes('api-principal')) return json({ success: true, data: { mode: 'none' } })
    return json({ success: true, data: [], total: 0 })
  })
  page = await ctx.newPage(); page.setDefaultTimeout(12000)
  const errors = []; page.on('pageerror', e => errors.push(e.message))
  async function shot(name) { await page.waitForLoadState('networkidle'); await page.screenshot({ path: `${output}/${name}.png`, fullPage: true, animations: 'disabled' }); report.screenshots.push(`docs/assets/graph-view/${name}.png`) }
  try {
    await page.goto(origin + '/platform/knowledge-bases/synthetic-kb')
    if (role === 'viewer') {
      await page.waitForURL('**/platform/creatChat'); assert.equal(await page.getByRole('button', { name: '知识图谱', exact: true }).count(), 0); assert.equal(graphCalls, 0); await shot('viewer-chat')
    } else {
      await page.locator('.kb-title-actions').waitFor()
      if (before) { assert.equal(await page.getByRole('button', { name: '知识图谱', exact: true }).count(), 0); await shot('before-knowledge-base') }
      else {
        await page.getByRole('button', { name: '知识图谱', exact: true }).click()
        await page.getByRole('button', { name: '实体：订单服务', exact: true }).waitFor()
        await page.getByRole('button', { name: '实体：订单服务', exact: true }).click()
        await page.getByText('维护团队：交易平台组', { exact: true }).waitFor()
        if (role === 'owner') {
          await shot('after-entity-details')
          await page.getByRole('button', { name: '展开相邻实体' }).click(); await page.getByRole('button', { name: '实体：运维平台', exact: true }).waitFor(); assert.ok(expansion)
          const movable = page.getByRole('button', { name: '实体：运维平台', exact: true }), beforePosition = await movable.getAttribute('transform'), box = await movable.locator('circle').boundingBox()
          await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2); await page.mouse.down(); await page.mouse.move(box.x + box.width / 2 + 55, box.y + box.height / 2 + 20, { steps: 5 }); await page.mouse.up()
          assert.notEqual(await movable.getAttribute('transform'), beforePosition, 'node drag')
          await page.getByRole('button', { name: '关系：订单服务 依赖于 库存服务', exact: true }).click(); await page.getByText('关系详情', { exact: true }).waitFor(); await shot('after-relation-details')
          await page.getByRole('button', { name: '查看来源文档 ↗', exact: true }).click(); await page.waitForTimeout(300); assert.ok(sourceCalls)
          await page.reload(); await page.getByRole('button', { name: '实体：订单服务', exact: true }).waitFor()
          await page.getByLabel('搜索实体').fill('库存'); await page.getByRole('button', { name: '搜索', exact: true }).click(); await page.getByText('已加载 1 个实体 · 0 条关系').waitFor()
          await page.getByRole('button', { name: '重置', exact: true }).click(); await page.getByText('已加载 6 个实体 · 5 条关系').waitFor()
          await page.getByLabel('关系类型', { exact: true }).fill('依赖于'); await page.getByRole('button', { name: '应用筛选' }).click(); await page.getByText('已加载 2 个实体 · 1 条关系').waitFor()
          const transform = page.locator('.graph-canvas svg > g'), initialTransform = await transform.getAttribute('transform')
          await page.getByRole('button', { name: '放大图谱' }).click(); assert.notEqual(await transform.getAttribute('transform'), initialTransform)
          await page.getByRole('button', { name: '复位视图' }).click(); assert.equal(await transform.getAttribute('transform'), initialTransform)
          const stateText = { disabled: '此知识库尚未启用知识图谱', empty: '暂无已提取的实体', error: '暂时无法连接图谱服务，请稍后重试。' }
          for (state of ['disabled', 'empty', 'error']) { await page.getByRole('button', { name: '重置', exact: true }).click(); await page.getByText(stateText[state], { exact: true }).waitFor(); await shot('after-' + state) }
          state = 'ready'; await page.getByRole('button', { name: '重新加载' }).click(); await page.getByRole('button', { name: '实体：订单服务', exact: true }).waitFor()
          await page.setViewportSize({ width: 900, height: 900 }); await shot('after-narrow')
        }
      }
    }
    assert.deepEqual(errors, [], 'Vue runtime errors'); report.checks.push({ role, before, status: 'passed' })
  } finally { await ctx.close() }
}
try {
  await scenario('owner', true)
  for (const role of ['owner', 'admin', 'contributor', 'viewer']) await scenario(role)
  report.status = 'passed'
} catch (error) { report.status = 'failed'; report.error = String(error); throw error }
finally { await browser.close(); server.close(); writeFileSync(`${build}/browser.json`, JSON.stringify(report, null, 2) + '\n') }
