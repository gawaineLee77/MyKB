import { del, get, patch, post, postUpload } from '@/utils/request'
import { listModels, type ModelConfig } from '@/api/model'

import type { CapabilityDocument, KnowledgeRole, KnowledgeSpaceRequest } from './contracts'

export interface KnowledgeLibraryItem {
  id: string
  name: string
  description: string
  type: string
  creator_id: string
  role: KnowledgeRole
  product_mode?: 'personal_notes' | 'rag' | 'ontology'
  access_source: 'owner' | 'user_grant' | 'subscription' | 'organization_public'
  publication_id?: string
  current_revision?: number
  last_seen_revision?: number
  updated?: boolean
}

export interface KnowledgeLibraryPage {
  items: KnowledgeLibraryItem[]
  total: number
  page: number
  page_size: number
}

export interface AgentScopeEntry {
  knowledge_base_id: string
  role: KnowledgeRole
  access_source: 'owner' | 'user_grant' | 'subscription' | 'organization_public'
  product_mode?: string
}

export interface AgentScopeResult {
  selection: 'default' | 'explicit'
  knowledge_base_ids: string[]
  entries: AgentScopeEntry[]
}

export interface KnowledgeAccess {
  knowledge_base_id: string
  role: KnowledgeRole
  product_mode?: 'personal_notes' | 'rag' | 'ontology'
  can_read: boolean
  can_edit_content: boolean
  can_edit_metadata: boolean
  can_manage_grants: boolean
  can_delete: boolean
  can_publish: boolean
  can_download: boolean
  access_source: 'owner' | 'user_grant' | 'subscription' | 'organization_public'
  publication_id?: string
}

export type PublicationAccessMode = 'subscriber' | 'organization_public'
export type PublicationAudience = { type: 'organization'; workspace_ids?: never[] } | { type: 'workspace_set'; workspace_ids: number[] }

export interface Publication {
  id: string
  knowledge_base_id: string
  publisher_id: string
  publisher_tenant_id: number
  title: string
  description: string
  tags: string[]
  usage_guidance: string
  audience: PublicationAudience
  access_mode: PublicationAccessMode
  status: 'published' | 'unpublished'
  published_revision: number
  created_at: string
  published_at: string
  unpublished_at?: string
  updated_at: string
  row_version: number
}

export interface Subscription {
  id: string
  publication_id: string
  subscriber_id: string
  subscriber_tenant_id: number
  status: 'active' | 'inactive' | 'unsubscribed'
  notification_enabled: boolean
  last_seen_revision: number
  created_at: string
  updated_at: string
  ended_at?: string
}

export interface CatalogItem {
  publication: Publication
  current_revision: number
  subscribed: boolean
  last_seen_revision?: number
  updated: boolean
  can_read: boolean
  can_subscribe: boolean
}

export interface CatalogPage {
  items: CatalogItem[]
  total: number
  page: number
  page_size: number
}

export interface SubscriptionItem {
  subscription: Subscription
  publication: Publication
  current_revision: number
  updated: boolean
}

export interface TenantMember {
  user_id: string
  email: string
  username: string
  avatar?: string
  role: string
  status: string
}

export interface TenantMemberPage {
  items: TenantMember[]
  total: number
  page: number
  page_size: number
}

export interface KnowledgeGrant {
  id: string
  knowledge_base_id: string
  subject_type: 'user'
  subject_id: string
  permission: 'viewer' | 'editor'
  granted_by: string
  created_at: string
  updated_at: string
  expires_at?: string
  revision: number
}

export interface KnowledgeSpaceResult {
  knowledge_base_id: string
  name: string
  product_mode: 'personal_notes' | 'rag'
  index_profile: 'notes_plain' | 'plain'
  access_policy: 'owner_only' | 'upstream'
  created: boolean
  reconciled: boolean
}

export interface ProductProfile {
  upstream_kb_id: string
  product_mode: 'personal_notes' | 'rag'
  access_policy: 'owner_only' | 'upstream'
  index_profile: 'notes_plain' | 'plain'
  index_profile_version: number
}

