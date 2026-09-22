import type { TenantMember } from '@/api/tenant/members'
export interface TransferIntent { tenant: number; from: string; to: string }
export function transferState(intent: TransferIntent, members: TenantMember[]): 'promote' | 'demote' | 'complete' | 'conflict' {
  const from = members.find(m => m.user_id === intent.from), to = members.find(m => m.user_id === intent.to)
  if (!from || !to || from.user_id === to.user_id || from.status !== 'active' || to.status !== 'active') return 'conflict'
  if (from.role !== 'owner') return from.role === 'admin' && to.role === 'owner' ? 'complete' : 'conflict'
  return to.role === 'owner' ? 'demote' : 'promote'
}
