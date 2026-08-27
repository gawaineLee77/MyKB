<template>
  <div v-if="visible" class="mc-modal" role="dialog" aria-modal="true" :aria-label="text.title">
    <button class="mc-backdrop" type="button" :aria-label="text.close" @click="close" />
    <section class="mc-dialog">
      <header>
        <div><span>MindCreek · {{ text.controlled }}</span><h2>{{ text.title }}</h2><p>{{ knowledgeBaseName }}</p></div>
        <button class="icon-button" type="button" :aria-label="text.close" @click="close">×</button>
      </header>

      <div class="mc-invite">
        <label>{{ text.findUser }}</label>
        <div class="mc-search"><input v-model.trim="query" :placeholder="text.searchHint" @keyup.enter="search" /><button type="button" :disabled="searching" @click="search">{{ text.search }}</button></div>
        <div v-if="members.length" class="mc-member-results">
          <button v-for="member in members" :key="member.user_id" type="button" :class="{ selected: selectedUserId === member.user_id }" @click="selectedUserId = member.user_id">
            <span>{{ initials(member.username || member.email) }}</span><div><strong>{{ member.username || member.email }}</strong><small>{{ member.email }}</small></div>
          </button>
        </div>
        <div class="mc-grant-form">
          <select v-model="permission" :aria-label="text.permission"><option value="viewer">{{ text.viewer }}</option><option value="editor">{{ text.editor }}</option></select>
          <input v-model="expiresLocal" type="datetime-local" :aria-label="text.expiry" />
          <button class="primary" type="button" :disabled="saving || !selectedUserId" @click="createGrant">{{ saving ? text.saving : text.share }}</button>
        </div>
        <small class="hint">{{ text.expiryHint }}</small>
      </div>

      <div class="mc-current">
        <div class="section-title"><h3>{{ text.current }}</h3><button type="button" :disabled="loading" @click="load">↻ {{ text.refresh }}</button></div>
        <div v-if="loading" class="mc-state">{{ text.loading }}</div>
        <div v-else-if="!grants.length" class="mc-state">{{ text.empty }}</div>
        <article v-for="item in grants" :key="item.id">
          <span class="avatar">{{ initials(memberLabel(item.subject_id)) }}</span>
          <div class="identity"><strong>{{ memberLabel(item.subject_id) }}</strong><small>{{ expiryLabel(item.expires_at) }}</small></div>
          <select :value="item.permission" :aria-label="text.permission" @change="changePermission(item, $event)"><option value="viewer">{{ text.viewer }}</option><option value="editor">{{ text.editor }}</option></select>
          <button class="danger" type="button" :disabled="saving" @click="revoke(item)">{{ text.revoke }}</button>
        </article>
      </div>

      <p v-if="error" class="mc-error">{{ error }} <button v-if="conflict" type="button" @click="load">{{ text.reload }}</button></p>
      <footer><span>{{ text.ownerOnly }}</span><button type="button" @click="close">{{ text.done }}</button></footer>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { createKnowledgeGrant, listKnowledgeGrants, revokeKnowledgeGrant, searchTenantUsers, updateKnowledgeGrant, type KnowledgeGrant, type TenantMember } from './api'

const props = defineProps<{ visible: boolean; knowledgeBaseId: string; knowledgeBaseName: string }>()
const emit = defineEmits<{ 'update:visible': [value: boolean]; changed: [] }>()
const { locale } = useI18n()
const loading = ref(false), searching = ref(false), saving = ref(false), conflict = ref(false)
const query = ref(''), selectedUserId = ref(''), permission = ref<'viewer' | 'editor'>('viewer'), expiresLocal = ref(''), error = ref('')
const grants = ref<KnowledgeGrant[]>([]), members = ref<TenantMember[]>([])
const words = {
  en: { title: 'Share knowledge base', controlled: 'Controlled access', close: 'Close', findUser: 'Add an internal user', searchHint: 'Name or email', search: 'Search', permission: 'Permission', viewer: 'Viewer', editor: 'Editor', expiry: 'Expiry', expiryHint: 'Expiry is optional and uses your local time.', share: 'Share', saving: 'Saving…', current: 'People with access', refresh: 'Refresh', loading: 'Loading access…', empty: 'Only you can access this knowledge base.', revoke: 'Revoke', reload: 'Reload current access', ownerOnly: 'Only the owner can manage access.', done: 'Done', never: 'No expiry', expires: 'Expires' },
  zh: { title: '共享知识库', controlled: '受控访问', close: '关闭', findUser: '添加内部用户', searchHint: '姓名或邮箱', search: '搜索', permission: '权限', viewer: '查看者', editor: '编辑者', expiry: '到期时间', expiryHint: '到期时间可选，并按本地时间填写。', share: '共享', saving: '保存中…', current: '已有访问权限', refresh: '刷新', loading: '正在加载权限…', empty: '目前仅你可以访问此知识库。', revoke: '撤销', reload: '重新加载当前权限', ownerOnly: '只有所有者可以管理访问权限。', done: '完成', never: '永久有效', expires: '到期' },
}
const text = computed(() => locale.value.startsWith('zh') ? words.zh : words.en)

