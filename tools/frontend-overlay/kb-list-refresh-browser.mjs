// Exercise the actual compiled Vue UI with synthetic authorization responses.
// No deployed service, private document, model provider, or configuration write.
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { createRequire } from 'node:module'
import { existsSync, statSync, readFileSync, mkdirSync, writeFileSync } from 'node:fs'
import { resolve, extname } from 'node:path'

const { chromium } = createRequire(import.meta.url)('playwright')
const base = resolve(process.env.KB_LIST_REFRESH_UI || '.local/kb-list-refresh-ui')
const before = resolve(process.env.KB_LIST_REFRESH_BEFORE || '.local/model-refresh-ui/after/dist')
const after = resolve(process.env.KB_LIST_REFRESH_AFTER || `${base}/after/dist`)
const modes = process.env.KB_LIST_REFRESH_MODE ? [process.env.KB_LIST_REFRESH_MODE] : ['before', 'after']
const selectedRole = process.env.KB_LIST_REFRESH_ROLE
const screenshotDir = `${base}/screenshots`
mkdirSync(screenshotDir, { recursive: true })

const report = {
  scope: 'Actual compiled product Vue knowledge-base list and Viewer chat UI; synthetic API data only.',
  builds: { before, after }, checks: [], screenshots: [], pageErrors: [], unexpectedRequests: [],
}
let directory = before
const server = createServer((req, res) => {
  let path = resolve(directory, '.' + new URL(req.url, 'http://localhost').pathname)
  if (!path.startsWith(directory + '/') || !existsSync(path) || !statSync(path).isFile()) path = `${directory}/index.html`
  res.setHeader('Content-Type', ({ '.html': 'text/html', '.js': 'application/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.json': 'application/json' })[extname(path)] || 'application/octet-stream')
  res.end(readFileSync(path))
})
await new Promise(done => server.listen(0, '127.0.0.1', done))
const origin = `http://127.0.0.1:${server.address().port}`
const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })

const tenant = { id: 42, name: '知识库刷新验收 · 合成空间', status: 'active', storage_quota: 10737418240, storage_used: 0 }
const kb = { id: 'synthetic-kb', tenant_id: 42, name: '合成知识库 · 访问验证', creator_id: 'synthetic-owner', creator_name: '合成 Owner', type: 'document', embedding_model_id: 'embedding', summary_model_id: 'chat', is_initialized: true, knowledge_count: 1, chunk_count: 1, created_at: '2026-10-21T00:00:00Z', updated_at: '2026-10-21T00:00:00Z', chunking_config: {}, indexing_strategy: { vector_enabled: true, keyword_enabled: true } }
const models = [{ id: 'embedding', type: 'Embedding', name: '合成向量模型', is_default: true }, { id: 'chat', type: 'KnowledgeQA', name: '合成问答模型', is_default: true }]
const config = { agent_mode: 'quick-answer', model_id: 'chat', kb_selection_mode: 'all', knowledge_bases: [], allowed_tools: [], memory_enabled: false, mcp_selection_mode: 'none', skills_selection_mode: 'none', web_search_enabled: false }
const agents = [{ id: 'builtin-quick-answer', name: '快速问答', is_builtin: true, is_enabled: true, config }]
const pause = ms => new Promise(done => setTimeout(done, ms))
const gate = () => { let release; const promise = new Promise(done => { release = done }); return { promise, release } }
const waitFor = async (condition, label) => {
  const deadline = Date.now() + 12000
  while (Date.now() < deadline) { if (condition()) return; await pause(20) }
  throw new Error(`Timed out: ${label}`)
}

