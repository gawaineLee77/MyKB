<template>
  <main class="mc-create-page">
    <header class="mc-create-header">
      <button class="mc-back" type="button" @click="router.push('/platform/knowledge-bases')">
        <t-icon name="chevron-left" /> {{ text.back }}
      </button>
      <div>
        <p class="mc-eyebrow">MindCreek</p>
        <h1>{{ text.title }}</h1>
        <p>{{ text.subtitle }}</p>
      </div>
    </header>

    <ol class="mc-steps" aria-label="Creation progress">
      <li v-for="item in 3" :key="item" :class="{ active: step === item, done: step > item }">
        <span>{{ step > item ? '✓' : item }}</span>{{ text.steps[item - 1] }}
      </li>
    </ol>

    <section v-if="loading" class="mc-panel mc-loading">
      <t-loading size="small" /> {{ text.loading }}
    </section>

    <section v-else-if="loadError" class="mc-panel mc-error" role="alert">
      <t-icon name="error-circle" />
      <div><strong>{{ text.loadFailed }}</strong><p>{{ loadError }}</p></div>
      <t-button variant="outline" @click="load">{{ text.retry }}</t-button>
    </section>

    <template v-else-if="capabilities">
      <section v-if="step === 1" class="mc-panel">
        <div class="mc-section-heading">
          <div><h2>{{ text.chooseMode }}</h2><p>{{ text.chooseModeHint }}</p></div>
        </div>
        <div class="mc-mode-grid">
          <button
            type="button"
            class="mc-mode-card"
            :class="{ selected: draft.mode === 'personal_notes' }"
            :disabled="!notesEnabled"
            @click="draft.mode = 'personal_notes'"
          >
            <span class="mc-mode-icon notes"><t-icon name="edit-1" /></span>
            <span><strong>{{ text.notes }}</strong><small>{{ text.notesHint }}</small></span>
            <em>{{ notesEnabled ? text.available : text.coming }}</em>
          </button>
          <button
            type="button"
            class="mc-mode-card"
            :class="{ selected: draft.mode === 'rag' }"
            :disabled="!ragEnabled"
            @click="draft.mode = 'rag'"
          >
            <span class="mc-mode-icon rag"><t-icon name="file-search" /></span>
            <span><strong>{{ text.rag }}</strong><small>{{ text.ragHint }}</small></span>
            <em>{{ ragEnabled ? text.available : text.coming }}</em>
          </button>
        </div>
        <h3 class="mc-future-title">{{ text.futureProfiles }}</h3>
        <div class="mc-future-grid">
          <div v-for="future in futureModes" :key="future.name" class="mc-future-card">
            <t-icon :name="future.icon" /><span><strong>{{ future.name }}</strong><small>{{ future.detail }}</small></span>
            <t-icon name="lock-on" />
          </div>
        </div>
      </section>

      <section v-else-if="step === 2" class="mc-panel">
        <div class="mc-section-heading">
          <div><h2>{{ text.configure }}</h2><p>{{ selectedModeHint }}</p></div>
          <span class="mc-pill">{{ selectedModeName }}</span>
        </div>
        <div class="mc-form-grid">
          <label class="mc-field mc-field-wide">
            <span>{{ text.name }} *</span>
            <t-input v-model="draft.name" :maxlength="120" :placeholder="text.namePlaceholder" />
          </label>
          <label class="mc-field mc-field-wide">
            <span>{{ text.description }}</span>
            <t-textarea v-model="draft.description" :maxlength="1000" :autosize="{ minRows: 3, maxRows: 6 }" />
          </label>
        </div>
        <div v-if="models.ready" class="mc-managed-models">
          <t-icon name="check-circle" />
          <span><strong>{{ text.managedReady }}</strong><small>{{ text.managedHint }}</small></span>
        </div>
        <div v-else class="mc-inline-warning">
          <t-icon name="info-circle" /> {{ text.managedUnavailable }}
        </div>
        <div v-if="models.overridesEnabled" class="mc-advanced">
          <button type="button" @click="advancedOpen = !advancedOpen"><t-icon name="setting" /> {{ text.advanced }} <t-icon :name="advancedOpen ? 'chevron-up' : 'chevron-down'" /></button>
          <div v-if="advancedOpen" class="mc-advanced-fields">
            <label class="mc-field">
              <span>{{ text.embedding }}</span>
              <t-select v-model="draft.embeddingModelId">
                <t-option v-for="model in models.embedding" :key="model.id" :value="model.id" :label="model.display_name" />
              </t-select>
            </label>
            <label class="mc-field">
              <span>{{ text.summary }}</span>
              <t-select v-model="draft.summaryModelId">
                <t-option v-for="model in models.summary" :key="model.id" :value="model.id" :label="model.display_name" />
              </t-select>
            </label>
            <t-button variant="text" @click="router.push('/platform/mindcreek/settings/models')">{{ text.manageOverrides }}</t-button>
          </div>
        </div>
      </section>

      <section v-else class="mc-panel">
        <div class="mc-section-heading"><div><h2>{{ text.review }}</h2><p>{{ text.reviewHint }}</p></div></div>
        <dl class="mc-review">
          <div><dt>{{ text.mode }}</dt><dd>{{ selectedModeName }}</dd></div>
          <div><dt>{{ text.name }}</dt><dd>{{ draft.name.trim() }}</dd></div>
          <div><dt>{{ text.indexProfile }}</dt><dd><code>{{ draft.mode === 'personal_notes' ? 'notes_plain' : 'plain' }}</code></dd></div>
          <div><dt>{{ text.access }}</dt><dd>{{ draft.mode === 'personal_notes' ? text.ownerOnly : text.workspacePolicy }}</dd></div>
          <div><dt>{{ text.storage }}</dt><dd>{{ text.localStorage }}</dd></div>
          <div><dt>{{ text.models }}</dt><dd>{{ selectedModelSummary }}</dd></div>
        </dl>
        <p v-if="submitError" class="mc-submit-error" role="alert"><t-icon name="error-circle" /> {{ submitError }}</p>
      </section>

      <footer class="mc-actions">
        <t-button v-if="step > 1" variant="outline" size="large" @click="step--">{{ text.previous }}</t-button>
        <span />
        <t-button v-if="step < 3" theme="primary" size="large" :disabled="!canContinue" @click="step++">
          {{ text.continue }} <template #suffix><t-icon name="chevron-right" /></template>
        </t-button>
        <t-button v-else theme="primary" size="large" :loading="submitting" @click="submit">
          {{ text.create }}
        </t-button>
      </footer>
    </template>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import { createKnowledgeSpace, getCreationModels, getKnowledgeModeCapabilities, type ManagedModelDescriptor } from './api'
