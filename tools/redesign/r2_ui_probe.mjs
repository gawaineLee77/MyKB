// Browser checks use real overlaid Vue pages and synthetic responses only.
import { createRequire } from 'node:module'
import { mkdirSync, writeFileSync, readFileSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { resolve } from 'node:path'
const { chromium } = createRequire(import.meta.url)('playwright')
const root = process.env.R2_REPORT_ROOT || resolve(import.meta.dirname, '../..')
const output = resolve(root, 'docs/assets/r2')
mkdirSync(output, { recursive: true })
mkdirSync(resolve(root, 'docs/plans/evidence'), { recursive: true })
const browser = await chromium.launch({ headless: true, executablePath: process.env.R2_CHROME_EXECUTABLE })
const report = { time: new Date().toISOString(), probe_sha256: createHash('sha256').update(readFileSync(import.meta.filename)).digest('hex'), scope: 'Real Vue pages with mocked HTTP responses; not real enterprise OAuth/browser deployment acceptance. Before: unchanged corporate login with R2 flag disabled; after: R2 enabled', checks: [], screenshots: [] }
async function scenario(name, state, run) {
  if (process.env.R2_UI_CASE && name !== process.env.R2_UI_CASE) return
  const before = state === 'before'
  const origin = process.env.R2_UI_ORIGIN || 'http://127.0.0.1:14821'
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 }, locale: 'zh-CN' })
  const signed = !['before', 'login', 'sso'].includes(state)
  const admin = ['setup', 'expired'].includes(state)
  let loggedIn = signed
  let joined = ['ready', 'invite'].includes(state)
  const errors = []
  await ctx.addInitScript(({ signed, admin, before }) => {
    if (before) sessionStorage.setItem('mindcreek_sso_attempted_at', String(Date.now()))
    localStorage.setItem('locale', 'zh-CN')
    if (signed) {
      localStorage.setItem('weknora_token', 'synthetic-access')
      localStorage.setItem('weknora_refresh_token', 'synthetic-refresh')
      localStorage.setItem('mindcreek_auth_kind', admin ? 'local-admin' : 'corporate')
    }
  }, { signed, admin, before })
  const requests = []
  await ctx.route('**/*', async route => {
    const url = new URL(route.request().url()), path = url.pathname
    if (url.origin !== origin) return route.abort()
    if (!path.startsWith('/api/') && path !== '/mcp') return route.continue()
    requests.push({ path, method: route.request().method() })
    const reply = (data, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(data) })
    if (path === '/api/v1/mindcreek/auth/config') return reply({ enterprise_enabled: !before }, before ? 404 : 200)
    if (path === '/api/v1/auth/oidc/config') return reply({ success: true, enabled: true, provider_display_name: '合成企业账号' })
    if (path === '/api/v1/auth/oidc/url') return state === 'before'
      ? reply({ message: 'Synthetic before-state sign-in unavailable' }, 503)
      : reply({ success: true, authorization_url: 'http://gateway:8080/api/v1/mindcreek/oidc/authorize?synthetic=1' })
    if (path === '/api/v1/mindcreek/oidc/authorize') return route.fulfill({ contentType: 'text/html; charset=utf-8', body: '<h1>合成企业登录</h1>' })
    const user = { id: admin || state === 'login' ? 'admin' : 'employee', username: 'Synthetic user', email: 'synthetic@example.invalid', is_active: true, is_system_admin: admin || state === 'login', tenant_id: 0 }
    if (path === '/api/v1/mindcreek/admin/auth/login') {
      const body = route.request().postDataJSON()
      if (body.email !== 'admin@example.invalid' || body.password !== 'Synthetic-only!42') throw new Error('login DTO changed')
      loggedIn = true
      return reply({ success: true, token: 'synthetic-admin-access', refresh_token: 'synthetic-admin-refresh', user, tenant: null, memberships: [] })
    }
    if (path === '/api/v1/mindcreek/admin/auth/refresh') return reply({ error: 'expired' }, 401)
    if (path === '/api/v1/auth/me') {
      if (!loggedIn || state === 'expired') return reply({ error: 'expired' }, 401)
      const memberships = joined ? [{ tenant_id: 42, tenant_name: '默认空间', role: 'viewer' }] : []
      return reply({ success: true, data: { user, tenant: joined ? { id: 42, name: '默认空间' } : null, memberships, capabilities: { can_create_tenant: user.is_system_admin }, mindcreek: { enterprise_enabled: true, local_admin: user.is_system_admin, installation_ready: !['setup', 'login'].includes(state) } } })
    }
    if (path === '/api/v1/me/invitations/accept-by-token') {
      if (route.request().postDataJSON().token !== 'synthetic-invite') throw new Error('Invitation token lost')
      return reply({ success: true, data: { membership: { tenant_id: 42, role: 'viewer' }, tenant_name: '默认空间' } })
    }
    if (path === '/api/v1/mindcreek/installation') return reply({ success: true, data: { stage: 'account_ready' } })
    if (path === '/api/v1/mindcreek/onboarding' && state === 'pending' && route.request().method() === 'POST') joined = true
    if (path === '/api/v1/mindcreek/onboarding') return reply({ success: true, data: { state: state === 'failed' ? 'failed' : joined ? 'ready' : state === 'pending' ? 'pending' : 'removed', memberships: joined ? [{ tenant_id: 42, role: 'viewer' }] : [], retryable: state === 'failed' } })
    if (path === '/api/v1/knowledge-bases') return reply({ success: true, data: [], total: 0 })
    return reply({ success: true, data: { capabilities: {}, items: [], memberships: [] } })
  })
  const page = await ctx.newPage()
  page.on('pageerror', error => errors.push(error.message))
  try {
    await run(page, origin, async file => {
      await page.screenshot({ path: resolve(output, file), fullPage: true })
      report.screenshots.push(`docs/assets/r2/${file}`)
    })
    if (state === 'pending' && requests.filter(r => r.path === '/api/v1/mindcreek/onboarding' && r.method === 'POST').length !== 1) throw new Error('Expected one automatic onboarding request')
    if (state === 'ready' && requests.some(r => r.path.endsWith('/oidc/url') || (r.path.endsWith('/onboarding') && r.method === 'POST'))) throw new Error('Existing session restarted login or onboarding')
    if (state === 'invite' && requests.filter(r => r.path.endsWith('/accept-by-token')).length !== 1) throw new Error('Invitation was not accepted exactly once')
    if (errors.length) throw new Error(`${name}: ${errors.join('; ')}`)
    report.checks.push({ name, passed: true, requests })
    process.stdout.write(`PASS ${name}\n`)
  } catch (error) {
    report.checks.push({ name, passed: false, requests, page_errors: errors })
    console.error('Failed page:', page.url(), (await page.locator('body').innerText()).slice(0,700))
    console.error('Requests:', JSON.stringify(requests))
    throw error
  } finally { await ctx.close() }
}
try {
  await scenario('before.employee_login', 'before', async (page, origin, shot) => {
    await page.goto(origin + '/login'); await page.getByRole('heading', { name: '使用组织账号登录' }).waitFor()
    await shot('before-employee-login.png')
  })
  await scenario('admin.login_and_setup', 'login', async (page, origin, shot) => {
    await page.goto(origin + '/admin/login'); await page.getByRole('heading', { name: '管理员登录' }).waitFor()
    await shot('after-admin-login.png')
    await page.getByLabel('邮箱', { exact: true }).fill('admin@example.invalid')
    await page.getByLabel('密码', { exact: true }).fill('Synthetic-only!42')
    await page.getByRole('button', { name: '登录', exact: true }).click()
    await page.getByRole('heading', { name: '企业空间初始化' }).waitFor()
    await shot('after-installation-status.png')
  })
  for (const state of ['removed', 'failed']) await scenario('employee.' + state, state, async (page, origin, shot) => {
    await page.goto(origin + '/onboarding/workspace'); await page.getByRole('heading', { name: '企业空间', exact: true }).waitFor()
    const expected = state === 'removed' ? '你目前没有可访问的空间' : '暂时无法加入默认空间'
    await page.getByText(expected, { exact: false }).waitFor()
    if (await page.locator('input[type=password]').count()) throw new Error('Employee password form exposed')
    await shot(`after-employee-${state}.png`)
  })
  await scenario('admin.expiration_returns_admin_login', 'expired', async (page, origin) => {
    await page.goto(origin + '/'); await page.getByRole('heading', { name: '管理员登录' }).waitFor()
    if (!page.url().endsWith('/admin/login')) throw new Error('Wrong expired admin landing')
  })
  await scenario('root.starts_corporate_login', 'sso', async (page, origin) => {
    await page.goto(origin + '/'); await page.getByRole('heading', { name: '合成企业登录' }).waitFor()
  })
  for (const state of ['ready', 'pending']) await scenario('employee.' + state + '_enters_workspace', state, async (page, origin) => {
    await page.goto(origin + '/')
    await page.waitForURL('**/platform/knowledge-bases')
  })
  await scenario('employee.signed_in_invitation', 'invite', async (page, origin) => {
    await page.goto(origin + '/register?token=synthetic-invite')
    await page.waitForURL('**/platform/knowledge-bases')
  })
  report.status = 'passed'
} catch (error) { report.status = 'failed'; report.error = String(error); throw error }
finally { await browser.close(); writeFileSync(resolve(root, 'docs/plans/evidence/r2-ui-probe.json'), JSON.stringify(report, null, 2) + '\n') }
