// UI affordances only. The product gateway authorizes each request independently.
export const canManageWorkspace = (role: string | null | undefined) => role === 'owner' || role === 'admin'
export const canManageMember = (role: string | null | undefined, targetRole: string) => role === 'owner' || (role === 'admin' && ['admin', 'contributor', 'viewer'].includes(targetRole))
export const assignableWorkspaceRoles = (role: string | null | undefined): string[] => role === 'owner'
  ? ['owner', 'admin', 'contributor', 'viewer'] : role === 'admin' ? ['admin', 'contributor', 'viewer'] : []

export const ADMIN_KEY_CAPABILITIES = new Set(['retrieve', 'chat', 'read_agents', 'ingest', 'manage_kbs', 'manage_agents', 'manage_channels'])
const OWNER_KEY_CAPABILITIES = new Set([...ADMIN_KEY_CAPABILITIES, 'message_history', 'manage_mcp_services', 'manage_datasources', 'manage_models', 'manage_vector_stores', 'manage_storage_backends', 'manage_web_search', 'run_evaluations', 'manage_members', 'manage_spaces', 'manage_tenant_settings'])
export const allowedKeyCapabilities = (role: string | null | undefined) => role === 'owner' ? OWNER_KEY_CAPABILITIES : role === 'admin' ? ADMIN_KEY_CAPABILITIES : new Set<string>()
export function keyScopeProblem(role: string | null | undefined, capabilities: string[], knowledgeBaseIds: string[]): string {
  if (!canManageWorkspace(role)) return '当前角色不能管理 API Key。'
  if (role === 'owner') return ''
  if (capabilities.some(value => !ADMIN_KEY_CAPABILITIES.has(value))) return '所选能力超出 Admin 可授权范围。'
  if (!knowledgeBaseIds.some(value => value.trim())) return '请选择至少一个获准的知识库。Admin 密钥必须限定知识库范围。'
  return ''
}
export function keyCreatorLabel(key: { creator_user_id?: string; creator_name?: string; ownership?: string }): string {
  if (!key.creator_user_id || key.ownership === 'legacy') return '历史密钥 · 归属不明'
  return `${key.creator_name || key.creator_user_id}${key.ownership === 'self' ? '（我）' : ''}`
}
