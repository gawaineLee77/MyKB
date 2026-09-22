// Focused UI verification: real compiled Vue pages with synthetic API responses.
// This is not an enterprise OAuth/backend integration acceptance test.
import { createServer } from 'node:http'
import { readFileSync, existsSync, statSync, mkdirSync, writeFileSync } from 'node:fs'
import { resolve, extname } from 'node:path'
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'
const root = process.cwd()
const { chromium } = createRequire(import.meta.url)('playwright')
const after = readFileSync(`${root}/.local/ui-links/build-path`, 'utf8').trim() + '/dist'
const before = '/private/tmp/mindcreek-r4-ui.C9KWya/dist'
let directory = before
const out = `${root}/docs/assets/ui-links`
mkdirSync(out, { recursive: true })
const report = { scope: 'Built Vue UI; synthetic API fixtures, no enterprise services or real credentials.', node: process.version, checks: [], screenshots: [], unexpected: [] }
const server = createServer((req, res) => {
  const path = decodeURIComponent(new URL(req.url, 'http://localhost').pathname)
  if (path.startsWith('/internal-docs/')) { res.setHeader('Content-Type', 'text/html; charset=utf-8'); res.end('<h1>企业内部文档 · 合成验证</h1>'); return }
  let file = resolve(directory, '.' + path)
  if (!file.startsWith(directory + '/') || !existsSync(file) || !statSync(file).isFile()) file = `${directory}/index.html`
  res.setHeader('Content-Type', ({ '.html': 'text/html', '.js': 'application/javascript', '.css': 'text/css', '.json': 'application/json', '.png': 'image/png', '.svg': 'image/svg+xml', '.woff2': 'font/woff2' })[extname(file)] || 'application/octet-stream')
  res.end(readFileSync(file))
})
await new Promise(r => server.listen(0, '127.0.0.1', r))
const origin = `http://127.0.0.1:${server.address().port}`
const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const keys = ['help', 'project', 'rbac', 'builtinModels', 'api', 'knowledgeGraph', 'migration', 'feedback', 'integration', 'sandbox']
const config = { version: 1, enabled: true, links: Object.fromEntries(keys.map(k => [k, `/internal-docs/${k}`])) }
const kb = { id: 'synthetic-kb', name: '操作手册 · 合成验证', type: 'document', summary_model_id: 'builtin-mindcreek-chat', embedding_model_id: 'builtin-mindcreek-embedding', chunking_config: {}, indexing_strategy: { vector_enabled: true, keyword_enabled: true, wiki_enabled: false, graph_enabled: false }, extract_config: { enabled: false }, vlm_config: { enabled: false }, storage_config: { provider: 'local' } }
const models = [
  { id: 'builtin-mindcreek-chat', name: '默认对话模型', type: 'KnowledgeQA', source: 'remote', is_builtin: true, status: 'ready', parameters: { base_url: 'https://model.example.invalid/v1', api_key: '', provider: 'generic' } },
  { id: 'builtin-mindcreek-embedding', name: '默认向量模型', type: 'Embedding', source: 'remote', is_builtin: true, status: 'ready', parameters: { dimension: 16, provider: 'generic' } },
]
async function scenario(mode, role = 'owner') {
  directory = mode === 'before' ? before : after
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: 'zh-CN' })
  const user = { id: 'synthetic-user', username: '企业管理员 · 合成验证', email: 'admin@example.invalid', tenant_id: 42, is_active: true, is_system_admin: role === 'owner', can_access_all_tenants: false, preferences: {} }
  const tenant = { id: 42, name: '企业默认空间 · 合成验证', status: 'active', storage_quota: 10737418240, storage_used: 0 }
  const memberships = [{ tenant_id: 42, tenant_name: tenant.name, role }]
  await ctx.addInitScript(({ user, tenant, memberships }) => {
    localStorage.setItem('weknora_token', 'synthetic-ui-token')
    localStorage.setItem('weknora_user', JSON.stringify(user))
    localStorage.setItem('weknora_tenant', JSON.stringify(tenant))
    localStorage.setItem('weknora_memberships', JSON.stringify(memberships))
    localStorage.setItem('mindcreek_auth_kind', 'local-admin')
    localStorage.setItem('weknora_selected_tenant_id', '42')
    localStorage.setItem('locale', 'zh-CN')
    localStorage.setItem('weknora:new-user-guide-done:v1', '1')
    for (const key of ['kb-list:v2', 'kb-create:v3', 'tenant-models:v1', 'kb-detail:v1', 'chat:v1']) localStorage.setItem('weknora:contextual-guide-' + key, '1')
  }, { user, tenant, memberships })
  await ctx.route('**/*', async route => {
    const u = new URL(route.request().url()), path = u.pathname
    const json = data => route.fulfill({ json: data })
    if (u.origin !== origin) { report.unexpected.push(u.origin); return route.abort() }
    if (path === '/mindcreek-links.json' && mode !== 'before') {
      if (mode === 'missing') return route.fulfill({ status: 404, body: '' })
      if (mode === 'invalid') return route.fulfill({ body: '{broken json' })
      if (mode === 'enabled') return json(config)
      if (mode === 'partial') return json({ version: 1, enabled: true, links: { rbac: '/internal-docs/rbac' } })
      return route.continue()
    }
    if (!path.startsWith('/api/')) return route.continue()
    if (path === '/api/v1/mindcreek/auth/config') return json({ enterprise_enabled: true })
    if (path === '/api/v1/auth/me') return json({ success: true, data: { user, tenant, memberships, active_tenant: tenant, capabilities: { can_create_tenant: role === 'owner' }, mindcreek: { local_admin: true, installation_ready: true } } })
    if (path === '/api/v1/auth/oidc/config') return json({ success: true, data: { enabled: true } })
    if (path === '/api/v1/auth/validate') return json({ success: true, valid: true, data: { valid: true } })
    if (path === '/api/v1/system/info') return json({ success: true, data: { version: 'v0.8.0', edition: 'standard', graph_database_engine: '', keyword_index_engine: 'pg_search', db_migration_error: 'Synthetic migration diagnostic', db_version: 86 } })
    if (path === '/api/v1/system/deployment-capabilities') return json({ success: true, data: { edition: 'standard', capabilities: {} } })
    if (path === '/api/v1/models') return json({ success: true, data: models })
    if (path === '/api/v1/knowledge-bases') return json({ success: true, data: [kb], total: 1 })
    if (path === '/api/v1/knowledge-bases/synthetic-kb') return json({ success: true, data: kb })
    if (path === '/api/v1/tenants/42') return json({ success: true, data: tenant })
    if (path === '/api/v1/tenants/42/members') return json({ success: true, data: { members: [{ user_id: user.id, username: user.username, email: user.email, role }], total: 1 } })
    if (path.includes('invitations')) return json({ success: true, data: { invitations: [], total: 0 } })
    if (path.includes('api-principal')) return json({ success: true, data: { mode: 'none' } })
    return json({ success: true, data: [], total: 0 })
  })
  const page = await ctx.newPage()
  page.setDefaultTimeout(15000)
  const errors = []
  page.on('pageerror', e => errors.push(e.message))
  const shot = async name => {
    await page.waitForLoadState('networkidle')
    await page.screenshot({ path: `${out}/${name}.png`, animations: 'disabled', fullPage: true })
    report.screenshots.push(`docs/assets/ui-links/${name}.png`)
  }
  const expected = mode === 'before' || mode === 'enabled'
  try {
    await page.goto(`${origin}/platform/settings?section=members`)
    if (role === 'viewer') {
      await page.waitForURL('**/platform/creatChat')
      await page.locator('[data-guide="user-menu"]').click()
      assert.equal(await page.locator('.user-dropdown').getByText('帮助与文档', { exact: true }).count(), 0)
      assert.equal(await page.locator('.user-dropdown').getByText('GitHub', { exact: true }).count(), 0)
      await shot('after-viewer-menu')
    } else {
      await page.locator('.tenant-members').waitFor()
      const rbac = page.locator('.tenant-members a.doc-link')
      assert.equal(await rbac.count(), Number(expected || mode === 'partial'))
      if (mode === 'enabled' || mode === 'partial') assert.equal(await rbac.getAttribute('href'), '/internal-docs/rbac')
      if (['before', 'hidden', 'enabled'].includes(mode)) await shot(`${mode}-members`)
      await page.goto(`${origin}/platform/knowledge-bases`)
      await page.locator('[data-guide="user-menu"]').click()
      await page.locator('.user-dropdown').waitFor()
      assert.equal(await page.locator('.user-dropdown').getByText('帮助与文档', { exact: true }).count(), Number(expected))
      assert.equal(await page.locator('.user-dropdown').getByText(mode === 'before' ? 'GitHub' : '项目主页', { exact: true }).count(), Number(expected))
      if (['before', 'hidden', 'enabled'].includes(mode)) await shot(`${mode}-menu`)
      if (mode === 'enabled') {
        const popup = ctx.waitForEvent('page')
        await page.locator('.user-dropdown').getByText('帮助与文档', { exact: true }).click()
        const p = await popup; await p.waitForLoadState(); assert.equal(p.url(), origin + '/internal-docs/help'); await p.close()
      }
      if (['before', 'hidden', 'enabled'].includes(mode)) {
        await page.goto(`${origin}/platform/settings?section=models`)
        await page.locator('.builtin-models-hint').waitFor()
        assert.equal(await page.locator('.builtin-models-hint a.doc-link').count(), Number(expected))
        await shot(`${mode}-models`)
        await page.goto(`${origin}/platform/settings?section=integration-api`)
        await page.locator('.api-integration .row').first().waitFor()
        assert.equal(await page.locator('.row--doc').count(), Number(expected))
        assert.equal(await page.locator('.row--doc a.doc-link').count(), Number(expected))
        await shot(`${mode}-api`)
        await page.goto(`${origin}/platform/settings?section=system`)
        await page.locator('.migration-error-row').waitFor()
        assert.equal(await page.locator('.migration-error-actions a').count(), expected ? 2 : 0)
        if (mode === 'enabled') {
          const hrefs = await page.locator('.migration-error-actions a').evaluateAll(a => a.map(x => x.getAttribute('href')))
          assert.deepEqual(hrefs, ['/internal-docs/migration', '/internal-docs/feedback'])
        }
        await shot(`${mode}-system`)
        await page.goto(`${origin}/platform/knowledge-bases/synthetic-kb`)
        await page.locator('.kb-settings-button').click()
        await page.locator('[data-guide="kb-editor-nav-graph"]').click()
        await page.locator('.graph-settings').waitFor()
        assert.equal(await page.locator('.graph-guide-link').count(), Number(expected))
        await shot(`${mode}-graph`)
      }
    }
    assert.deepEqual(errors, [], 'Vue runtime errors')
    report.checks.push({ mode, role, status: 'passed' })
  } catch (e) {
    report.failure = { mode, url: page.url(), message: String(e), errors }
    await page.screenshot({ path: `${root}/.local/ui-links/failure.png`, fullPage: true })
    throw e
  } finally { await ctx.close() }
}
try {
  for (const mode of ['before', 'hidden', 'enabled', 'partial', 'missing', 'invalid']) await scenario(mode)
  await scenario('hidden', 'viewer')
  assert.deepEqual(report.unexpected, [])
  report.status = 'passed'
} finally {
  await browser.close(); server.close()
  writeFileSync(`${root}/.local/ui-links/browser.json`, JSON.stringify(report, null, 2) + '\n')
}
console.log(JSON.stringify({ status: report.status, checks: report.checks.length, screenshots: report.screenshots.length }))
