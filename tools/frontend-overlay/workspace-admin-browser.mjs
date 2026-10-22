// Compiled Vue visual regression with synthetic API responses, not backend acceptance.
// Build before/after UI copies first; no production accounts, keys or services are used.
import { createServer } from 'node:http'
import { readFileSync, existsSync, statSync, mkdirSync, writeFileSync } from 'node:fs'
import { resolve, extname } from 'node:path'
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'
const root = process.cwd(), base = resolve(process.env.WORKSPACE_ADMIN_UI || '.local/workspace-admin-ui')
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
const tenant = { id: 42, name: '权限管理验收 · 合成空间', description: '仅用于权限界面测试', status: 'active', storage_quota: 10737418240, storage_used: 0 }
const roster = ['owner', 'admin', 'contributor', 'viewer'].map(role => ({ user_id: `synthetic-${role}`, username: `${role} · 合成员工`, email: `${role}@example.invalid`, role, status: 'active', joined_at: '2026-10-16T00:00:00Z' }))
const kb = { id: 'synthetic-kb', name: '员工手册 · 合成资料', type: 'document' }
const key = (id, creator, name) => ({ id, name, api_key: 'sk-synthetic-visual-fixture-only-' + id, full_access: false, knowledge_base_ids: [kb.id], capabilities: ['retrieve', 'chat'], created_at: '2026-10-16T00:00:00Z', creator_user_id: creator ? `synthetic-${creator}` : undefined, ownership: creator ? 'other' : 'legacy', restricted: creator === 'admin', disabled: false })
async function scenario(mode, role, delegated = true) {
  directory = `${base}/${mode}/dist`
  const ctx = await browser.newContext({ viewport: { width: 1536, height: 1100 }, locale: 'zh-CN' })
  const user = { id: `synthetic-${role}`, username: `${role} · 合成员工`, email: `${role}@example.invalid`, tenant_id: 42, is_active: true, is_system_admin: false, can_access_all_tenants: false, preferences: {} }
  const memberships = [{ tenant_id: 42, tenant_name: tenant.name, role }]
  let keys = [key(1, 'owner', 'Owner 服务密钥'), key(2, 'admin', 'Admin 自用密钥'), key(3, null, '历史密钥')]
  if (mode === 'after' && role === 'owner') keys.push({ ...key(8, null, '系统开户凭证'), protected: true, api_key: '' }, { ...key(9, 'owner', '已停用密钥'), disabled: true, api_key: '' })
  const calls = []
  await ctx.addInitScript(({ user, tenant, memberships }) => {
    localStorage.setItem('weknora_token', 'synthetic-ui-token')
    localStorage.setItem('weknora_user', JSON.stringify(user))
    localStorage.setItem('weknora_tenant', JSON.stringify(tenant))
    localStorage.setItem('weknora_memberships', JSON.stringify(memberships))
    localStorage.setItem('mindcreek_auth_kind', 'corporate')
    localStorage.setItem('weknora_selected_tenant_id', '42')
    localStorage.setItem('locale', 'zh-CN')
    localStorage.setItem('weknora:new-user-guide-done:v1', '1')
  }, { user, tenant, memberships })
  await ctx.route('**/*', async route => {
    const request = route.request(), u = new URL(request.url()), path = u.pathname, method = request.method()
    const json = data => route.fulfill({ json: data })
    if (u.origin !== origin) { report.unexpectedRequests.push(u.origin); return route.abort() }
    if (!path.startsWith('/api/')) return route.continue()
    calls.push({ path, method })
    if (path === '/api/v1/mindcreek/auth/config') return json({ enterprise_enabled: true })
    if (path === '/api/v1/mindcreek/onboarding') return json({ success: true, data: { state: 'ready', memberships, retryable: false } })
    if (path === '/api/v1/auth/me') return json({ success: true, data: { user, tenant, memberships, active_tenant: tenant, capabilities: { can_create_tenant: false }, mindcreek: { local_admin: false, installation_ready: true } } })
    if (path === '/api/v1/auth/oidc/config') return json({ success: true, data: { enabled: true } })
    if (path === '/api/v1/auth/validate') return json({ success: true, valid: true, data: { valid: true } })
    if (path === '/api/v1/system/info') return json({ success: true, data: { version: 'v0.8.0', edition: 'standard', graph_database_engine: '', keyword_index_engine: 'pg_search', db_version: 86 } })
    if (path === '/api/v1/system/deployment-capabilities') return json({ success: true, data: { edition: 'standard', capabilities: {} } })
    if (path === '/api/v1/models') return json({ success: true, data: [] })
    if (path === '/api/v1/knowledge-bases') return json({ success: true, data: [kb], total: 1 })
    if (path === '/api/v1/tenants/42') return json({ success: true, data: tenant })
    if (path === '/api/v1/tenants/42/members') return json({ success: true, data: { members: roster, total: roster.length } })
    if (path.includes('invitations')) return json({ success: true, data: { invitations: [], total: 0 } })
    if (path.includes('api-principal')) return json({ success: true, data: { mode: 'tenant' } })
    if (path === '/api/v1/mindcreek/workspace-admin/delegation') {
      if (method === 'POST') { assert.deepEqual(request.postDataJSON(), {}); delegated = true }
      if (method === 'DELETE') { delegated = false; return json({ success: true, data: null }) }
      return json({ success: true, data: { configured: true, authorized: delegated, owner_user_id: delegated ? 'synthetic-owner' : undefined } })
    }
    if (path === '/api/v1/tenants/42/api-keys') {
      if (method === 'POST') {
        const payload = request.postDataJSON()
        assert.equal(payload.full_access, false)
        assert.ok(payload.knowledge_base_ids.includes(kb.id))
        const created = { ...key(4, role, payload.name), ...payload }
        keys.push(created)
        return json({ success: true, data: created })
      }
      const visible = role === 'owner' ? keys : keys.filter(row => row.creator_user_id === user.id)
      return json({ success: true, data: visible.map(row => ({ ...row, ownership: row.creator_user_id === user.id ? 'self' : row.ownership })) })
    }
    return json({ success: true, data: [], total: 0 })
  })
  const page = await ctx.newPage(), errors = []
  page.setDefaultTimeout(12000)
  page.on('pageerror', e => errors.push(e.message))
  const shot = async name => { await page.waitForLoadState('networkidle'); await page.screenshot({ path: `${out}/${name}.png`, animations: 'disabled', fullPage: true }); report.screenshots.push(name + '.png') }
  try {
    if (!delegated) {
      await page.goto(`${origin}/platform/settings?section=integration-api`)
      await page.locator('.api-key-section__header button').waitFor()
      assert.equal(await page.locator('.api-key-section__header button').isDisabled(), true)
      assert.equal(await page.getByRole('button', { name: '授权 Admin 管理', exact: true }).count(), role === 'owner' ? 1 : 0)
      await shot(`after-${role}-authorization-required`)
      if (role === 'owner') {
        await page.getByRole('button', { name: '授权 Admin 管理', exact: true }).click()
        await page.getByRole('button', { name: '确认授权', exact: true }).click()
        await page.getByText('已授权：Admin 可使用获准的空间管理功能。', { exact: true }).waitFor()
        assert.equal(await page.locator('.api-key-section__header button').isDisabled(), false)
        await page.getByRole('button', { name: '撤销授权', exact: true }).click()
        await page.getByText('未授权或授权已失效：请由 Owner 授权后创建密钥或使用委托管理操作。', { exact: true }).waitFor()
        assert.equal(await page.locator('.api-key-section__header button').isDisabled(), true)
        assert.equal(await page.getByRole('button', { name: '撤销授权', exact: true }).count(), 0)
        assert.ok(calls.filter(call => call.path.endsWith('/workspace-admin/delegation') && call.method === 'GET').length >= 2)
        await shot('after-owner-authorization-revoked')
      }
    } else if (role === 'contributor' || role === 'viewer') {
      await page.goto(`${origin}/platform/settings?section=integration-api`)
      await page.waitForLoadState('networkidle')
      assert.equal(await page.locator('.api-integration').count(), 0)
      await shot(`after-${role}-api-denied`)
    } else {
      await page.goto(`${origin}/platform/settings?section=members`)
      await page.locator('.members-list-wrap tbody tr').first().waitFor()
      const ownerRow = page.locator('.members-list-wrap tbody tr').filter({ hasText: 'owner · 合成员工' })
      const otherRow = page.locator('.members-list-wrap tbody tr').filter({ hasText: 'contributor · 合成员工' })
      if (role === 'admin') {
        assert.equal(await ownerRow.locator('.member-role-select').count(), 0)
        assert.equal(await otherRow.locator('.member-role-select').count(), mode === 'after' ? 1 : 0)
        assert.equal(await page.getByRole('button', { name: '移交空间所有者', exact: true }).count(), 0)
        assert.equal(await page.getByRole('button', { name: '批量添加员工', exact: true }).count(), mode === 'after' ? 1 : 0)
      }
      await shot(`${mode}-${role}-members`)
      await page.goto(`${origin}/platform/settings?section=tenant`)
      await page.locator('.tenant-info').waitFor()
      await shot(`${mode}-${role}-space-config`)
      await page.goto(`${origin}/platform/settings?section=integration-api`)
      if (mode === 'before' && role === 'admin') {
        await page.waitForLoadState('networkidle'); assert.equal(await page.locator('.api-integration').count(), 0)
      } else {
        await page.locator('.api-key-table').waitFor()
        assert.equal(await page.locator('.api-key-table tbody tr').count(), role === 'owner' ? (mode === 'after' ? 5 : 3) : 1)
        if (mode === 'after') {
          assert.equal(await page.locator('.principal-section').count(), role === 'owner' ? 1 : 0)
          if (role === 'owner') {
            const protectedButtons = page.locator('.api-key-table tr').filter({ hasText: '系统开户凭证' }).locator('button')
            for (const button of await protectedButtons.all()) assert.equal(await button.isDisabled(), true)
            const disabledButtons = page.locator('.api-key-table tr').filter({ hasText: '已停用密钥' }).locator('button')
            assert.equal(await disabledButtons.nth(0).isDisabled(), true)
            assert.equal(await disabledButtons.nth(1).isDisabled(), true)
            assert.equal(await disabledButtons.nth(2).isDisabled(), false)
          }
          if (role === 'admin') assert.equal(calls.some(call => call.path.includes('api-principal')), false)
          await page.getByText(role === 'owner' ? '全部 API Keys' : '我的 API Keys', { exact: true }).waitFor()
        }
      }
      await shot(`${mode}-${role}-api-keys`)
      if (mode === 'after') {
        await page.locator('.api-key-section__header button').click()
        await page.locator('.api-key-create-drawer .api-key-dialog').waitFor()
        const radioValues = await page.locator('.api-key-create-drawer input[type=radio]').evaluateAll(nodes => nodes.map(node => node.value))
        assert.equal(radioValues.includes('full'), role === 'owner')
        await shot(`after-${role}-create-key`)
        if (role === 'admin') {
          const drawer = page.locator('.api-key-create-drawer')
          await drawer.locator('.api-key-dialog-row').first().locator('input').fill('Admin 浏览器创建密钥')
          await drawer.locator('.setting-drawer__footer-right button').last().click()
          await page.getByText('请选择至少一个获准的知识库。Admin 密钥必须限定知识库范围。', { exact: true }).waitFor()
          assert.equal(calls.some(call => call.path.endsWith('/api-keys') && call.method === 'POST'), false)
          await drawer.locator('.t-select').click()
          await page.locator('.t-select-option').filter({ hasText: kb.name }).click()
          await drawer.locator('.api-key-dialog-row').first().click()
          await drawer.locator('.setting-drawer__footer-right button').last().click()
          await page.locator('.api-key-table').getByText('Admin 浏览器创建密钥', { exact: true }).waitFor()
          assert.equal(calls.filter(call => call.path.endsWith('/api-keys') && call.method === 'POST').length, 1)
        }
      }
    }
    assert.deepEqual(errors, [])
    report.checks.push({ mode, role, delegated, passed: true })
  } catch (error) {
    await shot(`failure-${mode}-${role}`)
    console.error((await page.locator('body').innerText()).slice(0, 4000)); throw error
  } finally { await ctx.close() }
}
try {
  for (const mode of ['before', 'after']) for (const role of ['owner', 'admin']) await scenario(mode, role)
  for (const role of ['contributor', 'viewer']) await scenario('after', role)
  for (const role of ['owner', 'admin']) await scenario('after', role, false)
  assert.deepEqual(report.unexpectedRequests, [])
  writeFileSync(`${base}/browser-report.json`, JSON.stringify(report, null, 2) + '\n')
  console.log(JSON.stringify({ passed: report.checks.length, screenshots: report.screenshots.length, unexpectedRequests: report.unexpectedRequests.length }))
} finally { await browser.close(); await new Promise(r => server.close(r)) }