export interface RAGDocument {
  id: string
  knowledge_base_id: string
  type: string
  title: string
  file_name: string
  file_type: string
  file_size: number
  parse_status: string
  error_message?: string
  created_at: string
  updated_at: string
}

export interface RAGDocumentPage {
  items: RAGDocument[]
  total: number
  page: number
  page_size: number
}

export interface NoteSummary {
  id: string
  title: string
  status: string
  version: number
  content_size: number
  parse_status: string
  updated_at: string
}

export interface NotePage {
  items: NoteSummary[]
  total: number
  page: number
  page_size: number
}

export interface Note extends NoteSummary {
  knowledge_base_id: string
  content: string
  error_message?: string
  created_at: string
}

export interface NoteRevision {
  knowledge_base_id: string
  note_id: string
  version: number
  title: string
  content?: string
  status: string
  operation: 'create' | 'edit' | 'import' | 'restore' | 'snapshot'
  restored_from_version?: number
  actor_user_id: string
  recorded_at: string
}

export async function getKnowledgeModeCapabilities(): Promise<CapabilityDocument> {
  return get<CapabilityDocument>('/api/v1/capabilities/knowledge-modes')
}

export async function getCreationModels(): Promise<{
  embedding: ModelConfig[]
  summary: ModelConfig[]
}> {
  const models = await listModels()
  return {
    embedding: models.filter(model => model.type === 'Embedding'),
    summary: models.filter(model => model.type === 'KnowledgeQA'),
  }
}

export async function getSmartReasoningModelId(): Promise<string> {
  const response = await get<{ success: boolean; data: { config?: { model_id?: string; rerank_model_id?: string } } }>(
    '/api/v1/agents/builtin-smart-reasoning',
  )
  const config = response.data.config
  return config?.model_id && config.rerank_model_id ? config.model_id : ''
}

export async function createKnowledgeSpace(
  request: KnowledgeSpaceRequest,
  idempotencyKey: string,
): Promise<KnowledgeSpaceResult> {
  const response = await post<{ success: boolean; data: KnowledgeSpaceResult }>(
    '/api/v1/knowledge-spaces',
    request,
    { headers: { 'Idempotency-Key': idempotencyKey } },
  )
  return response.data
}

export async function getProductProfile(kbId: string): Promise<ProductProfile> {
  const response = await get<{ success: boolean; data: ProductProfile }>(`/api/v1/knowledge-bases/${kbId}/product-profile`)
  return response.data
}

export async function listKnowledgeLibrary(view: 'owned' | 'shared' | 'subscribed' | 'all', page = 1, pageSize = 24): Promise<KnowledgeLibraryPage> {
  const response = await get<{ success: boolean; data: KnowledgeLibraryPage }>(
    `/api/v1/mindcreek/knowledge-bases?view=${view}&page=${page}&page_size=${pageSize}`,
  )
  return response.data
}

export async function getKnowledgePublication(kbId: string): Promise<Publication> {
  const response = await get<{ success: boolean; data: Publication }>(`/api/v1/mindcreek/knowledge-bases/${kbId}/publication`)
  return response.data
}

export async function publishKnowledgeBase(kbId: string, input: {
  title: string; description: string; tags: string[]; usage_guidance: string
  audience: PublicationAudience; access_mode: PublicationAccessMode; expected_row_version?: number
}): Promise<Publication> {
  const response = await post<{ success: boolean; data: Publication }>(`/api/v1/mindcreek/knowledge-bases/${kbId}/publication`, input)
  return response.data
}

export async function updateKnowledgePublication(kbId: string, publication: Publication, input: {
  title: string; description: string; tags: string[]; usage_guidance: string
  audience: PublicationAudience; access_mode: PublicationAccessMode
}): Promise<Publication> {
  const response = await patch<{ success: boolean; data: Publication }>(`/api/v1/mindcreek/knowledge-bases/${kbId}/publication`, { ...input, expected_row_version: publication.row_version })
  return response.data
}

