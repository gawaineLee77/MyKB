<template><t-button v-if="enabled && auth.hasRole('admin')" variant="outline" @click="$router.push('/platform/assistant-publishing')">发布到网页</t-button></template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { get } from '@/utils/request'
import { useAuthStore } from '@/stores/auth'
const auth=useAuthStore(),enabled=ref(false)
onMounted(async()=>{try{enabled.value=Boolean((await get('/api/v1/mindcreek/auth/config')).assistant_enabled)}catch{enabled.value=false}})
</script>
