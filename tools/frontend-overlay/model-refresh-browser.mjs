// Exercise the real compiled Vue application with synthetic API fixtures.
// Before captures the focus-triggered false model warning; after checks the
// resource refresh and permission invalidation behavior without a backend.
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { createRequire } from 'node:module'
import { existsSync, statSync, readFileSync, mkdirSync, writeFileSync } from 'node:fs'
import { resolve, extname } from 'node:path'
const { chromium } = createRequire(import.meta.url)('playwright')
const base = resolve(process.env.MODEL_REFRESH_UI || '.local/model-refresh-ui')
const before = resolve(process.env.MODEL_REFRESH_BEFORE || '.local/image-access-ui/after')
const after = resolve(process.env.MODEL_REFRESH_AFTER || `${base}/after`)
const modes = process.env.MODEL_REFRESH_BEFORE_ONLY === '1' ? ['before'] : ['before', 'after']
mkdirSync(`${base}/screenshots`, { recursive: true })
let directory = `${before}/dist`
const report = {
  scope: 'Actual compiled product Vue chat UI with synthetic API data; no deployed service or model provider is used.',
  checks: [], screenshots: [], pageErrors: [], unexpectedRequests: [],
}
const server = createServer((req, res) => {
  let path = resolve(directory, '.' + new URL(req.url, 'http://localhost').pathname)
  if (!path.startsWith(directory + '/') || !existsSync(path) || !statSync(path).isFile()) path = `${directory}/index.html`
  res.setHeader('Content-Type', ({ '.html': 'text/html', '.js': 'application/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.json': 'application/json' })[extname(path)] || 'application/octet-stream')
  res.end(readFileSync(path))
})
await new Promise(done => server.listen(0, '127.0.0.1', done))
const origin = `http://127.0.0.1:${server.address().port}`
const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const tenant = { id: 42, name: '模型刷新验收 · 合成空间', status: 'active', storage_quota: 10737418240, storage_used: 0 }
const kb = { id: 'synthetic-kb', tenant_id: 42, name: '合成知识库', creator_id: 'synthetic-owner', type: 'document', embedding_model_id: 'embedding', summary_model_id: 'chat', is_initialized: true, knowledge_count: 1, chunk_count: 1, created_at: '2026-10-20T00:00:00Z', updated_at: '2026-10-20T00:00:00Z', chunking_config: {}, indexing_strategy: { vector_enabled: true, keyword_enabled: true } }
const models = [{ id: 'embedding', type: 'Embedding', name: '合成向量模型', is_default: true }, { id: 'chat', type: 'KnowledgeQA', name: '合成问答模型', is_default: true }]
const config = { agent_mode: 'quick-answer', model_id: 'chat', kb_selection_mode: 'all', knowledge_bases: [], allowed_tools: [], memory_enabled: false, mcp_selection_mode: 'none', skills_selection_mode: 'none', web_search_enabled: false }
const agents = [
  { id: 'builtin-quick-answer', name: '快速问答', is_builtin: true, is_enabled: true, config },
  { id: 'synthetic-agent', name: '研发助手 · 合成智能体', is_builtin: false, is_enabled: true, creator_id: 'synthetic-owner', config: { ...config } },
]
async function scenario(mode) {
  directory = `${mode === 'before' ? before : after}/dist`
  assert.ok(existsSync(`${directory}/index.html`), `Missing compiled UI: ${directory}`)
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: 'zh-CN' })
  const user = { id: 'synthetic-owner', username: '合成 Owner', email: 'owner@example.invalid', tenant_id: 42, is_active: true, preferences: {} }
  let role = 'owner', knowledgeBases = [kb], holdKnowledgeBases, requestCount = 0, modelFailure = false, modelRows = models
  const calls = [], errors = []
  const memberships = () => [{ tenant_id: 42, tenant_name: tenant.name, role }]
  await ctx.addInitScript(({ user, tenant, memberships }) => {
    const firstLoad = !sessionStorage.getItem('model-refresh-page-loads')
    sessionStorage.setItem('model-refresh-page-loads', String(Number(sessionStorage.getItem('model-refresh-page-loads') || 0) + 1))
    if (firstLoad) {
      localStorage.setItem('weknora_token', 'synthetic-token')
      localStorage.setItem('weknora_user', JSON.stringify(user))
      localStorage.setItem('weknora_tenant', JSON.stringify(tenant))
      localStorage.setItem('weknora_memberships', JSON.stringify(memberships))
      localStorage.setItem('mindcreek_auth_kind', 'corporate')
      localStorage.setItem('weknora_selected_tenant_id', '42')
      localStorage.setItem('weknora_last_chat_model_id', 'chat')
    }
    localStorage.setItem('locale', 'zh-CN')
    localStorage.setItem('weknora:new-user-guide-done:v1', '1')
    for (const key of ['kb-list:v2', 'kb-create:v3', 'tenant-models:v1', 'kb-detail:v1', 'chat:v1', 'agent-list:v1', 'agent-create:v1']) localStorage.setItem('weknora:contextual-guide-' + key, '1')
  }, { user, tenant, memberships: memberships() })
  await ctx.route('**/*', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname
    if (['blob:', 'data:', 'chrome-extension:', 'chrome:', 'about:'].includes(url.protocol)) return route.continue()
    if (url.origin !== origin) { report.unexpectedRequests.push(url.href); return route.abort() }
    if (!path.startsWith('/api/')) return route.continue()
    calls.push({ path, method: req.method() })
    const json = data => route.fulfill({ json: data })
    const success = data => json({ success: true, data, total: Array.isArray(data) ? data.length : undefined })
    if (path === '/api/v1/mindcreek/auth/config') return json({ enterprise_enabled: true })
    if (path === '/api/v1/mindcreek/onboarding') return success({ state: 'ready', memberships: memberships(), retryable: false })
    if (path === '/api/v1/auth/me') return success({ user, tenant, memberships: memberships(), active_tenant: tenant, capabilities: { can_create_tenant: false }, mindcreek: { local_admin: false, installation_ready: true } })
    if (path === '/api/v1/auth/oidc/config') return success({ enabled: true })
    if (path === '/api/v1/auth/validate') return json({ success: true, valid: true, data: { valid: true } })
    if (path === '/api/v1/mindcreek/branding') return success({ name: '小纳', revision: 1, logo_url: '', can_manage: false })
    if (path === '/api/v1/system/info') return success({ version: 'v0.8.0', edition: 'standard', graph_database_engine: '', keyword_index_engine: 'pg_search', db_version: 86 })
    if (path === '/api/v1/system/deployment-capabilities') return success({ edition: 'standard', capabilities: {} })
    if (path === '/api/v1/models') return modelFailure
      ? route.fulfill({ status: 503, json: { success: false, error: { code: 'model.catalog_unavailable', message: 'Synthetic temporary catalog failure' } } })
      : success(modelRows)
    if (path === '/api/v1/agents') return success(agents)
    if (path.startsWith('/api/v1/agents/')) return success(agents.find(agent => path.endsWith('/' + agent.id)) || [])
    if (path === '/api/v1/knowledge-bases') { requestCount++; if (holdKnowledgeBases) await holdKnowledgeBases; return success(knowledgeBases) }
    if (path === '/api/v1/knowledge-bases/' + kb.id) return success(kb)
    if (path === '/api/v1/tenants/42') return success(tenant)
    if (path.includes('/kb-permissions/') && path.endsWith('/effective')) return success({ visible: knowledgeBases.length > 0, manage: role === 'owner', creator: role === 'owner', visibility: 'workspace', revision: 1, operations: [] })
    if (path === '/api/v1/mindcreek/agent/scope') return success({ knowledge_base_ids: knowledgeBases.map(item => item.id), selection: 'default', authorization: 'native' })
    return success([])
  })
  const page = await ctx.newPage()
  page.setDefaultTimeout(12000)
  page.on('pageerror', error => errors.push(error.message))
  const shot = async name => {
    await page.screenshot({ path: `${base}/screenshots/${name}.png`, fullPage: true, animations: 'disabled' })
    report.screenshots.push(name + '.png')
  }
  const snapshot = () => page.evaluate(() => {
    const stores = document.querySelector('#app').__vue_app__.config.globalProperties.$pinia._s
    const resources = stores.get('chatResources'), settings = stores.get('settings')
    return { models: resources.allModels.map(model => model.id), agents: resources.agents.map(agent => agent.id), knowledgeBases: resources.rawKnowledgeBases.map(kb => kb.id), selectedKnowledgeBases: [...settings.settings.selectedKnowledgeBases], selectedAgent: settings.selectedAgentId, modelName: document.querySelector('.model-selector-name')?.textContent.trim(), role: stores.get('auth').currentTenantRole }
  })
  const refresh = async event => {
    const previous = requestCount
    const response = page.waitForResponse(res => new URL(res.url()).pathname === '/api/v1/knowledge-bases')
    await page.evaluate(event => event === 'visibilitychange' ? document.dispatchEvent(new Event(event)) : window.dispatchEvent(new Event(event)), event)
    await response
    await page.evaluate(() => new Promise(done => requestAnimationFrame(() => requestAnimationFrame(done))))
    assert.ok(requestCount > previous, `${event} must revalidate knowledge permissions`)
  }
  try {
    await page.goto(origin + '/platform/creatChat')
    await page.locator('.model-selector-name').filter({ hasText: '合成问答模型' }).waitFor()
    await page.locator('.agent-mode-btn').click()
    await page.locator('.agent-option').filter({ hasText: agents[1].name }).waitFor()
    assert.equal(await page.locator('.not-ready-icon').count(), 0)
    await page.locator('.agent-selector-overlay').click({ position: { x: 5, y: 5 } })
    await shot(`${mode}-initial-configured`)
    const initial = await snapshot()
    assert.deepEqual(initial.models, models.map(model => model.id))
    if (mode === 'after') await page.evaluate(() => {
      window.modelRefreshUnexpectedLabels = []
      window.modelRefreshObserver = new MutationObserver(() => {
        const name = document.querySelector('.model-selector-name')?.textContent.trim()
        if (name && name !== '合成问答模型') window.modelRefreshUnexpectedLabels.push(name)
      })
      window.modelRefreshObserver.observe(document.querySelector('.model-selector-name'), { subtree: true, childList: true, characterData: true })
    })
    await refresh('focus')
    if (mode === 'before') {
      const focused = await snapshot()
      assert.deepEqual(focused.models, [])
      assert.equal(focused.modelName, '未配置')
      await page.locator('.agent-mode-btn').click()
      await page.locator('.agent-option').filter({ hasText: agents[1].name }).waitFor()
      await page.locator('.not-ready-icon').first().waitFor()
      await shot('before-focus-false-model-warning')
      await page.locator('.agent-selector-overlay').click({ position: { x: 5, y: 5 } })
      await page.locator('.model-selector-trigger').click()
      await page.locator('.model-option').filter({ hasText: '合成问答模型' }).click()
      assert.equal((await snapshot()).modelName, '合成问答模型')
      await page.locator('.agent-mode-btn').click()
      assert.equal(await page.locator('.not-ready-icon').count(), 0)
      await shot('before-reselect-model-recovers')
      report.checks.push('before: returning to tab clears valid model cache, creates orange agent warnings; reselecting the same model restores UI without any configuration write')
    } else {
      for (const event of ['focus', 'visibilitychange', 'mindcreek-kb-permissions-changed']) {
        await refresh(event)
        const current = await snapshot()
        assert.deepEqual(current.models, initial.models)
        assert.deepEqual(current.agents, initial.agents)
        assert.equal(current.modelName, '合成问答模型')
      }
      await page.locator('.agent-mode-btn').click()
      await page.locator('.agent-option').filter({ hasText: agents[1].name }).waitFor()
      assert.equal(await page.locator('.not-ready-icon').count(), 0)
      assert.deepEqual(await page.evaluate(() => window.modelRefreshUnexpectedLabels), [], 'Refresh must not even transiently render a false missing-model label')
      await page.evaluate(() => window.modelRefreshObserver.disconnect())
      await shot('after-refresh-model-and-agent-remain-ready')
      await page.locator('.agent-selector-overlay').click({ position: { x: 5, y: 5 } })
      modelFailure = true
      const failedModels = page.waitForResponse(res => new URL(res.url()).pathname === '/api/v1/models' && res.status() === 503)
      await refresh('focus')
      await failedModels
      assert.equal((await snapshot()).modelName, '合成问答模型')
      assert.deepEqual((await snapshot()).models, initial.models)
      await shot('after-model-catalog-503-retains-known-model')
      modelFailure = false
      modelRows = []
      await refresh('focus')
      await page.locator('.model-selector-name').filter({ hasText: '未配置' }).waitFor()
      assert.deepEqual((await snapshot()).models, [])
      await shot('after-authoritative-empty-catalog-is-unconfigured')
      modelRows = models
      await refresh('focus')
      await page.locator('.model-selector-name').filter({ hasText: '合成问答模型' }).waitFor()
      report.checks.push('after: temporary model-catalog 503 retains the last successful list; successful empty catalog reflects real removal; later successful refresh restores the configured model')
      // Hold the refreshed KB list so stale accessibility can be checked during
      // a network round trip, without interpreting absent models as deleted.
      await page.evaluate(id => document.querySelector('#app').__vue_app__.config.globalProperties.$pinia._s.get('settings').selectKnowledgeBases([id]), kb.id)
      knowledgeBases = []
      let releaseKnowledgeBases
      holdKnowledgeBases = new Promise(done => { releaseKnowledgeBases = done })
      const requested = page.waitForRequest(req => new URL(req.url()).pathname === '/api/v1/knowledge-bases')
      await page.evaluate(() => window.dispatchEvent(new Event('mindcreek-kb-permissions-changed')))
      await requested
      const during = await snapshot()
      assert.deepEqual(during.knowledgeBases, [], 'KB access cache must clear before the replacement list arrives')
      assert.deepEqual(during.models, initial.models, 'Permission refresh must preserve same-identity model display')
      const response = page.waitForResponse(res => new URL(res.url()).pathname === '/api/v1/knowledge-bases')
      releaseKnowledgeBases()
      holdKnowledgeBases = undefined
      await response
      await page.waitForFunction(() => document.querySelector('#app').__vue_app__.config.globalProperties.$pinia._s.get('settings').settings.selectedKnowledgeBases.length === 0)
      assert.equal((await snapshot()).modelName, '合成问答模型')
      await shot('after-kb-revocation-model-remains-configured')
      report.checks.push('after: repeated focus, visibility and permission events retain configured models and ready agents; KB cache clears during revalidation and revoked selection is removed')
      // Membership changes still force a navigation and fresh authorized state.
      role = 'admin'
      const pageLoads = await page.evaluate(() => Number(sessionStorage.getItem('model-refresh-page-loads')))
      const navigation = page.waitForEvent('domcontentloaded')
      await page.evaluate(() => window.dispatchEvent(new Event('focus')))
      await navigation
      await page.waitForFunction(() => {
        const stores = document.querySelector('#app')?.__vue_app__?.config.globalProperties.$pinia?._s
        return stores?.get('auth')?.currentTenantRole === 'admin' && stores?.get('chatResources')?.allModels.length > 0
      })
      assert.ok(await page.evaluate(() => Number(sessionStorage.getItem('model-refresh-page-loads'))) > pageLoads)
      assert.equal((await snapshot()).role, 'admin')
      report.checks.push('after: Owner-to-Admin membership change still reloads the application and applies the new identity')
    }
    assert.equal(calls.some(call => !['GET', 'HEAD', 'OPTIONS'].includes(call.method)), false, 'The UI recovery must not modify a model or agent configuration')
    assert.deepEqual(errors, [])
    report.checks.push(`${mode}: no model/agent configuration writes and no page errors`)
  } catch (error) {
    await shot(`failure-${mode}`)
    report.failureState = await snapshot().catch(() => null)
    report.failureCalls = calls
    throw error
  } finally {
    report.pageErrors.push(...errors)
    await ctx.close()
  }
}
try {
  for (const mode of modes) await scenario(mode)
  assert.deepEqual(report.unexpectedRequests, [])
  report.success = true
} catch (error) { report.success = false; report.error = error.stack; process.exitCode = 1 }
finally {
  await browser.close()
  server.close()
  writeFileSync(`${base}/browser-report.json`, JSON.stringify(report, null, 2))
  console.log(JSON.stringify(report, null, 2))
}
