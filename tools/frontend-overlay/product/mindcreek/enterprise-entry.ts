import { get, post } from '@/utils/request'
import { getCurrentUser } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { enterpriseMode, loginPath, markCorporateSession, markLocalAdminSession } from './enterprise-auth'

export interface OnboardingState {
  state: 'pending' | 'joining' | 'ready' | 'failed' | 'removed'
  memberships: Array<{tenant_id:number; role:string; tenant_name?:string}>
  error_code?: string
  retryable: boolean
}
export async function onboarding(join = false): Promise<OnboardingState> {
  const result = join
    ? await post<{success:boolean; data:OnboardingState}>('/api/v1/mindcreek/onboarding', {})
    : await get<{success:boolean; data:OnboardingState}>('/api/v1/mindcreek/onboarding')
  return result.data
}

// undefined continues native guards; null admits only a recovery page; a
// string redirects. Backend gates enforce the same boundary independently.
export async function enterpriseDestination(path: string): Promise<string | null | undefined> {
  if (!await enterpriseMode()) return undefined
  const auth = useAuthStore()
  if (path === '/register') {
    const token = new URLSearchParams(window.location.search).get('token')
    if (token && token.length <= 4096) sessionStorage.setItem('weknora_pending_invite_token', token)
  }
  if (!auth.token && !localStorage.getItem('weknora_token')) {
    const login = loginPath()
    return path === login ? null : login
  }
  try {
    const response = await getCurrentUser()
    const data = response.data as typeof response.data & { mindcreek?: {local_admin:boolean; installation_ready:boolean} }
    if (!response.success || !data?.user || !data.mindcreek) throw new Error('Identity unavailable')
    if (data.mindcreek.local_admin) {
      markLocalAdminSession()
      if (!data.mindcreek.installation_ready) return path === '/admin/setup' ? null : '/admin/setup'
    } else {
      markCorporateSession()
      const invite = sessionStorage.getItem('weknora_pending_invite_token')
      if (invite) {
        const accepted = await auth.acceptInvitationByTokenAndRefresh(invite)
        if (!accepted.ok) throw new Error('Invitation acceptance failed')
        sessionStorage.removeItem('weknora_pending_invite_token')
      }
      let status = await onboarding()
      if (status.state === 'pending') status = await onboarding(true)
      const accessible = (status.state === 'ready' || status.state === 'removed') && status.memberships.length > 0
      if (!accessible) return path === '/onboarding/workspace' ? null : '/onboarding/workspace'
    }
    await auth.refreshFromAuthMe()
    if (!auth.hasValidTenant) return path === '/onboarding/workspace' ? null : '/onboarding/workspace'
    if (['/', '/login', '/register', '/admin/setup', '/onboarding/workspace'].includes(path)) return '/platform/knowledge-bases'
    return undefined
  } catch {
    // API errors already handle invalid credentials. Keep a transient onboarding
    // failure on a recovery page instead of repeatedly starting corporate SSO.
    if (localStorage.getItem('weknora_token')) return path === '/onboarding/workspace' ? null : '/onboarding/workspace'
    const login = loginPath()
    return path === login ? null : login
  }
}