import { buildKnowledgeSpaceRequest, isSelectionEnabled, type CapabilityDocument } from './contracts'

const router = useRouter()
const { locale } = useI18n()
const step = ref(1)
const loading = ref(true)
const submitting = ref(false)
const loadError = ref('')
const submitError = ref('')
const advancedOpen = ref(false)
const capabilities = ref<CapabilityDocument | null>(null)
const models = reactive<{ ready: boolean; overridesEnabled: boolean; embedding: ManagedModelDescriptor[]; summary: ManagedModelDescriptor[] }>({
  ready: false, overridesEnabled: false, embedding: [], summary: [],
})
const draft = reactive({
  mode: 'personal_notes' as 'personal_notes' | 'rag',
  name: '',
  description: '',
  embeddingModelId: '',
  summaryModelId: '',
})
let requestFingerprint = ''
let idempotencyKey = ''

const copy = {
  en: {
    back: 'Knowledge bases', title: 'Create a knowledge space', subtitle: 'Choose a focused workspace now; advanced profiles remain safely gated.',
    steps: ['Choose', 'Configure', 'Review'], loading: 'Loading approved capabilities…', loadFailed: 'The creation service is unavailable.', retry: 'Retry',
    chooseMode: 'What do you want to build?', chooseModeHint: 'Each space has one purpose and a controlled indexing profile.',
    notes: 'Personal Notes', notesHint: 'Private Markdown and text notes for your daily work.', rag: 'Document RAG', ragHint: 'Multi-format documents with approved hybrid retrieval.',
    available: 'Available', coming: 'Coming later', futureProfiles: 'Future profiles', configure: 'Configure your space', name: 'Name', description: 'Description',
    namePlaceholder: 'For example: Research notes', embedding: 'Embedding model', summary: 'Chat and summary model',
    managedReady: 'Managed AI is ready', managedHint: 'MindCreek will use organization defaults. No API key or model setup is required.', managedUnavailable: 'Managed models are not ready. Contact your administrator.',
    advanced: 'Advanced model selection', manageOverrides: 'Manage workspace model overrides', review: 'Review and create', models: 'Models',
    reviewHint: 'MindCreek will apply the approved server-side profile.', mode: 'Mode', indexProfile: 'Index profile', access: 'Access', storage: 'Storage',
    ownerOnly: 'Owner only', workspacePolicy: 'Workspace policy', localStorage: 'Managed local storage', previous: 'Previous', continue: 'Continue', create: 'Create space',
  },
  zh: {
    back: '知识库', title: '创建知识空间', subtitle: '现在选择一个清晰用途；未来的高级能力仍由服务端安全管控。',
    steps: ['选择', '配置', '确认'], loading: '正在载入已批准能力…', loadFailed: '创建服务暂时不可用。', retry: '重试',
    chooseMode: '你想创建什么？', chooseModeHint: '每个空间只承担一种用途，并使用受控的索引配置。',
    notes: '个人笔记', notesHint: '记录日常工作的私人 Markdown 与文本笔记。', rag: '文档 RAG', ragHint: '使用已批准混合检索的多格式文档知识库。',
    available: '可使用', coming: '后续开放', futureProfiles: '未来能力', configure: '配置知识空间', name: '名称', description: '描述',
    namePlaceholder: '例如：研究笔记', embedding: '向量模型', summary: '对话与摘要模型',
    managedReady: '托管 AI 已就绪', managedHint: 'MindCreek 将自动使用组织默认模型，无需填写 API 密钥或配置模型。', managedUnavailable: '托管模型尚未就绪，请联系管理员。',
    advanced: '高级模型选择', manageOverrides: '管理工作空间模型覆盖', review: '确认并创建', reviewHint: 'MindCreek 将在服务端应用已批准的配置。', models: '模型',
    mode: '模式', indexProfile: '索引配置', access: '访问策略', storage: '存储', ownerOnly: '仅创建者', workspacePolicy: '工作空间策略',
    localStorage: '受管本地存储', previous: '上一步', continue: '继续', create: '创建空间',
  },
}
const text = computed(() => locale.value.startsWith('zh') ? copy.zh : copy.en)
const notesEnabled = computed(() => capabilities.value ? isSelectionEnabled(capabilities.value, 'personal_notes') : false)
const ragEnabled = computed(() => capabilities.value ? isSelectionEnabled(capabilities.value, 'rag') : false)
const canContinue = computed(() => step.value === 1
  ? (draft.mode === 'personal_notes' ? notesEnabled.value : ragEnabled.value)
  : draft.name.trim().length > 0 && draft.embeddingModelId.length > 0)
