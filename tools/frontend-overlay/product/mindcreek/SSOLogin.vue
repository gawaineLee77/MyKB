<template>
  <main class="sso-page">
    <section class="sso-card" aria-live="polite">
      <img src="@/assets/img/mindcreek-mark.png" alt="" class="sso-logo" />
      <p class="eyebrow">MindCreek</p>
      <h1>{{ copy.title }}</h1>
      <p class="summary">{{ statusMessage }}</p>
      <t-button theme="primary" size="large" block :loading="loading" :disabled="!enabled" @click="startLogin(true)">
        {{ loading ? copy.redirecting : copy.continue }}
      </t-button>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <p class="policy">{{ copy.policy }}</p>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { getOIDCAuthorizationURL, getOIDCConfig } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'

const ATTEMPT_KEY = 'mindcreek_sso_attempted_at'
const ATTEMPT_GUARD_MS = 30_000
const { locale } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const loading = ref(false)
const enabled = ref(false)
const provider = ref('Corporate account')
const error = ref('')

const copy = computed(() => locale.value === 'zh-CN' ? {
  title: '使用组织账号登录',
  preparing: `正在连接${provider.value}。`,
  ready: `继续前往${provider.value}完成身份验证。`,
  unavailable: '企业单点登录尚未由管理员配置。',
  continue: '使用组织账号继续',
  redirecting: '正在跳转…',
  policy: 'MindCreek 不提供公开注册。首次通过验证的组织用户会自动创建内部账号。',
  failed: '无法启动企业登录，请重试或联系管理员。',
} : {
  title: 'Sign in with your organization',
  preparing: `Connecting to ${provider.value}.`,
  ready: `Continue to ${provider.value} to verify your identity.`,
  unavailable: 'Corporate single sign-on has not been configured by an administrator.',
  continue: 'Continue with organization account',
  redirecting: 'Redirecting…',
  policy: 'MindCreek has no public registration. A verified organization user is provisioned on first sign-in.',
  failed: 'Corporate sign-in could not be started. Retry or contact an administrator.',
})

const statusMessage = computed(() => {
  if (!enabled.value) return copy.value.unavailable
  return loading.value ? copy.value.preparing : copy.value.ready
})

const callbackURI = () => `${window.location.origin}/api/v1/auth/oidc/callback`

const publicBrokerAuthorizationURL = (raw: string) => {
  const target = new URL(raw)
  if (target.protocol !== 'http:' || target.hostname !== 'gateway' || target.port !== '8080' ||
      target.pathname !== '/api/v1/mindcreek/oidc/authorize') {
    throw new Error('unexpected broker authorization URL')
  }
  return `${window.location.origin}${target.pathname}${target.search}${target.hash}`
}

async function startLogin(force = false) {
  if (!enabled.value || loading.value) return
  const previous = Number(sessionStorage.getItem(ATTEMPT_KEY) || 0)
  if (!force && Date.now() - previous < ATTEMPT_GUARD_MS) return
  loading.value = true
  error.value = ''
  sessionStorage.setItem(ATTEMPT_KEY, String(Date.now()))
  try {
    const response = await getOIDCAuthorizationURL(callbackURI())
    if (!response.success || !response.authorization_url) throw new Error('authorization URL unavailable')
    window.location.assign(publicBrokerAuthorizationURL(response.authorization_url))
  } catch {
    error.value = copy.value.failed
    loading.value = false
  }
}

onMounted(async () => {
  if (authStore.isLoggedIn) {
    await router.replace('/platform/knowledge-bases')
    return
  }
  try {
    const response = await getOIDCConfig()
    enabled.value = Boolean(response.success && response.enabled)
    provider.value = response.provider_display_name || provider.value
    if (enabled.value) await startLogin(false)
  } catch {
    enabled.value = false
    error.value = copy.value.failed
  }
})
</script>

<style scoped>
.sso-page { min-height: 100vh; display: grid; place-items: center; padding: 24px; background: radial-gradient(circle at 20% 10%, #dff8ee 0, transparent 38%), linear-gradient(145deg, #f6fbfa, #edf5f7); color: #12333c; }
.sso-card { width: min(440px, 100%); padding: 42px; border: 1px solid rgba(29, 108, 105, .18); border-radius: 24px; background: rgba(255,255,255,.94); box-shadow: 0 24px 70px rgba(23, 65, 73, .14); text-align: center; }
.sso-logo { width: 72px; height: 72px; object-fit: contain; }
.eyebrow { margin: 14px 0 4px; color: #16867f; font-weight: 700; letter-spacing: .08em; text-transform: uppercase; }
h1 { margin: 0; font-size: clamp(26px, 5vw, 36px); line-height: 1.18; }
.summary { min-height: 48px; margin: 18px 0 24px; color: #547078; line-height: 1.55; }
.error { margin: 16px 0 0; color: #b42318; }
.policy { margin: 22px 0 0; color: #70878d; font-size: 13px; line-height: 1.55; }
</style>
