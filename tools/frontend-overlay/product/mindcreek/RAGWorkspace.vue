<template>
  <main class="mc-rag">
    <header class="mc-rag-header">
      <button class="mc-back" type="button" @click="router.push('/platform/knowledge-bases')"><t-icon name="chevron-left" /> {{ text.back }}</button>
      <div class="mc-title"><span>MindCreek · Plain RAG</span><h1>{{ text.title }}</h1><p>{{ text.subtitle }}</p></div>
      <div class="mc-header-actions">
        <button type="button" @click="router.push(`/platform/knowledge-bases/${kbId}`)">{{ text.details }}</button>
        <button class="primary" type="button" @click="router.push(`/platform/knowledge-bases/${kbId}/creatChat`)"><t-icon name="chat" /> {{ text.ask }}</button>
      </div>
    </header>

    <section class="mc-upload" :class="{ dragging }" @dragover.prevent="dragging = true" @dragleave.prevent="dragging = false" @drop.prevent="onDrop">
      <input ref="fileInput" type="file" multiple :accept="acceptedTypes" hidden @change="onSelect" />
      <span class="mc-upload-icon"><t-icon name="upload" /></span>
      <div><strong>{{ text.upload }}</strong><p>{{ text.formats }}</p></div>
      <button class="primary" type="button" :disabled="uploading" @click="fileInput?.click()">{{ uploading ? text.uploading : text.choose }}</button>
    </section>
    <p v-if="operationError" class="mc-operation-error"><t-icon name="error-circle" /> {{ operationError }}</p>

    <section class="mc-documents">
      <header><div><h2>{{ text.documents }}</h2><span>{{ page.total }} {{ text.files }}</span></div><button type="button" :disabled="loading" @click="loadList"><t-icon name="refresh" /> {{ text.refresh }}</button></header>
      <div v-if="loading && page.items.length === 0" class="mc-empty"><t-loading /> {{ text.loading }}</div>
      <div v-else-if="listError" class="mc-empty error">{{ listError }}<button @click="loadList">{{ text.retry }}</button></div>
      <div v-else-if="page.items.length === 0" class="mc-empty"><t-icon name="file" /><strong>{{ text.empty }}</strong><span>{{ text.emptyHint }}</span></div>
      <div v-else class="mc-table-wrap">
        <table>
          <thead><tr><th>{{ text.document }}</th><th>{{ text.size }}</th><th>{{ text.status }}</th><th>{{ text.updated }}</th><th><span class="sr-only">{{ text.actions }}</span></th></tr></thead>
          <tbody>
            <tr v-for="document in page.items" :key="document.id">
              <td><div class="mc-file"><span>{{ extension(document) }}</span><div><strong>{{ document.file_name || document.title }}</strong><small v-if="document.error_message">{{ document.error_message }}</small></div></div></td>
              <td>{{ formatBytes(document.file_size) }}</td>
              <td><span class="mc-status" :class="`status-${document.parse_status}`"><i />{{ statusLabel(document.parse_status) }}</span></td>
              <td>{{ formatDate(document.updated_at) }}</td>
              <td class="mc-row-actions">
                <button v-if="isActive(document.parse_status)" type="button" :disabled="busyId === document.id" @click="cancel(document)">{{ text.cancel }}</button>
                <button v-if="canRetry(document.parse_status)" type="button" :disabled="busyId === document.id" @click="retry(document)">{{ text.retry }}</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <footer v-if="page.total > page.page_size" class="mc-pagination">
        <button :disabled="page.page <= 1" @click="changePage(-1)"><t-icon name="chevron-left" /></button>
        <span>{{ page.page }} / {{ Math.ceil(page.total / page.page_size) }}</span>
        <button :disabled="page.page * page.page_size >= page.total" @click="changePage(1)"><t-icon name="chevron-right" /></button>
      </footer>
    </section>

    <aside class="mc-preset"><t-icon name="verified" /><div><strong>{{ text.preset }}</strong><p>{{ text.presetHint }}</p></div><span>v1</span></aside>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import { cancelRAGDocument, listRAGDocuments, retryRAGDocument, uploadRAGDocument, type RAGDocument, type RAGDocumentPage } from './api'

const route = useRoute()
const router = useRouter()
const { locale } = useI18n()
const kbId = computed(() => String(route.params.kbId || ''))
const acceptedTypes = '.md,.txt,.pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.csv,.html,.htm,.json,.xml'
const fileInput = ref<HTMLInputElement | null>(null)
const loading = ref(false)
const uploading = ref(false)
const dragging = ref(false)
const busyId = ref('')
const listError = ref('')
const operationError = ref('')
const page = reactive<RAGDocumentPage>({ items: [], total: 0, page: 1, page_size: 20 })
let pollTimer: number | undefined

