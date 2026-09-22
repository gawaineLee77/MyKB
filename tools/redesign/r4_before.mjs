// Four-role screenshots of the actual pre-R4 bundle, with synthetic HTTP fixtures.
import { createRequire } from 'node:module'
import { mkdirSync, writeFileSync } from 'node:fs'
const { chromium } = createRequire(import.meta.url)('playwright')
const root = process.env.R4_ROOT || process.cwd()
const origin = 'http://127.0.0.1:14824'
const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const report = { scope: 'Pre-R4 actual Vue bundle; synthetic responses; not live OAuth', checks: [] }
mkdirSync(`${root}/docs/assets/r4`, { recursive: true })
try {
  for (const role of ['owner', 'admin', 'contributor', 'viewer']) {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: 'zh-CN' })
    await context.addInitScript(() => {
      localStorage.setItem('weknora_token', 'synthetic-r4-before')
      localStorage.setItem('locale', 'zh-CN')
      localStorage.setItem('weknora_guide_completed', 'true')
    })
    await context.route('**/*', async route => {
      const url = new URL(route.request().url()), path = url.pathname
      if (url.origin !== origin) return route.abort()
      if (!path.startsWith('/api/')) return route.continue()
      const reply = data => route.fulfill({ contentType: 'application/json', body: JSON.stringify(data) })
      const memberships = [{ tenant_id: 42, tenant_name: '默认空间', role }]
      if (path.endsWith('/mindcreek/auth/config')) return reply({ enterprise_enabled: true })
      if (path.endsWith('/auth/me')) return reply({ success: true, data: { user: { id: role, username: `合成员工 · ${role}`, email: `${role}@example.invalid`, tenant_id: 42, is_active: true }, tenant: { id: 42, name: '默认空间' }, memberships, capabilities: { can_create_tenant: false }, mindcreek: { local_admin: false, installation_ready: true } } })
      if (path.endsWith('/mindcreek/onboarding')) return reply({ success: true, data: { state: 'ready', memberships, retryable: false } })
      if (path.includes('/mindcreek/knowledge-bases')) return reply({ items: [], total: 0, page: 1, page_size: 24 })
      if (path.includes('pending-count')) return reply({ success: true, data: { pending_count: 0 } })
      return reply({ success: true, data: [], items: [], total: 0, capabilities: {} })
    })
    const page = await context.newPage()
    await page.goto(origin + '/platform/knowledge-bases')
    await page.getByRole('heading', { name: '清晰管理你的知识' }).waitFor()
    await page.screenshot({ path: `${root}/docs/assets/r4/before-${role}.png`, fullPage: true })
    report.checks.push({ role, screenshot: `docs/assets/r4/before-${role}.png`, passed: true })
    await context.close()
  }
  report.status = 'passed'
} finally {
  await browser.close()
  writeFileSync(`${root}/docs/plans/evidence/r4-before.json`, JSON.stringify(report, null, 2) + '\n')
}
