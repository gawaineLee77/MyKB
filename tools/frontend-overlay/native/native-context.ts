import { onMounted, onUnmounted, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useSettingsStore } from '@/stores/settings'
import { useChatResourcesStore } from '@/stores/chatResources'
import { useOrganizationStore } from '@/stores/organization'
import { workspaceHome } from './native-policy'
import { KB_PERMISSIONS_CHANGED } from './kb-permissions'
import { createWorkspaceRefresh, provideWorkspaceRefresh } from './workspace-refresh'

// Revalidate on returning to the tab. A full navigation discards active requests
// and resource caches when the effective membership changes.
export function useNativeWorkspaceContext() {
  const auth = useAuthStore()
  const settings = useSettingsStore(), resources = useChatResourcesStore(), organizations = useOrganizationStore()
  let disposed = false
  const identity = () => JSON.stringify([auth.user?.id, auth.effectiveTenantId, auth.currentTenantRole])
  const coordinator = createWorkspaceRefresh(async () => {
    const startedAs = identity()
    const authenticated = await auth.refreshFromAuthMe()
    if (disposed || identity() !== startedAs) return
    // Recheck access-sensitive content without erasing the last known model
    // catalog. An empty cache is otherwise rendered as missing configuration.
    resources.invalidate('knowledgeBases', 'agents', 'models')
    resources.rawKnowledgeBases = []
    resources.agents = []
    resources.disabledOwnAgentIds = []
    organizations.clearState()
    if (!authenticated) throw new Error('Workspace authentication could not be refreshed')
    const [knowledgeBases] = await Promise.allSettled([
      resources.ensureKnowledgeBases(true),
      resources.ensureAgents(true),
      resources.ensureModels(true),
    ])
    if (disposed || identity() !== startedAs) return
    if (knowledgeBases.status === 'rejected') throw knowledgeBases.reason
    const visible = new Set(resources.validKnowledgeBases.map(kb => kb.id))
    settings.selectKnowledgeBases(settings.settings.selectedKnowledgeBases.filter(id => visible.has(id)))
  })
  provideWorkspaceRefresh(coordinator)
  const refresh = () => {
    if (document.visibilityState !== 'hidden') void coordinator.refresh()
  }
  watch(identity, (value, previous) => {
    if (!previous || value === previous) return
    coordinator.dispose()
    settings.selectAgent('builtin-quick-answer')
    resources.invalidate()
    organizations.clearState()
    // Reload also prevents an old in-flight cache request committing after reset.
    window.location.replace(auth.hasValidTenant ? workspaceHome(auth.currentTenantRole) : '/onboarding/workspace')
  })
  onMounted(() => { window.addEventListener('focus', refresh); window.addEventListener(KB_PERMISSIONS_CHANGED, refresh); document.addEventListener('visibilitychange', refresh) })
  onUnmounted(() => { disposed = true; coordinator.dispose(); window.removeEventListener('focus', refresh); window.removeEventListener(KB_PERMISSIONS_CHANGED, refresh); document.removeEventListener('visibilitychange', refresh) })
}
