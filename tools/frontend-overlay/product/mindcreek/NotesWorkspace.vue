<template>
  <main class="mc-notes">
    <aside class="mc-notes-sidebar">
      <header>
        <button type="button" class="mc-icon-button" :aria-label="text.back" @click="router.push('/platform/knowledge-bases')">
          <t-icon name="chevron-left" />
        </button>
        <div><span>MindCreek</span><h1>{{ text.notes }}</h1></div>
        <div class="mc-sidebar-actions">
          <button type="button" class="mc-import-button" :aria-label="text.importFile" @click="fileInput?.click()"><t-icon name="upload" /></button>
          <button type="button" class="mc-new-button" @click="newNote"><t-icon name="add" /> {{ text.new }}</button>
          <input ref="fileInput" type="file" accept=".md,.txt,text/plain,text/markdown" hidden @change="handleImport" />
        </div>
      </header>
      <div v-if="listLoading" class="mc-list-state"><t-loading size="small" /></div>
      <div v-else-if="listError" class="mc-list-state error">{{ listError }}<button @click="loadList">{{ text.retry }}</button></div>
      <nav v-else class="mc-note-list" :aria-label="text.notes">
        <button
          v-for="item in notePage.items"
          :key="item.id"
          type="button"
          :class="{ active: current?.id === item.id }"
          @click="openNote(item.id)"
        >
          <strong>{{ item.title }}</strong>
          <small>{{ formatDate(item.updated_at) }} · v{{ item.version }}</small>
          <span :class="`status-${item.parse_status}`">{{ item.parse_status }}</span>
        </button>
        <div v-if="notePage.items.length === 0" class="mc-list-state">{{ text.empty }}</div>
      </nav>
      <footer v-if="notePage.total > notePage.page_size" class="mc-pagination">
        <button :disabled="notePage.page <= 1" @click="changePage(-1)"><t-icon name="chevron-left" /></button>
        <span>{{ notePage.page }} / {{ Math.ceil(notePage.total / notePage.page_size) }}</span>
        <button :disabled="notePage.page * notePage.page_size >= notePage.total" @click="changePage(1)"><t-icon name="chevron-right" /></button>
      </footer>
    </aside>

    <section class="mc-editor-shell">
      <div v-if="!current" class="mc-welcome">
        <span class="mc-welcome-icon"><t-icon name="edit-1" /></span>
        <h2>{{ text.welcome }}</h2><p>{{ text.welcomeHint }}</p>
        <t-button theme="primary" @click="newNote"><t-icon name="add" /> {{ text.new }}</t-button>
      </div>
      <template v-else>
        <header class="mc-editor-header">
          <input v-model="current.title" :placeholder="text.untitled" maxlength="200" @input="dirty = true" />
          <div>
            <span v-if="dirty" class="mc-unsaved">{{ text.unsaved }}</span>
            <span v-else-if="current.id">v{{ current.version }}</span>
            <t-button v-if="current.id" variant="text" :disabled="saving" @click="toggleHistory"><t-icon name="history" /> {{ text.history }}</t-button>
            <t-button v-if="current.id" variant="text" theme="danger" :disabled="saving" @click="removeCurrent"><t-icon name="delete" /></t-button>
            <t-button theme="primary" :loading="saving" :disabled="!canSave" @click="save">{{ text.save }}</t-button>
          </div>
        </header>
        <div class="mc-editor-tabs">
          <button :class="{ active: view === 'write' }" @click="view = 'write'">{{ text.write }}</button>
          <button :class="{ active: view === 'preview' }" @click="view = 'preview'">{{ text.preview }}</button>
          <span>.md</span>
        </div>
        <textarea
          v-if="view === 'write'"
          v-model="current.content"
          class="mc-markdown-editor"
          :placeholder="text.placeholder"
          spellcheck="true"
          @input="dirty = true"
        />
        <article v-else class="mc-preview"><pre>{{ current.content || text.nothing }}</pre></article>
        <footer class="mc-editor-footer">
          <span>{{ byteSize }} bytes</span><span>{{ text.markdownOnly }}</span>
          <span v-if="saveError" class="error">{{ saveError }}</span>
        </footer>
        <aside v-if="historyOpen" class="mc-history-panel">
          <header><strong>{{ text.history }}</strong><button @click="historyOpen = false"><t-icon name="close" /></button></header>
          <div v-if="historyLoading" class="mc-list-state"><t-loading size="small" /></div>
          <div v-else class="mc-history-list">
            <button v-for="revision in revisions" :key="revision.version" :class="{ active: selectedRevision?.version === revision.version }" @click="previewRevision(revision.version)">
              <strong>v{{ revision.version }} · {{ revision.operation }}</strong>
              <small>{{ formatDate(revision.recorded_at) }}</small>
            </button>
          </div>
          <article v-if="selectedRevision" class="mc-history-preview">
            <h3>{{ selectedRevision.title }}</h3><pre>{{ selectedRevision.content }}</pre>
            <t-button v-if="current && selectedRevision.version < current.version" theme="primary" variant="outline" :loading="saving" @click="restoreRevision(selectedRevision.version)">{{ text.restore }}</t-button>
          </article>
        </aside>
      </template>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import { createNote, deleteNote, getNote, getNoteRevision, importNote, listNoteRevisions, listNotes, restoreNoteRevision, updateNote, type Note, type NotePage, type NoteRevision } from './api'

