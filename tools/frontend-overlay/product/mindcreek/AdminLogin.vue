<template>
  <main class="admin-page">
    <form class="admin-card" @submit.prevent="signIn">
      <img src="@/assets/img/mindcreek-mark.png" alt="" class="logo" />
      <h1>{{ zh ? '管理员登录' : 'Administrator sign-in' }}</h1>
      <p>{{ zh ? '使用安装时配置的本地管理员账号。' : 'Use the local administrator account configured during installation.' }}</p>
      <label>{{ zh ? '邮箱' : 'Email' }}<input v-model="email" type="email" autocomplete="username" required :disabled="busy" /></label>
      <label>{{ zh ? '密码' : 'Password' }}<input v-model="password" type="password" autocomplete="current-password" required :disabled="busy" /></label>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <t-button type="submit" theme="primary" block :loading="busy">{{ zh ? '登录' : 'Sign in' }}</t-button>
      <a href="/login" @click="markCorporateSession">{{ zh ? '企业员工登录' : 'Employee sign-in' }}</a>
    </form>
  </main>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { post } from '@/utils/request'
import { useAuthStore } from '@/stores/auth'
import { userInfoFromApi, type LoginResponse } from '@/api/auth'
import { markCorporateSession, markLocalAdminSession } from './enterprise-auth'
const { locale } = useI18n()
const zh = computed(() => locale.value === 'zh-CN')
const email = ref(''), password = ref(''), busy = ref(false), error = ref('')
const auth = useAuthStore()
async function signIn() {
  if (busy.value) return
  busy.value = true; error.value = ''
  try {
    const response = await post<LoginResponse>('/api/v1/mindcreek/admin/auth/login', { email: email.value, password: password.value })
    if (!response.success || !response.token || !response.refresh_token || !response.user) throw new Error('invalid session')
    auth.logout()
    markLocalAdminSession()
    auth.setToken(response.token); auth.setRefreshToken(response.refresh_token)
    auth.setUser(userInfoFromApi(response.user))
    password.value = ''
    window.location.replace('/admin/setup')
  } catch {
    error.value = zh.value ? '无法登录，请检查管理员账号、密码及安装状态。' : 'Sign-in failed. Check the administrator credentials and installation status.'
  } finally { busy.value = false }
}
</script>

<style scoped>
.admin-page{min-height:100vh;display:grid;place-items:center;background:#f4f8f8;padding:24px;color:#12333c}.admin-card{width:min(420px,100%);padding:32px;background:white;border:1px solid #dce8e7;border-radius:18px;display:grid;gap:18px}.logo{width:52px;height:52px}h1,p{margin:0}p{color:#547078;line-height:1.6}label{display:grid;gap:8px}input{width:100%;box-sizing:border-box;padding:11px;border:1px solid #c6d8d5;border-radius:6px;font:inherit}.error{color:#b42318}a{color:#087d77;text-align:center}
</style>
