<template>
  <t-dialog v-model:visible="palette.open" header="搜索我的对话" :footer="false" width="640px">
    <form @submit.prevent="search" class="history-search"><t-input v-model="query" placeholder="输入历史对话中的关键词" autofocus /><t-button type="submit" :loading="loading">搜索</t-button></form>
    <t-alert v-if="error" theme="warning" :message="error" />
    <p v-if="searched && !items.length && !error">没有匹配的可访问对话。</p>
    <button class="history-result" v-for="item in items" :key="item.request_id" @click="open(item.session_id)"><strong>{{ item.session_title || '未命名对话' }}</strong><span>{{ item.query_content }}</span></button>
  </t-dialog>
</template>
<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useCommandPaletteStore } from '@/stores/commandPalette'
import { searchMessages, type MessageSearchGroupItem } from '@/api/chat-history'
const palette = useCommandPaletteStore(), router = useRouter()
const query = ref(''), error = ref(''), loading = ref(false), searched = ref(false), items = ref<MessageSearchGroupItem[]>([])
let generation = 0
watch(() => palette.open, () => { generation++; items.value = []; error.value = ''; searched.value = false; query.value = palette.initialQuery })
async function search() { const current = ++generation; if (!query.value.trim()) return; loading.value = true; error.value = ''; items.value = []; try { const result = await searchMessages({ query: query.value, mode: 'keyword', limit: 30 }); if (current === generation) items.value = result.data?.items || [] } catch (e: any) { if (current === generation) error.value = e.message || '历史搜索失败' } finally { if (current === generation) { loading.value = false; searched.value = true } } }
function open(id: string) { palette.closePalette(); void router.push(`/platform/chat/${encodeURIComponent(id)}`) }
function key(event: KeyboardEvent) { if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); palette.openPalette() } }
onMounted(() => window.addEventListener('keydown', key)); onUnmounted(() => window.removeEventListener('keydown', key))
</script>
<style scoped>.history-search { display:flex; gap:10px; margin-bottom:20px; }.history-result { display:flex; flex-direction:column; gap:8px; text-align:left; width:100%; padding:14px; background:var(--td-bg-color-container); color:var(--td-text-color-primary); border:0; border-bottom:1px solid var(--td-component-border); cursor:pointer; }</style>
