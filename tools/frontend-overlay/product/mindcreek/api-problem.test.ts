import assert from 'node:assert/strict'
import test from 'node:test'

import { apiProblem } from './api-problem.ts'

test('reads the flattened error emitted by request.ts', () => {
  assert.deepEqual(apiProblem({
    status: 404,
    message: 'Resource not found',
    error: { code: 'resource.not_found', message: 'Resource not found' },
  }), { code: 'resource.not_found', message: 'Resource not found' })
})

test('also accepts an Axios-shaped error', () => {
  assert.deepEqual(apiProblem({
    response: { data: { error: { code: 'publication.unavailable', message: 'Publication service is unavailable' } } },
  }), { code: 'publication.unavailable', message: 'Publication service is unavailable' })
})