const words = {
  en: { back: 'Knowledge bases', title: 'Document knowledge base', subtitle: 'Upload documents and let WeKnora parse, chunk, and index them for hybrid retrieval.', details: 'Advanced view', ask: 'Ask this KB', upload: 'Drop documents here', formats: 'Markdown, text, PDF, Office, CSV, HTML, JSON or XML · up to 50 MiB each', choose: 'Choose files', uploading: 'Uploading…', documents: 'Documents', files: 'files', refresh: 'Refresh', loading: 'Loading documents…', retry: 'Retry', empty: 'No documents yet', emptyHint: 'Upload a file to start building this knowledge base.', document: 'Document', size: 'Size', status: 'Status', updated: 'Updated', actions: 'Actions', cancel: 'Cancel', preset: 'Managed Plain RAG preset', presetHint: 'Vector + keyword indexing, local storage, GraphRAG and Wiki generation disabled.' },
  zh: { back: '知识库', title: '文档知识库', subtitle: '上传文档，由 WeKnora 完成解析、切分与索引，用于混合检索。', details: '高级视图', ask: '向知识库提问', upload: '拖放文档到这里', formats: '支持 Markdown、文本、PDF、Office、CSV、HTML、JSON 或 XML · 单文件不超过 50 MiB', choose: '选择文件', uploading: '正在上传…', documents: '文档', files: '个文件', refresh: '刷新', loading: '正在加载文档…', retry: '重试', empty: '暂无文档', emptyHint: '上传文件以开始构建知识库。', document: '文档', size: '大小', status: '状态', updated: '更新时间', actions: '操作', cancel: '取消', preset: '受管控的 Plain RAG 预设', presetHint: '向量 + 关键词索引、本地存储；GraphRAG 与 Wiki 生成功能保持关闭。' },
}
const text = computed(() => locale.value.startsWith('zh') ? words.zh : words.en)

