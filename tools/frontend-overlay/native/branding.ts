import { computed, onMounted, onUnmounted, readonly, ref, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { put } from '@/utils/request'
import fallbackLogo from '@/assets/img/mindcreek-mark.png'
import { BRANDING_ENDPOINT, DEFAULT_SERVICE_NAME, brandingInput, configuredBrandText, parseBranding, type ServiceBranding } from './branding-policy'

const branding = ref<ServiceBranding>({ name: DEFAULT_SERVICE_NAME, logo_url: '', revision: 0 })
const installationAdmin = ref(false)
export const canManageServiceBranding = readonly(installationAdmin)
export function setBrandingAdmin(value: boolean) { installationAdmin.value = value }
export const brandName = computed(() => branding.value.name)
export const brandLogo = computed(() => branding.value.logo_url || fallbackLogo)
export const serviceBranding = readonly(branding)
// These strings are compiled translations owned by the product, not user content.
export function brandText(text: string): string { return configuredBrandText(text, branding.value.name) }
let pending: Promise<ServiceBranding> | undefined
export function refreshBranding(): Promise<ServiceBranding> {
  return pending ??= fetch(BRANDING_ENDPOINT, { credentials: 'same-origin', cache: 'no-store' })
    .then(async response => {
      if (!response.ok) throw new Error('无法读取服务外观。')
      const result = await response.json()
      if (!result.success) throw new Error('无法读取服务外观。')
      const next = parseBranding(result.data)
      if (next.revision >= branding.value.revision) branding.value = next
      return branding.value
    }).finally(() => { pending = undefined })
}
export async function saveBranding(revision: number, name: string, logo?: string | null) {
  const response: any = await put(BRANDING_ENDPOINT, brandingInput(revision, name, logo))
  if (!response.success) throw new Error(response.message || '无法保存服务外观。')
  branding.value = parseBranding(response.data)
  // This event contains no configuration. Other tabs fetch authoritative public data.
  localStorage.setItem('mindcreek_branding_changed', String(branding.value.revision))
  return branding.value
}
export function useBrandingRuntime() {
  const auth = useAuthStore()
  const refresh = () => { if (document.visibilityState !== 'hidden') void refreshBranding().catch(() => {}) }
  const changed = (event: StorageEvent) => { if (event.key === 'mindcreek_branding_changed') refresh() }
  watch(() => auth.token, token => { if (!token) installationAdmin.value = false })
  watch(branding, value => {
    document.title = value.name
    document.querySelector<HTMLMetaElement>('meta[name="description"]')?.setAttribute('content', `${value.name} 是面向组织内部用户的私有知识库与智能体平台。`)
    for (const link of document.querySelectorAll<HTMLLinkElement>('link[rel="icon"], link[rel="shortcut icon"], link[rel="apple-touch-icon"]')) {
      link.href = value.logo_url || '/mindcreek-favicon.png'
      link.type = 'image/png'
    }
  }, { immediate: true })
  onMounted(() => { refresh(); window.addEventListener('focus', refresh); window.addEventListener('storage', changed); document.addEventListener('visibilitychange', refresh) })
  onUnmounted(() => { window.removeEventListener('focus', refresh); window.removeEventListener('storage', changed); document.removeEventListener('visibilitychange', refresh) })
}