async function fixture(mode, role) {
  directory = mode === 'before' ? before : after
  assert.ok(existsSync(`${directory}/index.html`), `Missing compiled UI: ${directory}`)
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: 'zh-CN' })
  const user = { id: `synthetic-${role}`, username: `合成 ${role}`, email: `${role}@example.invalid`, tenant_id: 42, is_active: true, preferences: {} }
  const state = { role, rows: [kb], aclVisible: true, listFailure: false, aclFailure: false, authHold: null, kbHold: null, aclHold: null, heldKB: false, kbStarted: 0, kbFinished: 0, authStarted: 0, aclStarted: 0, aclFinished: 0 }
  const calls = [], errors = [], pendingGates = []
  const makeGate = () => { const value = gate(); pendingGates.push(value); return value }
  const memberships = () => [{ tenant_id: 42, tenant_name: tenant.name, role: state.role }]
  await ctx.addInitScript(({ user, tenant, memberships }) => {
    if (!sessionStorage.getItem('kb-list-refresh-loaded')) {
      sessionStorage.setItem('kb-list-refresh-loaded', '1')
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
    const call = { path, method: req.method() }
    calls.push(call)
    const json = data => route.fulfill({ json: data })
    const success = data => { call.status = 200; return json({ success: true, data, total: Array.isArray(data) ? data.length : undefined }) }
    const failure = code => { call.status = 503; return route.fulfill({ status: 503, json: { success: false, error: { code, message: '合成临时请求失败' } } }) }
    if (path === '/api/v1/mindcreek/auth/config') return json({ enterprise_enabled: true })
    if (path === '/api/v1/mindcreek/onboarding') return success({ state: 'ready', memberships: memberships(), retryable: false })
    if (path === '/api/v1/auth/me') {
      state.authStarted++
      if (state.authHold) await state.authHold.promise
      return success({ user, tenant, memberships: memberships(), active_tenant: tenant, capabilities: { can_create_tenant: false }, mindcreek: { local_admin: false, installation_ready: true } })
    }
    if (path === '/api/v1/auth/oidc/config') return success({ enabled: true })
    if (path === '/api/v1/auth/validate') return json({ success: true, valid: true, data: { valid: true } })
    if (path === '/api/v1/mindcreek/branding') return success({ name: '小纳', revision: 1, logo_url: '', can_manage: false })
    if (path === '/api/v1/system/info') return success({ version: 'v0.8.0', edition: 'standard', graph_database_engine: '', keyword_index_engine: 'pg_search', db_version: 86 })
    if (path === '/api/v1/system/deployment-capabilities') return success({ edition: 'standard', capabilities: {} })
    if (path === '/api/v1/models') return success(models)
    if (path === '/api/v1/agents') return success(agents)
    if (path.startsWith('/api/v1/agents/')) return success(agents.find(agent => path.endsWith('/' + agent.id)) || [])
    if (path === '/api/v1/knowledge-bases') {
      state.kbStarted++
      // Capture the authoritative result at request time. Releasing an older
      // response after revocation must not resurrect its original KB records.
      const rows = [...state.rows], failed = state.listFailure
      if (state.kbHold && !state.heldKB) { state.heldKB = true; await state.kbHold.promise }
      call.kbIDs = rows.map(item => item.id)
      state.kbFinished++
      return failed ? failure('synthetic.kb_list_unavailable') : success(rows)
    }
    if (path === '/api/v1/knowledge-bases/' + kb.id) return success(kb)
    if (path === '/api/v1/tenants/42') return success(tenant)
    if (path.includes('/kb-permissions/') && path.endsWith('/effective')) {
      state.aclStarted++
      if (state.aclHold) await state.aclHold.promise
      state.aclFinished++
      if (state.aclFailure) return failure('synthetic.acl_unavailable')
      return success({ visible: state.aclVisible, manage: ['owner', 'admin'].includes(role), creator: role === 'owner', visibility: 'restricted', revision: 1, operations: role === 'viewer' ? [] : ['document.create'] })
    }
    if (path === '/api/v1/mindcreek/agent/scope') return success({ knowledge_base_ids: state.rows.map(item => item.id), selection: 'default', authorization: 'native' })
    return success([])
  })
  const page = await ctx.newPage()
  page.setDefaultTimeout(12000)
  page.on('pageerror', error => errors.push(error.message))
  const shot = async name => {
    const filename = `${mode}-${role}-${name}.png`
    await page.screenshot({ path: `${screenshotDir}/${filename}`, fullPage: true, animations: 'disabled' })
    report.screenshots.push(filename)
  }
  const snapshot = () => page.evaluate(() => {
    const stores = document.querySelector('#app')?.__vue_app__?.config.globalProperties.$pinia?._s
    const resources = stores?.get('chatResources'), settings = stores?.get('settings')
    return {
      url: location.pathname + location.search,
      role: stores?.get('auth')?.currentTenantRole,
      storeKBIDs: resources?.rawKnowledgeBases.map(item => item.id),
      selectedKBIDs: settings ? [...settings.settings.selectedKnowledgeBases] : [],
      emptyText: document.querySelector('.kb-list-main > .empty-state')?.textContent.trim(),
      cards: [...document.querySelectorAll('.kb-card:not(.kb-card-skeleton)')].map(card => card.textContent.trim()),
      modelName: document.querySelector('.model-selector-name')?.textContent.trim(),
      alerts: [...document.querySelectorAll('[data-testid="kb-list-error"]')].map(el => el.textContent.trim()),
    }
  })
  const settle = () => page.evaluate(() => new Promise(done => requestAnimationFrame(() => requestAnimationFrame(done))))
  const focus = event => page.evaluate(event => event === 'visibilitychange' ? document.dispatchEvent(new Event(event)) : window.dispatchEvent(new Event(event)), event || 'focus')
  const card = page.locator('.kb-card:not(.kb-card-skeleton)').filter({ hasText: kb.name })
  const empty = page.locator('.kb-list-main > .empty-state')
  const loading = page.getByTestId('kb-list-loading')
  const error = page.getByTestId('kb-list-error')
  const record = async (name, details = {}) => report.checks.push({ mode, role, name, state: await snapshot(), ...details })
  const assertNoEmpty = async () => assert.equal(await empty.count(), 0, 'Loading or a temporary failure must not render create-first-KB')
  const startObserve = () => page.evaluate(() => {
    window.kbRefreshFalseEmpty = []
    window.kbRefreshObserver = new MutationObserver(() => {
      const empty = document.querySelector('.kb-list-main > .empty-state')
      if (empty) window.kbRefreshFalseEmpty.push(empty.textContent.trim())
    })
    window.kbRefreshObserver.observe(document.querySelector('.kb-list-main'), { childList: true, subtree: true, characterData: true })
  })
  const endObserve = async () => {
    const seen = await page.evaluate(() => { window.kbRefreshObserver.disconnect(); return window.kbRefreshFalseEmpty })
    assert.deepEqual(seen, [], 'A refresh must not even transiently render create-first-KB for a known nonempty list')
  }
  const retry = async () => { await error.getByRole('button', { name: /重试/ }).click(); await card.waitFor(); await error.waitFor({ state: 'hidden' }); await settle() }
  const clean = async () => {
    for (const value of pendingGates) value.release()
    report.pageErrors.push(...errors)
    report.scenarios ??= []
    report.scenarios.push({ mode, role, calls })
    await ctx.close()
  }
  return { page, state, calls, errors, makeGate, shot, snapshot, settle, focus, card, empty, loading, error, record, assertNoEmpty, startObserve, endObserve, retry, clean }
}

