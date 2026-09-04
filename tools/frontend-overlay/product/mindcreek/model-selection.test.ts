import assert from 'node:assert/strict'
import test from 'node:test'

import { chooseChatModel, MANAGED_CHAT_MODEL_ID } from './model-selection.ts'

const models = [
  { id: 'workspace-chat', type: 'KnowledgeQA', status: 'active' },
  { id: MANAGED_CHAT_MODEL_ID, type: 'KnowledgeQA', status: 'active', is_default: true },
]

test('selects the managed chat model for a new browser session', () => {
  assert.equal(chooseChatModel(models), MANAGED_CHAT_MODEL_ID)
})

test('preserves an available user selection', () => {
  assert.equal(chooseChatModel(models, 'workspace-chat'), 'workspace-chat')
})

test('repairs stale selections with the managed default', () => {
  assert.equal(chooseChatModel(models, 'removed-model', 'also-removed'), MANAGED_CHAT_MODEL_ID)
})

test('does not select inactive or non-chat models', () => {
  assert.equal(chooseChatModel([
    { id: MANAGED_CHAT_MODEL_ID, type: 'KnowledgeQA', status: 'inactive', is_default: true },
    { id: 'embedding', type: 'Embedding', status: 'active', is_default: true },
  ]), '')
})
