export type KnowledgeModeID = 'personal_notes' | 'rag' | 'ontology'
export type KnowledgeRole = 'owner' | 'editor' | 'viewer'
export type PublicationAccessMode = 'subscriber' | 'organization_public'

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

export function publicationAffordances(
  accessMode: PublicationAccessMode,
  subscribed: boolean,
  owner = false,
): { canRead: boolean; canSubscribe: boolean; canDownload: boolean } {
  return {
    canRead: owner || accessMode === 'organization_public' || subscribed,
    canSubscribe: !owner && !subscribed,
    canDownload: owner,
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

type BrowserCrypto = {
  randomUUID?: unknown
  getRandomValues?: unknown
}

let fallbackIdempotencySequence = 0

/**
 * Create an ASCII idempotency key on HTTPS, HTTP LAN deployments, and older
 * browsers. randomUUID is restricted to secure contexts in some engines,
 * while getRandomValues remains widely available.
 */
export function createIdempotencyKey(
  source: BrowserCrypto | null = typeof globalThis.crypto === 'undefined' ? null : globalThis.crypto,
  now = Date.now(),
  random = Math.random,
): string {
  if (typeof source?.randomUUID === 'function') {
    try {
      return source.randomUUID.call(source)
    } catch {
      // Fall through when a browser exposes the method but blocks this origin.
    }
  }

  if (typeof source?.getRandomValues === 'function') {
    try {
      const bytes = new Uint8Array(16)
      source.getRandomValues.call(source, bytes)
      bytes[6] = (bytes[6] & 0x0f) | 0x40
      bytes[8] = (bytes[8] & 0x3f) | 0x80
      const hex = Array.from(bytes, value => value.toString(16).padStart(2, '0'))
      return `${hex.slice(0, 4).join('')}-${hex.slice(4, 6).join('')}-${hex.slice(6, 8).join('')}-${hex.slice(8, 10).join('')}-${hex.slice(10).join('')}`
    } catch {
      // A non-cryptographic key is sufficient for retry deduplication.
    }
  }

  fallbackIdempotencySequence += 1
  const randomPart = Math.floor(random() * Number.MAX_SAFE_INTEGER).toString(36)
  return `mc-${now.toString(36)}-${fallbackIdempotencySequence.toString(36)}-${randomPart}`
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