const route = useRoute()
const router = useRouter()
const { locale } = useI18n()
const kbId = computed(() => String(route.params.kbId || ''))
const listLoading = ref(true)
const listError = ref('')
const saving = ref(false)
const saveError = ref('')
const dirty = ref(false)
const view = ref<'write' | 'preview'>('write')
const fileInput = ref<HTMLInputElement | null>(null)
const historyOpen = ref(false)
const historyLoading = ref(false)
const revisions = ref<NoteRevision[]>([])
const selectedRevision = ref<NoteRevision | null>(null)
const notePage = reactive<NotePage>({ items: [], total: 0, page: 1, page_size: 10 })
const current = ref<Note | null>(null)

const words = {
  en: { back: 'Knowledge bases', notes: 'Personal Notes', new: 'New note', importFile: 'Import Markdown or text', retry: 'Retry', empty: 'No notes yet.', welcome: 'A quiet place for working notes', welcomeHint: 'Write in Markdown, keep drafts focused, and publish them to your private retrieval index.', untitled: 'Untitled note', unsaved: 'Unsaved changes', save: 'Save & index', write: 'Write', preview: 'Preview', history: 'History', restore: 'Restore this version', placeholder: 'Start writing in Markdown…', nothing: 'Nothing to preview yet.', markdownOnly: 'Markdown / UTF-8' },
  zh: { back: '知识库', notes: '个人笔记', new: '新建笔记', importFile: '导入 Markdown 或文本', retry: '重试', empty: '还没有笔记。', welcome: '安静记录工作的地方', welcomeHint: '使用 Markdown 记录内容，并将其发布到你的私人检索索引。', untitled: '未命名笔记', unsaved: '有未保存修改', save: '保存并索引', write: '编辑', preview: '预览', history: '版本历史', restore: '恢复此版本', placeholder: '开始使用 Markdown 记录…', nothing: '暂无可预览内容。', markdownOnly: 'Markdown / UTF-8' },
}
const text = computed(() => locale.value.startsWith('zh') ? words.zh : words.en)
const byteSize = computed(() => new TextEncoder().encode(current.value?.content || '').length)
const canSave = computed(() => !saving.value && !!current.value?.title.trim() && !!current.value?.content.trim())

function errorMessage(error: unknown) {
  return error && typeof error === 'object' && 'message' in error ? String(error.message) : String(error || 'Unknown error')
}

async function loadList() {
  listLoading.value = true
  listError.value = ''
  try {
    Object.assign(notePage, await listNotes(kbId.value, notePage.page))
  } catch (error) {
    listError.value = errorMessage(error)
  } finally {
    listLoading.value = false
  }
}

async function openNote(id: string) {
  if (dirty.value && !window.confirm(locale.value.startsWith('zh') ? '放弃未保存的修改？' : 'Discard unsaved changes?')) return
  saveError.value = ''
  try {
    current.value = await getNote(kbId.value, id)
    dirty.value = false
    view.value = 'write'
  } catch (error) {
    saveError.value = errorMessage(error)
  }
}

function newNote() {
  if (dirty.value && !window.confirm(locale.value.startsWith('zh') ? '放弃未保存的修改？' : 'Discard unsaved changes?')) return
  current.value = { id: '', knowledge_base_id: kbId.value, title: '', content: '', status: 'draft', version: 0, content_size: 0, parse_status: 'draft', created_at: '', updated_at: '' }
  dirty.value = true
  view.value = 'write'
}

async function save() {
  if (!current.value || !canSave.value) return
  saving.value = true
  saveError.value = ''
  try {
    current.value = current.value.id
      ? await updateNote(kbId.value, current.value.id, current.value.title, current.value.content, current.value.version)
      : await createNote(kbId.value, current.value.title, current.value.content)
    dirty.value = false
    await loadList()
    if (historyOpen.value) await loadHistory()
  } catch (error) {
    saveError.value = errorMessage(error)
  } finally {
    saving.value = false
  }
}

