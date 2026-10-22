// Compiled Vue visual regression with synthetic API responses, not backend acceptance.
// Build before/after UI copies first; no production accounts, keys or services are used.
import { createServer } from 'node:http'
import { readFileSync, existsSync, statSync, mkdirSync, writeFileSync } from 'node:fs'
import { resolve, extname } from 'node:path'
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'
const root = process.cwd(), base = resolve(process.env.KB_ACL_UI || '.local/kb-acl-ui')
const { chromium } = createRequire(import.meta.url)('playwright')
const out = `${base}/screenshots`
mkdirSync(out, { recursive: true })
let directory = `${base}/before/dist`
const report = { scope: 'Compiled Vue UI with synthetic API fixtures only; not enterprise OAuth/backend integration acceptance.', node: process.version, checks: [], screenshots: [], unexpectedRequests: [] }
const server = createServer((req, res) => {
  const path = decodeURIComponent(new URL(req.url, 'http://localhost').pathname)
  let file = resolve(directory, '.' + path)
  if (!file.startsWith(directory + '/') || !existsSync(file) || !statSync(file).isFile()) file = `${directory}/index.html`
  res.setHeader('Content-Type', ({ '.html': 'text/html', '.js': 'application/javascript', '.css': 'text/css', '.json': 'application/json', '.png': 'image/png', '.svg': 'image/svg+xml', '.woff2': 'font/woff2' })[extname(file)] || 'application/octet-stream')
  res.end(readFileSync(file))
})
await new Promise(r => server.listen(0, '127.0.0.1', r))
const origin = `http://127.0.0.1:${server.address().port}`
const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const operations = ['document.create', 'document.update', 'document.delete', 'document.download', 'chunk.update', 'chunk.delete', 'chunk.export']
const tenant = { id: 42, name: '知识协作验收 · 合成空间', status: 'active', storage_quota: 10737418240, storage_used: 0 }
const kb = { id: 'synthetic-kb', tenant_id: 42, name: '项目协作手册 · 合成资料', creator_id: 'synthetic-owner', creator_name: '合成 Owner', type: 'document', embedding_model_id: 'embedding', summary_model_id: 'chat', is_initialized: true, knowledge_count: 1, chunk_count: 1, created_at: '2026-10-17T00:00:00Z', updated_at: '2026-10-17T00:00:00Z', chunking_config: {}, indexing_strategy: { vector_enabled: true, keyword_enabled: true } }
const document = { id: 'synthetic-document', knowledge_base_id: kb.id, title: '协作说明.pdf', file_name: '协作说明.pdf', type: 'file', file_type: 'pdf', parse_status: 'completed', enable_status: 'enabled', updated_at: '2026-10-17T00:00:00Z', description: '这是完全合成的文档，验证逐项权限。', summary_status: 'completed', chunk_count: 1, tags: [] }
const chunk = { id: 'synthetic-chunk', knowledge_id: document.id, knowledge_base_id: kb.id, content: '## 可见内容\n\n可阅读解析内容并用于问答，不代表可以下载原始 PDF。', is_enabled: true, chunk_index: 0, index_status: 'completed', content_revision: 1, start_at: 0, end_at: 80 }
const roster = ['owner', 'admin', 'contributor', 'viewer'].map(role => ({ user_id: `synthetic-${role}`, username: `合成 ${role}`, display_name: `合成 ${role}`, role, status: 'active' }))
async function scenario(mode, role, granted = operations) {
  directory = `${base}/${mode}/dist`
  const ctx = await browser.newContext({ viewport: { width: 1536, height: 1100 }, locale: 'zh-CN' })
  const user = { id: `synthetic-${role}`, username: `合成 ${role}`, email: `${role}@example.invalid`, tenant_id: 42, is_active: true, is_system_admin: false, can_access_all_tenants: false, preferences: {} }
  const memberships = [{ tenant_id: 42, tenant_name: tenant.name, role }], manager = ['owner', 'admin'].includes(role)
  let policy = { kb_id: kb.id, creator_user_id: 'synthetic-owner', creator_revoked: false, visibility: 'restricted', revision: 1, members: [{ user_id: 'synthetic-contributor', visible: true, operations: ['document.create'] }, { user_id: 'synthetic-viewer', visible: true, operations: [] }] }
  let injectConflict = false, delegated = false
  const calls = [], errors = []
  await ctx.addInitScript(({ user, tenant, memberships }) => {
    if (window.top !== window) return
    localStorage.setItem('weknora_token', 'synthetic-token')
    localStorage.setItem('weknora_user', JSON.stringify(user)); localStorage.setItem('weknora_tenant', JSON.stringify(tenant)); localStorage.setItem('weknora_memberships', JSON.stringify(memberships)); localStorage.setItem('mindcreek_auth_kind', 'corporate'); localStorage.setItem('weknora_selected_tenant_id', '42'); localStorage.setItem('locale', 'zh-CN'); localStorage.setItem('weknora:new-user-guide-done:v1', '1')
  }, { user, tenant, memberships })
  await ctx.route('**/*', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname, method = req.method()
    if (['blob:', 'data:', 'chrome-extension:', 'chrome:', 'about:'].includes(url.protocol)) return route.continue()
    if (url.origin !== origin) { report.unexpectedRequests.push(url.href); return route.abort() }
    if (!path.startsWith('/api/')) return route.continue()
    calls.push({ path, method, body: req.postData() })
    const json = data => route.fulfill({ json: data }), success = data => json({ success: true, data, total: Array.isArray(data) ? data.length : undefined })
    if (path === '/api/v1/mindcreek/auth/config') return json({ enterprise_enabled: true })
    if (path === '/api/v1/mindcreek/onboarding') return success({ state: 'ready', memberships, retryable: false })
    if (path === '/api/v1/auth/me') return success({ user, tenant, memberships, active_tenant: tenant, capabilities: { can_create_tenant: false }, mindcreek: { local_admin: false, installation_ready: true } })
    if (path === '/api/v1/auth/oidc/config') return success({ enabled: true })
    if (path === '/api/v1/auth/validate') return json({ success: true, valid: true, data: { valid: true } })
    if (path === '/api/v1/system/info') return success({ version: 'v0.8.0', edition: 'standard', graph_database_engine: '', keyword_index_engine: 'pg_search', db_version: 86 })
    if (path === '/api/v1/system/deployment-capabilities') return success({ edition: 'standard', capabilities: {} })
    if (path === '/api/v1/models') return success([{ id: 'embedding', type: 'Embedding', name: '合成向量模型', is_default: true }, { id: 'chat', type: 'KnowledgeQA', name: '合成问答模型', is_default: true }])
    if (path === '/api/v1/knowledge-bases') return success([kb])
    if (path === '/api/v1/knowledge-bases/' + kb.id) return success(kb)
    if (path === `/api/v1/knowledge-bases/${kb.id}/knowledge`) return success([document])
    if (path === '/api/v1/knowledge/' + document.id) return success(document)
    // Verify authenticated raw-preview access without exercising Chrome's PDF extension.
    if (path === '/api/v1/knowledge/' + document.id + '/preview') return route.fulfill({ contentType: 'text/plain', body: 'Synthetic original-file preview fixture' })
    if (path === '/api/v1/chunks/' + document.id) return success([chunk])
    if (path.includes('/knowledge/folders')) return success({ folders: [] })
    if (path === '/api/v1/tenants/42') return success(tenant)
    if (path === '/api/v1/tenants/42/members') return success({ members: roster, total: roster.length })
    if (path.includes('/kb-permissions/')) {
      if (path.endsWith('/effective')) return success({ visible: true, manage: manager, creator: role === 'owner', visibility: policy.visibility, revision: policy.revision, operations: role === 'viewer' ? [] : granted })
      if (path.endsWith('/members')) return success(roster)
      if (path.endsWith('/events')) return success([{ actor_user_id: 'synthetic-owner', action: 'policy.update', revision: policy.revision, outcome: 'allowed', created_at: '2026-10-17T12:00:00Z' }])
      if (path.endsWith('/export-chunks')) return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ knowledge_base_id: kb.id, chunks: [chunk] }), headers: { 'Content-Disposition': 'attachment; filename="chunks.json"' } })
      if (method === 'PUT') {
        if (injectConflict) { injectConflict = false; return route.fulfill({ status: 409, json: { success: false, error: { code: 'kb_acl.revision_conflict', message: 'revision conflict' } } }) }
        const input = req.postDataJSON(); assert.deepEqual(Object.keys(input).sort(), ['members', 'revision', 'visibility']); assert.equal(input.revision, policy.revision); policy = { ...policy, ...input, revision: policy.revision + 1 }
      }
      return success(policy)
    }
    if (path.endsWith('/kb-collaboration/delegation')) {
      if (method === 'POST') { assert.deepEqual(req.postDataJSON(), {}); delegated = true }
      if (method === 'DELETE') { delegated = false; return success(null) }
      return success({ configured: true, authorized: delegated, owner_user_id: delegated ? 'synthetic-owner' : undefined })
    }
    if (path.endsWith('/workspace-admin/delegation')) return success({ configured: true, authorized: true, owner_user_id: 'synthetic-owner' })
    if (path === '/api/v1/mindcreek/kb-creation-operations') return success([{ id: 'synthetic-create-operation', actor_user_id: 'synthetic-owner', state: 'pending', created_at: '2026-10-17T12:00:00Z' }])
    if (path.includes('api-principal')) return success({ mode: 'tenant' })
    if (path === '/api/v1/mindcreek/agent/scope') return success({ knowledge_base_ids: [kb.id], selection: 'default', authorization: 'native' })
    return success([])
  })
  const page = await ctx.newPage(); page.setDefaultTimeout(12000); page.on('pageerror', error => errors.push(error.stack || error.message))
  const shot = async name => { await page.locator('body').waitFor(); await page.screenshot({ path: `${out}/${name}.png`, fullPage: true, animations: 'disabled' }); report.screenshots.push(name + '.png') }
  try {
    if (role === 'viewer') {
      await page.goto(origin + '/platform/knowledge-bases/' + kb.id)
      await page.waitForURL('**/platform/creatChat')
      assert.equal(await page.getByRole('button', { name: '访问与协作', exact: true }).count(), 0)
      await shot(`${mode}-viewer-chat`)
      report.checks.push(`${mode}: Viewer retains chat navigation and no management entry`)
    } else {
      await page.goto(origin + '/platform/knowledge-bases/' + kb.id)
      await page.getByText(kb.name, { exact: true }).first().waitFor()
      if (mode === 'before') {
        await shot(`before-${role}-knowledge-base`)
        if (role === 'owner') { await page.goto(origin + '/platform/knowledge-bases'); await page.getByText(kb.name, { exact: true }).first().waitFor(); await shot('before-owner-library'); await page.goto(origin + '/platform/settings?section=integration-api'); await page.locator('.api-key-section__header button').click(); await page.locator('.api-key-dialog-row').first().waitFor(); await shot('before-owner-machine-key-scope') }
        return
      }
      assert.equal(await page.getByRole('button', { name: '访问与协作', exact: true }).count(), manager ? 1 : 0)
      if (manager) {
        await page.getByRole('button', { name: '访问与协作', exact: true }).click()
        await page.getByTestId('kb-access-panel').waitFor()
        await page.getByText('权限版本 1', { exact: true }).waitFor()
        await shot(`after-${role}-access-panel`)
        const viewerRow = page.locator('.member-card').filter({ hasText: '合成 viewer' })
        assert.equal(await viewerRow.locator('input[type=checkbox]').count(), 1)
        const editorRow = page.locator('.member-card').filter({ hasText: '合成 contributor' })
        assert.equal(await editorRow.locator('input[type=checkbox]').count(), 8)
        await page.locator('.range-label select').selectOption('workspace')
        await page.getByText(/移除某人的可见勾选只移除单独授权/).waitFor()
        injectConflict = true
        await page.getByRole('button', { name: '保存访问权限', exact: true }).click()
        await page.getByText(/权限已被其他管理者修改/).waitFor()
        assert.equal(await page.getByRole('button', { name: '保存访问权限', exact: true }).isDisabled(), true)
        await shot(`after-${role}-conflict`)
        await page.getByRole('button', { name: '重新载入', exact: true }).click()
        await page.getByText('权限版本 1', { exact: true }).waitFor()
        await page.locator('.range-label select').selectOption('workspace')
        await page.getByRole('button', { name: '保存访问权限', exact: true }).click()
        await page.getByText('权限版本 2', { exact: true }).waitFor()
        assert.equal(policy.visibility, 'workspace')
        report.checks.push(`${role}: seven independent Contributor operations, Viewer read-only, public warning, conflict reload and revision save`)
        await page.goto(origin + '/platform/knowledge-bases')
        await page.getByText(kb.name, { exact: true }).first().waitFor()
        await page.getByText('知识库创建恢复', { exact: true }).click()
        await page.getByText(/操作 synthetic-create-operation/).waitFor()
        await shot(`after-${role}-library-and-recovery`)
        await page.goto(origin + '/platform/settings?section=integration-api')
        await page.locator('.api-key-section__header button').click()
        await page.getByText('知识内容操作（不勾选只允许原生能力范围内的阅读）。ingest 不会自动获得下列操作。', { exact: true }).filter({ visible: true }).waitFor()
        assert.equal(await page.getByLabel('明确授权全部知识库，包括以后新建的私有库', { exact: false }).filter({ visible: true }).count(), role === 'owner' ? 1 : 0)
        await page.locator('.kb-machine-permissions').filter({ visible: true }).scrollIntoViewIfNeeded()
        await shot(`after-${role}-machine-key-knowledge-scope`)
        if (role === 'owner') {
          await page.goto(origin + '/platform/settings?section=integration-api')
          await page.getByTestId('kb-collaboration-delegation').waitFor()
          await page.getByRole('button', { name: '启用协作写入', exact: true }).click()
          await page.getByRole('button', { name: '确认授权', exact: true }).click()
          await page.getByText('已启用：协作者可执行逐项获准的知识库写入。', { exact: true }).waitFor()
          await shot('after-owner-separate-collaboration-authorization')
          await page.getByTestId('kb-collaboration-delegation').getByRole('button', { name: '撤销授权', exact: true }).click()
          await page.getByText(/未启用或授权已失效：请由 Owner 启用/).waitFor()
          report.checks.push('Owner collaboration authorization is separate and successful DELETE clears status')
        }
      } else {
        assert.equal(await page.locator('.doc-filter-actions').count(), granted.includes('document.create') ? 1 : 0)
        assert.equal(await page.locator('.kb-settings-button').count(), 0)
        await shot('after-contributor-' + granted.join('-').replaceAll('.', '_'))
        await page.locator('.knowledge-card').first().click()
        await page.getByRole('button', { name: '查看分块', exact: true }).click()
        await page.getByText('可阅读解析内容并用于问答，不代表可以下载原始 PDF。', { exact: false }).first().waitFor()
        if (!granted.includes('document.download')) assert.equal(calls.filter(call => call.path.includes('/download') || call.path.includes('/preview')).length, 0)
        assert.equal(await page.getByRole('button', { name: '预览', exact: true }).count(), granted.includes('document.download') ? 1 : 0)
        assert.equal(await page.getByRole('button', { name: '导出分块', exact: true }).count(), granted.includes('chunk.export') ? 1 : 0)
        assert.equal(await page.getByRole('button', { name: '删除分块', exact: true }).count(), granted.includes('chunk.delete') ? 1 : 0)
        if (granted.includes('chunk.export')) {
          const download = page.waitForEvent('download')
          await page.getByRole('button', { name: '导出分块', exact: true }).click()
          assert.equal((await download).suggestedFilename(), 'chunks.json')
          const exported = calls.find(call => call.path.endsWith('/export-chunks'))
          assert.deepEqual(JSON.parse(exported.body), { knowledge_ids: [document.id] })
        }
        await shot('after-contributor-parsed-' + granted.join('-').replaceAll('.', '_'))
        report.checks.push(`Contributor ${granted.join(',')}: independent upload, preview, chunk delete/export; parsed reading retained, KB management hidden`)
      }
    }
    assert.deepEqual(errors, [])
  } finally { await ctx.close() }
}
try { if (process.env.KB_ACL_SCENARIO) await scenario('after', 'contributor', [process.env.KB_ACL_SCENARIO]); else { await scenario('before', 'owner'); await scenario('before', 'admin'); await scenario('before', 'contributor', []); await scenario('before', 'viewer', []); await scenario('after', 'owner'); await scenario('after', 'admin'); for (const operation of operations) await scenario('after', 'contributor', [operation]); await scenario('after', 'viewer', []) } assert.deepEqual(report.unexpectedRequests, []); report.success = true }
catch (error) { report.success = false; report.error = error.stack; process.exitCode = 1 }
finally { writeFileSync(`${base}/report.json`, JSON.stringify(report, null, 2)); await browser.close(); server.close(); console.log(JSON.stringify(report, null, 2)) }
