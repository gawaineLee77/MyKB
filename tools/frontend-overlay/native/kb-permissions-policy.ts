export const KB_OPERATIONS = [
  { value: 'document.create', label: '上传或新增文档', help: '文件、网页和手工文档；新文档自动解析。' },
  { value: 'document.update', label: '修改文档', help: '正文、名称、摘要、标签和目录归类。' },
  { value: 'document.delete', label: '删除文档', help: '同时删除所选文档的全部分块和索引。' },
  { value: 'document.download', label: '下载原文件', help: '包含返回同一原文件的 PDF、音视频预览。' },
  { value: 'chunk.update', label: '修改分块', help: '正文、启用状态、关联问题和版本回退。' },
  { value: 'chunk.delete', label: '删除分块', help: '删除指定分块及其索引，不删除父文档。' },
  { value: 'chunk.export', label: '导出分块', help: '使用产品导出功能；不包含原文件。' },
] as const
export type KBOperation = typeof KB_OPERATIONS[number]['value']
export interface KBEffective { visible: boolean; manage: boolean; creator: boolean; visibility: 'restricted' | 'workspace'; revision: number; operations: string[] }
export interface KBMember { user_id: string; visible: boolean; operations: string[] }
export interface KBPolicy { kb_id: string; creator_user_id?: string; creator_revoked?: boolean; visibility: 'restricted' | 'workspace'; revision: number; members: KBMember[] }
export interface KBDirectoryMember { user_id: string; username?: string; display_name?: string; role: string }
export function hasKBOperation(access: KBEffective | null | undefined, operation: string): boolean {
  return access?.visible === true && access.operations?.includes(operation) === true
}
export function canReparseKB(access: KBEffective | null | undefined): boolean {
  return ['document.update', 'chunk.update', 'chunk.delete'].every(op => hasKBOperation(access, op))
}
export function normalizeKBMember(member: KBMember, role: string): KBMember {
  // Member search is capped at 100. An unresolved existing row must not lose
  // its grants on an unrelated save; the server revalidates its current role.
  return { user_id: member.user_id, visible: member.visible, operations: member.visible && (role === 'contributor' || role === '') ? KB_OPERATIONS.map(op => op.value).filter(op => member.operations.includes(op)) : [] }
}
export function kbPolicyInput(policy: KBPolicy, range: string, roleFor: (id: string) => string) {
  return { revision: policy.revision, visibility: range === 'workspace' ? 'workspace' : 'restricted', members: range === 'private' ? [] : policy.members.map(member => normalizeKBMember(member, roleFor(member.user_id))) }
}
export function matchesKBFilter(access: KBEffective | undefined, filter: string): boolean {
  if (!access?.visible) return false
  if (filter === 'mine') return access.creator
  if (filter === 'workspace') return access.visibility === 'workspace'
  if (filter === 'shared') return !access.creator && access.visibility === 'restricted'
  return true
}
export function kbPermissionProblem(error: any): string {
  if (error?.status === 409) return '权限已被其他管理者修改。请重新载入最新权限，再保存你的调整。'
  if ([403, 404].includes(error?.status)) return '访问权限已变更，请刷新后重试。'
  return error?.message || '无法读取知识库权限，请重试。'
}