const selectedModeName = computed(() => draft.mode === 'personal_notes' ? text.value.notes : text.value.rag)
const selectedModeHint = computed(() => draft.mode === 'personal_notes' ? text.value.notesHint : text.value.ragHint)
const selectedModelSummary = computed(() => {
  const embedding = models.embedding.find(model => model.id === draft.embeddingModelId)?.display_name || '—'
  const chat = models.summary.find(model => model.id === draft.summaryModelId)?.display_name || '—'
  return `${embedding} · ${chat}`
})
const futureModes = computed(() => [
  { name: 'GraphRAG', detail: locale.value.startsWith('zh') ? '图谱增强检索' : 'Graph-enhanced retrieval', icon: 'relation' },
  { name: 'PixelRAG', detail: locale.value.startsWith('zh') ? '视觉页面检索' : 'Visual page retrieval', icon: 'image-search' },
  { name: locale.value.startsWith('zh') ? '本体知识图谱' : 'Ontology', detail: locale.value.startsWith('zh') ? '本体引导知识抽取' : 'Ontology-guided extraction', icon: 'tree-round-dot-vertical' },
])

function messageOf(error: unknown): string {
  if (error && typeof error === 'object' && 'message' in error) return String(error.message)
  return String(error || 'Unknown error')
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [document, availableModels] = await Promise.all([
      getKnowledgeModeCapabilities(),
      getCreationModels(),
    ])
    capabilities.value = document
    models.ready = availableModels.ready
    models.overridesEnabled = availableModels.overridesEnabled
    models.embedding = availableModels.embedding
    models.summary = availableModels.summary
    draft.embeddingModelId = availableModels.embedding.find(model => model.default)?.id || availableModels.embedding[0]?.id || ''
    draft.summaryModelId = availableModels.summary.find(model => model.default)?.id || availableModels.summary[0]?.id || ''
    if (!notesEnabled.value && ragEnabled.value) draft.mode = 'rag'
  } catch (error) {
    loadError.value = messageOf(error)
  } finally {
    loading.value = false
  }
}

