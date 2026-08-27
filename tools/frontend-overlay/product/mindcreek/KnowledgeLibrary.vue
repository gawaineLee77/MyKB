<template>
  <main class="mc-library">
    <header class="mc-library-header">
      <div><span>MindCreek · {{ text.eyebrow }}</span><h1>{{ text.title }}</h1><p>{{ text.subtitle }}</p></div>
      <button class="primary" type="button" @click="router.push('/platform/mindcreek/create')"><t-icon name="add" /> {{ text.create }}</button>
    </header>

    <nav class="mc-tabs" :aria-label="text.views">
      <button type="button" :class="{ active: view === 'owned' }" data-testid="owned-tab" @click="selectView('owned')"><t-icon name="folder" /> {{ text.owned }} <span>{{ view === 'owned' ? page.total : '' }}</span></button>
      <button type="button" :class="{ active: view === 'shared' }" data-testid="shared-tab" @click="selectView('shared')"><t-icon name="usergroup" /> {{ text.shared }} <span>{{ view === 'shared' ? page.total : '' }}</span></button>
    </nav>

    <section class="mc-content" aria-live="polite">
      <div v-if="loading && !page.items.length" class="mc-state"><t-loading /><strong>{{ text.loading }}</strong></div>
      <div v-else-if="error" class="mc-state error"><t-icon name="error-circle" /><strong>{{ text.loadFailed }}</strong><span>{{ error }}</span><button type="button" @click="load">{{ text.retry }}</button></div>
      <div v-else-if="!page.items.length" class="mc-state"><span class="empty-icon"><t-icon :name="view === 'owned' ? 'folder-open' : 'usergroup'" /></span><strong>{{ view === 'owned' ? text.emptyOwned : text.emptyShared }}</strong><p>{{ view === 'owned' ? text.emptyOwnedHint : text.emptySharedHint }}</p><button v-if="view === 'owned'" class="primary" type="button" @click="router.push('/platform/mindcreek/create')">{{ text.createFirst }}</button></div>
      <div v-else class="mc-grid">
        <article v-for="item in page.items" :key="item.id" class="mc-card" @click="open(item)">
          <header>
            <span class="mode-icon" :class="item.product_mode"><t-icon :name="item.product_mode === 'personal_notes' ? 'edit-1' : 'layers'" /></span>
            <span class="role" :class="item.role">{{ roleLabel(item.role) }}</span>
          </header>
          <div><small>{{ modeLabel(item.product_mode) }}</small><h2>{{ item.name }}</h2><p>{{ item.description || text.noDescription }}</p></div>
          <footer>
            <span><t-icon :name="item.role === 'owner' ? 'user' : 'secured'" /> {{ item.role === 'owner' ? text.privateDefault : text.explicitAccess }}</span>
            <button v-if="item.role === 'owner' && item.product_mode !== 'personal_notes'" type="button" @click.stop="share(item)"><t-icon name="share" /> {{ text.share }}</button>
            <span v-if="item.product_mode === 'personal_notes'" class="not-shareable"><t-icon name="lock-on" /> {{ text.ownerOnly }}</span>
          </footer>
        </article>
      </div>

      <footer v-if="page.total > page.page_size" class="mc-pagination">
        <button type="button" :disabled="page.page <= 1" @click="changePage(-1)"><t-icon name="chevron-left" /></button>
        <span>{{ page.page }} / {{ Math.ceil(page.total / page.page_size) }}</span>
        <button type="button" :disabled="page.page * page.page_size >= page.total" @click="changePage(1)"><t-icon name="chevron-right" /></button>
      </footer>
    </section>

    <aside class="mc-privacy"><t-icon name="shield" /><div><strong>{{ text.privacy }}</strong><p>{{ text.privacyHint }}</p></div></aside>
    <SharingDialog v-model:visible="sharingVisible" :knowledge-base-id="sharingItem?.id || ''" :knowledge-base-name="sharingItem?.name || ''" @changed="load" />
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import SharingDialog from './SharingDialog.vue'
import { listKnowledgeLibrary, type KnowledgeLibraryItem, type KnowledgeLibraryPage } from './api'
import type { KnowledgeRole } from './contracts'

