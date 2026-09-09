import assert from 'node:assert/strict'
import test from 'node:test'

import { mergeDocumentReferences } from './knowledge-scope.ts'

test('groups multiple retrieved chunks from the same document', () => {
  assert.deepEqual(mergeDocumentReferences([
    { id: 'chunk-1', knowledge_id: 'doc-1', knowledge_base_id: 'kb-1', knowledge_title: 'Handbook.pdf' },
    { id: 'chunk-2', knowledge_id: 'doc-1', knowledge_base_id: 'kb-1', knowledge_title: 'Handbook.pdf' },
    { id: 'chunk-3', knowledge_id: 'doc-2', knowledge_base_id: 'kb-1', knowledge_title: 'Policy.pdf' },
  ]), [
    { id: 'chunk-1', knowledge_id: 'doc-1', knowledge_base_id: 'kb-1', knowledge_title: 'Handbook.pdf', match_count: 2 },
    { id: 'chunk-3', knowledge_id: 'doc-2', knowledge_base_id: 'kb-1', knowledge_title: 'Policy.pdf', match_count: 1 },
  ])
})

test('does not merge same-named documents from different knowledge bases', () => {
  assert.equal(mergeDocumentReferences([
    { id: 'chunk-a', knowledge_base_id: 'kb-a', knowledge_filename: 'Guide.md' },
    { id: 'chunk-b', knowledge_base_id: 'kb-b', knowledge_filename: 'Guide.md' },
  ]).length, 2)
})
