<template>
  <section class="managed-models" aria-labelledby="mindcreek-managed-models-title">
    <header>
      <div>
        <p class="eyebrow">MindCreek</p>
        <h3 id="mindcreek-managed-models-title">{{ text.title }}</h3>
        <p>{{ text.hint }}</p>
      </div>
      <span class="overall" :class="snapshot?.ready ? 'ready' : 'unavailable'">
        {{ snapshot?.ready ? text.ready : text.unavailable }}
      </span>
    </header>

    <div v-if="loading" class="state"><t-loading size="small" /> {{ text.loading }}</div>
    <div v-else-if="error" class="state error">
      <span>{{ error }}</span><t-button variant="text" @click="load">{{ text.retry }}</t-button>
    </div>
    <div v-else class="model-grid">
      <article v-for="model in snapshot?.defaults || []" :key="model.id" class="model-card">
        <span class="type">{{ typeName(model.type) }}</span>
        <strong>{{ model.display_name }}</strong>
        <small>{{ text.organizationDefault }}</small>
        <footer>
          <span :class="model.available ? 'available' : 'unavailable'">
            <t-icon :name="model.available ? 'check-circle' : 'error-circle'" />
            {{ model.available ? text.available : text.unavailable }}
          </span>
          <t-button
            v-if="canTest"
            size="small"
            variant="outline"
            :loading="testing === model.id"
            :disabled="!model.available || Boolean(testing)"
            @click="test(model)"
          >
            {{ text.test }}
          </t-button>
        </footer>
        <p v-if="results[model.id]" class="result" :class="results[model.id].available ? 'success' : 'failure'">
          {{ resultText(results[model.id]) }}
        </p>
      </article>
    </div>
    <p v-if="!canTest && !loading" class="admin-hint">{{ text.adminHint }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'

import {
  getManagedModels,
  testManagedModel,
  type ManagedModelDescriptor,
  type ManagedModelSnapshot,
  type ManagedModelType,
  type ModelTestResult,
} from './api'

const { locale } = useI18n()
const authStore = useAuthStore()
const loading = ref(true)
const error = ref('')
const testing = ref('')
const snapshot = ref<ManagedModelSnapshot | null>(null)
const results = reactive<Record<string, ModelTestResult>>({})
const canTest = computed(() => authStore.hasRole('admin'))

const copy = {
  en: {
    title: 'Organization-managed defaults', hint: 'Available to every user. Provider endpoints and credentials remain on the server.',
    loading: 'Loading managed models…', retry: 'Retry', ready: 'Ready', unavailable: 'Unavailable', available: 'Available',
    organizationDefault: 'Organization default · read only', test: 'Test connection', adminHint: 'Workspace administrators can run connection tests.',
    success: 'Connection successful', failure: 'Connection failed', dimension: 'dimension', latency: 'ms',
    types: { KnowledgeQA: 'Chat / LLM', Embedding: 'Embedding', Rerank: 'Rerank' },
  },
  zh: {
    title: '组织托管默认模型', hint: '所有用户均可使用；服务地址和凭据仅保存在服务端。',
    loading: '正在载入托管模型…', retry: '重试', ready: '已就绪', unavailable: '不可用', available: '可用',
    organizationDefault: '组织默认 · 只读', test: '测试连接', adminHint: '工作空间管理员可以执行连接测试。',
    success: '连接成功', failure: '连接失败', dimension: '维度', latency: '毫秒',
    types: { KnowledgeQA: '对话 / LLM', Embedding: '向量模型', Rerank: '重排模型' },
  },
}
const text = computed(() => locale.value.startsWith('zh') ? copy.zh : copy.en)

function messageOf(value: unknown): string {
  const response = value && typeof value === 'object' && 'response' in value ? (value as any).response?.data : undefined
  return response?.error?.message || (value && typeof value === 'object' && 'message' in value ? String((value as any).message) : String(value || 'Unknown error'))
}

function typeName(type: ManagedModelType): string { return text.value.types[type] }

function resultText(result: ModelTestResult): string {
  const details: string[] = []
  if (result.elapsed_ms !== undefined) details.push(`${result.elapsed_ms} ${text.value.latency}`)
  if (result.dimension) details.push(`${text.value.dimension} ${result.dimension}`)
  return `${result.available ? text.value.success : text.value.failure}${details.length ? ` · ${details.join(' · ')}` : ''}`
}

async function load() {
  loading.value = true
  error.value = ''
  try { snapshot.value = await getManagedModels() } catch (value) { error.value = messageOf(value) } finally { loading.value = false }
}

async function test(model: ManagedModelDescriptor) {
  testing.value = model.id
  delete results[model.id]
  try { results[model.id] = await testManagedModel(model.id) } catch (value) {
    results[model.id] = { available: false, message: messageOf(value) }
  } finally { testing.value = '' }
}

onMounted(load)
</script>

<style scoped>
.managed-models { margin: 18px 0 24px; padding: 20px; border: 1px solid var(--td-component-stroke); border-radius: 12px; background: var(--td-bg-color-container); }
.managed-models > header { display: flex; justify-content: space-between; gap: 24px; align-items: flex-start; }
.managed-models h3 { margin: 2px 0 5px; font-size: 17px; }
.managed-models header p:not(.eyebrow) { margin: 0; color: var(--td-text-color-secondary); }
.eyebrow { margin: 0; color: #1d8063; font-size: 11px; font-weight: 700; letter-spacing: 1.4px; text-transform: uppercase; }
.overall, .model-card footer > span { display: inline-flex; align-items: center; gap: 5px; white-space: nowrap; }
.overall { padding: 5px 10px; border-radius: 99px; font-size: 12px; }
.ready, .available, .success { color: #177458; }.overall.ready { background: #e5f6ef; }
.unavailable, .failure, .error { color: var(--td-error-color); }.overall.unavailable { background: var(--td-error-color-1); }
.model-grid { margin-top: 18px; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.model-card { padding: 15px; border: 1px solid var(--td-component-stroke); border-radius: 10px; background: var(--td-bg-color-secondarycontainer); }
.model-card .type, .model-card strong, .model-card small { display: block; }
.model-card .type { color: #23775e; font-size: 11px; font-weight: 700; text-transform: uppercase; }
.model-card strong { margin-top: 7px; }.model-card small { margin-top: 4px; color: var(--td-text-color-placeholder); }
.model-card footer { margin-top: 14px; display: flex; justify-content: space-between; gap: 8px; align-items: center; font-size: 12px; }
.result { margin: 10px 0 0; font-size: 12px; }.state { min-height: 100px; display: flex; gap: 8px; align-items: center; justify-content: center; }
.admin-hint { margin: 14px 0 0; color: var(--td-text-color-placeholder); font-size: 12px; }
@media (max-width: 850px) { .model-grid { grid-template-columns: 1fr; } }
</style>
