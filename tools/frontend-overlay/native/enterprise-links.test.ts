import test from 'node:test'
import assert from 'node:assert/strict'
import { enterpriseLinks, loadEnterpriseLinks, parseEnterpriseLinks, safeEnterpriseURL } from './enterprise-links'

test('missing, disabled and unknown config versions hide every entry', () => {
  for (const config of [null, [], {}, { version: 2, enabled: true, links: { help: '/docs' } },
    { version: 1, enabled: false, links: { help: '/docs' } },
    { version: 1, enabled: 'true', links: { help: '/docs' } }]) {
    assert.ok(Object.values(parseEnterpriseLinks(config)).every(url => url === ''))
  }
})

test('entries are independent; private HTTP/S and same-origin paths are supported without fallback', () => {
  const links = parseEnterpriseLinks({ version: 1, enabled: true, links: {
    help: 'https://docs.example.invalid/guide#intro', rbac: '/docs/rbac',
    api: 'http://docs.intranet/api', project: '', builtinModels: 'javascript:alert(1)',
    unknown: 'https://github.com/Tencent/WeKnora',
  } })
  assert.equal(links.help, 'https://docs.example.invalid/guide#intro')
  assert.equal(links.rbac, '/docs/rbac')
  assert.equal(links.api, 'http://docs.intranet/api')
  assert.equal(links.project, '')
  assert.equal(links.builtinModels, '')
  assert.equal(Object.hasOwn(links, 'unknown'), false)
})

test('executable, credential-bearing, protocol-relative and malformed addresses are rejected', () => {
  for (const url of ['//outside.invalid/path', '/\\outside.invalid', 'javascript:alert(1)',
    'data:text/html,<script>', 'file:///etc/passwd', 'https://user:pass@docs.invalid',
    'https:\\docs.invalid', 'https://docs.invalid/\npath', 'docs/help', '#help', 23]) {
    assert.equal(safeEnterpriseURL(url), '', String(url))
  }
})

test('runtime loading bypasses cache, refuses redirects, and fails closed after a previous valid load', async () => {
  let called = false
  await loadEnterpriseLinks((async (path, options) => {
    called = true
    assert.equal(path, '/mindcreek-links.json')
    assert.equal(options?.cache, 'no-store')
    assert.equal(options?.redirect, 'error')
    return new Response(JSON.stringify({ version: 1, enabled: true, links: { help: '/docs' } }))
  }) as typeof fetch)
  assert.equal(called, true)
  assert.equal(enterpriseLinks.value.help, '/docs')
  for (const response of [new Response('', { status: 404 }), new Response('<html>SPA fallback</html>')]) {
    await loadEnterpriseLinks((async () => response) as typeof fetch)
    assert.ok(Object.values(enterpriseLinks.value).every(url => !url))
  }
  await loadEnterpriseLinks((async () => { throw new Error('unavailable') }) as typeof fetch)
  assert.ok(Object.values(enterpriseLinks.value).every(url => !url))
})
