<template>
  <main class="model-page">
    <header>
      <button type="button" class="back" @click="router.back()"><t-icon name="chevron-left" /> {{ text.back }}</button>
      <p class="eyebrow">MindCreek · {{ text.advanced }}</p>
      <h1>{{ text.title }}</h1>
      <p>{{ text.subtitle }}</p>
    </header>

    <section v-if="loading" class="panel state"><t-loading size="small" /> {{ text.loading }}</section>
    <section v-else-if="error" class="panel state error" role="alert">
      <span>{{ error }}</span><button type="button" @click="load">{{ text.retry }}</button>
    </section>
    <section v-else-if="!snapshot?.overrides_enabled" class="panel state">
      <h2>{{ text.disabled }}</h2><p>{{ text.disabledHint }}</p>
    </section>

    <template v-else>
      <section class="panel">
        <div class="heading"><div><h2>{{ text.managed }}</h2><p>{{ text.managedHint }}</p></div><span class="ready">{{ snapshot.ready ? text.ready : text.unavailable }}</span></div>
        <div class="cards">
          <article v-for="model in snapshot.defaults" :key="model.id">
            <small>{{ model.type }}</small><strong>{{ model.display_name }}</strong><span>{{ model.available ? text.available : text.unavailable }}</span>
          </article>
        </div>
      </section>

      <section class="panel">
        <div class="heading">
          <div><h2>{{ text.overrides }}</h2><p>{{ text.overrideHint }}</p></div>
          <button type="button" @click="resetForm">{{ text.add }}</button>
        </div>
        <p v-if="!snapshot.overrides.length" class="empty">{{ text.empty }}</p>
        <ul v-else class="override-list">
          <li v-for="model in snapshot.overrides" :key="model.id">
            <span><strong>{{ model.display_name }}</strong><small>{{ model.type }} · {{ model.scope }}</small></span>
            <span class="row-actions">
              <button type="button" @click="edit(model)">{{ text.replace }}</button>
              <button type="button" class="danger" @click="remove(model)">{{ text.delete }}</button>
            </span>
          </li>
        </ul>
      </section>

      <section class="panel">
        <div class="heading"><div><h2>{{ editingId ? text.replaceTitle : text.addTitle }}</h2><p>{{ editingId ? text.replaceHint : text.addHint }}</p></div></div>
        <form class="form" @submit.prevent="save">
          <label><span>{{ text.displayName }}</span><input v-model.trim="form.display_name" maxlength="128" required /></label>
          <label><span>{{ text.modelName }}</span><input v-model.trim="form.name" maxlength="128" required autocomplete="off" /></label>
          <label><span>{{ text.type }}</span><select v-model="form.type" :disabled="Boolean(editingId)"><option value="KnowledgeQA">KnowledgeQA</option><option value="Embedding">Embedding</option><option value="Rerank">Rerank</option></select></label>
          <label><span>{{ text.provider }}</span><select v-model="form.provider"><option value="generic">generic</option><option value="openai">openai</option></select></label>
          <label class="wide"><span>{{ text.endpoint }}</span><input v-model.trim="form.base_url" type="url" placeholder="https://provider.example/v1" required autocomplete="off" /></label>
          <label class="wide"><span>{{ editingId ? text.newKey : text.key }}</span><input v-model="form.api_key" type="password" :required="!editingId" autocomplete="new-password" /></label>
          <label v-if="form.type === 'Embedding'"><span>{{ text.dimension }}</span><input v-model.number="form.dimension" type="number" min="1" max="65536" required /></label>
          <label class="disclosure wide"><input v-model="acknowledged" type="checkbox" /><span>{{ text.disclosure }}</span></label>
          <p v-if="actionMessage" class="action-message" :class="{ error: actionFailed }" role="status">{{ actionMessage }}</p>
          <footer>
            <button type="button" :disabled="busy || !acknowledged" @click="testConnection">{{ text.test }}</button>
            <button type="submit" class="primary" :disabled="busy || !acknowledged">{{ busy ? text.working : text.save }}</button>
          </footer>
        </form>
      </section>
    </template>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import {
  createModelOverride,
  deleteModelOverride,
  getManagedModels,
  testModelOverride,
  updateModelOverride,
  type ManagedModelDescriptor,
  type ManagedModelSnapshot,
  type ModelOverrideInput,
} from './api'

