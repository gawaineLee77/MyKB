<template>
  <div class="agent-icon-editor" data-testid="agent-icon-editor">
    <div class="agent-icon-preview" aria-live="polite">
      <span>{{ name?.trim().slice(0, 1) || '?' }}</span>
      <AgentIconPicture v-if="modelValue" :avatar="modelValue" :agent-id="modelValue === uploaded ? undefined : agentId" :name="name" />
    </div>
    <div class="agent-icon-controls">
      <label class="upload-button" :class="{ busy }">{{ busy ? '图片上传中…' : modelValue ? '替换图片' : '上传图片' }}
        <input type="file" accept="image/png,image/jpeg,image/webp" :disabled="busy" aria-label="上传智能体图标" @change="selectFile" />
      </label>
      <button v-if="modelValue" type="button" :disabled="busy" @click="remove">移除图片</button>
      <p>支持 PNG、JPEG、WebP，最大 2 MB。图片随智能体保存生效。</p>
      <p v-if="error" role="alert" class="error">{{ error }}</p>
    </div>
  </div>
</template>
<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { post } from '@/utils/request'
import { useAuthStore } from '@/stores/auth'
import AgentIconPicture from './AgentIconPicture.vue'
import { normalizeUIImage } from './image-upload'
import { agentIconID } from './agent-icon-policy'
const props = defineProps<{ modelValue?: string; agentId?: string; name?: string }>()
const emit = defineEmits<{ 'update:modelValue': [string]; busy: [boolean] }>()
const busy = ref(false), error = ref(''), uploaded = ref('')
const auth = useAuthStore()
let sequence = 0
watch(() => [props.agentId, auth.user?.id, auth.effectiveTenantId], () => { sequence++; busy.value = false; uploaded.value = ''; error.value = ''; emit('busy', false) }, { flush: 'sync' })
onBeforeUnmount(() => { sequence++; emit('busy', false) })
function remove() { error.value = ''; uploaded.value = ''; emit('update:modelValue', '') }
async function selectFile(event: Event) {
  const input = event.target as HTMLInputElement, file = input.files?.[0]
  input.value = ''
  if (!file || busy.value) return
  const current = ++sequence
  busy.value = true; emit('busy', true); error.value = ''
  try {
    const image = await normalizeUIImage(file)
    if (current !== sequence) return
    const result = await post<any>('/api/v1/mindcreek/agent-icons', image, { headers: { 'Content-Type': 'image/png' } })
    if (!result.success || !agentIconID(result.data?.avatar)) throw new Error('图片上传未完成，请重试。')
    if (current !== sequence) return
    uploaded.value = result.data.avatar
    emit('update:modelValue', result.data.avatar)
  } catch (e: any) {
    if (current !== sequence) return
    error.value = e?.code === 'agent_icon.quota_exceeded' || e?.error?.code === 'agent_icon.quota_exceeded'
      ? '已达到图片存储限额，请联系安装管理员。'
      : e instanceof Error ? e.message : '图片上传失败，请检查图片格式或稍后重试。'
  } finally { if (current === sequence) { busy.value = false; emit('busy', false) } }
}
</script>
<style scoped>
.agent-icon-editor { display: flex; align-items: center; gap: 16px; }
.agent-icon-preview { position: relative; flex: none; width: 64px; height: 64px; overflow: hidden; border-radius: 12px; background: #e8f4ef; display: grid; place-items: center; color: #227455; font-size: 24px; }
.agent-icon-controls { min-width: 0; }
.upload-button,button { display: inline-flex; border: 1px solid var(--td-component-border); padding: 7px 12px; border-radius: 6px; background: var(--td-bg-color-container); color: var(--td-text-color-primary); cursor: pointer; font: inherit; }
.upload-button { position: relative; overflow: hidden; margin-right: 10px; }
.upload-button:focus-within { outline: 2px solid var(--td-brand-color); outline-offset: 2px; }
.upload-button input { position: absolute; inset: 0; opacity: 0; width: 100%; cursor: pointer; }
.busy,button:disabled { opacity: .6; cursor: wait; }
p { margin: 7px 0 0; color: var(--td-text-color-secondary); font-size: 12px; }
.error { color: var(--td-error-color); }
</style>
