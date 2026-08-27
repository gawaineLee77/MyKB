import assert from 'node:assert/strict'
import test from 'node:test'

import {
  buildKnowledgeSpaceRequest,
  isSelectionEnabled,
  permissionAffordances,
  type CapabilityDocument,
} from './contracts.ts'

const capabilities: CapabilityDocument = {
  schema_version: 1,
  phase: 'phase1',
  knowledge_modes: [
    { id: 'personal_notes', enabled: true },
    {
      id: 'rag',
      enabled: true,
      profiles: [
        { id: 'plain', enabled: true },
        { id: 'graph', enabled: false },
        { id: 'pixel', enabled: false },
      ],
    },
    { id: 'ontology', enabled: false },
  ],
}

test('builds only approved Phase 1 profiles and local storage', () => {
  assert.deepEqual(
    buildKnowledgeSpaceRequest(capabilities, {
      mode: 'personal_notes',
      name: '  Field notes  ',
      description: '  private work log  ',
      embeddingModelId: 'embedding-1',
    }),
    {
      mode: 'personal_notes',
      index_profile: 'notes_plain',
      name: 'Field notes',
      description: 'private work log',
      embedding_model_id: 'embedding-1',
      summary_model_id: undefined,
      storage_provider: 'local',
    },
  )
})

test('requires the advertised plain profile for RAG', () => {
  const disabled = structuredClone(capabilities)
  disabled.knowledge_modes[1].profiles![0].enabled = false
  assert.equal(isSelectionEnabled(disabled, 'rag'), false)
  assert.throws(
    () => buildKnowledgeSpaceRequest(disabled, {
      mode: 'rag',
      name: 'Documents',
      description: '',
      embeddingModelId: 'embedding-1',
    }),
    /not enabled/,
  )
})

test('rejects missing required fields before submission', () => {
  assert.throws(
    () => buildKnowledgeSpaceRequest(capabilities, {
      mode: 'rag',
      name: ' ',
      description: '',
      embeddingModelId: 'embedding-1',
    }),
    /required/,
  )
})

test('derives permission-aware UI affordances without elevating viewers or editors', () => {
  assert.deepEqual(permissionAffordances('viewer'), {
    canRead: true, canEditContent: false, canEditMetadata: false, canManageGrants: false, canDelete: false,
  })
  assert.deepEqual(permissionAffordances('editor'), {
    canRead: true, canEditContent: true, canEditMetadata: true, canManageGrants: false, canDelete: false,
  })
  assert.deepEqual(permissionAffordances('owner'), {
    canRead: true, canEditContent: true, canEditMetadata: true, canManageGrants: true, canDelete: true,
  })
})
