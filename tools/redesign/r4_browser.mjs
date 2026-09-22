// Real overlaid Vue pages against the disposable gateway/native database.
// No API interception or response fixtures are installed in these scenarios.
import { createRequire } from 'node:module'
import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs'
import { createHash } from 'node:crypto'
const { chromium } = createRequire(import.meta.url)('playwright')
const root = process.env.R4_ROOT
const origin = process.env.R4_UI_ORIGIN
const fixture = JSON.parse(readFileSync(process.env.R4_BROWSER_SECRETS, 'utf8'))
const uiBuild = JSON.parse(readFileSync(`${root}/docs/plans/evidence/r4-ui-build.json`, 'utf8'))
const out = `${root}/docs/assets/r4`
mkdirSync(out, { recursive: true })
const report = { ui_source_sha256: uiBuild.ui_source_sha256, bundle_sha256: uiBuild.bundle_sha256, scope: 'Actual Vue browser + product gateway + pinned native API/DB; synthetic employees/models only. Employee tokens established with synthetic OAuth harness; not real enterprise OAuth.', checks: [], requests: [], screenshots: [], probe_sha256: createHash('sha256').update(readFileSync(import.meta.filename)).digest('hex') }
const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const filterPath = `${root}/.local/redesign-r4/browser-only`
const developmentCases = process.env.R4_BROWSER_ITERATE === '1' && existsSync(filterPath) ? readFileSync(filterPath, 'utf8').trim().split(',') : null
async function scenario(name, role, run) {
  if (developmentCases && !developmentCases.includes(name)) return
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1000 }, locale: 'zh-CN' })
  // Browser access is restricted to this synthetic loopback origin.
  await ctx.route('**/*', route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort())
  await ctx.addInitScript(({ actor, tenant }) => {
    if (actor && !sessionStorage.getItem('r4-seeded')) {
      localStorage.setItem('weknora_token', actor.token)
      localStorage.setItem('mindcreek_auth_kind', 'corporate')
      localStorage.setItem('weknora_selected_tenant_id', String(tenant))
      sessionStorage.setItem('r4-seeded', '1')
    }
    localStorage.setItem('locale', 'zh-CN')
    localStorage.setItem('weknora:new-user-guide-done:v1', '1')
    for (const key of ['kb-list:v2','kb-create:v3','tenant-models:v1','kb-detail:v1','chat:v1','agent-list:v1','agent-create:v1']) localStorage.setItem('weknora:contextual-guide-' + key, '1')
  }, { actor: role ? fixture.actors[role] : null, tenant: fixture.tenant })
  const page = await ctx.newPage(), errors = [], diagnostics = []
  page.on('console', message => { diagnostics.push(message.text()); if (diagnostics.length > 25) diagnostics.shift() })
  page.on('pageerror', error => errors.push(error.message))
  page.on('response', response => {
    const url = new URL(response.url())
    if (url.pathname.startsWith('/api/')) report.requests.push({ scenario: name, path: url.pathname, method: response.request().method(), status: response.status() })
  })
  const shot = async (file, settled = true) => {
    if (settled) {
      await page.waitForLoadState('networkidle')
      await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
    }
    await page.screenshot({ path: `${out}/${file}`, fullPage: true, animations: 'disabled' })
    report.screenshots.push(`docs/assets/r4/${file}`)
  }
  try {
    await run(page, shot, ctx)
    if (errors.length) throw new Error(`Page errors: ${errors.join('; ').slice(0,500)}`)
    report.checks.push({ name, passed: true }); console.log(`PASS ${name}`)
  } catch (error) {
    await shot(`failure-${name.replaceAll('.', '-')}.png`, false)
    console.error('PAGE', page.url(), (await page.locator('body').innerText()).slice(0,2400))
    console.error('LAST REQUESTS', JSON.stringify(report.requests.slice(-15)))
    console.error('BROWSER ERRORS', JSON.stringify(errors), JSON.stringify(diagnostics))
    throw error
  } finally { await ctx.close() }
}
const goto = (page, path) => page.goto(origin + path)
const waitAPI = (page, path, method = 'POST') => page.waitForResponse(r => new URL(r.url()).pathname === path && r.request().method() === method)
try {
  for (const role of ['owner', 'admin', 'contributor', 'viewer']) {
    await scenario(`navigation.${role}`, role, async (page, shot) => {
      await goto(page, '/')
      if (role === 'viewer') {
        await page.waitForURL('**/platform/creatChat')
        await page.locator('[data-guide="chat-input"]').waitFor()
        if (await page.locator('[data-guide="nav-knowledge-bases"], [data-guide="nav-agents"]').count()) throw new Error('Viewer management navigation exposed')
      } else {
        await page.waitForURL('**/platform/knowledge-bases')
        await page.getByText('员工手册 · 合成验证', { exact: true }).first().waitFor()
        await page.locator('[data-guide="kb-list-create"]').first().waitFor()
      }
      await shot(`after-${role}.png`)
      if (role === 'viewer') {
        for (const path of ['/platform/agents', `/platform/knowledge-bases/${fixture.kb}`, '/platform/settings?section=members']) {
          await goto(page, path); await page.waitForURL('**/platform/creatChat')
        }
        await page.reload(); await page.waitForURL('**/platform/creatChat')
      }
    })
  }
  await scenario('kb.native_create_and_settings', 'contributor', async (page, shot) => {
    await goto(page, '/platform/knowledge-bases')
    await page.locator('[data-guide="kb-list-create"]').first().click()
    await page.locator('[data-guide="kb-create-name"] input').fill('浏览器新建知识库')
    await shot('after-create-knowledge-base.png')
    const created = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/knowledge-bases' && r.request().method() === 'POST')
    const [response] = await Promise.all([created, page.locator('[data-guide="kb-create-submit"]').click()])
    const body = await response.json()
    if (!response.ok() || body.data?.summary_model_id !== 'builtin-mindcreek-chat') throw new Error('Native KB/default model create failed')
    await goto(page, `/platform/knowledge-bases/${body.data.id}`)
    await page.getByText('浏览器新建知识库', { exact: true }).first().waitFor()
    await shot('after-knowledge-detail.png')
  })
  await scenario('kb.native_upload', 'owner', async (page, shot, ctx) => {
    await goto(page, `/platform/knowledge-bases/${fixture.kb}`)
    await page.getByText('合成操作指南', { exact: true }).first().waitFor()
    const uploadName = `r4-browser-upload-${Date.now()}`
    await page.locator('.kb-upload-source-dropdown input[type=file]').first().setInputFiles({ name: uploadName + '.md', mimeType: 'text/markdown', buffer: Buffer.from(`# R4 browser upload ${uploadName}\n\nSynthetic employee operations guide. Recovery code R4-VERIFY.`) })
    await page.getByRole('button', { name: '确认上传并解析', exact: true }).waitFor()
    await page.getByRole('button', { name: /解析引擎/ }).click()
    await page.locator('.kb-parser-settings:visible .t-select').click()
    await page.getByText('Simple', { exact: true }).last().click()
    await shot('after-native-upload-confirm.png')
    const [response] = await Promise.all([waitAPI(page, `/api/v1/knowledge-bases/${fixture.kb}/knowledge/file`), page.getByRole('button', { name: '确认上传并解析', exact: true }).click()])
    if (!response.ok()) throw new Error(`Native upload failed ${response.status()}`)
    const uploaded = (await response.json()).data
    await page.getByText(uploadName, { exact: true }).first().waitFor()
    let document
    for (let attempt = 0; attempt < 90; attempt++) {
      document = (await (await ctx.request.get(origin + `/api/v1/knowledge/${uploaded.id}`, { headers: { Authorization: `Bearer ${fixture.actors.owner.token}`, 'X-Tenant-ID': String(fixture.tenant) } })).json()).data
      if (['completed', 'failed'].includes(document.parse_status)) break
      await page.waitForTimeout(1000)
    }
    if (document.parse_status !== 'completed') throw new Error(`Native browser upload processing ${document.parse_status}: ${document.error_message || ''}`)
    await shot('after-native-upload.png')
  })
  await scenario('faq.native_page', 'owner', async (page, shot) => {
    await goto(page, `/platform/knowledge-bases/${fixture.faq}`)
    await page.getByText('合成恢复码是什么？', { exact: true }).first().waitFor()
    await shot('after-faq.png')
  })
  await scenario('agent.native_editor', 'contributor', async (page, shot) => {
    await goto(page, '/platform/agents')
    await page.locator('[data-guide="agent-list-create"]').first().click()
    await page.locator('[data-guide="agent-create-submit"]').waitFor()
    await page.locator('[data-guide="agent-create-name"] input').fill('浏览器创建的智能体')
    await shot('after-agent-editor.png')
    const created = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/agents' && r.request().method() === 'POST')
    const [response] = await Promise.all([created, page.locator('[data-guide="agent-create-submit"]').click()])
    if (!response.ok()) throw new Error('Native Agent create failed')
  })
  await scenario('viewer.native_chat_and_history', 'viewer', async (page, shot) => {
    await goto(page, '/platform/creatChat')
    await page.getByText('mindcreek-test-chat', { exact: true }).first().waitFor()
    await page.waitForLoadState('networkidle')
    await page.locator('[data-guide="chat-input"] textarea').fill('What is the synthetic recovery code?')
    const answer = page.waitForResponse(r => /\/api\/v1\/(knowledge-chat|agent-chat)\//.test(new URL(r.url()).pathname), { timeout: 45000 })
    const [response] = await Promise.all([answer, page.locator('[data-guide="chat-send"]').click()])
    if (!response.ok()) throw new Error(`Native chat failed ${response.status()}`)
    await page.waitForURL('**/platform/chat/*')
    await page.locator('[data-guide="chat-input"]').waitFor()
    await page.locator('.bot_msg .markdown-content').filter({ hasText: 'MindCreek synthetic answer grounded in the retrieved knowledge.' }).first().waitFor()
    await shot('after-viewer-conversation.png')
    await page.keyboard.press('Meta+k')
    await page.getByText('搜索我的对话', { exact: true }).waitFor()
    await page.getByPlaceholder('输入历史对话中的关键词').fill('synthetic')
    const search = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/messages/search')
    await page.getByRole('button', { name: '搜索', exact: true }).click()
    const searchResponse = await search
    if (!searchResponse.ok() || searchResponse.request().postDataJSON().mode !== 'keyword') throw new Error('Scoped keyword search failed')
  })
  await scenario('members.batch_preview_and_repeat', 'owner', async (page, shot) => {
    await goto(page, '/platform/settings?section=members')
    await page.getByRole('button', { name: '批量添加员工', exact: true }).click()
    await page.getByLabel('员工邮箱', { exact: true }).fill('email\nr4-owner@example.invalid\nr4-new@example.invalid\nr4-new@example.invalid\nnot-registered@example.invalid')
    await page.getByRole('button', { name: '预览成员', exact: true }).click()
    await page.getByText('有效邮箱 3 个', { exact: false }).waitFor()
    await shot('after-member-preview.png')
    await page.getByRole('button', { name: '确认添加', exact: true }).click()
    await page.getByText('处理完成：成功 1', { exact: false }).waitFor()
    await page.getByRole('button', { name: '核对并重试失败项', exact: true }).and(page.locator(':not([disabled])')).waitFor()
    const count = report.requests.filter(r => r.scenario === 'members.batch_preview_and_repeat' && r.method === 'POST' && r.path.endsWith('/members')).length
    await page.getByRole('button', { name: '核对并重试失败项', exact: true }).click()
    await page.getByText('处理完成：成功 1', { exact: false }).waitFor()
    await page.getByRole('button', { name: '核对并重试失败项', exact: true }).and(page.locator(':not([disabled])')).waitFor()
    if (report.requests.filter(r => r.scenario === 'members.batch_preview_and_repeat' && r.method === 'POST' && r.path.endsWith('/members')).length !== count) throw new Error('Repeated batch rewrote successful members')
    await shot('after-member-results.png')
  })
  await scenario('admin.real_login', null, async (page, shot) => {
    await goto(page, '/admin/login')
    await page.getByLabel('邮箱', { exact: true }).fill('admin@example.invalid')
    await page.getByLabel('密码', { exact: true }).fill(fixture.password)
    await page.getByRole('button', { name: '登录', exact: true }).click()
    await page.waitForURL('**/platform/knowledge-bases')
    await page.getByText('员工手册 · 合成验证', { exact: true }).first().waitFor()
    await shot('after-local-admin-session.png')
  })
  await scenario('admin.create_switch_and_expiry', null, async (page, shot, ctx) => {
    await goto(page, '/admin/login')
    await page.getByLabel('邮箱', { exact: true }).fill('admin@example.invalid')
    await page.getByLabel('密码', { exact: true }).fill(fixture.password)
    await page.getByRole('button', { name: '登录', exact: true }).click()
    await page.waitForURL('**/platform/knowledge-bases')
    const token = await page.evaluate(() => localStorage.getItem('weknora_token'))
    const me = await ctx.request.get(origin + '/api/v1/auth/me', { headers: { Authorization: `Bearer ${token}`, 'X-Tenant-ID': String(fixture.tenant) } })
    const defaultName = (await me.json()).data.memberships.find(m => m.tenant_id === fixture.tenant).tenant_name
    await page.locator('[data-guide="user-menu"]').click()
    await page.locator('.dropdown-tenant-panel').hover()
    await page.locator('.tenant-submenu-create').click()
    await page.getByPlaceholder('例如：我的新项目').fill('浏览器创建的团队空间')
    const [created] = await Promise.all([waitAPI(page, '/api/v1/tenants'), page.getByRole('button', { name: '创建', exact: true }).click()])
    if (!created.ok()) throw new Error('Platform create workspace failed')
    const space = (await created.json()).data.id
    await page.waitForFunction(id => localStorage.getItem('weknora_selected_tenant_id') === String(id), space)
    await page.locator('[data-guide="kb-list-create"]').first().waitFor()
    await shot('after-platform-create-space.png')
    await page.locator('[data-guide="user-menu"]').click()
    await page.locator('.dropdown-tenant-panel').hover()
    await page.locator('.tenant-submenu-item').filter({ hasText: defaultName }).click()
    await page.waitForFunction(id => localStorage.getItem('weknora_selected_tenant_id') === String(id), fixture.tenant)
    await page.getByText('员工手册 · 合成验证', { exact: true }).first().waitFor()
    const revoked = await ctx.request.post(origin + '/api/v1/mindcreek/admin/auth/logout', { headers: { Authorization: `Bearer ${token}` }, data: {} })
    if (!revoked.ok()) throw new Error('Admin session revocation failed')
    await page.reload(); await page.waitForURL('**/admin/login')
    await shot('after-admin-expired.png')
  })
  await scenario('roles.refresh_downgrade', 'contributor', async (page, shot, ctx) => {
    const headers = { Authorization: `Bearer ${fixture.actors.owner.token}`, 'X-Tenant-ID': String(fixture.tenant) }
    const path = origin + `/api/v1/tenants/${fixture.tenant}/members/${fixture.actors.contributor.id}`
    await goto(page, '/platform/agents')
    await page.locator('[data-guide="agent-list-create"]').first().waitFor()
    try {
      const lower = await ctx.request.put(path, { headers, data: { role: 'viewer' } })
      if (!lower.ok()) throw new Error('Role downgrade fixture failed')
      await page.evaluate(() => window.dispatchEvent(new Event('focus')))
      await page.waitForURL('**/platform/creatChat')
      if (await page.locator('[data-guide="nav-knowledge-bases"], [data-guide="nav-agents"]').count()) throw new Error('Stale management navigation after downgrade')
      await shot('after-role-refresh.png')
    } finally {
      const restored = await ctx.request.put(path, { headers, data: { role: 'contributor' } })
      if (!restored.ok()) throw new Error('Restore synthetic contributor failed')
    }
  })
  await scenario('members.removed_relogin_restricted', 'removed', async (page, shot) => {
    await goto(page, '/')
    await page.waitForURL('**/onboarding/workspace')
    await page.getByText('你目前没有可访问的空间', { exact: false }).waitFor()
    await page.getByRole('button', { name: '刷新状态', exact: true }).click()
    await page.getByText('你目前没有可访问的空间', { exact: false }).waitFor()
    if (report.requests.some(r => r.scenario === 'members.removed_relogin_restricted' && r.path.endsWith('/onboarding') && r.method === 'POST')) throw new Error('Removed employee was re-onboarded')
    await shot('after-member-removed.png')
  })
  await scenario('members.two_step_transfer_and_reload', 'owner', async (page, shot, ctx) => {
    await goto(page, '/platform/settings?section=members')
    await page.getByRole('button', { name: '移交空间所有者', exact: true }).click()
    await page.getByLabel(/^目标所有者/).selectOption(fixture.actors.viewer.id)
    await page.getByRole('button', { name: '第一步：确认目标成为 Owner', exact: true }).click()
    await page.getByText('第一步已完成', { exact: false }).waitFor()
    await page.reload()
    await page.getByText('第一步已完成', { exact: false }).waitFor()
    await shot('after-transfer-recovery.png')
    await page.getByRole('button', { name: '第二步：确认自己成为 Admin', exact: true }).click()
    await page.waitForURL('**/platform/knowledge-bases')
    const me = await ctx.request.get(origin + '/api/v1/auth/me', { headers: { Authorization: `Bearer ${fixture.actors.owner.token}`, 'X-Tenant-ID': String(fixture.tenant) } })
    const body = await me.json()
    if (!body.data.memberships.some(m => m.tenant_id === fixture.tenant && m.role === 'admin') || body.data.user.is_system_admin) throw new Error('Ownership transfer changed platform identity or failed')
  })
  await scenario('retired.bookmark', 'owner', async (page, shot) => {
    await goto(page, '/platform/mindcreek/notes/old')
    await page.getByRole('heading', { name: '此功能已退役' }).waitFor()
    await shot('after-retired-link.png')
  })
  if (report.requests.some(r => /\/mindcreek\/(catalog|knowledge-bases|publications|me\/subscriptions)|\/product-profile|\/ingestions/.test(r.path))) throw new Error('Current UI used a retired endpoint')
  report.status = developmentCases ? 'development-subset-passed' : 'passed'
} catch (error) { report.status = 'failed'; report.error = String(error); throw error }
finally { await browser.close(); writeFileSync(`${root}/docs/plans/evidence/r4-browser.json`, JSON.stringify(report, null, 2) + '\n') }