async function listScenario(mode, role) {
  const f = await fixture(mode, role)
  const { page, state, card, empty, loading, error } = f
  try {
    await page.goto(origin + '/platform/knowledge-bases?scope=all')
    await card.waitFor()
    await f.settle()
    await f.record('initial-authorized-library')
    if (role === 'admin') await f.shot('initial-library')

    if (mode === 'after') await f.startObserve()
    // The old UI creates two independent requests on focus. The fixed UI
    // may correctly coalesce them; either way release the oldest last.
    const kbStart = state.kbStarted, authStart = state.authStarted
    state.authHold = f.makeGate(); state.kbHold = f.makeGate(); state.heldKB = false
    await f.focus()
    await waitFor(() => state.authStarted > authStart, 'focus auth refresh')
    state.authHold.release(); state.authHold = null
    await waitFor(() => state.kbStarted > kbStart, 'focus KB refresh')
    await pause(250)
    const refreshRequests = state.kbStarted - kbStart
    if (refreshRequests > 1) await waitFor(() => state.kbFinished >= state.kbStarted - 1, 'newer KB request finishes before held original')
    state.kbHold.release(); state.kbHold = null
    if (mode === 'before') {
      assert.ok(refreshRequests >= 2, 'The old UI must reproduce the original two-list-request race')
      await empty.waitFor()
      const result = await f.snapshot()
      assert.deepEqual(result.storeKBIDs, [kb.id])
      assert.equal(result.cards.length, 0)
      await f.shot('race-false-empty-library')
      await f.record('reproduced-old-focus-race-with-valid-store', { refreshRequests })
      await page.reload(); await card.waitFor()
    } else {
      await card.waitFor(); await loading.waitFor({ state: 'hidden' }); await f.settle()
      await f.assertNoEmpty()
      await f.record('focus-race-recovers-without-navigation', { refreshRequests })
      if (role === 'admin') await f.shot('race-safe-library')
    }

    // Test ACL loading independently of the list race: the backend list is
    // always nonempty, and only effective-permission responses are delayed.
    state.aclHold = f.makeGate()
    if (mode === 'before') state.authHold = f.makeGate()
    const aclStart = state.aclStarted
    await f.focus()
    await waitFor(() => state.aclStarted > aclStart, 'effective permissions requested')
    if (mode === 'before') {
      await empty.waitFor()
      await f.shot('acl-pending-false-empty')
      await f.record('reproduced-old-ACL-loading-false-empty')
      state.authHold.release(); state.authHold = null
    } else {
      await loading.waitFor()
      await f.assertNoEmpty()
      await f.record('ACL-pending-is-loading-not-empty')
      if (role === 'admin') await f.shot('acl-pending-loading')
    }
    state.aclFailure = true
    state.aclHold.release(); state.aclHold = null
    await waitFor(() => state.aclFinished > aclStart, 'ACL failure response')
    if (mode === 'before') {
      await empty.waitFor()
      await f.shot('acl-503-false-empty')
      await f.record('reproduced-old-ACL-failure-false-empty')
    } else {
      await error.waitFor(); await loading.waitFor({ state: 'hidden' }); await f.assertNoEmpty()
      await f.record('ACL-503-is-error-not-empty')
      if (role === 'admin') await f.shot('acl-503-retry')
      state.aclFailure = false
      await f.retry()
      await f.record('ACL-retry-restores-library')

      state.listFailure = true
      await f.focus('visibilitychange')
      await error.waitFor(); await loading.waitFor({ state: 'hidden' }); await f.assertNoEmpty()
      await f.record('list-503-is-error-not-empty')
      if (role === 'admin') await f.shot('list-503-retry')
      state.listFailure = false
      await f.retry()
      await f.record('list-retry-restores-library')
      await f.endObserve()

      // A successful authoritative empty list must still render empty.
      state.rows = []
      await f.focus('mindcreek-kb-permissions-changed')
      await empty.waitFor(); await error.waitFor({ state: 'hidden' }); await loading.waitFor({ state: 'hidden' })
      assert.equal(await card.count(), 0)
      await f.record('successful-empty-list-is-real-empty')
      if (role === 'admin') await f.shot('authoritative-empty')
      state.rows = [kb]
      await f.focus(); await card.waitFor()

      // Revocation applies even when a stale discovery/list response still
      // includes the KB. Current effective ACL denies access.
      state.aclVisible = false
      await f.focus('mindcreek-kb-permissions-changed')
      await empty.waitFor(); await loading.waitFor({ state: 'hidden' }); await error.waitFor({ state: 'hidden' })
      assert.equal(await card.count(), 0)
      await f.record('authoritative-ACL-revocation-hides-KB')
      if (role === 'admin') await f.shot('revoked-library')
      state.aclVisible = true
      await f.focus(); await card.waitFor(); await loading.waitFor({ state: 'hidden' })
      await f.shot('restored-library')
      await f.record('later-authorization-restores-library')
    }
    assert.equal(f.calls.some(call => !['GET', 'HEAD', 'OPTIONS'].includes(call.method)), false)
    assert.deepEqual(f.errors, [])
  } catch (err) {
    await f.shot('failure')
    report.failureState = await f.snapshot().catch(() => null)
    throw err
  } finally { await f.clean() }
}

