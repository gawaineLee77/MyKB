<template>
  <main class="installation-page">
    <section>
      <h1>{{ zh ? '企业空间初始化' : 'Enterprise setup' }}</h1>
      <p>{{ message }}</p>
      <p v-if="stage">{{ zh ? '当前状态：' : 'Current stage: ' }}{{ stage }}</p>
      <p>{{ zh ? '由部署人员使用安装命令继续或恢复。' : 'Ask the deployment operator to continue or recover the installation.' }}</p>
      <t-button theme="primary" :loading="busy" @click="load">{{ zh ? '刷新状态' : 'Refresh status' }}</t-button>
      <t-button variant="text" @click="exit">{{ zh ? '退出' : 'Sign out' }}</t-button>
    </section>
  </main>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { get } from '@/utils/request'
import { logout } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
const { locale } = useI18n(), auth = useAuthStore()
const zh = computed(() => locale.value === 'zh-CN')
const message = ref(''), stage = ref(''), busy = ref(false)
async function load() {
  busy.value = true
  try {
    const result = await get<{success:boolean; data:{stage:string}}>('/api/v1/mindcreek/installation')
    stage.value = result.data.stage
    if (stage.value === 'ready') {
      await auth.refreshFromAuthMe()
      window.location.replace(auth.hasValidTenant ? '/platform/knowledge-bases' : '/onboarding/workspace')
      return
    }
    message.value = zh.value ? '管理员已登录，正在等待默认空间初始化。' : 'Signed in. Waiting for default workspace initialization.'
  } catch { message.value = zh.value ? '无法读取安装状态，请重新登录或联系部署人员。' : 'Setup status is unavailable. Sign in again or contact the deployment operator.' }
  finally { busy.value = false }
}
async function exit() { await logout(); auth.logout(); window.location.assign('/admin/login') }
onMounted(load)
</script>
<style scoped>
.installation-page{min-height:100vh;display:grid;place-items:center;padding:24px;background:#f4f8f8;color:#12333c}section{max-width:560px;padding:32px;border:1px solid #dce8e7;background:white;border-radius:18px}p{line-height:1.7;color:#547078}
</style>