const router = useRouter()
const { locale } = useI18n()
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const actionMessage = ref('')
const actionFailed = ref(false)
const acknowledged = ref(false)
const editingId = ref('')
const snapshot = ref<ManagedModelSnapshot | null>(null)
const form = reactive<ModelOverrideInput>({ name: '', display_name: '', type: 'KnowledgeQA', provider: 'generic', base_url: '', api_key: '', dimension: 1024 })

const copy = {
  en: {
    back: 'Back', advanced: 'Advanced settings', title: 'Workspace model overrides', subtitle: 'Optional provider connections for workspace administrators. Managed defaults remain available to everyone.',
    loading: 'Loading redacted model status…', retry: 'Retry', disabled: 'Advanced model overrides are disabled', disabledHint: 'Your administrator has kept this optional capability closed.',
    managed: 'Managed defaults', managedHint: 'Connection details and credentials are controlled by the server and are never shown here.', ready: 'Ready', unavailable: 'Unavailable', available: 'Available',
    overrides: 'Workspace overrides', overrideHint: 'Overrides belong to the active workspace and do not replace organization defaults.', add: 'Add override', empty: 'No workspace overrides are configured.', replace: 'Replace', delete: 'Delete',
    addTitle: 'Add a provider', addHint: 'Only allow-listed providers and HTTPS hosts are accepted.', replaceTitle: 'Replace an override', replaceHint: 'For security, the saved endpoint and credential cannot be retrieved. Re-enter the complete configuration; leave the key blank only to retain it.',
    displayName: 'Display name', modelName: 'Provider model name', type: 'Model type', provider: 'Provider protocol', endpoint: 'API base URL', key: 'API key', newKey: 'Replacement API key (optional)', dimension: 'Embedding dimension',
    disclosure: 'I understand that prompts, document excerpts, and test data may be sent to this external provider under my organization’s policy.', test: 'Test connection', save: 'Save override', working: 'Working…', tested: 'Connection test passed.', testFailed: 'Connection test failed.', saved: 'Override saved.', deleted: 'Override deleted.', confirmDelete: 'Delete this workspace model override?',
  },
  zh: {
    back: '返回', advanced: '高级设置', title: '工作空间模型覆盖', subtitle: '供工作空间管理员使用的可选模型连接；所有用户仍可直接使用组织托管默认模型。',
    loading: '正在载入脱敏模型状态…', retry: '重试', disabled: '高级模型覆盖未启用', disabledHint: '管理员尚未开放此可选能力。',
    managed: '托管默认模型', managedHint: '连接地址与凭据由服务端管理，此处永不显示。', ready: '就绪', unavailable: '不可用', available: '可用',
    overrides: '工作空间覆盖', overrideHint: '覆盖模型仅属于当前工作空间，不会替换组织默认模型。', add: '新增覆盖', empty: '当前没有工作空间覆盖模型。', replace: '替换', delete: '删除',
    addTitle: '添加模型服务', addHint: '仅接受允许清单内的供应商和 HTTPS 主机。', replaceTitle: '替换覆盖模型', replaceHint: '出于安全原因，已保存的地址和凭据无法读取。请重新填写完整配置；仅在保留原密钥时留空密钥字段。',
    displayName: '显示名称', modelName: '供应商模型名称', type: '模型类型', provider: '供应商协议', endpoint: 'API 基础地址', key: 'API 密钥', newKey: '替换 API 密钥（可选）', dimension: '向量维度',
    disclosure: '我了解提示词、文档片段和测试数据可能依据组织策略发送给此外部供应商。', test: '测试连接', save: '保存覆盖', working: '处理中…', tested: '连接测试通过。', testFailed: '连接测试失败。', saved: '覆盖模型已保存。', deleted: '覆盖模型已删除。', confirmDelete: '确认删除此工作空间模型覆盖？',
  },
}
const text = computed(() => locale.value.startsWith('zh') ? copy.zh : copy.en)

function messageOf(value: unknown): string {
  return value && typeof value === 'object' && 'message' in value ? String(value.message) : String(value || 'Unknown error')
}

function clearSecret() { form.api_key = '' }

function resetForm() {
  editingId.value = ''
  Object.assign(form, { name: '', display_name: '', type: 'KnowledgeQA', provider: 'generic', base_url: '', api_key: '', dimension: 1024 })
  acknowledged.value = false
  actionMessage.value = ''
}

function edit(model: ManagedModelDescriptor) {
  editingId.value = model.id
  Object.assign(form, { name: '', display_name: model.display_name, type: model.type, provider: 'generic', base_url: '', api_key: '', dimension: 1024 })
  acknowledged.value = false
  actionMessage.value = ''
  window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' })
}

