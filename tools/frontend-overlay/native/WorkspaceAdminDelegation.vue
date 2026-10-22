<template>
  <section class="workspace-admin-delegation" data-testid="workspace-admin-delegation">
    <div class="delegation-copy">
      <h3>空间管理授权</h3>
      <p>允许本空间 Admin 管理非 Owner 成员、空间名称和描述，以及各自创建的 API Keys。授权也用于校验密钥创建者的有效身份。</p>
      <p v-if="status" role="status">{{ statusText }}</p>
      <t-alert v-if="error" theme="error" :message="error" />
    </div>
    <div class="delegation-actions">
      <t-button variant="outline" :loading="busy" @click="load">刷新状态</t-button>
      <t-button v-if="isOwner && status?.configured" :disabled="busy" @click="confirmOpen = true">{{ status.authorized ? '重新授权' : '授权 Admin 管理' }}</t-button>
      <t-button v-if="isOwner && status?.owner_user_id" theme="danger" variant="outline" :disabled="busy" @click="revoke">撤销授权</t-button>
    </div>
    <t-dialog v-model:visible="confirmOpen" header="授权本空间 Admin 管理" :confirm-btn="{ content: '确认授权', loading: busy }" :close-on-overlay-click="false" @confirm="authorize">
      <p>服务器将建立专用的 Owner 授权会话，仅用于本空间经权限校验的 Admin 管理操作，并记录实际操作者。</p>
      <p>Admin 无法管理 Owner、查看他人的 API Key，或修改空间级身份与签名配置。你可随时在此撤销授权。</p>
      <p>授权 Owner 退出登录、失去 Owner 身份或会话失效后，可能需要重新授权。</p>
    </t-dialog>
  </section>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { get, post, del } from '@/utils/request'
const emit = defineEmits<{ (event: 'ready', value: boolean): void }>()
const auth = useAuthStore(), tenant = String(auth.effectiveTenantId)
const isOwner = computed(() => auth.currentTenantRole === 'owner')
const endpoint = '/api/v1/mindcreek/workspace-admin/delegation'
interface DelegationStatus { configured: boolean; authorized: boolean; owner_user_id?: string; updated_at?: string; error_code?: string }
const status = ref<DelegationStatus | null>(null), error = ref(''), busy = ref(false), confirmOpen = ref(false)
const statusText = computed(() => !status.value?.configured ? '服务器尚未启用管理授权，请联系平台管理员完成配置。' : status.value.authorized ? '已授权：Admin 可使用获准的空间管理功能。' : '未授权或授权已失效：请由 Owner 授权后创建密钥或使用委托管理操作。')
function assertContext() { if (String(auth.effectiveTenantId) !== tenant || !['owner', 'admin'].includes(auth.currentTenantRole || '')) throw new Error('空间或角色已变化，请刷新页面。') }
async function run(action: () => Promise<any>) {
  if (busy.value) return
  busy.value = true; error.value = ''
  try { assertContext(); const response = await action(); if (!response.success || !response.data) throw new Error(response.message || '无法读取授权状态。'); status.value = response.data; emit('ready', Boolean(status.value?.configured && status.value?.authorized)); confirmOpen.value = false } catch (err: any) { emit('ready', false); error.value = err?.message || '授权操作失败，请重试。' } finally { busy.value = false }
}
const load = () => run(() => get(endpoint))
const authorize = () => run(() => post(endpoint, {}))
const revoke = () => run(async () => {
  const response = await del(endpoint)
  if (!response.success) throw new Error(response.message || '撤销授权失败，请重试。')
  // DELETE confirms success without returning status. Clear the local grant
  // immediately, then re-read the server so the UI cannot show stale authority.
  status.value = { configured: status.value?.configured === true, authorized: false }
  emit('ready', false)
  return get(endpoint)
})
onMounted(load)
</script>
<style scoped>
.workspace-admin-delegation { display: flex; gap: 20px; flex-wrap: wrap; align-items: center; justify-content: space-between; padding: 20px; margin-bottom: 24px; border: 1px solid var(--td-component-border); border-radius: 10px; background: var(--td-bg-color-container); }
.delegation-copy { flex: 1; min-width: 280px; }.delegation-copy h3 { margin: 0 0 8px; font-size: 16px; }.delegation-copy p { margin: 6px 0; color: var(--td-text-color-secondary); line-height: 1.6; }.delegation-actions { display: flex; gap: 8px; flex-wrap: wrap; }
</style>