async function viewerScenario() {
  const f = await fixture('after', 'viewer')
  const { page, state } = f
  try {
    await page.goto(origin + '/platform/knowledge-bases')
    await page.waitForURL('**/platform/creatChat')
    await page.locator('.model-selector-name').filter({ hasText: '合成问答模型' }).waitFor()
    await page.locator('[data-guide="chat-kb-mention"]').click()
    await page.locator('.mention-group-entry').filter({ hasText: '知识库' }).click()
    await page.locator('.mention-item').filter({ hasText: kb.name }).waitFor()
    await f.shot('authorized-question-selection')
    await page.locator('.mention-item').filter({ hasText: kb.name }).click()
    await page.waitForFunction(id => document.querySelector('#app').__vue_app__.config.globalProperties.$pinia._s.get('settings').settings.selectedKnowledgeBases.includes(id), kb.id)
    await f.record('Viewer-selects-authorized-KB-without-management-access')
    assert.equal(await page.locator('.kb-list-container').count(), 0)
    state.rows = []
    const before = state.kbFinished
    await f.focus('mindcreek-kb-permissions-changed')
    await waitFor(() => state.kbFinished > before, 'Viewer revoked list revalidated')
    await page.waitForFunction(() => document.querySelector('#app').__vue_app__.config.globalProperties.$pinia._s.get('settings').settings.selectedKnowledgeBases.length === 0)
    await page.locator('[data-guide="chat-kb-mention"]').click()
    await f.settle()
    assert.equal(await page.locator('.mention-item').filter({ hasText: kb.name }).count(), 0)
    assert.equal(await page.locator('.mention-group-entry').filter({ hasText: '知识库' }).count(), 0)
    assert.equal((await f.snapshot()).modelName, '合成问答模型')
    await f.shot('revoked-question-selection')
    await f.record('Viewer-revocation-clears-selection-and-removes-KB-option')
    assert.equal(f.calls.some(call => !['GET', 'HEAD', 'OPTIONS'].includes(call.method)), false)
    assert.deepEqual(f.errors, [])
  } catch (err) {
    await f.shot('failure')
    report.failureState = await f.snapshot().catch(() => null)
    throw err
  } finally { await f.clean() }
}

try {
  if (modes.includes('before')) await listScenario('before', 'admin')
  if (modes.includes('after')) {
    for (const role of selectedRole ? [selectedRole] : ['owner', 'admin', 'contributor']) {
      if (role !== 'viewer') await listScenario('after', role)
    }
    if (!selectedRole || selectedRole === 'viewer') await viewerScenario()
  }
  assert.deepEqual(report.unexpectedRequests, [])
  report.success = true
} catch (err) { report.success = false; report.error = err.stack; process.exitCode = 1 }
finally {
  await browser.close()
  server.close()
  writeFileSync(`${base}/browser-report.json`, JSON.stringify(report, null, 2) + '\n')
  console.log(JSON.stringify({ success: report.success, checks: report.checks.length, screenshots: report.screenshots.length, error: report.error }, null, 2))
}
