// Real compiled chat UI, synthetic history/API fixtures, and real browser clicks.
// This intentionally covers HTML anchors in answers, not SVG/Mermaid links.
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { createRequire } from 'node:module'
import { existsSync, statSync, readFileSync, mkdirSync, writeFileSync } from 'node:fs'
import { resolve, extname } from 'node:path'

const { chromium } = createRequire(import.meta.url)('playwright')
const base = resolve(process.env.CHAT_LINKS_UI || '.local/chat-links-20261022')
const before = resolve(process.env.CHAT_LINKS_BEFORE || `${base}/before`)
const after = resolve(process.env.CHAT_LINKS_AFTER || `${base}/after`)
mkdirSync(`${base}/screenshots`, { recursive: true })
let directory = `${before}/dist`
const report = { scope: 'Actual compiled product Vue ordinary and Agent answer components; synthetic history, API responses and destination pages. No external network or deployed service.', checks: [], screenshots: [], pageErrors: [], unexpectedRequests: [], linkAttributes: [] }
const server = createServer((req, res) => {
  let path = resolve(directory, '.' + new URL(req.url, 'http://localhost').pathname)
  if (!path.startsWith(directory + '/') || !existsSync(path) || !statSync(path).isFile()) path = `${directory}/index.html`
  res.setHeader('Content-Type', ({ '.html': 'text/html', '.js': 'application/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.json': 'application/json' })[extname(path)] || 'application/octet-stream')
  res.end(readFileSync(path))
})
await new Promise(done => server.listen(0, '127.0.0.1', done))
const origin = `http://127.0.0.1:${server.address().port}`
const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })
const tenant = { id: 42, name: '链接验收 · 合成空间', status: 'active', storage_quota: 10737418240, storage_used: 0 }
const user = { id: 'synthetic-owner', username: '合成 Owner', email: 'owner@example.invalid', tenant_id: 42, is_active: true, preferences: {} }
const memberships = [{ tenant_id: 42, tenant_name: tenant.name, role: 'owner' }]
const kb = { id: 'synthetic-kb', tenant_id: 42, name: '合成知识库', creator_id: user.id, type: 'document', embedding_model_id: 'embedding', summary_model_id: 'chat', is_initialized: true, knowledge_count: 1, chunk_count: 1, created_at: '2026-10-22T00:00:00Z', updated_at: '2026-10-22T00:00:00Z', chunking_config: {}, indexing_strategy: { vector_enabled: true, keyword_enabled: true } }
const modelConfig = { agent_mode: 'quick-answer', model_id: 'chat', kb_selection_mode: 'all', knowledge_bases: [], allowed_tools: [], memory_enabled: false, mcp_selection_mode: 'none', skills_selection_mode: 'none', web_search_enabled: false }
const agents = [{ id: 'builtin-quick-answer', name: '快速问答', is_builtin: true, is_enabled: true, config: modelConfig }, { id: 'synthetic-agent', name: '合成智能体', is_builtin: false, is_enabled: true, creator_id: user.id, config: { ...modelConfig, agent_mode: 'agent' } }]
const resource = 'resource://AbCdEfGhIjKlMnOpQrStUv'
const picture = '<svg xmlns="http://www.w3.org/2000/svg" width="320" height="100"><rect width="320" height="100" rx="12" fill="#d8eee3"/><text x="160" y="57" text-anchor="middle" font-family="sans-serif" font-size="20" fill="#27644a">Synthetic image preview</text></svg>'
const reference = { id: '11111111-1111-4111-8111-111111111111', knowledge_id: 'synthetic-document', knowledge_base_id: kb.id, knowledge_title: '合成参考文档.pdf', knowledge_filename: '合成参考文档.pdf', chunk_index: 0, chunk_type: 'text', content: '这是保留引用抽屉交互的合成内容。', score: 0.9 }
const links = [
  ['HTTPS 文档', 'https://links.example.invalid/https'],
  ['HTTP 文档', 'http://links.example.invalid/http'],
  ['相对地址', '/synthetic-destination/relative'],
  ['同站绝对地址', `${origin}/synthetic-destination/same-origin`],
  ['原始 HTML', 'https://links.example.invalid/raw'],
  ['原有 self', 'https://links.example.invalid/self'],
]
const markdown = `## 链接打开行为\n\n${links.slice(0, 4).map(([label, href]) => `[${label}](${href})`).join(' · ')}\n\n<a href="${links[4][1]}">原始 HTML</a> · <a href="${links[5][1]}" target="_self" rel="opener">原有 self</a>\n\n危险链接应失效：<a href="javascript:window.syntheticUnsafe=1">JavaScript 危险链接</a> · <a href="data:text/html,%3Cscript%3Ealert(1)%3C/script%3E">HTML data 危险链接</a>\n\n参考资料：<kb doc="合成参考文档.pdf" kb_id="${kb.id}" chunk_id="${reference.id}" />\n\n![合成图片](${resource})`

