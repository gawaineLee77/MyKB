import { computed, inject, onMounted, onUnmounted, provide, ref, watch, type InjectionKey } from 'vue'
import { get } from '@/utils/request'
import { useAuthStore } from '@/stores/auth'
import { canReparseKB, hasKBOperation, kbPermissionProblem, type KBEffective } from './kb-permissions-policy'
export const KB_PERMISSIONS_CHANGED = 'mindcreek-kb-permissions-changed'
export const kbPermissionEndpoint = (id: string) => '/api/v1/mindcreek/kb-permissions/' + encodeURIComponent(id)
export async function fetchKBEffective(id: string): Promise<KBEffective> {
  const response: any = await get(kbPermissionEndpoint(id) + '/effective')
  if (!response.success || !response.data) throw new Error(response.message || '无法读取知识库权限。')
  return response.data
}
export function announceKBPermissions() { window.dispatchEvent(new Event(KB_PERMISSIONS_CHANGED)) }
export function useKBPermissions(kbId: () => string | undefined) {
  const auth = useAuthStore(), state = ref<KBEffective | null>(null), error = ref(''), loading = ref(false)
  let sequence = 0
  const refresh = async () => {
    const id = kbId(), request = ++sequence
    error.value = ''
    if (!id) return
    loading.value = true
    try { const value = await fetchKBEffective(id); if (request === sequence) { state.value = value; if (!value.visible) error.value = '访问权限已变更，请重新选择知识库。' } }
    catch (err) { if (request === sequence) { state.value = null; error.value = kbPermissionProblem(err) } }
    finally { if (request === sequence) loading.value = false }
  }
  watch(() => `${auth.effectiveTenantId}:${auth.currentTenantRole}:${kbId()}`, () => { state.value = null; void refresh() }, { immediate: true })
  const onFocus = () => { if (document.visibilityState !== 'hidden') void refresh() }
  onMounted(() => { window.addEventListener('focus', onFocus); window.addEventListener(KB_PERMISSIONS_CHANGED, onFocus); document.addEventListener('visibilitychange', onFocus) })
  onUnmounted(() => { sequence++; window.removeEventListener('focus', onFocus); window.removeEventListener(KB_PERMISSIONS_CHANGED, onFocus); document.removeEventListener('visibilitychange', onFocus) })
  return { state, error, loading, refresh, can: (op: string) => hasKBOperation(state.value, op), canManage: () => state.value?.visible === true && state.value.manage, canReparse: () => canReparseKB(state.value), visible: computed(() => state.value?.visible === true) }
}
export type KBPermissions = ReturnType<typeof useKBPermissions>
const key: InjectionKey<KBPermissions> = Symbol('mindcreek-kb-permissions')
export function provideKBPermissions(value: KBPermissions) { provide(key, value) }
export function useKBPermissionsContext() {
  return inject(key, { state: ref(null), error: ref(''), loading: ref(false), refresh: async () => {}, visible: computed(() => false), can: () => false, canManage: () => false, canReparse: () => false })
}
