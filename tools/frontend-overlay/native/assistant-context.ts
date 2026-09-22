// Employee access tokens remain in this iframe's memory, never in the host page.
const match = typeof window !== 'undefined' ? window.location.pathname.match(/^\/assistant\/(\d+)\/([\w-]+)$/) : null
export const assistantActive = Boolean(match)
export const assistantTenant = match?.[1] || ''
export const assistantChannel = match?.[2] || ''
export const assistantOrigin = typeof window !== 'undefined' ? new URLSearchParams(window.location.search).get('host_origin') || window.location.origin : ''
let accessToken = ''
let sessionID = ''
export const setAssistantToken = (token: string) => { accessToken = token; sessionID = '' }
export const setAssistantSession = (id: string) => { sessionID = id }
export const assistantPrefix = () => `/api/v1/mindcreek/assistant/${assistantTenant}/${assistantChannel}`
export const assistantHeaders = (): Record<string, string> => ({
  Authorization: `Bearer ${accessToken}`, 'X-Tenant-ID': assistantTenant,
  'X-MindCreek-Host-Origin': assistantOrigin, 'X-MindCreek-Session-ID': sessionID,
})
export function assistantNativeURL(path: string): string {
  const target = new URL(path, window.location.origin)
  if (target.origin !== window.location.origin) throw new Error('不允许请求其他服务')
  if (target.pathname.startsWith(assistantPrefix() + '/')) return target.pathname + target.search
  const citation = target.pathname.match(/^\/api\/v1\/embed\/[\w-]+\/chunks\/([\w-]+)$/)
  if (citation) target.pathname = `/api/v1/chunks/by-id/${citation[1]}`
  if (!target.pathname.startsWith('/api/v1/')) throw new Error('不支持此资源路径')
  return `${assistantPrefix()}/proxy${target.pathname}${target.search}`
}
export async function assistantFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const response = await fetch(path.startsWith('/api/') ? assistantNativeURL(path) : `${assistantPrefix()}/${path}`, {
    ...init, credentials: 'omit', headers: { ...Object.fromEntries(new Headers(init.headers)), ...assistantHeaders() },
  })
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    const code = body?.error?.code || `HTTP ${response.status}`
    if ([401, 403].includes(response.status)) window.dispatchEvent(new CustomEvent('mindcreek-assistant-denied', { detail: code }))
    throw new Error(code)
  }
  return response
}
export async function assistantJSON(path: string, body?: unknown) {
  const response = await assistantFetch(path, body === undefined ? {} : { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
  return response.json()
}

export const ASSISTANT_LOGIN_KEY = 'mindcreek_assistant_login'
export function pendingAssistantLogin(): string | undefined {
  try {
    const value = JSON.parse(sessionStorage.getItem(ASSISTANT_LOGIN_KEY) || 'null')
    if (value && /^[a-f0-9]{64}$/.test(value.nonce) && Date.now() - value.at < 10 * 60_000) return `/assistant-login?nonce=${value.nonce}`
    sessionStorage.removeItem(ASSISTANT_LOGIN_KEY)
  } catch { sessionStorage.removeItem(ASSISTANT_LOGIN_KEY) }
}