export async function unpublishKnowledgeBase(kbId: string, publication: Publication): Promise<Publication> {
  const response = await del<{ success: boolean; data: Publication }>(`/api/v1/mindcreek/knowledge-bases/${kbId}/publication`, { expected_row_version: publication.row_version })
  return response.data
}

export async function listCatalog(filters: { query?: string; tag?: string; accessMode?: PublicationAccessMode | ''; page?: number; pageSize?: number } = {}): Promise<CatalogPage> {
  const query = new URLSearchParams({ page: String(filters.page || 1), page_size: String(filters.pageSize || 24) })
  if (filters.query) query.set('q', filters.query)
  if (filters.tag) query.set('tag', filters.tag)
  if (filters.accessMode) query.set('access_mode', filters.accessMode)
  const response = await get<{ success: boolean; data: CatalogPage }>(`/api/v1/mindcreek/catalog?${query}`)
  return response.data
}

export async function getDefaultAgentScope(): Promise<AgentScopeResult> {
  const response = await get<{ success: boolean; data: AgentScopeResult }>('/api/v1/mindcreek/agent/scope')
  return response.data
}

export async function resolveAgentScope(knowledgeBaseIds: string[]): Promise<AgentScopeResult> {
  const response = await post<{ success: boolean; data: AgentScopeResult }>('/api/v1/mindcreek/agent/scope/resolve', {
    selection: 'explicit', knowledge_base_ids: knowledgeBaseIds,
  })
  return response.data
}

export async function subscribePublication(publicationId: string): Promise<{ item: SubscriptionItem; changed: boolean }> {
  const response = await post<{ success: boolean; data: { item: SubscriptionItem; changed: boolean } }>(`/api/v1/mindcreek/publications/${publicationId}/subscription`)
  return response.data
}

export async function unsubscribePublication(publicationId: string): Promise<{ item?: SubscriptionItem; changed: boolean }> {
  const response = await del<{ success: boolean; data: { item?: SubscriptionItem; changed: boolean } }>(`/api/v1/mindcreek/publications/${publicationId}/subscription`)
  return response.data
}

export async function markPublicationSeen(publicationId: string): Promise<{ item: SubscriptionItem; changed: boolean }> {
  const response = await post<{ success: boolean; data: { item: SubscriptionItem; changed: boolean } }>(`/api/v1/mindcreek/publications/${publicationId}/mark-seen`)
  return response.data
}

export async function getKnowledgeAccess(kbId: string): Promise<KnowledgeAccess> {
  const response = await get<{ success: boolean; data: KnowledgeAccess }>(
    `/api/v1/mindcreek/knowledge-bases/${kbId}/access`,
  )
  return response.data
}

export async function searchTenantUsers(query: string): Promise<TenantMemberPage> {
  const response = await get<{ success: boolean; data: TenantMemberPage }>(
    `/api/v1/mindcreek/users?q=${encodeURIComponent(query)}&page=1&page_size=20`,
  )
  return response.data
}

export async function listKnowledgeGrants(kbId: string): Promise<KnowledgeGrant[]> {
  const response = await get<{ success: boolean; data: KnowledgeGrant[] }>(
    `/api/v1/mindcreek/knowledge-bases/${kbId}/grants`,
  )
  return response.data
}

export async function createKnowledgeGrant(
  kbId: string,
  subjectId: string,
  permission: 'viewer' | 'editor',
  expiresAt?: string,
): Promise<KnowledgeGrant> {
  const response = await post<{ success: boolean; data: KnowledgeGrant }>(
    `/api/v1/mindcreek/knowledge-bases/${kbId}/grants`,
    { subject_type: 'user', subject_id: subjectId, permission, expires_at: expiresAt || undefined },
  )
  return response.data
}

export async function updateKnowledgeGrant(
  kbId: string,
  grant: KnowledgeGrant,
  permission: 'viewer' | 'editor',
  expiresAt?: string,
): Promise<KnowledgeGrant> {
  const response = await patch<{ success: boolean; data: KnowledgeGrant }>(
    `/api/v1/mindcreek/knowledge-bases/${kbId}/grants/${grant.id}`,
    { expected_revision: grant.revision, permission, expires_at: expiresAt || undefined },
  )
  return response.data
}

