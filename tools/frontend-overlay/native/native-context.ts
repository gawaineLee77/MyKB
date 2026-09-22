import { onMounted, onUnmounted, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useSettingsStore } from '@/stores/settings'
import { useChatResourcesStore } from '@/stores/chatResources'
import { useOrganizationStore } from '@/stores/organization'
import { workspaceHome } from './native-policy'

// Revalidate on returning to the tab. A full navigation discards active requests
// and resource caches when the effective membership changes.
export function useNativeWorkspaceContext() {
  const auth = useAuthStore()
  const settings = useSettingsStore(), resources = useChatResourcesStore(), organizations = useOrganizationStore()
  let busy = false
  const refresh = async () => {
    if (document.visibilityState === 'hidden' || busy) return
    busy = true
    try { await auth.refreshFromAuthMe() } finally { busy = false }
  }
  watch(() => `${auth.effectiveTenantId}:${auth.currentTenantRole}`, (value, previous) => {
    if (!previous || value === previous) return
    settings.selectAgent('builtin-quick-answer')
    resources.invalidate()
    organizations.clearState()
    // Reload also prevents an old in-flight cache request committing after reset.
    window.location.replace(auth.hasValidTenant ? workspaceHome(auth.currentTenantRole) : '/onboarding/workspace')
  })
  onMounted(() => { window.addEventListener('focus', refresh); document.addEventListener('visibilitychange', refresh) })
  onUnmounted(() => { window.removeEventListener('focus', refresh); document.removeEventListener('visibilitychange', refresh) })
}