async function toggleHistory() {
  historyOpen.value = !historyOpen.value
  if (historyOpen.value) await loadHistory()
}

async function loadHistory() {
  if (!current.value?.id) return
  historyLoading.value = true
  try {
    revisions.value = await listNoteRevisions(kbId.value, current.value.id)
    selectedRevision.value = null
  } catch (error) {
    saveError.value = errorMessage(error)
  } finally {
    historyLoading.value = false
  }
}

async function previewRevision(version: number) {
  if (!current.value?.id) return
  try {
    selectedRevision.value = await getNoteRevision(kbId.value, current.value.id, version)
  } catch (error) {
    saveError.value = errorMessage(error)
  }
}

async function restoreRevision(version: number) {
  if (!current.value?.id || dirty.value || !window.confirm(locale.value.startsWith('zh') ? '恢复此版本并创建一个新版本？' : 'Restore this content as a new version?')) return
  saving.value = true
  try {
    current.value = await restoreNoteRevision(kbId.value, current.value.id, current.value.version, version)
    dirty.value = false
    await Promise.all([loadList(), loadHistory()])
  } catch (error) {
    saveError.value = errorMessage(error)
  } finally {
    saving.value = false
  }
}

async function removeCurrent() {
  if (!current.value?.id || !window.confirm(locale.value.startsWith('zh') ? '确定删除这篇笔记？' : 'Delete this note?')) return
  saving.value = true
  try {
    await deleteNote(kbId.value, current.value.id)
    current.value = null
    dirty.value = false
    await loadList()
  } catch (error) {
    saveError.value = errorMessage(error)
  } finally {
    saving.value = false
  }
}

async function handleImport(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  saving.value = true
  saveError.value = ''
  try {
    current.value = await importNote(kbId.value, file)
    dirty.value = false
    await loadList()
  } catch (error) {
    saveError.value = errorMessage(error)
  } finally {
    saving.value = false
  }
}

async function changePage(delta: number) {
  notePage.page += delta
  current.value = null
  dirty.value = false
  await loadList()
}

function formatDate(value: string) {
  return value ? new Intl.DateTimeFormat(locale.value, { month: 'short', day: 'numeric' }).format(new Date(value)) : ''
}

onMounted(loadList)
</script>