function close() { emit('update:visible', false) }
function initials(value: string) { return value.trim().slice(0, 2).toUpperCase() || 'U' }
function memberLabel(id: string) { const member = members.value.find(item => item.user_id === id); return member?.username || member?.email || `${id.slice(0, 8)}…` }
function expiryLabel(value?: string) { return value ? `${text.value.expires} ${new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))}` : text.value.never }
function messageOf(value: unknown) {
  const response = value && typeof value === 'object' && 'response' in value ? (value as any).response?.data : undefined
  const code = response?.error?.code
  conflict.value = code === 'grant.revision_conflict'
  return response?.error?.message || (value && typeof value === 'object' && 'message' in value ? String((value as any).message) : String(value || 'Unknown error'))
}
async function load() { loading.value = true; error.value = ''; try { const [items, directory] = await Promise.all([listKnowledgeGrants(props.knowledgeBaseId), searchTenantUsers(query.value)]); grants.value = items; members.value = directory.items.filter(item => item.status === 'active') } catch (value) { error.value = messageOf(value) } finally { loading.value = false } }
async function search() { searching.value = true; error.value = ''; try { const page = await searchTenantUsers(query.value); members.value = page.items.filter(item => item.status === 'active') } catch (value) { error.value = messageOf(value) } finally { searching.value = false } }
function expirationISO() { return expiresLocal.value ? new Date(expiresLocal.value).toISOString() : undefined }
async function createGrant() { saving.value = true; error.value = ''; try { await createKnowledgeGrant(props.knowledgeBaseId, selectedUserId.value, permission.value, expirationISO()); selectedUserId.value = ''; expiresLocal.value = ''; emit('changed'); await load() } catch (value) { error.value = messageOf(value) } finally { saving.value = false } }
async function changePermission(item: KnowledgeGrant, event: Event) { saving.value = true; error.value = ''; try { await updateKnowledgeGrant(props.knowledgeBaseId, item, (event.target as HTMLSelectElement).value as 'viewer' | 'editor', item.expires_at); emit('changed'); await load() } catch (value) { error.value = messageOf(value) } finally { saving.value = false } }
async function revoke(item: KnowledgeGrant) { saving.value = true; error.value = ''; try { await revokeKnowledgeGrant(props.knowledgeBaseId, item); emit('changed'); await load() } catch (value) { error.value = messageOf(value) } finally { saving.value = false } }
watch(() => props.visible, value => { if (value) void load() })
</script>

<style scoped>
.mc-modal { position: fixed; inset: 0; z-index: 3000; display: grid; place-items: center; padding: 20px; }.mc-backdrop { position: absolute; inset: 0; width: 100%; border: 0; background: rgba(12,35,29,.45); }.mc-dialog { position: relative; width: min(720px, 100%); max-height: min(820px, 92vh); overflow: auto; border-radius: 18px; background: #fff; box-shadow: 0 28px 80px rgba(13,45,36,.25); color: #173c35; }.mc-dialog > header { padding: 24px 26px 18px; display: flex; justify-content: space-between; border-bottom: 1px solid #e5eeea; }.mc-dialog h2 { margin: 4px 0; font-size: 24px; }.mc-dialog header p { margin: 0; color: #758b83; }.mc-dialog header span { color: #1c8063; font-size: 11px; font-weight: 700; letter-spacing: 1.4px; text-transform: uppercase; }.icon-button { border: 0; background: transparent; font-size: 26px; cursor: pointer; }.mc-invite, .mc-current { padding: 20px 26px; }.mc-invite { border-bottom: 1px solid #e5eeea; }.mc-invite label { display: block; margin-bottom: 8px; font-weight: 650; }.mc-search, .mc-grant-form { display: flex; gap: 8px; }.mc-search input { flex: 1; }.mc-dialog input, .mc-dialog select, .mc-dialog button { padding: 9px 11px; border: 1px solid #ccddd6; border-radius: 8px; background: #fff; }.mc-dialog button { cursor: pointer; }.mc-member-results { margin: 8px 0; display: grid; grid-template-columns: 1fr 1fr; gap: 6px; max-height: 130px; overflow: auto; }.mc-member-results button { display: flex; gap: 9px; text-align: left; }.mc-member-results button.selected { border-color: #207d61; background: #eaf6f1; }.mc-member-results button > span, .avatar { width: 32px; height: 32px; display: grid; flex: 0 0 32px; place-items: center; border-radius: 50%; color: #176047; background: #dff1ea; font-size: 11px; font-weight: 700; }.mc-member-results strong, .mc-member-results small { display: block; max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.mc-member-results small, .hint, .identity small { color: #7d918a; }.mc-grant-form { margin-top: 10px; }.mc-grant-form input { flex: 1; }.primary { color: #fff; border-color: #207d61 !important; background: #207d61 !important; }.section-title { display: flex; justify-content: space-between; align-items: center; }.section-title h3 { margin: 0; }.mc-current article { padding: 13px 0; display: grid; grid-template-columns: 36px 1fr 105px auto; gap: 10px; align-items: center; border-bottom: 1px solid #edf3f0; }.identity strong, .identity small { display: block; }.danger { color: #a13b3b; }.mc-state { padding: 28px; text-align: center; color: #7d918a; }.mc-error { margin: 0 26px 16px; padding: 10px 12px; border-radius: 8px; color: #913838; background: #fcecec; }.mc-error button { margin-left: 8px; }.mc-dialog > footer { padding: 15px 26px; display: flex; justify-content: space-between; align-items: center; color: #73877f; background: #f7faf8; }.mc-dialog > footer button { color: #fff; border-color: #207d61; background: #207d61; }button:disabled { opacity: .55; cursor: not-allowed; }@media (max-width: 600px) { .mc-member-results { grid-template-columns: 1fr; }.mc-grant-form { flex-direction: column; }.mc-current article { grid-template-columns: 36px 1fr; }.mc-current article select, .mc-current article .danger { grid-column: 2; } }
</style>
