<template><img v-if="url" :src="url" :alt="name ? `${name}图标` : '智能体图标'" class="agent-icon-picture" data-testid="agent-icon-preview" /></template>
<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { get } from '@/utils/request'
import { agentIconURL } from './agent-icon-policy'
const props = defineProps<{ avatar?: string; agentId?: string; name?: string }>()
const auth = useAuthStore(), url = ref('')
let sequence = 0
function clear() { if (url.value) URL.revokeObjectURL(url.value); url.value = '' }
async function refresh() {
  const current = ++sequence
  clear()
  if (!auth.token || !auth.user?.id) return
  const path = agentIconURL(props.avatar, props.agentId)
  if (!path) return
  try {
    const blob = await get<Blob>(path, { responseType: 'blob' })
    if (sequence === current && blob instanceof Blob && blob.type === 'image/png') url.value = URL.createObjectURL(blob)
  } catch { /* Keep the existing name-based avatar if access has changed. */ }
}
watch(() => [props.avatar, props.agentId, auth.token, auth.user?.id, auth.effectiveTenantId, auth.currentTenantRole], refresh, { immediate: true, flush: 'sync' })
const onFocus = () => { if (document.visibilityState !== 'hidden') void refresh() }
onMounted(() => { window.addEventListener('focus', onFocus); document.addEventListener('visibilitychange', onFocus) })
onBeforeUnmount(() => { sequence++; clear(); window.removeEventListener('focus', onFocus); document.removeEventListener('visibilitychange', onFocus) })
</script>
<style scoped>.agent-icon-picture { display: block; position: absolute; inset: 0; width: 100%; height: 100%; object-fit: contain; background: var(--td-bg-color-container); z-index: 2; }</style>