function messageOf(error: unknown) {
  return error && typeof error === 'object' && 'message' in error ? String(error.message) : String(error || 'Unknown error')
}
function isActive(status: string) { return ['pending', 'processing', 'finalizing'].includes(status) }
function canRetry(status: string) { return ['failed', 'cancelled'].includes(status) }
function statusLabel(status: string) {
  const labels: Record<string, [string, string]> = { pending: ['Queued', '等待中'], processing: ['Processing', '处理中'], finalizing: ['Finalizing', '收尾中'], completed: ['Ready', '已就绪'], failed: ['Failed', '失败'], cancelled: ['Cancelled', '已取消'] }
  const pair = labels[status] || [status || 'Unknown', status || '未知']
  return locale.value.startsWith('zh') ? pair[1] : pair[0]
}
function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MiB`
}
function formatDate(value: string) { return value ? new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '—' }
function extension(document: RAGDocument) { return (document.file_type || document.file_name.split('.').pop() || 'file').replace('.', '').slice(0, 5).toUpperCase() }

async function loadList() {
  if (loading.value) return
  loading.value = true
  listError.value = ''
  try { Object.assign(page, await listRAGDocuments(kbId.value, page.page)) }
  catch (error) { listError.value = messageOf(error) }
  finally { loading.value = false }
}
async function uploadFiles(files: FileList | File[]) {
  if (!files.length || uploading.value) return
  uploading.value = true
  operationError.value = ''
  const failures: string[] = []
  for (const file of Array.from(files)) {
    try { await uploadRAGDocument(kbId.value, file) }
    catch (error) { failures.push(`${file.name}: ${messageOf(error)}`) }
  }
  operationError.value = failures.join(' · ')
  uploading.value = false
  if (fileInput.value) fileInput.value.value = ''
  await loadList()
}
function onSelect(event: Event) { const files = (event.target as HTMLInputElement).files; if (files) void uploadFiles(files) }
function onDrop(event: DragEvent) { dragging.value = false; if (event.dataTransfer?.files) void uploadFiles(event.dataTransfer.files) }
async function retry(document: RAGDocument) { busyId.value = document.id; operationError.value = ''; try { await retryRAGDocument(kbId.value, document.id); await loadList() } catch (error) { operationError.value = messageOf(error) } finally { busyId.value = '' } }
async function cancel(document: RAGDocument) { busyId.value = document.id; operationError.value = ''; try { await cancelRAGDocument(kbId.value, document.id); await loadList() } catch (error) { operationError.value = messageOf(error) } finally { busyId.value = '' } }
async function changePage(delta: number) { page.page += delta; await loadList() }

onMounted(async () => { await loadList(); pollTimer = window.setInterval(() => { if (page.items.some(item => isActive(item.parse_status))) void loadList() }, 3500) })
onUnmounted(() => { if (pollTimer) window.clearInterval(pollTimer) })
</script>

<style scoped>
.mc-rag { min-height: 100%; overflow: auto; padding: 38px clamp(24px, 5vw, 76px) 60px; color: #173c35; background: radial-gradient(circle at 90% 0, #def4eb 0, transparent 30%), #f7faf8; }
.mc-rag-header { max-width: 1120px; margin: 0 auto 28px; display: grid; grid-template-columns: 150px 1fr auto; gap: 20px; align-items: start; }.mc-title { text-align: center; }.mc-title span { color: #1c8063; font: 700 11px/1.3 sans-serif; letter-spacing: 1.7px; text-transform: uppercase; }.mc-title h1 { margin: 5px 0 7px; font: 650 34px/1.15 Georgia, serif; }.mc-title p { margin: 0; color: #698079; }.mc-back, .mc-header-actions button, .mc-documents button, .mc-row-actions button { padding: 8px 11px; border: 1px solid #ccddd6; border-radius: 9px; color: #41665b; background: #fff; cursor: pointer; }.mc-back { border: 0; background: transparent; }.mc-header-actions { display: flex; gap: 8px; }.primary { color: #fff !important; border-color: #207d61 !important; background: #207d61 !important; }
.mc-upload { max-width: 1060px; margin: 0 auto 16px; padding: 24px 28px; display: grid; grid-template-columns: 48px 1fr auto; gap: 17px; align-items: center; border: 1.5px dashed #9fc6b8; border-radius: 15px; background: rgba(255,255,255,.87); transition: .15s; }.mc-upload.dragging { border-color: #207d61; background: #eef9f4; transform: scale(1.005); }.mc-upload-icon { width: 48px; height: 48px; display: grid; place-items: center; border-radius: 13px; color: #207d61; background: #dff3eb; font-size: 24px; }.mc-upload strong { font-size: 16px; }.mc-upload p { margin: 4px 0 0; color: #758a83; font-size: 12px; }.mc-upload button { padding: 10px 18px; border-radius: 9px; cursor: pointer; }.mc-operation-error { max-width: 1060px; margin: 0 auto 16px; color: #a33b3b; }
.mc-documents { max-width: 1120px; margin: 0 auto; border: 1px solid #dce8e3; border-radius: 16px; background: #fff; box-shadow: 0 14px 40px rgba(43,84,72,.07); overflow: hidden; }.mc-documents > header { padding: 18px 22px; display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid #e6efeb; }.mc-documents h2 { display: inline; margin: 0 9px 0 0; font-size: 19px; }.mc-documents header span { color: #83958f; font-size: 12px; }.mc-table-wrap { overflow-x: auto; }table { width: 100%; border-collapse: collapse; }th, td { padding: 14px 18px; text-align: left; border-bottom: 1px solid #edf2f0; font-size: 13px; }th { color: #72867f; background: #fafcfb; font-size: 11px; letter-spacing: .5px; text-transform: uppercase; }.mc-file { display: flex; gap: 12px; align-items: center; }.mc-file > span { width: 42px; padding: 8px 3px; text-align: center; border-radius: 8px; color: #276d59; background: #e5f3ed; font-size: 9px; font-weight: 700; }.mc-file strong, .mc-file small { display: block; max-width: 440px; }.mc-file small { margin-top: 3px; color: #a23c3c; white-space: normal; }.mc-status { display: inline-flex; gap: 7px; align-items: center; white-space: nowrap; }.mc-status i { width: 7px; height: 7px; border-radius: 50%; background: #82938e; }.status-completed { color: #197050; }.status-completed i { background: #21a171; }.status-pending, .status-processing, .status-finalizing { color: #8b6a1d; }.status-pending i, .status-processing i, .status-finalizing i { background: #d5a125; }.status-failed, .status-cancelled { color: #a13b3b; }.status-failed i, .status-cancelled i { background: #c84b4b; }.mc-row-actions { text-align: right; }.mc-empty { min-height: 230px; display: flex; flex-direction: column; gap: 9px; align-items: center; justify-content: center; color: #81938d; }.mc-empty > .t-icon { font-size: 34px; }.mc-empty.error { color: #a13b3b; }.mc-pagination { padding: 12px 18px; display: flex; justify-content: flex-end; align-items: center; gap: 10px; }
.mc-preset { max-width: 1060px; margin: 16px auto 0; padding: 14px 18px; display: grid; grid-template-columns: 28px 1fr auto; gap: 10px; align-items: center; border-radius: 12px; color: #49665e; background: #eaf4f0; }.mc-preset p { margin: 3px 0 0; font-size: 12px; }.mc-preset > span { padding: 4px 9px; border-radius: 99px; color: #207d61; background: #fff; font-size: 11px; }.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0,0,0,0); }
@media (max-width: 800px) { .mc-rag { padding: 24px 14px 45px; }.mc-rag-header { grid-template-columns: 1fr; }.mc-title { text-align: left; }.mc-header-actions { flex-wrap: wrap; }.mc-upload { grid-template-columns: 45px 1fr; }.mc-upload button { grid-column: 1 / -1; }.mc-file strong { max-width: 220px; }.mc-preset { margin-inline: 0; } }
</style>