async function scenario(mode, renderer) {
  directory = `${mode === 'before' ? before : after}/dist`
  assert.ok(existsSync(`${directory}/index.html`), `Missing compiled UI: ${directory}`)
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1080 }, locale: 'zh-CN' })
  const session = { id: `synthetic-${renderer}-session`, tenant_id: 42, title: `${renderer === 'agent' ? '智能体' : '普通问答'}链接验收 · 合成会话`, agent_id: renderer === 'agent' ? 'synthetic-agent' : 'builtin-quick-answer', created_at: '2026-10-22T00:00:00Z', updated_at: '2026-10-22T00:00:00Z', is_pinned: false }
  const messages = [
    { id: 'synthetic-user-message', session_id: session.id, role: 'user', content: '请提供资料链接，保留当前会话。', created_at: '2026-10-22T00:00:00Z', is_completed: true },
    { id: 'synthetic-assistant-message', session_id: session.id, role: 'assistant', content: markdown, created_at: '2026-10-22T00:00:01Z', is_completed: true, knowledge_references: [reference], ...(renderer === 'agent' ? { agent_steps: [{ iteration: 1, reasoning_content: '已整理合成资料链接。', tool_calls: [] }] } : {}) },
  ]
  const errors = [], calls = []
  await ctx.addInitScript(({ user, tenant, memberships, renderer }) => {
    if (!['127.0.0.1', 'localhost'].includes(location.hostname)) return
    localStorage.setItem('weknora_token', 'synthetic-token')
    localStorage.setItem('weknora_user', JSON.stringify(user))
    localStorage.setItem('weknora_tenant', JSON.stringify(tenant))
    localStorage.setItem('weknora_memberships', JSON.stringify(memberships))
    localStorage.setItem('mindcreek_auth_kind', 'corporate')
    localStorage.setItem('weknora_selected_tenant_id', '42')
    localStorage.setItem('weknora_last_chat_model_id', 'chat')
    localStorage.setItem('WeKnora_settings', JSON.stringify({ selectedAgentId: renderer === 'agent' ? 'synthetic-agent' : 'builtin-quick-answer', isAgentEnabled: renderer === 'agent' }))
    localStorage.setItem('locale', 'zh-CN')
    localStorage.setItem('weknora:new-user-guide-done:v1', '1')
    for (const key of ['kb-list:v2','kb-create:v3','tenant-models:v1','kb-detail:v1','chat:v1','agent-list:v1','agent-create:v1']) localStorage.setItem('weknora:contextual-guide-' + key, '1')
  }, { user, tenant, memberships, renderer })
  await ctx.route('**/*', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname
    if (url.hostname === 'links.example.invalid' || (url.origin === origin && path.startsWith('/synthetic-destination/'))) {
      return route.fulfill({ contentType: 'text/html', body: '<!doctype html><meta charset="utf-8"><title>合成链接目标</title><h1>链接目标已打开</h1><p>这是测试页面，不包含真实外部请求。</p>' })
    }
    if (['blob:', 'data:', 'about:'].includes(url.protocol)) return route.continue()
    if (url.origin !== origin) { report.unexpectedRequests.push(url.href); return route.abort() }
    if (!path.startsWith('/api/')) return route.continue()
    calls.push(path)
    const json = data => route.fulfill({ json: data }), success = data => json({ success: true, data, total: Array.isArray(data) ? data.length : undefined })
    if (path.endsWith('/files')) return route.fulfill({ contentType: 'image/svg+xml', body: picture })
    if (path === '/api/v1/mindcreek/auth/config') return json({ enterprise_enabled: true })
    if (path === '/api/v1/mindcreek/onboarding') return success({ state: 'ready', memberships, retryable: false })
    if (path === '/api/v1/auth/me') return success({ user, tenant, memberships, active_tenant: tenant, capabilities: { can_create_tenant: false }, mindcreek: { local_admin: false, installation_ready: true } })
    if (path === '/api/v1/auth/oidc/config') return success({ enabled: true })
    if (path === '/api/v1/auth/validate') return json({ success: true, valid: true, data: { valid: true } })
    if (path === '/api/v1/mindcreek/branding') return success({ name: '小纳', revision: 1, logo_url: '', can_manage: false })
    if (path === '/api/v1/system/info') return success({ version: 'v0.8.0', edition: 'standard', graph_database_engine: '', keyword_index_engine: 'pg_search', db_version: 86 })
    if (path === '/api/v1/system/deployment-capabilities') return success({ edition: 'standard', capabilities: {} })
    if (path === '/api/v1/models') return success([{ id: 'embedding', type: 'Embedding', name: '合成向量模型', is_default: true }, { id: 'chat', type: 'KnowledgeQA', name: '合成问答模型', is_default: true }])
    if (path === '/api/v1/agents') return success(agents)
    if (path.startsWith('/api/v1/agents/')) return success(agents.find(agent => path.endsWith('/' + agent.id)) || [])
    if (path === '/api/v1/knowledge-bases') return success([kb])
    if (path === '/api/v1/knowledge-bases/' + kb.id) return success(kb)
    if (path === '/api/v1/tenants/42') return success(tenant)
    if (path.includes('/kb-permissions/') && path.endsWith('/effective')) return success({ visible: true, manage: true, creator: true, visibility: 'workspace', revision: 1, operations: [] })
    if (path === '/api/v1/mindcreek/agent/scope') return success({ knowledge_base_ids: [kb.id], selection: 'default', authorization: 'native' })
    if (path === '/api/v1/sessions') return success([session])
    if (path === '/api/v1/sessions/' + session.id) return success(session)
    if (path === `/api/v1/messages/${session.id}/load`) return success(messages)
    if (path.includes('/chunks/')) return success(reference)
    if (path.endsWith('/suggestions')) return success(null)
    return success([])
  })
  const page = await ctx.newPage()
  page.setDefaultTimeout(12000)
  page.on('pageerror', error => errors.push(error.message))
  const shot = async (name, target = page) => { await target.screenshot({ path: `${base}/screenshots/${name}.png`, animations: 'disabled' }); report.screenshots.push(name + '.png') }
  try {
    const conversationURL = `${origin}/platform/chat/${session.id}`
    await page.goto(conversationURL)
    await page.getByRole('heading', { name: '链接打开行为', exact: true }).waitFor()
    const answer = page.locator('.bot_msg')
    if (renderer === 'agent') assert.equal(await answer.locator('.agent-stream-display:not(.is-rag-mode)').count(), 1, 'Agent answer renderer must actually mount')
    else assert.equal(await answer.locator('.agent-stream-display.is-rag-mode').count(), 1, 'Quick-answer history renderer must actually mount')
    const input = page.locator('textarea[name="description"]')
    const draft = '保留这段未发送的问题，继续当前会话。'
    await input.fill(draft)
    await answer.locator('img[data-protected-src][src^="blob:"]').waitFor()
    const attributes = await answer.locator('a').evaluateAll(elements => elements.map(a => ({ text: a.textContent.trim(), href: a.getAttribute('href'), target: a.getAttribute('target'), rel: a.getAttribute('rel') })))
    report.linkAttributes.push({ mode, renderer, attributes })
    await shot(`${mode}-${renderer}-conversation`)
    if (mode === 'before') {
      assert.equal(await answer.getByRole('link', { name: links[0][0], exact: true }).getAttribute('target'), null)
      await answer.getByRole('link', { name: links[0][0], exact: true }).click()
      await page.waitForURL(links[0][1])
      assert.equal(ctx.pages().length, 1)
      await shot(`${mode}-${renderer}-conversation-replaced`)
      report.checks.push(`before ${renderer}: actual HTTPS link click replaces the conversation tab and its unsent input`)
    } else {
      for (const [label, href] of links) {
        const link = answer.getByRole('link', { name: label, exact: true })
        assert.equal(await link.getAttribute('target'), '_blank', label)
        assert.equal(await link.getAttribute('rel'), 'noopener noreferrer', label)
        const popupPromise = ctx.waitForEvent('page')
        await link.click()
        const popup = await popupPromise
        await popup.waitForLoadState('domcontentloaded')
        assert.equal(popup.url(), new URL(href, origin).href, label)
        assert.equal(await popup.evaluate(() => window.opener), null, label)
        assert.equal(page.url(), conversationURL, label)
        assert.equal(await input.inputValue(), draft, label)
        if (label === links[0][0]) await shot(`${mode}-${renderer}-new-tab`, popup)
        await popup.close()
        report.checks.push(`after ${renderer}: ${label} opens a real new tab with no opener; conversation URL and unsent input stay intact`)
      }
      for (const label of ['JavaScript 危险链接', 'HTML data 危险链接']) {
        const link = answer.locator('a').filter({ hasText: label })
        assert.equal(await link.getAttribute('href'), null, label)
        await link.click()
        assert.equal(page.url(), conversationURL)
        assert.equal(await page.evaluate(() => window.syntheticUnsafe), undefined)
        assert.equal(ctx.pages().length, 1)
        report.checks.push(`after ${renderer}: ${label} loses href and cannot navigate or execute`)
      }
      await answer.locator('.citation-kb').click()
      await page.locator('.chat-references-panel').waitFor()
      assert.equal(ctx.pages().length, 1)
      assert.equal(page.url(), conversationURL)
      await shot(`${mode}-${renderer}-citation-drawer`)
      await page.locator('.chat-references-panel__close').click()
      await answer.locator('img[data-protected-src][src^="blob:"]').click()
      await page.locator('.t-image-viewer__modal-mask').waitFor()
      assert.equal(ctx.pages().length, 1)
      assert.equal(page.url(), conversationURL)
      await shot(`${mode}-${renderer}-image-preview`)
      await page.keyboard.press('Escape')
      assert.equal(await input.inputValue(), draft)
      report.checks.push(`after ${renderer}: citation still opens its reference drawer and image still opens in-page preview; conversation and draft remain intact`)
      await shot(`${mode}-${renderer}-conversation-retained`)
    }
    assert.deepEqual(errors, [])
  } catch (error) {
    await shot(`failure-${mode}-${renderer}`)
    report.failureBody = await page.locator('body').innerText()
    report.failureHTML = await page.locator('.bot_msg').innerHTML().catch(() => '')
    report.failureCalls = calls
    throw error
  } finally { report.pageErrors.push(...errors); await ctx.close() }
}
try {
  for (const mode of ['before', 'after']) for (const renderer of ['ordinary', 'agent']) await scenario(mode, renderer)
  assert.deepEqual(report.unexpectedRequests, [])
  report.success = true
} catch (error) { report.success = false; report.error = error.stack; process.exitCode = 1 }
finally { await browser.close(); server.close(); writeFileSync(`${base}/browser-report.json`, JSON.stringify(report, null, 2)); console.log(JSON.stringify(report, null, 2)) }