const router = useRouter(), { locale } = useI18n()
const view = ref<'owned' | 'shared'>('owned'), loading = ref(false), error = ref('')
const sharingVisible = ref(false), sharingItem = ref<KnowledgeLibraryItem | null>(null)
const page = reactive<KnowledgeLibraryPage>({ items: [], total: 0, page: 1, page_size: 24 })
const words = {
  en: { eyebrow: 'Knowledge library', title: 'Your knowledge, clearly governed', subtitle: 'Work privately by default. Share a RAG knowledge base only with the people who need it.', create: 'Create knowledge base', views: 'Knowledge base views', owned: 'My KBs', shared: 'Shared with me', loading: 'Loading your authorized knowledge bases…', loadFailed: 'Could not load the library', retry: 'Try again', emptyOwned: 'Your library is ready', emptyOwnedHint: 'Create private notes or a document knowledge base to begin.', emptyShared: 'Nothing has been shared with you', emptySharedHint: 'Explicit Viewer and Editor grants will appear here.', createFirst: 'Create your first KB', noDescription: 'No description provided.', privateDefault: 'Private by default', explicitAccess: 'Explicit access', share: 'Share', ownerOnly: 'Owner only', privacy: 'No workspace-wide fallback', privacyHint: 'Every card in these views comes from MindCreek authorization decisions. Personal Notes are never shareable.' },
  zh: { eyebrow: '知识库目录', title: '清晰管理你的知识', subtitle: '默认私有；仅将 RAG 知识库明确共享给真正需要的人。', create: '创建知识库', views: '知识库视图', owned: '我的知识库', shared: '共享给我的', loading: '正在加载已授权的知识库…', loadFailed: '无法加载知识库目录', retry: '重试', emptyOwned: '你的知识库目录已就绪', emptyOwnedHint: '创建私人笔记或文档知识库以开始使用。', emptyShared: '暂时没有共享给你的知识库', emptySharedHint: '明确授予的查看者或编辑者权限会显示在这里。', createFirst: '创建第一个知识库', noDescription: '暂无描述。', privateDefault: '默认私有', explicitAccess: '明确授权', share: '共享', ownerOnly: '仅所有者', privacy: '不回退到工作区全量列表', privacyHint: '这里的每张卡片均来自 MindCreek 授权决策；私人笔记永远不可共享。' },
}
const text = computed(() => locale.value.startsWith('zh') ? words.zh : words.en)
function messageOf(value: unknown) { const response = value && typeof value === 'object' && 'response' in value ? (value as any).response?.data : undefined; return response?.error?.message || (value && typeof value === 'object' && 'message' in value ? String((value as any).message) : String(value || 'Unknown error')) }
function roleLabel(role: KnowledgeRole) { const labels = { owner: ["Owner", "所有者"], editor: ["Editor", "编辑者"], viewer: ["Viewer", "查看者"] }; return labels[role][locale.value.startsWith('zh') ? 1 : 0] }
function modeLabel(mode?: string) { if (mode === 'personal_notes') return locale.value.startsWith('zh') ? '私人笔记' : 'Personal Notes'; return locale.value.startsWith('zh') ? '文档 RAG' : 'Document RAG' }
async function load() { loading.value = true; error.value = ''; try { Object.assign(page, await listKnowledgeLibrary(view.value, page.page)) } catch (value) { error.value = messageOf(value); page.items = [] } finally { loading.value = false } }
async function selectView(next: 'owned' | 'shared') { if (view.value === next) return; view.value = next; page.page = 1; page.items = []; await load() }
async function changePage(delta: number) { page.page += delta; await load() }
function share(item: KnowledgeLibraryItem) { sharingItem.value = item; sharingVisible.value = true }
function open(item: KnowledgeLibraryItem) { if (item.product_mode === 'personal_notes') void router.push(`/platform/mindcreek/notes/${item.id}`); else void router.push(`/platform/mindcreek/rag/${item.id}`) }
onMounted(load)
</script>

