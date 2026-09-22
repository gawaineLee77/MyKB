import { get, post } from '@/utils/request'
import { loginPath } from './enterprise-auth'
import { nativeProblem } from './native-problems'

export interface NativeScope { knowledge_base_ids: string[]; selection: string; authorization: 'native' }
export const getNativeScope = async () => (await get<{ data: NativeScope }>('/api/v1/mindcreek/agent/scope')).data
export async function resolveNativeScope(ids: string[] | undefined, agentID?: string): Promise<NativeScope> {
  const response = await post<{ data: NativeScope }>('/api/v1/mindcreek/agent/scope/resolve', {
    selection: ids?.length ? 'explicit' : 'default', knowledge_base_ids: ids || [], agent_id: agentID || '',
  })
  return response.data
}

// Only fill an implicit scope. Explicit selections remain intact on denial.
// The gateway rechecks file parents, source tenant and session binding itself.
export async function prepareNativeChat(body: Record<string, any>): Promise<void> {
  const scope = await resolveNativeScope(body.knowledge_base_ids, body.agent_id)
  if (!body.knowledge_base_ids?.length && scope.knowledge_base_ids.length) body.knowledge_base_ids = scope.knowledge_base_ids
  if (!body.agent_id && !scope.knowledge_base_ids.length && !body.knowledge_ids?.length) throw new Error('当前没有可用于提问的知识库，请选择其他空间或联系空间所有者。')
}

export async function streamFailure(response: Response): Promise<string> {
  if (response.status === 401) {
    // Never replay a query after reauthentication; it may already have run.
    window.location.assign(loginPath())
  }
  const data = await response.json().catch(() => null)
  return nativeProblem(data, response.status === 403 ? '访问权限已变化，请重新选择空间和知识范围。' : `请求失败（${response.status}）`)
}