<style scoped>
.mc-notes { height: 100%; min-height: 620px; display: grid; grid-template-columns: 310px 1fr; color: #173c35; background: #fbfdfc; }.mc-notes-sidebar { display: flex; min-width: 0; flex-direction: column; border-right: 1px solid #dce8e3; background: #f3f8f5; }.mc-notes-sidebar header { padding: 22px 16px 16px; display: grid; grid-template-columns: 34px 1fr auto; gap: 10px; align-items: center; border-bottom: 1px solid #dce8e3; }.mc-notes-sidebar header span { color: #268064; font-size: 10px; font-weight: 700; letter-spacing: 1px; text-transform: uppercase; }.mc-notes-sidebar h1 { margin: 2px 0 0; font: 650 20px/1.1 Georgia, serif; }.mc-icon-button, .mc-new-button, .mc-import-button, .mc-note-list button, .mc-pagination button, .mc-editor-tabs button { border: 0; cursor: pointer; }.mc-icon-button { width: 32px; height: 32px; border-radius: 8px; background: #fff; color: #44665c; }.mc-sidebar-actions { display: flex; gap: 5px; }.mc-import-button { width: 32px; border-radius: 8px; color: #34755f; background: #e2f0ea; }.mc-new-button { padding: 8px 10px; border-radius: 8px; color: #fff; background: #237b60; white-space: nowrap; }.mc-note-list { flex: 1; overflow: auto; padding: 10px; }.mc-note-list > button { position: relative; width: 100%; padding: 14px 64px 14px 13px; display: block; text-align: left; border-radius: 10px; color: #2f4d45; background: transparent; }.mc-note-list > button:hover { background: #e8f2ed; }.mc-note-list > button.active { background: #dcefe7; box-shadow: inset 3px 0 #287e63; }.mc-note-list strong, .mc-note-list small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.mc-note-list small { margin-top: 5px; color: #7b9089; }.mc-note-list button > span { position: absolute; right: 10px; top: 15px; color: #6d847c; font-size: 10px; }.mc-note-list .status-failed { color: #b24444; }.mc-list-state { padding: 32px 15px; text-align: center; color: #7b9089; }.mc-list-state button { margin-left: 7px; }.mc-list-state.error { color: #ad4545; }.mc-pagination { padding: 10px; display: flex; justify-content: center; gap: 12px; align-items: center; border-top: 1px solid #dce8e3; }.mc-pagination button { width: 28px; height: 28px; border-radius: 6px; background: #fff; }.mc-editor-shell { min-width: 0; display: flex; flex-direction: column; }.mc-welcome { margin: auto; max-width: 450px; text-align: center; }.mc-welcome-icon { width: 64px; height: 64px; margin: 0 auto 18px; display: grid; place-items: center; border-radius: 18px; color: #25775e; background: #def1e9; font-size: 30px; }.mc-welcome h2 { margin: 0 0 8px; font: 650 28px/1.2 Georgia, serif; }.mc-welcome p { margin: 0 0 22px; color: #73877f; line-height: 1.6; }.mc-editor-header { height: 74px; padding: 0 26px; display: flex; align-items: center; gap: 20px; border-bottom: 1px solid #e2ebe7; }.mc-editor-header input { flex: 1; min-width: 0; border: 0; outline: 0; color: #173c35; background: transparent; font: 650 25px/1.2 Georgia, serif; }.mc-editor-header > div { display: flex; gap: 10px; align-items: center; color: #7a8d87; font-size: 12px; }.mc-unsaved { color: #9a7125; }.mc-editor-tabs { padding: 0 26px; display: flex; align-items: center; gap: 4px; border-bottom: 1px solid #e7eeeb; }.mc-editor-tabs button { padding: 12px 14px 10px; border-bottom: 2px solid transparent; color: #72877f; background: transparent; }.mc-editor-tabs button.active { border-color: #257b60; color: #1f7057; }.mc-editor-tabs span { margin-left: auto; color: #93a29d; font: 12px monospace; }.mc-markdown-editor { flex: 1; min-height: 430px; resize: none; padding: 28px clamp(28px, 6vw, 80px); border: 0; outline: 0; color: #263f38; background: #fff; font: 15px/1.75 ui-monospace, SFMono-Regular, Menlo, monospace; }.mc-preview { flex: 1; overflow: auto; padding: 28px clamp(28px, 6vw, 80px); background: #fff; }.mc-preview pre { white-space: pre-wrap; word-break: break-word; color: #29453d; font: 15px/1.75 system-ui, sans-serif; }.mc-editor-footer { min-height: 34px; padding: 0 24px; display: flex; gap: 18px; align-items: center; border-top: 1px solid #e5eeea; color: #899a95; font-size: 11px; }.mc-editor-footer .error { margin-left: auto; color: #b23e3e; }
.mc-editor-shell { position: relative; }.mc-history-panel { position: absolute; z-index: 8; inset: 0 0 0 auto; width: min(410px, 70%); display: flex; flex-direction: column; border-left: 1px solid #d9e6e1; background: #f8fbf9; box-shadow: -14px 0 35px rgba(29,74,61,.12); }.mc-history-panel > header { height: 58px; padding: 0 18px; display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid #dce8e3; }.mc-history-panel > header button { border: 0; background: transparent; cursor: pointer; }.mc-history-list { max-height: 230px; overflow: auto; padding: 10px; border-bottom: 1px solid #dce8e3; }.mc-history-list button { width: 100%; padding: 10px 12px; display: flex; justify-content: space-between; border: 0; border-radius: 8px; color: #35574d; background: transparent; cursor: pointer; }.mc-history-list button.active { background: #dcefe7; }.mc-history-list small { color: #7b9089; }.mc-history-preview { flex: 1; overflow: auto; padding: 18px; }.mc-history-preview h3 { margin: 0 0 12px; }.mc-history-preview pre { max-height: 360px; overflow: auto; padding: 14px; white-space: pre-wrap; border-radius: 8px; background: #fff; font: 12px/1.6 monospace; }
@media (max-width: 760px) { .mc-notes { grid-template-columns: 120px 1fr; }.mc-notes-sidebar header { grid-template-columns: 30px 1fr; }.mc-notes-sidebar header div span, .mc-notes-sidebar h1 { display: none; }.mc-sidebar-actions { grid-column: 1 / -1; }.mc-note-list > button { padding-right: 10px; }.mc-note-list button > span, .mc-note-list small { display: none; }.mc-editor-header { padding: 0 14px; }.mc-editor-header input { font-size: 19px; }.mc-unsaved { display: none; } }
</style>
