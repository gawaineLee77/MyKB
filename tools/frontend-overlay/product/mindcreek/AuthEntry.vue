<template>
  <component :is="selected" v-if="selected" />
  <main v-else class="auth-loading" aria-live="polite">Preparing secure sign-in…</main>
</template>

<script setup lang="ts">
import { defineAsyncComponent, markRaw, onMounted, shallowRef } from 'vue'
import { getOIDCConfig } from '@/api/auth'

const legacyLogin = markRaw(defineAsyncComponent(() => import('@/views/auth/Login.vue')))
const corporateLogin = markRaw(defineAsyncComponent(() => import('@/mindcreek/SSOLogin.vue')))
const selected = shallowRef()

onMounted(async () => {
  try {
    const response = await getOIDCConfig()
    selected.value = response.success && response.enabled === false ? legacyLogin : corporateLogin
  } catch {
    // A configuration/network failure must not expose a password form in a
    // deployment that may have closed registration. The SSO view fails closed.
    selected.value = corporateLogin
  }
})
</script>

<style scoped>
.auth-loading { min-height: 100vh; display: grid; place-items: center; color: #547078; background: #f6fbfa; }
</style>
