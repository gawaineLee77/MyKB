import { transferState, type TransferIntent } from './member-transfer'
export { transferState, type TransferIntent } from './member-transfer'
import { get, post } from '@/utils/request'
import { addMember, updateMemberRole, type TenantMember } from '@/api/tenant/members'
import type { BatchRow, BatchRole } from './member-batch'

export async function previewMembers(tenant: number, emails: string[]): Promise<BatchRow[]> {
  const result = await post<{ data: { tenant_id: number; rows: BatchRow[] } }>('/api/v1/mindcreek/members/preview', { emails }, { headers: { 'X-Tenant-ID': String(tenant) } })
  if (String(result.data.tenant_id) !== String(tenant)) throw new Error('空间已变化，请重新预览。')
  return result.data.rows
}
export async function readMembers(tenant: number): Promise<TenantMember[]> {
  const members: TenantMember[] = []
  for (let page = 1; page <= 500; page++) {
    const result = await get<any>(`/api/v1/tenants/${tenant}/members?page=${page}&page_size=100`, { headers: { 'X-Tenant-ID': String(tenant) } })
    if (!result.success || !Array.isArray(result.data?.members)) throw new Error('无法完整读取空间成员。')
    members.push(...result.data.members)
    if (members.length >= result.data.total) return members
    if (!result.data.members.length) break
  }
  throw new Error('成员列表不完整，请重试。')
}
export const addBatchMember = async (tenant: number, email: string, role: BatchRole) => {
  const result = await addMember(tenant, { email, role })
  if (!result.success) throw new Error(result.message || '添加成员失败。')
}

export async function transferStep(intent: TransferIntent, step: 'promote' | 'demote') {
  const current = transferState(intent, await readMembers(intent.tenant))
  if (current !== step) return current
  // One native write per explicit step. The last Owner constraint stays native.
  try {
    const result = await updateMemberRole(intent.tenant, step === 'promote' ? intent.to : intent.from, step === 'promote' ? 'owner' : 'admin')
    if (!result.success) throw new Error(result.message || '移交未完成。')
  } catch (error) {
    const actual = transferState(intent, await readMembers(intent.tenant))
    if (actual === current) throw error
    return actual
  }
  return transferState(intent, await readMembers(intent.tenant))
}
