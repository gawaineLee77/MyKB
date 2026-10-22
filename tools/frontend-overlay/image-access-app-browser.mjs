// Actual compiled Vue pages with synthetic API data, separate from backend tests.
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { createRequire } from 'node:module'
import { existsSync, statSync, readFileSync, mkdirSync, writeFileSync } from 'node:fs'
import { resolve, extname } from 'node:path'
const { chromium } = createRequire(import.meta.url)('playwright')
const base = resolve(process.env.IMAGE_ACCESS_UI || '.local/image-access-ui')
const before = resolve(process.env.IMAGE_ACCESS_BEFORE || '.local/product-details-ui/after')
const after = resolve(process.env.IMAGE_ACCESS_AFTER || `${base}/after`)
mkdirSync(`${base}/screenshots`, { recursive: true })
let directory = `${before}/dist`
const report = { scope: 'Actual compiled product Vue document and conversation pages with synthetic API responses; not deployed backend authorization acceptance.', checks: [], screenshots: [], pageErrors: [], unexpectedRequests: [] }
const server = createServer((req, res) => {
  let path = resolve(directory, '.' + new URL(req.url, 'http://localhost').pathname)
  if (!path.startsWith(directory + '/') || !existsSync(path) || !statSync(path).isFile()) path = `${directory}/index.html`
  res.setHeader('Content-Type', ({ '.html': 'text/html', '.js': 'application/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.json': 'application/json' })[extname(path)] || 'application/octet-stream')
  res.end(readFileSync(path))
})
await new Promise(done => server.listen(0, '127.0.0.1', done))
const origin = `http://127.0.0.1:${server.address().port}`
const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const resource = 'resource://AbCdEfGhIjKlMnOpQrStUv'
const tenant = { id: 42, name: '图片权限验收 · 合成空间', status: 'active', storage_quota: 10737418240, storage_used: 0 }
const kb = { id: 'synthetic-kb', tenant_id: 42, name: '历史图片资料 · 合成知识库', creator_id: 'synthetic-owner', type: 'document', embedding_model_id: 'embedding', summary_model_id: 'chat', is_initialized: true, knowledge_count: 1, chunk_count: 1, created_at: '2026-10-20T00:00:00Z', updated_at: '2026-10-20T00:00:00Z', chunking_config: {}, indexing_strategy: { vector_enabled: true, keyword_enabled: true } }
const document = { id: 'synthetic-document', knowledge_base_id: kb.id, title: '图片权限说明.pdf', file_name: '图片权限说明.pdf', type: 'file', file_type: 'pdf', parse_status: 'completed', enable_status: 'enabled', updated_at: '2026-10-20T00:00:00Z', description: '合成资料，用于验证历史解析图片恢复。', summary_status: 'completed', chunk_count: 1, tags: [] }
const chunk = { id: 'synthetic-chunk', knowledge_id: document.id, knowledge_base_id: kb.id, content: `## 图片权限说明\n\n下面的示意图来自合成解析资料。\n\n![合成解析图片](${resource})`, is_enabled: true, chunk_index: 0, index_status: 'completed', content_revision: 1, start_at: 0, end_at: 180 }
const session = { id: 'synthetic-session', tenant_id: 42, title: '历史图片问答 · 合成会话', created_at: '2026-10-20T00:00:00Z', updated_at: '2026-10-20T00:00:00Z', is_pinned: false }
const messages = [
  { id: 'synthetic-user-message', session_id: session.id, role: 'user', content: '请展示知识库中的示意图。', images: [{ url: resource }], created_at: '2026-10-20T00:00:00Z', is_completed: true },
  { id: 'synthetic-assistant-message', session_id: session.id, role: 'assistant', content: `这是知识库中的合成示意图。\n\n![合成回答图片](${resource})`, created_at: '2026-10-20T00:00:01Z', is_completed: true, knowledge_references: [] },
]
const image = `<svg xmlns="http://www.w3.org/2000/svg" width="480" height="240"><rect width="480" height="240" rx="16" fill="#e4f4eb"/><rect x="30" y="70" width="110" height="90" rx="12" fill="#2b8b64"/><rect x="185" y="70" width="110" height="90" rx="12" fill="#4876a0"/><rect x="340" y="70" width="110" height="90" rx="12" fill="#8c699a"/><path d="M140 115h45m110 0h45" stroke="#436554" stroke-width="5"/><g fill="white" font-family="sans-serif" font-size="20" text-anchor="middle"><text x="85" y="123">Document</text><text x="240" y="123">Image</text><text x="395" y="123">Answer</text></g></svg>`
async function scenario(mode, role) {
  directory = `${mode === 'before' ? before : after}/dist`
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1080 }, locale: 'zh-CN' })
  const user = { id: `synthetic-${role}`, username: `合成 ${role}`, email: `${role}@example.invalid`, tenant_id: 42, is_active: true, preferences: {} }
  const memberships = [{ tenant_id: 42, tenant_name: tenant.name, role }]
  const calls = [], errors = []
  await ctx.addInitScript(({ user, tenant, memberships }) => {
    localStorage.setItem('weknora_token', 'synthetic-token'); localStorage.setItem('weknora_user', JSON.stringify(user)); localStorage.setItem('weknora_tenant', JSON.stringify(tenant)); localStorage.setItem('weknora_memberships', JSON.stringify(memberships)); localStorage.setItem('mindcreek_auth_kind', 'corporate'); localStorage.setItem('weknora_selected_tenant_id', '42'); localStorage.setItem('locale', 'zh-CN'); localStorage.setItem('weknora:new-user-guide-done:v1', '1')
    for (const key of ['kb-list:v2','kb-create:v3','tenant-models:v1','kb-detail:v1','chat:v1','agent-list:v1','agent-create:v1']) localStorage.setItem('weknora:contextual-guide-' + key, '1')
  }, { user, tenant, memberships })
  await ctx.route('**/*', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname
    if (['blob:', 'data:', 'chrome-extension:', 'chrome:', 'about:', 'resource:'].includes(url.protocol)) return route.continue()
    if (url.origin !== origin) { report.unexpectedRequests.push(url.href); return route.abort() }
    if (!path.startsWith('/api/') && path !== '/files') return route.continue()
    calls.push(path)
    const json = data => route.fulfill({ json: data }), success = data => json({ success: true, data, total: Array.isArray(data) ? data.length : undefined })
    if (path === '/files' || path.endsWith('/files')) {
      if (mode === 'before') return route.fulfill({ status: 409, json: { error: 'kb_acl.resource_binding_unavailable' } })
      assert.notEqual(path, '/files', 'All product image reads must use explicit resource scopes')
      assert.equal(req.headers().authorization, 'Bearer synthetic-token')
      return route.fulfill({ contentType: 'image/svg+xml', body: image, headers: { 'Cache-Control': 'no-store' } })
    }
    if (path === '/api/v1/mindcreek/auth/config') return json({ enterprise_enabled: true })
    if (path === '/api/v1/mindcreek/onboarding') return success({ state: 'ready', memberships, retryable: false })
    if (path === '/api/v1/auth/me') return success({ user, tenant, memberships, active_tenant: tenant, capabilities: { can_create_tenant: false }, mindcreek: { local_admin: false, installation_ready: true } })
    if (path === '/api/v1/auth/oidc/config') return success({ enabled: true })
    if (path === '/api/v1/auth/validate') return json({ success: true, valid: true, data: { valid: true } })
    if (path === '/api/v1/mindcreek/branding') return success({ name: '小纳', revision: 1, logo_url: '', can_manage: false })
    if (path === '/api/v1/system/info') return success({ version: 'v0.8.0', edition: 'standard', graph_database_engine: '', keyword_index_engine: 'pg_search', db_version: 86 })
    if (path === '/api/v1/system/deployment-capabilities') return success({ edition: 'standard', capabilities: {} })
    if (path === '/api/v1/models') return success([{ id: 'embedding', type: 'Embedding', name: '合成向量模型', is_default: true }, { id: 'chat', type: 'KnowledgeQA', name: '合成问答模型', is_default: true }])
    if (path === '/api/v1/knowledge-bases') return success([kb])
    if (path === '/api/v1/knowledge-bases/' + kb.id) return success(kb)
    if (path === `/api/v1/knowledge-bases/${kb.id}/knowledge`) return success([document])
    if (path === '/api/v1/knowledge/' + document.id) return success(document)
    if (path === '/api/v1/chunks/' + document.id) return success([chunk])
    if (path.includes('/knowledge/folders')) return success({ folders: [] })
    if (path === '/api/v1/tenants/42') return success(tenant)
    if (path.includes('/kb-permissions/') && path.endsWith('/effective')) return success({ visible: true, manage: ['owner', 'admin'].includes(role), creator: role === 'owner', visibility: 'workspace', revision: 1, operations: [] })
    if (path === '/api/v1/mindcreek/agent/scope') return success({ knowledge_base_ids: [kb.id], selection: 'default', authorization: 'native' })
    if (path === '/api/v1/sessions') return success([session])
    if (path === '/api/v1/sessions/' + session.id) return success(session)
    if (path === `/api/v1/messages/${session.id}/load`) return success(messages)
    if (path.endsWith('/suggestions')) return success(null)
    return success([])
  })
  const page = await ctx.newPage(); page.setDefaultTimeout(12000); page.on('pageerror', e => errors.push(e.message))
  const shot = async name => { await page.screenshot({ path: `${base}/screenshots/${name}.png`, fullPage: true, animations: 'disabled' }); report.screenshots.push(name + '.png') }
  try {
    if (role !== 'viewer') {
      await page.goto(origin + '/platform/knowledge-bases/' + kb.id)
      await page.getByText(kb.name, { exact: true }).first().waitFor()
      await page.locator('.knowledge-card').first().click()
      await page.getByRole('button', { name: '查看分块', exact: true }).click()
      await page.getByText('下面的示意图来自合成解析资料。', { exact: false }).first().waitFor()
      if (mode === 'before') await page.getByText('此图片尚未完成权限归属验证，暂不可显示。', { exact: true }).first().waitFor()
      else await page.locator('img[data-protected-src][src^="blob:"]').first().waitFor()
      await shot(`${mode}-${role}-actual-document`)
      report.checks.push(`${mode} ${role}: actual compiled document component displays ${mode === 'before' ? 'unverified notice' : 'scoped historical image'}`)
    }
    await page.goto(origin + '/platform/chat/' + session.id)
    await page.getByText('这是知识库中的合成示意图。', { exact: false }).first().waitFor()
    if (mode === 'before') await page.getByText('此图片尚未完成权限归属验证，暂不可显示。', { exact: true }).first().waitFor()
    else {
      await page.locator('.bot_msg img[data-protected-src][src^="blob:"]').first().waitFor()
      await page.locator('.user_msg_container img[src^="blob:"]').first().waitFor()
      assert.ok(calls.includes('/api/v1/sessions/synthetic-session/messages/synthetic-assistant-message/files'))
      assert.ok(calls.includes('/api/v1/sessions/synthetic-session/messages/synthetic-user-message/files'))
      assert.ok(!calls.includes('/files'))
    }
    await shot(`${mode}-${role}-actual-conversation`)
    report.checks.push(`${mode} ${role}: actual quick-answer history and uploaded-image components ${mode === 'before' ? 'reproduce unavailable images' : 'use persisted session/message scopes'}`)
    assert.deepEqual(errors, [])
  } catch (error) { await shot(`failure-${mode}-${role}`); console.error(await page.locator('body').innerText(), calls, errors); throw error }
  finally { report.pageErrors.push(...errors); await ctx.close() }
}
try {
  await scenario('before', 'owner')
  for (const role of ['owner', 'admin', 'contributor', 'viewer']) await scenario('after', role)
  assert.deepEqual(report.unexpectedRequests, [])
  report.success = true
} catch (error) { report.success = false; report.error = error.stack; process.exitCode = 1 }
finally { await browser.close(); server.close(); writeFileSync(`${base}/app-report.json`, JSON.stringify(report, null, 2)); console.log(JSON.stringify(report, null, 2)) }
