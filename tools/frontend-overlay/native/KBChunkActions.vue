<template><span class="kb-chunk-actions"><t-popconfirm v-if="chunkId && allowedDelete" content="删除此分块及其索引？父文档仍保留。" @confirm="remove"><t-button theme="danger" variant="text" size="small" :loading="busy">删除分块</t-button></t-popconfirm><t-button v-if="!chunkId && allowedExport" variant="outline" size="small" :loading="busy" @click="exportChunks">导出分块</t-button></span></template>
<script setup lang="ts">
import { ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { del, post } from '@/utils/request'
import { announceKBPermissions, kbPermissionEndpoint } from './kb-permissions'
import { kbPermissionProblem } from './kb-permissions-policy'
const props = defineProps<{ kbId: string; knowledgeId: string; chunkId?: string; allowedDelete?: boolean; allowedExport?: boolean }>()
const emit = defineEmits<{ (event: 'changed'): void }>(), busy = ref(false)
async function run(action: () => Promise<void>) { if (busy.value) return; busy.value = true; try { await action() } catch (err) { MessagePlugin.error(kbPermissionProblem(err)); announceKBPermissions() } finally { busy.value = false } }
const remove = () => run(async () => { if (!props.allowedDelete || !props.chunkId) return; await del(`/api/v1/chunks/${encodeURIComponent(props.knowledgeId)}/${encodeURIComponent(props.chunkId)}`); emit('changed') })
const exportChunks = () => run(async () => { if (!props.allowedExport) return; const blob = await post<Blob>(kbPermissionEndpoint(props.kbId) + '/export-chunks', { knowledge_ids: [props.knowledgeId] }, { responseType: 'blob' }); const url = URL.createObjectURL(blob); const anchor = document.createElement('a'); anchor.href = url; anchor.download = 'chunks.json'; anchor.click(); setTimeout(() => URL.revokeObjectURL(url), 1000) })
</script>
