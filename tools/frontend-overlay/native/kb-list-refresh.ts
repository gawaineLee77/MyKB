import { onUnmounted, ref, watch, type Ref } from 'vue'
import type { KBEffective } from './kb-permissions-policy'
import { useWorkspaceRefresh } from './workspace-refresh'

interface Row { id: string }
interface SharedRow { knowledge_base: Row }

// Publish a list and its permissions as one snapshot. A superseded request
// must never publish either its rows or its ACL into the current view.
export function useKBListRefresh<T extends Row, S extends SharedRow>(options: {
  identity: () => string
  load: (force: boolean) => Promise<{ items: T[]; organizationItems: S[] }>
  access: (id: string) => Promise<KBEffective>
}) {
  const workspace = useWorkspaceRefresh()
  const items = ref<T[]>([]) as Ref<T[]>
  const organizationItems = ref<S[]>([]) as Ref<S[]>
  const access = ref<Record<string, KBEffective>>({})
  const loading = ref(true), ready = ref(false), error = ref('')
  let generation = 0, disposed = false

  function invalidate() {
    generation++
    items.value = []
    organizationItems.value = []
    access.value = {}
    ready.value = false
    error.value = ''
    loading.value = true
  }

  async function refresh(force = false) {
    if (disposed) return
    invalidate()
    // The parent's completion notification will load the list using the new
    // cache. Do not race its authentication/permission invalidation here.
    if (workspace?.state.value.status === 'loading') return
    if (workspace?.state.value.status === 'error') {
      loading.value = false
      error.value = '空间访问状态刷新失败，请重试。'
      return
    }
    const request = generation, identity = options.identity()
    const current = () => !disposed && request === generation && identity === options.identity()
    try {
      const result = await options.load(force)
      if (!current()) return
      const ids = [...new Set([...result.items.map(row => row.id), ...result.organizationItems.map(row => row.knowledge_base.id)])]
      let unavailable = false
      const entries = await Promise.all(ids.map(async id => {
        try { return [id, await options.access(id)] as const }
        catch (cause: any) {
          // Confirmed removal/denial is different from an unavailable check.
          if (![403, 404].includes(cause?.status)) unavailable = true
          return null
        }
      }))
      if (!current()) return
      const verified = Object.fromEntries(entries.filter((entry): entry is readonly [string, KBEffective] => entry !== null))
      access.value = verified
      items.value = result.items.filter(row => verified[row.id]?.visible === true)
      organizationItems.value = result.organizationItems.filter(row => verified[row.knowledge_base.id]?.visible === true)
      error.value = unavailable ? '部分知识库的访问权限未能核验，请重试。' : ''
      ready.value = true
    } catch {
      if (current()) error.value = '知识库列表加载失败，请重试。'
    } finally {
      if (current()) loading.value = false
    }
  }

  async function retry() {
    if (workspace) await workspace.refresh()
    else await refresh(true)
  }

  watch(options.identity, () => { void refresh(true) }, { flush: 'sync' })
  if (workspace) watch(workspace.state, state => {
    if (state.status === 'loading') invalidate()
    else if (state.status === 'ready') void refresh(false)
    else if (state.status === 'error') {
      invalidate()
      loading.value = false
      error.value = '空间访问状态刷新失败，请重试。'
    }
  }, { flush: 'sync' })
  onUnmounted(() => { disposed = true; invalidate() })
  return { items, organizationItems, access, loading, ready, error, refresh, retry }
}
