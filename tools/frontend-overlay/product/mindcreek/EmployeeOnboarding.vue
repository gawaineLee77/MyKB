<template>
  <NativeWorkspaceOnboarding v-if="localAdmin" />
  <main v-else class="onboarding-page">
    <section aria-live="polite">
      <h1>{{ zh ? '企业空间' : 'Enterprise workspace' }}</h1>
      <p>{{ message }}</p>
      <t-button v-if="state?.retryable && !busy" theme="primary" @click="load(true)">{{ zh ? '重试加入' : 'Retry joining' }}</t-button>
      <t-button variant="outline" :loading="busy" @click="load(false)">{{ zh ? '刷新状态' : 'Refresh status' }}</t-button>
      <t-button variant="text" @click="exit">{{ zh ? '退出' : 'Sign out' }}</t-button>
    </section>
  </main>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { logout } from '@/api/auth'
import NativeWorkspaceOnboarding from '@/views/auth/WorkspaceOnboarding.vue'
import { onboarding, type OnboardingState } from './enterprise-entry'
import { enterpriseMode, isLocalAdminSession, logoutPath } from './enterprise-auth'
const { locale } = useI18n(), auth = useAuthStore()
const zh = computed(() => locale.value === 'zh-CN'), localAdmin = ref(false)
const state = ref<OnboardingState>(), busy = ref(false), failed = ref(false)
const message = computed(() => {
  if (busy.value) return zh.value ? '正在确认你的空间成员关系…' : 'Checking workspace membership…'
  if (failed.value) return zh.value ? '暂时无法读取开户状态，请稍后刷新或联系管理员。' : 'Onboarding status is unavailable. Refresh later or contact an administrator.'
  if (state.value?.state === 'removed') return zh.value ? '你目前没有可访问的空间，请联系空间所有者添加成员。' : 'You have no accessible workspace. Contact a workspace owner.'
  if (state.value?.state === 'failed') return state.value.retryable
    ? (zh.value ? '暂时无法加入默认空间，可以重试或联系管理员。' : 'Unable to join the default workspace. Retry or contact an administrator.')
    : (zh.value ? '加入结果需要管理员核对，请联系空间所有者恢复成员关系。' : 'An owner needs to verify and restore your membership.')
  return zh.value ? '账号已创建，正在准备默认空间。' : 'Your account is ready. Preparing the default workspace.'
})
async function load(join = false) {
  if (busy.value) return
  busy.value = true; failed.value = false
  try {
    state.value = await onboarding(join)
    if (state.value.state === 'pending' && !join) state.value = await onboarding(true)
    if (['ready', 'removed'].includes(state.value.state) && state.value.memberships.length) {
      await auth.refreshFromAuthMe(); window.location.replace('/platform/knowledge-bases')
    }
  } catch { failed.value = true } finally { busy.value = false }
}
async function exit() { const target = logoutPath(); await logout(); auth.logout(); window.location.assign(target) }
onMounted(async () => {
  localAdmin.value = !await enterpriseMode() || isLocalAdminSession()
  if (localAdmin.value) await auth.refreshFromAuthMe()
  else await load()
})
</script>
<style scoped>
.onboarding-page{min-height:100vh;display:grid;place-items:center;background:#f4f8f8;padding:24px;color:#12333c}section{width:min(540px,100%);background:white;border:1px solid #dce8e7;border-radius:18px;padding:32px}p{line-height:1.7;color:#547078}.detail{font-size:12px;overflow-wrap:anywhere}
</style>
