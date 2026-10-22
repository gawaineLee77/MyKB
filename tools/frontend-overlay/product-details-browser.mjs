// Compiled product UI with synthetic API fixtures. No deployment or private data.
import { createServer } from 'node:http'
import { readFileSync, existsSync, statSync, mkdirSync, writeFileSync } from 'node:fs'
import { resolve, extname } from 'node:path'
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'
const { chromium } = createRequire(import.meta.url)('playwright')
const base = resolve(process.env.PRODUCT_DETAILS_UI || '.local/product-details-ui'), out = `${base}/screenshots`
mkdirSync(out, { recursive: true })
let directory = `${base}/before/dist`
const report = { scope: 'Compiled Vue UI with synthetic API fixtures; not deployed integration acceptance.', checks: [], screenshots: [], unexpectedRequests: [], pageErrors: [] }
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
const tenant = { id: 42, name: '企业研发 · 合成空间', status: 'active', storage_quota: 10737418240, storage_used: 0 }
const kb = { id: 'synthetic-kb', tenant_id: 42, name: '员工协作手册 · 合成资料', creator_id: 'synthetic-owner', creator_name: '合成 Owner', type: 'document', embedding_model_id: 'embedding', summary_model_id: 'chat', is_initialized: true, knowledge_count: 2, chunk_count: 2, created_at: '2026-10-19T00:00:00Z', updated_at: '2026-10-19T00:00:00Z', chunking_config: {}, indexing_strategy: { vector_enabled: true, keyword_enabled: true } }
const documents = ['web', 'feishu'].map((source, i) => ({ id: `synthetic-document-${i}`, knowledge_base_id: kb.id, title: i ? '历史飞书资料.pdf' : '协作说明.pdf', file_name: i ? '历史飞书资料.pdf' : '协作说明.pdf', type: 'file', source, file_type: 'pdf', parse_status: 'completed', enable_status: 'enabled', updated_at: '2026-10-19T00:00:00Z', description: '完全合成的验收资料。', summary_status: 'completed', chunk_count: 1, tags: [] }))
let branding = { name: '小纳', logo_url: '', revision: 1 }, iconBytes = readFileSync('branding/mindcreek/assets/mindcreek-mark-ui.png')
let agent = { id: 'synthetic-agent', tenant_id: 42, name: '研发助手 · 合成智能体', description: '只使用已授权知识库回答问题。', avatar: '', creator_id: 'synthetic-owner', creator_name: '合成 Owner', is_builtin: false, is_enabled: true, created_at: '2026-10-19T00:00:00Z', config: { agent_mode: 'quick-answer', system_prompt: '依据知识回答合成验收问题。', context_template: '{{knowledge}}', model_id: 'chat', knowledge_base_ids: [kb.id], kb_selection_mode: 'selected', allowed_tools: [], memory_enabled: false, mcp_selection_mode: 'none', mcp_services: [], skills_selection_mode: 'none', selected_skills: [], sandbox_config_id: '', web_search_enabled: false } }
async function scenario(mode, role, localAdmin = false, signedIn = true) {
  directory = `${base}/${mode}/dist`
  const ctx = await browser.newContext({ viewport: { width: 1536, height: 1100 }, locale: 'zh-CN' })
  const user = { id: `synthetic-${role}`, username: `合成 ${localAdmin ? '安装管理员' : role}`, email: `${role}@example.invalid`, tenant_id: 42, is_active: true, is_system_admin: localAdmin, can_access_all_tenants: localAdmin, preferences: {} }
  const memberships = [{ tenant_id: 42, tenant_name: tenant.name, role }], calls = [], errors = []
  let conflict = false, iconCount = 0
  await ctx.addInitScript(({ user, tenant, memberships, localAdmin, signedIn }) => {
    if (window.top !== window) return
    localStorage.setItem('locale', 'zh-CN'); localStorage.setItem('weknora:new-user-guide-done:v1', '1')
    sessionStorage.setItem('mindcreek_sso_attempted_at', String(Date.now()))
    localStorage.setItem('mindcreek_auth_kind', localAdmin ? 'local-admin' : 'corporate')
    if (signedIn) {
      localStorage.setItem('weknora_token', 'synthetic-token'); localStorage.setItem('weknora_user', JSON.stringify(user)); localStorage.setItem('weknora_tenant', JSON.stringify(tenant)); localStorage.setItem('weknora_memberships', JSON.stringify(memberships)); localStorage.setItem('weknora_selected_tenant_id', '42')
    }
  }, { user, tenant, memberships, localAdmin, signedIn })
  await ctx.route('**/*', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname, method = req.method()
    if (['blob:', 'data:', 'chrome-extension:', 'chrome:', 'about:'].includes(url.protocol)) return route.continue()
    if (url.origin !== origin) { report.unexpectedRequests.push(url.href); return route.abort() }
    if (!path.startsWith('/api/')) return route.continue()
    calls.push({ path, query: url.search, method, body: req.postData() })
    const json = data => route.fulfill({ json: data }), success = data => json({ success: true, data, total: Array.isArray(data) ? data.length : undefined })
    if (path === '/api/v1/mindcreek/branding/logo') return route.fulfill({ contentType: 'image/png', body: iconBytes })
    if (path === '/api/v1/mindcreek/branding') {
      if (method === 'PUT') {
        assert.equal(localAdmin, true)
        if (conflict) { conflict = false; return route.fulfill({ status: 409, json: { success: false, error: { code: 'branding.revision_conflict', message: 'conflict' } } }) }
        const input = req.postDataJSON(); assert.equal(input.revision, branding.revision)
        assert.equal(Object.keys(input).every(key => ['name', 'revision', 'logo_data'].includes(key)), true)
        if (input.logo_data) { assert.match(input.logo_data, /^data:image\/png;base64,/); iconBytes = Buffer.from(input.logo_data.split(',')[1], 'base64') }
        branding = { name: input.name, revision: branding.revision + 1, logo_url: input.logo_data === null ? '' : input.logo_data ? `/api/v1/mindcreek/branding/logo?v=${branding.revision + 1}` : branding.logo_url }
      }
      return success(branding)
    }
    if (path === '/api/v1/mindcreek/auth/config') return json({ enterprise_enabled: true })
    if (path === '/api/v1/mindcreek/onboarding') return success({ state: 'ready', memberships, retryable: false })
    if (path === '/api/v1/auth/me') return success({ user, tenant, memberships, active_tenant: tenant, capabilities: { can_create_tenant: false }, mindcreek: { local_admin: localAdmin, installation_ready: true } })
    if (path === '/api/v1/auth/oidc/config') return success({ enabled: false })
    if (path === '/api/v1/auth/validate') return json({ success: true, valid: true, data: { valid: true } })
    if (path === '/api/v1/system/info') return success({ version: 'v0.8.0', edition: 'standard', graph_database_engine: '', keyword_index_engine: 'pg_search', db_version: 86 })
    if (path === '/api/v1/system/deployment-capabilities') return success({ edition: 'standard', capabilities: {} })
    if (path === '/api/v1/models') return success([{ id: 'embedding', type: 'Embedding', name: '合成向量模型', is_default: true }, { id: 'chat', type: 'KnowledgeQA', name: '合成问答模型', is_default: true }])
    if (path === '/api/v1/knowledge-bases') return success([kb])
    if (path === '/api/v1/knowledge-bases/' + kb.id) return success(kb)
    if (path === `/api/v1/knowledge-bases/${kb.id}/knowledge`) return success(url.searchParams.get('source') ? documents.filter(doc => doc.source === url.searchParams.get('source')) : documents)
    if (path.includes('/knowledge/folders')) return success({ folders: [] })
    if (path === '/api/v1/tenants/42') return success(tenant)
    if (path.includes('/kb-permissions/') && path.endsWith('/effective')) return success({ visible: true, manage: role === 'owner' || role === 'admin', creator: role === 'owner', visibility: 'restricted', revision: 1, operations: ['document.create', 'document.update', 'document.delete', 'document.download', 'chunk.update', 'chunk.delete', 'chunk.export'] })
    if (path === '/api/v1/agents/placeholders') return success({ agent_system_prompt: [], context_template: [], rewrite_prompt: [], fallback_prompt: [] })
    if (path === '/api/v1/agents/type-presets') return success([])
    if (path === '/api/v1/agents') return success([agent])
    if (path === '/api/v1/agents/' + agent.id) { if (method === 'PUT') agent = { ...agent, ...req.postDataJSON() }; return success(agent) }
    if (path === '/api/v1/mindcreek/agent-icons' && method === 'POST') { iconCount++; iconBytes = req.postDataBuffer(); assert.equal(req.headers()['content-type'], 'image/png'); return success({ avatar: 'mc-avatar:' + String(iconCount).padStart(32, '0') }) }
    if (path.startsWith('/api/v1/mindcreek/agent-icons/')) { assert.equal(req.headers().authorization, 'Bearer synthetic-token'); return route.fulfill({ contentType: 'image/png', body: iconBytes }) }
    if (path === '/api/v1/mindcreek/agent/scope') return success({ knowledge_base_ids: [kb.id], selection: 'default', authorization: 'native' })
    return success([])
  })
  const page = await ctx.newPage(); page.setDefaultTimeout(15000); page.on('pageerror', error => errors.push(error.stack || error.message))
  const shot = async name => { await page.screenshot({ path: `${out}/${name}.png`, fullPage: true, animations: 'disabled' }); report.screenshots.push(name + '.png') }
  const showSources = async () => {
    await page.goto(origin + '/platform/knowledge-bases/' + kb.id)
    await page.getByText(kb.name, { exact: true }).first().waitFor()
    await page.getByPlaceholder('来源', { exact: true }).click()
  }
  try {
    if (!signedIn) {
      await page.goto(origin + (localAdmin ? '/admin/login' : '/login'))
      await page.getByRole('heading', { name: localAdmin ? '管理员登录' : '使用组织账号登录', exact: true }).waitFor()
      if (mode === 'after') { await page.getByText(branding.name, { exact: true }).first().waitFor(); assert.equal(await page.title(), branding.name); if (branding.logo_url) assert.ok((await page.locator('link[rel="icon"], link[rel="shortcut icon"]').first().getAttribute('href')).includes('/branding/logo?v=')) }
      await shot(`${mode}-${localAdmin ? 'local-admin' : 'corporate'}-login`)
      report.checks.push(`${mode}: ${localAdmin ? 'local-admin' : 'corporate'} public login and title`)
      return
    }
    if (mode === 'before') {
      await showSources(); await page.getByText('飞书云盘', { exact: true }).last().waitFor(); await shot('before-source-options')
      await page.goto(origin + '/platform/agents?edit=' + agent.id)
      await page.locator('[data-guide="agent-create-name"]').waitFor(); await shot('before-agent-editor')
      await page.goto(origin + '/platform/settings?section=general'); await page.getByText('常规设置', { exact: true }).first().waitFor(); await shot('before-service-settings')
      report.checks.push('before: source filters, agent editor and existing settings captured')
      return
    }
    if (!localAdmin) {
      await page.goto(origin + '/platform/settings?section=general'); await page.getByText('常规设置', { exact: true }).first().waitFor()
      assert.equal(await page.getByText('服务外观', { exact: true }).count(), 0)
      await shot(`after-space-${role}-no-service-branding`)
      await page.goto(origin + '/platform/settings?section=service-branding'); assert.equal(await page.getByTestId('service-branding').count(), 0)
      report.checks.push(`${role}: workspace role does not expose installation branding, including direct link`)
      return
    }
    await page.goto(origin + '/platform/settings?section=service-branding')
    await page.getByTestId('service-branding').waitFor(); await page.getByLabel('服务名称', { exact: true }).waitFor()
    await shot('after-service-branding-default')
    const logoData = await page.evaluate(() => { const canvas = document.createElement('canvas'); canvas.width = 120; canvas.height = 80; const c = canvas.getContext('2d'); c.fillStyle = '#176b64'; c.fillRect(0, 0, 120, 80); c.fillStyle = '#ffffff'; c.font = 'bold 46px sans-serif'; c.textAlign = 'center'; c.fillText('纳', 60, 57); return canvas.toDataURL('image/png') })
    await page.getByTestId('brand-logo-file').setInputFiles({ name: 'synthetic-logo.png', mimeType: 'image/png', buffer: Buffer.from(logoData.split(',')[1], 'base64') })
    await page.getByLabel('服务名称', { exact: true }).fill('小纳 · 研发知识中心')
    await page.getByRole('button', { name: '保存服务外观', exact: true }).click()
    await page.getByRole('status').filter({ hasText: '已保存' }).waitFor()
    assert.equal(await page.title(), '小纳 · 研发知识中心')
    await page.getByText('版本 2', { exact: true }).waitFor(); await shot('after-service-branding-saved')
    // Conflict must preserve the draft until an explicit reload.
    conflict = true
    await page.getByLabel('服务名称', { exact: true }).fill('另一个名称')
    await page.getByRole('button', { name: '保存服务外观', exact: true }).click()
    await page.getByText('服务外观已被更新，请重新载入后再保存。', { exact: true }).waitFor()
    assert.equal(await page.getByRole('button', { name: '保存服务外观', exact: true }).isDisabled(), true); await shot('after-service-branding-conflict')
    await page.getByRole('button', { name: '重新载入', exact: true }).click(); await page.getByText('版本 2', { exact: true }).waitFor()
    await page.getByLabel('服务名称', { exact: true }).fill('小纳 $& 知识中心')
    await page.getByRole('button', { name: '保存服务外观', exact: true }).click(); await page.getByText('版本 3', { exact: true }).waitFor()
    assert.ok(branding.logo_url); assert.equal(Object.hasOwn(JSON.parse(calls.filter(call => call.path === '/api/v1/mindcreek/branding' && call.method === 'PUT').at(-1).body), 'logo_data'), false)
    await page.goto(origin + '/platform/creatChat'); await page.getByText('Hi，我是 小纳 $& 知识中心，让你的知识触手可及', { exact: true }).waitFor()
    await page.goto(origin + '/platform/settings?section=service-branding'); await page.getByLabel('服务名称', { exact: true }).fill('小纳 · 研发知识中心')
    await page.getByRole('button', { name: '保存服务外观', exact: true }).click(); await page.getByText('版本 4', { exact: true }).waitFor()
    await showSources()
    await page.getByText('URL 导入', { exact: true }).last().waitFor()
    const options = await page.locator('.t-select-option').filter({ visible: true }).allTextContents()
    assert.deepEqual(options.map(item => item.trim()), ['全部来源', '本地上传', 'URL 导入', '手动创建', 'API'])
    await shot('after-source-options')
    await Promise.all([page.waitForResponse(r => r.url().includes('/knowledge?') && r.url().includes('source=web')), page.getByText('本地上传', { exact: true }).last().click()])
    await page.getByText('协作说明', { exact: true }).first().waitFor(); assert.equal(await page.getByText('历史飞书资料', { exact: true }).count(), 0)
    await page.getByPlaceholder('来源', { exact: true }).click(); await page.getByText('全部来源', { exact: true }).last().click()
    await page.getByText('历史飞书资料', { exact: true }).first().waitFor(); await shot('after-all-sources-retains-legacy-documents')
    report.checks.push('local-admin: configurable name+logo, live title/sidebar/favicon, revision conflict/reload; five filters; legacy document retained under all')
    await page.goto(origin + '/platform/agents?edit=' + agent.id)
    await page.getByTestId('agent-icon-editor').waitFor()
    await page.getByLabel('上传智能体图标', { exact: true }).setInputFiles({ name: 'synthetic-agent.png', mimeType: 'image/png', buffer: Buffer.from(logoData.split(',')[1], 'base64') })
    await page.getByTestId('agent-icon-editor').getByTestId('agent-icon-preview').waitFor(); await shot('after-agent-icon-uploaded')
    await Promise.all([page.waitForResponse(r => r.url().endsWith('/api/v1/agents/' + agent.id) && r.request().method() === 'PUT'), page.locator('[data-guide="agent-create-submit"]').click()])
    assert.equal(agent.avatar, 'mc-avatar:' + '1'.padStart(32, '0'))
    await page.getByTestId('agent-icon-editor').waitFor({ state: 'hidden' })
    await page.getByTestId('agent-icon-preview').first().waitFor(); await shot('after-agent-icon-list')
    await page.evaluate(() => { const auth = document.querySelector('#app').__vue_app__.config.globalProperties.$pinia._s.get('auth'); auth.logout() })
    await page.getByTestId('agent-icon-preview').waitFor({ state: 'hidden' })
    report.checks.push('avatar logout: current authenticated image is removed within the same mounted page')
    await page.goto(origin + '/platform/agents?edit=' + agent.id)
    await page.getByTestId('agent-icon-editor').getByTestId('agent-icon-preview').waitFor()
    await page.getByLabel('上传智能体图标', { exact: true }).setInputFiles({ name: 'invalid.svg', mimeType: 'image/svg+xml', buffer: Buffer.from('<svg/>') })
    await page.getByText('请选择 PNG、JPEG 或 WebP 图片。', { exact: true }).waitFor(); assert.equal(iconCount, 1)
    await Promise.all([
      page.waitForResponse(r => r.url().endsWith('/api/v1/mindcreek/agent-icons') && r.request().method() === 'POST'),
      page.getByLabel('上传智能体图标', { exact: true }).setInputFiles({ name: 'replacement.png', mimeType: 'image/png', buffer: Buffer.from(logoData.split(',')[1], 'base64') }),
    ])
    await page.getByText('图片上传中…', { exact: true }).waitFor({ state: 'hidden' })
    await page.getByTestId('agent-icon-editor').getByTestId('agent-icon-preview').waitFor(); assert.equal(iconCount, 2)
    await Promise.all([page.waitForResponse(r => r.url().endsWith('/api/v1/agents/' + agent.id) && r.request().method() === 'PUT'), page.locator('[data-guide="agent-create-submit"]').click()]); assert.equal(agent.avatar, 'mc-avatar:' + '2'.padStart(32, '0'))
    await page.goto(origin + '/platform/agents?edit=' + agent.id); await page.getByTestId('agent-icon-editor').getByTestId('agent-icon-preview').waitFor(); await shot('after-agent-icon-replaced')
    await page.getByRole('button', { name: '移除图片', exact: true }).click()
    assert.equal(await page.getByTestId('agent-icon-editor').getByTestId('agent-icon-preview').count(), 0); await shot('after-agent-icon-removed')
    await Promise.all([page.waitForResponse(r => r.url().endsWith('/api/v1/agents/' + agent.id) && r.request().method() === 'PUT'), page.locator('[data-guide="agent-create-submit"]').click()]); assert.equal(agent.avatar, '')
    report.checks.push('agent avatar: bounded PNG upload, authenticated preview, persisted/list rendering, reopen, invalid SVG rejected before upload, removal saved')
    assert.deepEqual(errors, [])
  } catch (error) { await shot(`failure-${mode}-${role}-${localAdmin}`); throw error }
  finally { if (errors.length) report.pageErrors.push({ mode, role, localAdmin, errors }); await ctx.close() }
}
try {
  if (process.env.PRODUCT_DETAILS_SCENARIO !== 'after') { await scenario('before', 'owner', true); await scenario('before', 'owner', true, false); await scenario('before', 'owner', false, false) }
  if (process.env.PRODUCT_DETAILS_SCENARIO !== 'before') { await scenario('after', 'owner', true); await scenario('after', 'admin'); await scenario('after', 'owner'); await scenario('after', 'owner', true, false); await scenario('after', 'owner', false, false) }
  assert.deepEqual(report.unexpectedRequests, []); assert.deepEqual(report.pageErrors, []); report.success = true
} catch (error) { report.success = false; report.error = error.stack; process.exitCode = 1 }
finally { writeFileSync(`${base}/report.json`, JSON.stringify(report, null, 2)); await browser.close(); server.close(); console.log(JSON.stringify(report, null, 2)) }
