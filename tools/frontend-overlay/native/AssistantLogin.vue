<template>
  <main class="assistant-login">
    <h1>登录企业小助手</h1>
    <p role="status">{{ status }}</p>
    <t-button v-if="error" @click="connect">重试</t-button>
  </main>
</template>
<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { enterpriseDestination } from './enterprise-entry'
import { ASSISTANT_LOGIN_KEY } from './assistant-context'
import { getOIDCAuthorizationURL } from '@/api/auth'
const router = useRouter()
const status = ref('正在验证企业身份…')
const error = ref(false)
const nonce = new URLSearchParams(window.location.search).get('nonce') || ''
let ready = false
function send(event: MessageEvent) {
  if (!ready || event.source !== window.opener || event.origin !== window.location.origin || event.data?.type !== 'mindcreek-assistant-token-request' || event.data?.nonce !== nonce) return
  window.opener.postMessage({ type: 'mindcreek-assistant-token', nonce, token: localStorage.getItem('weknora_token') }, window.location.origin)
  sessionStorage.removeItem(ASSISTANT_LOGIN_KEY)
  ready = false
  status.value = '登录完成，可以关闭此窗口。'
  window.close()
}
async function connect() {
  error.value = false
  if (!/^[a-f0-9]{64}$/.test(nonce) || !window.opener) { status.value = '登录窗口已失去连接，请回到小助手重新登录。'; return }
  sessionStorage.setItem(ASSISTANT_LOGIN_KEY, JSON.stringify({ nonce, at: Date.now() }))
  try {
    const destination = await enterpriseDestination('/assistant-login')
    if (destination) { await router.replace(destination); return }
    if (!localStorage.getItem('weknora_token')) { await router.replace('/login'); return }
    if (localStorage.getItem('mindcreek_auth_kind') !== 'corporate') {
      // Local admin manages channels; consumption requires an employee identity.
      const response = await getOIDCAuthorizationURL(`${window.location.origin}/api/v1/auth/oidc/callback`)
      const target = new URL(response.authorization_url || '')
      if (target.origin !== 'http://gateway:8080' || target.pathname !== '/api/v1/mindcreek/oidc/authorize') throw new Error('Invalid login endpoint')
      window.location.assign(`${window.location.origin}${target.pathname}${target.search}`)
      return
    }
    ready = true
    status.value = '身份验证完成，正在连接小助手…'
    window.opener.postMessage({ type: 'mindcreek-assistant-ready', nonce }, window.location.origin)
  } catch { status.value = '身份验证失败，请重试。'; error.value = true }
}
onMounted(() => { window.addEventListener('message', send); void connect() })
onUnmounted(() => window.removeEventListener('message', send))
</script>
<style scoped>
.assistant-login { min-height:100vh; padding:64px 28px; text-align:center; background:#f4f8f7; color:#163c39; }
.assistant-login h1 { font-size:26px; }.assistant-login p { margin:24px 0; }
</style>
