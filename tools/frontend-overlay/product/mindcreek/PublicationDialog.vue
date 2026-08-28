<template>
  <div v-if="visible" class="mc-modal" role="dialog" aria-modal="true" :aria-label="text.title">
    <button class="mc-backdrop" type="button" :aria-label="text.close" @click="close" />
    <section class="mc-dialog">
      <header><div><span>MindCreek · {{ text.catalog }}</span><h2>{{ text.title }}</h2><p>{{ knowledgeBaseName }}</p></div><button type="button" class="icon" @click="close">×</button></header>
      <div v-if="loading" class="state">{{ text.loading }}</div>
      <form v-else @submit.prevent="save">
        <label>{{ text.publicTitle }}<input v-model.trim="draft.title" maxlength="160" required /></label>
        <label>{{ text.description }}<textarea v-model.trim="draft.description" maxlength="2000" rows="3" /></label>
        <label>{{ text.tags }}<input v-model="tagsText" :placeholder="text.tagsHint" /></label>
        <label>{{ text.guidance }}<textarea v-model.trim="draft.usage_guidance" maxlength="2000" rows="2" /></label>
        <div class="two">
          <label>{{ text.access }}<select v-model="draft.access_mode"><option value="subscriber">{{ text.subscriber }}</option><option value="organization_public">{{ text.organizationPublic }}</option></select></label>
          <label>{{ text.audience }}<select v-model="audienceType"><option value="organization">{{ text.organization }}</option><option value="workspace_set">{{ text.workspaces }}</option></select></label>
        </div>
        <label v-if="audienceType === 'workspace_set'">{{ text.workspaceIds }}<input v-model="workspaceIDs" inputmode="numeric" :placeholder="text.workspaceHint" /></label>
        <aside><t-icon name="info-circle" /><span>{{ draft.access_mode === 'organization_public' ? text.publicHint : text.subscriberHint }}</span></aside>
        <p v-if="error" class="error">{{ error }} <button v-if="conflict" type="button" @click="load">{{ text.reload }}</button></p>
        <footer><button v-if="current?.status === 'published'" class="danger" type="button" :disabled="saving" @click="unpublish">{{ text.unpublish }}</button><span /><button type="button" @click="close">{{ text.cancel }}</button><button class="primary" type="submit" :disabled="saving">{{ saving ? text.saving : current?.status === 'published' ? text.update : text.publish }}</button></footer>
      </form>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getKnowledgePublication, publishKnowledgeBase, unpublishKnowledgeBase, updateKnowledgePublication, type Publication, type PublicationAccessMode } from './api'

