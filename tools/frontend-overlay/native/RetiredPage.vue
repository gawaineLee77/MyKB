<template>
  <main class="retired-page">
    <h1>此功能已退役</h1>
    <p>知识库发布订阅与个人笔记已关闭。文档、手工 Markdown、FAQ 和 Wiki 可在知识库中使用。</p>
    <t-button @click="router.replace(workspaceHome(auth.currentTenantRole))">返回工作空间</t-button>
  </main>
</template>
<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { workspaceHome } from './native-policy'
const router = useRouter(), auth = useAuthStore()
</script>
<style scoped>.retired-page { padding: 64px; max-width: 800px; } p { margin: 20px 0; line-height: 1.8; }</style>