async function submit() {
  if (!capabilities.value || submitting.value) return
  submitError.value = ''
  try {
    const request = buildKnowledgeSpaceRequest(capabilities.value, draft)
    const fingerprint = JSON.stringify(request)
    if (fingerprint !== requestFingerprint) {
      requestFingerprint = fingerprint
      idempotencyKey = crypto.randomUUID()
    }
    submitting.value = true
    const result = await createKnowledgeSpace(request, idempotencyKey)
    const target = result.product_mode === 'personal_notes'
      ? `/platform/mindcreek/notes/${result.knowledge_base_id}`
      : `/platform/mindcreek/rag/${result.knowledge_base_id}`
    await router.push(target)
  } catch (error) {
    submitError.value = messageOf(error)
  } finally {
    submitting.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.mc-create-page { min-height: 100%; overflow: auto; padding: 42px clamp(28px, 6vw, 88px) 64px; color: #173c35; background: radial-gradient(circle at 90% 0, #e0f6ed 0, transparent 31%), #f7fbf8; }
.mc-create-header { max-width: 980px; margin: 0 auto; display: grid; grid-template-columns: 150px 1fr 150px; align-items: start; text-align: center; }
.mc-create-header h1 { margin: 2px 0 8px; font: 650 clamp(28px, 4vw, 42px)/1.15 Georgia, serif; letter-spacing: -.6px; }
.mc-create-header p { margin: 0; color: #678078; }
.mc-eyebrow { color: #1d8a69 !important; font: 700 12px/1.3 sans-serif; letter-spacing: 2px; text-transform: uppercase; }
.mc-back { display: flex; align-items: center; gap: 4px; border: 0; background: transparent; color: #47675e; cursor: pointer; padding: 8px 0; }
.mc-steps { max-width: 660px; margin: 32px auto; padding: 0; display: flex; list-style: none; color: #82958f; }
.mc-steps li { flex: 1; display: flex; align-items: center; gap: 9px; font-size: 13px; }
.mc-steps li:not(:last-child)::after { content: ''; flex: 1; height: 1px; background: #cadbd5; margin: 0 12px; }
.mc-steps span { width: 28px; height: 28px; display: grid; place-items: center; border: 1px solid #b9cec7; border-radius: 50%; background: #fff; }
.mc-steps .active { color: #176f57; font-weight: 650; }.mc-steps .active span, .mc-steps .done span { color: #fff; background: #207d61; border-color: #207d61; }
.mc-panel { max-width: 900px; margin: 0 auto; padding: 32px; border: 1px solid #dbe8e2; border-radius: 18px; background: rgba(255,255,255,.94); box-shadow: 0 18px 50px rgba(40,84,71,.08); }
.mc-loading, .mc-error { display: flex; align-items: center; justify-content: center; gap: 12px; min-height: 180px; }.mc-error div { flex: 1; }.mc-error p { margin: 3px 0 0; color: #8b5353; }
.mc-section-heading { display: flex; justify-content: space-between; gap: 24px; align-items: start; margin-bottom: 24px; }.mc-section-heading h2 { margin: 0 0 6px; font-size: 22px; }.mc-section-heading p { margin: 0; color: #70847e; }
.mc-mode-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 16px; }
.mc-mode-card { position: relative; min-height: 144px; padding: 22px; display: grid; grid-template-columns: 46px 1fr; gap: 15px; text-align: left; border: 1px solid #d6e4df; border-radius: 14px; background: #fff; cursor: pointer; transition: .16s ease; }
.mc-mode-card:hover:not(:disabled) { transform: translateY(-2px); border-color: #6db59f; box-shadow: 0 10px 25px rgba(37,112,88,.1); }.mc-mode-card.selected { border: 2px solid #258063; background: #f2fbf7; padding: 21px; }.mc-mode-card:disabled { cursor: not-allowed; opacity: .55; }
.mc-mode-card strong, .mc-mode-card small { display: block; }.mc-mode-card strong { margin: 2px 0 8px; font-size: 17px; }.mc-mode-card small { color: #6d827b; line-height: 1.45; }.mc-mode-card em { position: absolute; right: 16px; bottom: 13px; color: #23775f; font-size: 11px; font-style: normal; text-transform: uppercase; letter-spacing: .7px; }
.mc-mode-icon { width: 44px; height: 44px; display: grid; place-items: center; border-radius: 12px; font-size: 22px; }.mc-mode-icon.notes { color: #7a6434; background: #fff1c9; }.mc-mode-icon.rag { color: #226e5b; background: #d9f2e9; }
.mc-future-title { margin: 28px 0 12px; color: #6e837c; font-size: 12px; letter-spacing: .8px; text-transform: uppercase; }.mc-future-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; }.mc-future-card { padding: 13px; display: grid; grid-template-columns: 24px 1fr 18px; gap: 8px; align-items: center; border-radius: 10px; background: #f5f8f7; color: #789088; }.mc-future-card strong, .mc-future-card small { display: block; }.mc-future-card small { margin-top: 2px; font-size: 11px; }
.mc-pill { padding: 6px 12px; border-radius: 99px; color: #176b54; background: #def3ea; font-size: 12px; white-space: nowrap; }.mc-form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }.mc-field { display: flex; flex-direction: column; gap: 8px; color: #436159; font-size: 13px; font-weight: 600; }.mc-field-wide { grid-column: 1 / -1; }.mc-inline-warning { margin-top: 20px; padding: 12px 14px; display: flex; gap: 8px; align-items: center; border-radius: 10px; background: #fff7e1; color: #7e6522; }
.mc-managed-models { margin-top: 20px; padding: 14px; display: flex; gap: 10px; align-items: center; border-radius: 11px; color: #176b54; background: #edf9f4; }.mc-managed-models strong, .mc-managed-models small { display: block; }.mc-managed-models small { margin-top: 3px; color: #678078; }.mc-advanced { margin-top: 14px; border-top: 1px solid #e3ece8; padding-top: 12px; }.mc-advanced > button { display: flex; align-items: center; gap: 7px; padding: 5px 0; color: #58736b; border: 0; background: transparent; cursor: pointer; }.mc-advanced-fields { margin-top: 12px; padding: 16px; display: grid; grid-template-columns: 1fr 1fr; gap: 14px; border-radius: 10px; background: #f5f8f7; }.mc-advanced-fields .t-button { grid-column: 1 / -1; justify-self: start; }
.mc-review { margin: 0; border: 1px solid #e0e9e5; border-radius: 12px; overflow: hidden; }.mc-review div { padding: 14px 18px; display: grid; grid-template-columns: 160px 1fr; border-bottom: 1px solid #e8efec; }.mc-review div:last-child { border: 0; }.mc-review dt { color: #71857e; }.mc-review dd { margin: 0; font-weight: 600; }.mc-review code { color: #1e755c; }.mc-submit-error { display: flex; gap: 7px; color: #b23b3b; }
.mc-actions { max-width: 900px; margin: 18px auto 0; display: grid; grid-template-columns: 150px 1fr 180px; }
@media (max-width: 760px) { .mc-create-page { padding: 24px 16px 48px; }.mc-create-header { grid-template-columns: 1fr; text-align: left; }.mc-create-header .mc-back { margin-bottom: 16px; }.mc-mode-grid, .mc-future-grid, .mc-form-grid { grid-template-columns: 1fr; }.mc-field-wide { grid-column: auto; }.mc-panel { padding: 22px; }.mc-review div { grid-template-columns: 1fr; gap: 4px; }.mc-actions { grid-template-columns: auto 1fr auto; }.mc-steps li { font-size: 0; } }
</style>