async function load() {
  loading.value = true
  error.value = ''
  try { snapshot.value = await getManagedModels() } catch (value) { error.value = messageOf(value) } finally { loading.value = false }
}

async function testConnection() {
  if (busy.value || !acknowledged.value) return
  busy.value = true
  actionMessage.value = ''
  try {
    const result = await testModelOverride({ ...form }, editingId.value)
    actionFailed.value = !result.available
    actionMessage.value = result.available ? text.value.tested : text.value.testFailed
  } catch (value) {
    actionFailed.value = true
    actionMessage.value = messageOf(value)
  } finally { clearSecret(); busy.value = false }
}

async function save() {
  if (busy.value || !acknowledged.value) return
  busy.value = true
  actionMessage.value = ''
  try {
    if (editingId.value) await updateModelOverride(editingId.value, { ...form })
    else await createModelOverride({ ...form })
    actionFailed.value = false
    actionMessage.value = text.value.saved
    clearSecret()
    await load()
    resetForm()
  } catch (value) {
    actionFailed.value = true
    actionMessage.value = messageOf(value)
    clearSecret()
  } finally { busy.value = false }
}

async function remove(model: ManagedModelDescriptor) {
  if (!window.confirm(text.value.confirmDelete)) return
  try { await deleteModelOverride(model.id); actionMessage.value = text.value.deleted; await load() } catch (value) { error.value = messageOf(value) }
}

onMounted(load)
</script>

<style scoped>
.model-page { min-height: 100%; overflow: auto; padding: 40px clamp(20px, 6vw, 84px) 64px; color: #173c35; background: #f5faf7; }
header, .panel { max-width: 920px; margin: 0 auto; } header { margin-bottom: 24px; } h1 { margin: 4px 0 8px; font: 650 36px/1.15 Georgia, serif; } header p { color: #6b817a; }
.back, button { border: 1px solid #bad1c8; border-radius: 8px; padding: 8px 12px; color: #1d6d57; background: #fff; cursor: pointer; }.back { border: 0; padding-left: 0; background: transparent; }.eyebrow { margin-top: 20px; color: #228067 !important; font-weight: 700; letter-spacing: 1px; text-transform: uppercase; }
.panel { margin-bottom: 18px; padding: 26px; border: 1px solid #d9e7e1; border-radius: 16px; background: #fff; box-shadow: 0 12px 34px rgba(39,88,72,.06); }.state { min-height: 120px; display: flex; gap: 12px; align-items: center; justify-content: center; }.error { color: #a33; }
.heading { display: flex; align-items: start; justify-content: space-between; gap: 20px; }.heading h2 { margin: 0 0 5px; }.heading p { margin: 0 0 20px; color: #6d817a; }.ready { padding: 5px 10px; border-radius: 99px; background: #e1f4eb; color: #23745d; }
.cards { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; }.cards article { padding: 16px; border-radius: 11px; background: #f2f8f5; }.cards small, .cards strong, .cards span { display: block; }.cards small { color: #788c85; }.cards strong { margin: 6px 0; }.cards span { color: #2a7a62; font-size: 12px; }
.override-list { padding: 0; list-style: none; }.override-list li { padding: 12px 0; display: flex; justify-content: space-between; border-top: 1px solid #e6eeeb; }.override-list small { display: block; margin-top: 4px; color: #7b8e87; }.row-actions { display: flex; gap: 8px; }.danger { color: #a33; border-color: #e5c1c1; }.empty { color: #7b8e87; }
.form { display: grid; grid-template-columns: 1fr 1fr; gap: 18px; }.form label { display: flex; flex-direction: column; gap: 7px; color: #46645b; font-size: 13px; font-weight: 650; }.form input:not([type=checkbox]), .form select { min-height: 38px; padding: 8px 10px; border: 1px solid #cbdcd6; border-radius: 8px; background: #fff; }.wide { grid-column: 1 / -1; }.disclosure { flex-direction: row !important; align-items: start; padding: 12px; border-radius: 9px; background: #fff8e7; font-weight: 500 !important; line-height: 1.45; }.action-message { grid-column: 1 / -1; margin: 0; color: #23745d; }.action-message.error { color: #a33; }.form footer { grid-column: 1 / -1; display: flex; justify-content: flex-end; gap: 10px; }.primary { color: #fff; background: #237b60; border-color: #237b60; }button:disabled { opacity: .5; cursor: not-allowed; }
@media (max-width: 720px) { .cards, .form { grid-template-columns: 1fr; }.wide { grid-column: auto; }.heading, .override-list li { flex-direction: column; } }
</style>
