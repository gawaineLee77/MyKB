import assert from 'node:assert/strict'
import test from 'node:test'
import { configuredBrandText, brandingConflict, brandingProblem, brandingInput, enterpriseSourceOptions, parseBranding, validBrandName } from './branding-policy.ts'

test('branding accepts plain configurable names and only the local managed logo route', () => {
  assert.equal(validBrandName('小纳 · 研发'), true)
  for (const name of ['', '  ', '<script>', 'x\n', 'x'.repeat(65)]) assert.equal(validBrandName(name), false)
  assert.deepEqual(parseBranding({ name: '研发知识中心', logo_url: '/api/v1/mindcreek/branding/logo?v=4', revision: 4 }), { name: '研发知识中心', logo_url: '/api/v1/mindcreek/branding/logo?v=4', revision: 4 })
  for (const logo_url of ['https://example.com/logo.png', '//example.com/x', 'data:image/svg+xml,x', '/api/v1/mindcreek/branding/logo?v=1&redirect=x']) assert.throws(() => parseBranding({ name: '小纳', logo_url, revision: 1 }))
})
test('name-only updates retain the uploaded logo, removal is explicit and revision is preserved', () => {
  assert.deepEqual(brandingInput(4, ' 小纳 ', undefined), { revision: 4, name: '小纳' })
  assert.deepEqual(brandingInput(4, '小纳', null), { revision: 4, name: '小纳', logo_data: null })
})
test('source filters retain their native API values and all-source remains unfiltered', () => {
  assert.deepEqual(enterpriseSourceOptions.map(item => item.value), ['', 'web', 'url', 'manual', 'api'])
  assert.deepEqual(enterpriseSourceOptions.map(item => item.label), ['全部来源', '本地上传', 'URL 导入', '手动创建', 'API'])
})

test('revision conflict recognizes normalized request errors as well as axios errors', () => {
  assert.equal(brandingConflict({ status: 409 }), true)
  assert.equal(brandingConflict({ response: { status: 409 } }), true)
  assert.equal(brandingConflict({ status: 403 }), false)
  assert.equal(brandingProblem({ status: 400, error: { code: 'branding.image_invalid' } }), '图片无法处理，请重新上传有效的 PNG、JPEG 或 WebP 图片。')
})

test('service names render literally instead of interpreting replacement patterns', () => {
  for (const name of ['小纳 $&', '$`', "$'", '$$']) assert.equal(configuredBrandText('欢迎 MindCreek', name), '欢迎 ' + name)
})
