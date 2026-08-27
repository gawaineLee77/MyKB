export type KnowledgeModeID = 'personal_notes' | 'rag' | 'ontology'
export type KnowledgeRole = 'owner' | 'editor' | 'viewer'

export interface PermissionAffordances {
  canRead: boolean
  canEditContent: boolean
  canEditMetadata: boolean
  canManageGrants: boolean
  canDelete: boolean
}

export function permissionAffordances(role: KnowledgeRole): PermissionAffordances {
  return {
    canRead: true,
    canEditContent: role === 'owner' || role === 'editor',
    canEditMetadata: role === 'owner' || role === 'editor',
    canManageGrants: role === 'owner',
    canDelete: role === 'owner',
  }
}

export interface IndexProfileCapability {
  id: string
  enabled: boolean
}

export interface KnowledgeModeCapability {
  id: KnowledgeModeID
  enabled: boolean
  profiles?: IndexProfileCapability[]
}

export interface CapabilityDocument {
  schema_version: number
  phase: string
  knowledge_modes: KnowledgeModeCapability[]
}

export interface KnowledgeSpaceDraft {
  mode: 'personal_notes' | 'rag'
  name: string
  description: string
  embeddingModelId: string
  summaryModelId?: string
}

export interface KnowledgeSpaceRequest {
  mode: 'personal_notes' | 'rag'
  index_profile: 'notes_plain' | 'plain'
  name: string
  description?: string
  embedding_model_id: string
  summary_model_id?: string
  storage_provider: 'local'
}

export function isSelectionEnabled(
  document: CapabilityDocument,
  mode: KnowledgeSpaceDraft['mode'],
): boolean {
  const capability = document.knowledge_modes.find(item => item.id === mode)
  if (!capability?.enabled) return false
  if (mode === 'personal_notes') return true
  return capability.profiles?.some(profile => profile.id === 'plain' && profile.enabled) === true
}

export function buildKnowledgeSpaceRequest(
  document: CapabilityDocument,
  draft: KnowledgeSpaceDraft,
): KnowledgeSpaceRequest {
  if (!isSelectionEnabled(document, draft.mode)) {
    throw new Error('The selected knowledge mode is not enabled')
  }
  const name = draft.name.trim()
  const embeddingModelId = draft.embeddingModelId.trim()
  if (!name || !embeddingModelId) {
    throw new Error('Name and embedding model are required')
  }
  return {
    mode: draft.mode,
    index_profile: draft.mode === 'personal_notes' ? 'notes_plain' : 'plain',
    name,
    description: draft.description.trim() || undefined,
    embedding_model_id: embeddingModelId,
    summary_model_id: draft.summaryModelId?.trim() || undefined,
    storage_provider: 'local',
  }
}