export async function revokeKnowledgeGrant(kbId: string, grant: KnowledgeGrant): Promise<KnowledgeGrant> {
  const response = await del<{ success: boolean; data: KnowledgeGrant }>(
    `/api/v1/mindcreek/knowledge-bases/${kbId}/grants/${grant.id}`,
    { expected_revision: grant.revision },
  )
  return response.data
}

export async function listNotes(kbId: string, page = 1): Promise<NotePage> {
  const response = await get<{ success: boolean; data: NotePage }>(`/api/v1/knowledge-bases/${kbId}/notes?page=${page}&page_size=10`)
  return response.data
}

export async function getNote(kbId: string, noteId: string): Promise<Note> {
  const response = await get<{ success: boolean; data: Note }>(`/api/v1/knowledge-bases/${kbId}/notes/${noteId}`)
  return response.data
}

export async function createNote(kbId: string, title: string, content: string): Promise<Note> {
  const response = await post<{ success: boolean; data: Note }>(`/api/v1/knowledge-bases/${kbId}/notes`, { title, content, status: 'publish' })
  return response.data
}

export async function updateNote(kbId: string, noteId: string, title: string, content: string, expectedVersion: number): Promise<Note> {
  const response = await patch<{ success: boolean; data: Note }>(`/api/v1/knowledge-bases/${kbId}/notes/${noteId}`, { title, content, status: 'publish', expected_version: expectedVersion })
  return response.data
}

export async function deleteNote(kbId: string, noteId: string): Promise<void> {
  await del(`/api/v1/knowledge-bases/${kbId}/notes/${noteId}`)
}

export async function importNote(kbId: string, file: File): Promise<Note> {
  const form = new FormData()
  form.append('file', file)
  const response = await postUpload(`/api/v1/knowledge-bases/${kbId}/notes/import`, form)
  return response.data
}

export async function listNoteRevisions(kbId: string, noteId: string): Promise<NoteRevision[]> {
  const response = await get<{ success: boolean; data: NoteRevision[] }>(`/api/v1/knowledge-bases/${kbId}/notes/${noteId}/revisions`)
  return response.data
}

export async function getNoteRevision(kbId: string, noteId: string, version: number): Promise<NoteRevision> {
  const response = await get<{ success: boolean; data: NoteRevision }>(`/api/v1/knowledge-bases/${kbId}/notes/${noteId}/revisions/${version}`)
  return response.data
}

export async function restoreNoteRevision(kbId: string, noteId: string, expectedVersion: number, targetVersion: number): Promise<Note> {
  const response = await post<{ success: boolean; data: Note }>(`/api/v1/knowledge-bases/${kbId}/notes/${noteId}/restore`, {
    expected_version: expectedVersion,
    target_version: targetVersion,
  })
  return response.data
}

export async function uploadRAGDocument(kbId: string, file: File): Promise<RAGDocument> {
  const form = new FormData()
  form.append('file', file)
  const response = await postUpload(`/api/v1/knowledge-bases/${kbId}/ingestions`, form)
  return response.data
}

export async function listRAGDocuments(kbId: string, page = 1): Promise<RAGDocumentPage> {
  const response = await get<{ success: boolean; data: RAGDocumentPage }>(`/api/v1/knowledge-bases/${kbId}/ingestions?page=${page}&page_size=20`)
  return response.data
}

export async function retryRAGDocument(kbId: string, documentId: string): Promise<RAGDocument> {
  const response = await post<{ success: boolean; data: RAGDocument }>(`/api/v1/knowledge-bases/${kbId}/ingestions/${documentId}/retry`)
  return response.data
}

export async function cancelRAGDocument(kbId: string, documentId: string): Promise<RAGDocument> {
  const response = await post<{ success: boolean; data: RAGDocument }>(`/api/v1/knowledge-bases/${kbId}/ingestions/${documentId}/cancel`)
  return response.data
}