<style scoped>
.mc-library { min-height: 100%; overflow: auto; padding: 42px clamp(24px, 5vw, 76px) 60px; color: #173c35; background: radial-gradient(circle at 90% 0, #dff3eb 0, transparent 31%), #f7faf8; }.mc-library-header { max-width: 1180px; margin: 0 auto 28px; display: flex; justify-content: space-between; gap: 24px; align-items: end; }.mc-library-header span { color: #1c8063; font-size: 11px; font-weight: 750; letter-spacing: 1.7px; text-transform: uppercase; }.mc-library-header h1 { margin: 5px 0 7px; font: 650 34px/1.15 Georgia, serif; }.mc-library-header p { margin: 0; color: #6c827a; }.mc-library button { padding: 9px 13px; border: 1px solid #ccddd6; border-radius: 9px; color: #41665b; background: #fff; cursor: pointer; }.primary { color: #fff !important; border-color: #207d61 !important; background: #207d61 !important; }.mc-tabs { max-width: 1180px; margin: 0 auto 18px; display: flex; gap: 6px; border-bottom: 1px solid #dce8e3; }.mc-tabs button { padding: 11px 15px; border: 0; border-bottom: 2px solid transparent; border-radius: 9px 9px 0 0; background: transparent; }.mc-tabs button.active { color: #17684f; border-bottom-color: #207d61; background: #eaf5f0; }.mc-tabs span { min-width: 10px; display: inline-block; }.mc-content { max-width: 1180px; margin: 0 auto; }.mc-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(310px, 1fr)); gap: 15px; }.mc-card { min-height: 238px; padding: 20px; display: flex; flex-direction: column; justify-content: space-between; border: 1px solid #dce8e3; border-radius: 16px; background: #fff; box-shadow: 0 11px 32px rgba(43,84,72,.055); cursor: pointer; transition: .15s; }.mc-card:hover { border-color: #9fc8ba; transform: translateY(-2px); box-shadow: 0 16px 38px rgba(43,84,72,.1); }.mc-card > header, .mc-card > footer { display: flex; justify-content: space-between; gap: 10px; align-items: center; }.mode-icon { width: 43px; height: 43px; display: grid; place-items: center; border-radius: 12px; color: #1d7458; background: #dff2ea; font-size: 21px; }.mode-icon.personal_notes { color: #7c6120; background: #f5edd7; }.role { padding: 5px 9px; border-radius: 99px; color: #4d6d63; background: #edf3f0; font-size: 10px; font-weight: 700; text-transform: uppercase; }.role.editor { color: #805f16; background: #fff0c9; }.role.viewer { color: #43628a; background: #e9f1fb; }.mc-card small { color: #1d795d; font-size: 10px; font-weight: 750; letter-spacing: 1.2px; text-transform: uppercase; }.mc-card h2 { margin: 7px 0; font-size: 20px; }.mc-card p { min-height: 40px; margin: 0; color: #758980; line-height: 1.5; }.mc-card > footer { padding-top: 15px; border-top: 1px solid #eaf0ed; color: #768a82; font-size: 11px; }.mc-card footer button { padding: 7px 9px; }.not-shareable { color: #806b35; }.mc-state { min-height: 360px; display: flex; flex-direction: column; gap: 10px; align-items: center; justify-content: center; text-align: center; color: #7b8f87; }.mc-state strong { color: #31564b; font-size: 19px; }.mc-state p { margin: 0 0 5px; }.empty-icon { width: 58px; height: 58px; display: grid; place-items: center; border-radius: 17px; color: #287b61; background: #dff2ea; font-size: 28px; }.mc-state.error { color: #993d3d; }.mc-pagination { padding: 22px; display: flex; justify-content: center; gap: 11px; align-items: center; }.mc-privacy { max-width: 1140px; margin: 22px auto 0; padding: 14px 18px; display: grid; grid-template-columns: 28px 1fr; gap: 10px; align-items: center; border-radius: 12px; color: #49665e; background: #e7f3ee; }.mc-privacy p { margin: 3px 0 0; font-size: 12px; }button:disabled { opacity: .55; cursor: not-allowed; }@media (max-width: 700px) { .mc-library { padding: 26px 14px 44px; }.mc-library-header { align-items: start; flex-direction: column; }.mc-library-header h1 { font-size: 29px; }.mc-tabs { overflow-x: auto; }.mc-grid { grid-template-columns: 1fr; } }
</style>