const props = defineProps<{ visible: boolean; knowledgeBaseId: string; knowledgeBaseName: string }>()
const emit = defineEmits<{ 'update:visible': [value: boolean]; changed: [] }>()
const { locale } = useI18n(), loading = ref(false), saving = ref(false), error = ref(''), conflict = ref(false)
const current = ref<Publication | null>(null), tagsText = ref(''), audienceType = ref<'organization' | 'workspace_set'>('organization'), workspaceIDs = ref('')
const draft = reactive({ title: '', description: '', usage_guidance: '', access_mode: 'subscriber' as PublicationAccessMode })
const words = {
  en: { title: 'Publish knowledge base', catalog: 'Internal catalog', close: 'Close', loading: 'Loading publication…', publicTitle: 'Catalog title', description: 'Description', tags: 'Tags', tagsHint: 'policy, onboarding, engineering', guidance: 'Usage guidance', access: 'Access mode', subscriber: 'Subscribers only', organizationPublic: 'Organization public', audience: 'Audience', organization: 'All authenticated internal users', workspaces: 'Selected workspaces', workspaceIds: 'Workspace IDs', workspaceHint: '42, 77', publicHint: 'Eligible authenticated users can read without subscribing. Subscription follows updates.', subscriberHint: 'Eligible users receive read access only after subscribing.', reload: 'Reload', unpublish: 'Unpublish', cancel: 'Cancel', saving: 'Saving…', update: 'Update publication', publish: 'Publish' },
  zh: { title: '发布知识库', catalog: '内部目录', close: '关闭', loading: '正在加载发布信息…', publicTitle: '目录标题', description: '描述', tags: '标签', tagsHint: '制度, 入职, 工程', guidance: '使用说明', access: '访问模式', subscriber: '仅订阅者', organizationPublic: '组织内公开', audience: '受众', organization: '所有已认证内部用户', workspaces: '指定工作区', workspaceIds: '工作区 ID', workspaceHint: '42, 77', publicHint: '符合条件的已认证用户无需订阅即可读取；订阅用于跟踪更新。', subscriberHint: '符合条件的用户仅在订阅后获得读取权限。', reload: '重新加载', unpublish: '取消发布', cancel: '取消', saving: '保存中…', update: '更新发布', publish: '发布' },
}
const text = computed(() => locale.value.startsWith('zh') ? words.zh : words.en)
function close() { emit('update:visible', false) }
function failure(value: unknown) { const body = value && typeof value === 'object' && 'response' in value ? (value as any).response?.data : undefined; conflict.value = body?.error?.code === 'publication.revision_conflict'; return { code: body?.error?.code, message: body?.error?.message || String((value as any)?.message || value) } }
function reset() { current.value = null; draft.title = props.knowledgeBaseName; draft.description = ''; draft.usage_guidance = ''; draft.access_mode = 'subscriber'; tagsText.value = ''; audienceType.value = 'organization'; workspaceIDs.value = '' }
async function load() { loading.value = true; error.value = ''; conflict.value = false; reset(); try { const value = await getKnowledgePublication(props.knowledgeBaseId); current.value = value; draft.title = value.title; draft.description = value.description; draft.usage_guidance = value.usage_guidance; draft.access_mode = value.access_mode; tagsText.value = value.tags.join(', '); audienceType.value = value.audience.type; workspaceIDs.value = value.audience.type === 'workspace_set' ? value.audience.workspace_ids.join(', ') : '' } catch (value) { const problem = failure(value); if (problem.code !== 'resource.not_found') error.value = problem.message } finally { loading.value = false } }
function input() { const tags = tagsText.value.split(',').map(value => value.trim()).filter(Boolean); const ids = workspaceIDs.value.split(',').map(value => Number(value.trim())).filter(value => Number.isInteger(value) && value > 0); if (audienceType.value === 'workspace_set' && !ids.length) throw new Error(locale.value.startsWith('zh') ? '至少需要一个工作区 ID。' : 'At least one workspace ID is required.'); return { title: draft.title, description: draft.description, tags, usage_guidance: draft.usage_guidance, access_mode: draft.access_mode, audience: audienceType.value === 'organization' ? { type: 'organization' as const } : { type: 'workspace_set' as const, workspace_ids: [...new Set(ids)] } } }
async function save() { saving.value = true; error.value = ''; try { const value = input(); current.value = current.value?.status === 'published' ? await updateKnowledgePublication(props.knowledgeBaseId, current.value, value) : await publishKnowledgeBase(props.knowledgeBaseId, { ...value, expected_row_version: current.value?.row_version }); emit('changed') } catch (value) { error.value = failure(value).message } finally { saving.value = false } }
async function unpublish() { if (!current.value) return; saving.value = true; error.value = ''; try { current.value = await unpublishKnowledgeBase(props.knowledgeBaseId, current.value); emit('changed') } catch (value) { error.value = failure(value).message } finally { saving.value = false } }
watch(() => props.visible, value => { if (value) void load() })
</script>

<style scoped>
.mc-modal{position:fixed;inset:0;z-index:3100;display:grid;place-items:center;padding:20px}.mc-backdrop{position:absolute;inset:0;border:0;background:rgba(14,40,33,.48)}.mc-dialog{position:relative;width:min(620px,100%);max-height:90vh;overflow:auto;padding:25px;border-radius:17px;background:#fff;color:#24483e;box-shadow:0 28px 90px rgba(10,40,31,.3)}header{display:flex;justify-content:space-between;gap:20px;margin-bottom:20px}header span{color:#258064;font-size:10px;font-weight:750;letter-spacing:1.4px;text-transform:uppercase}h2{margin:5px 0}header p{margin:0;color:#71877f}.icon{border:0;background:none;font-size:25px}form,label{display:grid;gap:6px}form{gap:14px}label{font-size:12px;font-weight:650}input,textarea,select{width:100%;box-sizing:border-box;padding:10px;border:1px solid #cfddd7;border-radius:8px;background:#fff;color:#294b42}.two{display:grid;grid-template-columns:1fr 1fr;gap:12px}aside{display:flex;gap:8px;padding:11px;border-radius:9px;background:#eaf5f0;color:#4d6c62;font-size:12px}.error{color:#a13f3f}.error button{border:0;color:#1d765a;background:none;text-decoration:underline}footer{display:flex;gap:9px;align-items:center;border-top:1px solid #e6eeea;padding-top:15px}footer span{flex:1}button{padding:9px 12px;border:1px solid #cbdad4;border-radius:8px;background:#fff;cursor:pointer}.primary{color:#fff;border-color:#207d61;background:#207d61}.danger{color:#a33d3d;border-color:#e8caca}.state{padding:50px;text-align:center;color:#71877f}@media(max-width:600px){.two{grid-template-columns:1fr}.mc-dialog{padding:18px}}
</style>
