import { get, post, put, del } from '@/utils/request'
export type ManagedModelType = 'KnowledgeQA' | 'Embedding' | 'Rerank' | 'VLLM'

// This deliberately mirrors the redacted MindCreek facade. Provider URLs,
// credentials, and provider-specific parameters must never be added here.
export interface ManagedModelDescriptor {
  id: string
  display_name: string
  type: ManagedModelType
  managed: boolean
  default: boolean
  available: boolean
  scope: 'organization' | 'workspace'
}

export interface ManagedModelSnapshot {
  ready: boolean
  defaults: ManagedModelDescriptor[]
  overrides: ManagedModelDescriptor[]
  overrides_enabled: boolean
}

export interface ModelOverrideInput {
  name: string
  display_name: string
  type: ManagedModelType
  provider: string
  base_url: string
  api_key?: string
  dimension?: number
}

export interface ModelTestResult {
  available: boolean
  dimension?: number
  elapsed_ms?: number
  message?: string
}

export async function getManagedModels(): Promise<ManagedModelSnapshot> {
  const response = await get<{ success: boolean; data: ManagedModelSnapshot }>('/api/v1/mindcreek/models')
  return response.data
}

export async function getCreationModels(): Promise<{
  ready: boolean
  overridesEnabled: boolean
  embedding: ManagedModelDescriptor[]
  summary: ManagedModelDescriptor[]
  rerank: ManagedModelDescriptor[]
  vlm: ManagedModelDescriptor[]
}> {
  const snapshot = await getManagedModels()
  const models = [...snapshot.defaults, ...snapshot.overrides].filter(model => model.available)
  return {
    ready: snapshot.ready,
    overridesEnabled: snapshot.overrides_enabled,
    embedding: models.filter(model => model.type === 'Embedding'),
    summary: models.filter(model => model.type === 'KnowledgeQA'),
    rerank: models.filter(model => model.type === 'Rerank'),
    vlm: models.filter(model => model.type === 'VLLM'),
  }
}

export async function getSmartReasoningModelId(): Promise<string> {
  const snapshot = await getManagedModels()
  const chat = snapshot.defaults.find(model => model.type === 'KnowledgeQA' && model.available)
  const rerank = snapshot.defaults.find(model => model.type === 'Rerank' && model.available)
  return snapshot.ready && chat && rerank ? chat.id : ''
}

export async function createModelOverride(input: ModelOverrideInput): Promise<ManagedModelDescriptor> {
  const response = await post<{ success: boolean; data: ManagedModelDescriptor }>(
    '/api/v1/mindcreek/models/overrides',
    input,
  )
  return response.data
}

export async function updateModelOverride(id: string, input: ModelOverrideInput): Promise<ManagedModelDescriptor> {
  const response = await put<{ success: boolean; data: ManagedModelDescriptor }>(
    `/api/v1/mindcreek/models/overrides/${encodeURIComponent(id)}`,
    input,
  )
  return response.data
}

export async function deleteModelOverride(id: string): Promise<void> {
  await del(`/api/v1/mindcreek/models/overrides/${encodeURIComponent(id)}`)
}

export async function testModelOverride(input: ModelOverrideInput, id = ''): Promise<ModelTestResult> {
  const query = id ? `?model_id=${encodeURIComponent(id)}` : ''
  const response = await post<{ success: boolean; data: ModelTestResult }>(
    `/api/v1/mindcreek/models/overrides/test${query}`,
    input,
  )
  return response.data
}

export async function testManagedModel(id: string): Promise<ModelTestResult> {
  const response = await post<{ success: boolean; data: ModelTestResult }>(
    `/api/v1/mindcreek/models/${encodeURIComponent(id)}/test`,
    {},
  )
  return response.data
}

