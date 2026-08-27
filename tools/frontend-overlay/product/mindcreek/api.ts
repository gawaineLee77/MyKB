import { del, get, patch, post, postUpload } from '@/utils/request'
import { listModels, type ModelConfig } from '@/api/model'

import type { CapabilityDocument, KnowledgeSpaceRequest } from './contracts'

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
