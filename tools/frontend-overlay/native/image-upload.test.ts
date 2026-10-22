import assert from 'node:assert/strict'
import test from 'node:test'
import { imageUploadProblem, normalizedImageSize } from './image-upload.ts'
test('UI image uploads allow only bounded raster files', () => {
  for (const type of ['image/png', 'image/jpeg', 'image/webp']) assert.equal(imageUploadProblem({ type, size: 1024 }), undefined)
  for (const file of [{ type: 'image/svg+xml', size: 10 }, { type: 'image/png', size: 0 }, { type: 'image/jpeg', size: 2097153 }]) assert.ok(imageUploadProblem(file))
})
test('image normalization preserves aspect ratio and prevents excessive decoded surfaces', () => {
  assert.deepEqual(normalizedImageSize(1600, 800), { width: 512, height: 256 })
  assert.deepEqual(normalizedImageSize(400, 800), { width: 256, height: 512 })
  assert.deepEqual(normalizedImageSize(16, 16), { width: 16, height: 16 })
  for (const size of [[0, 1], [4097, 1], [4096, 4096], [1.5, 2]]) assert.throws(() => normalizedImageSize(size[0], size[1]))
})
