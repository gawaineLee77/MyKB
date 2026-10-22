<template>
  <section class="kb-access" data-testid="kb-access-panel">
    <header><div><h3>访问与协作</h3><p>仅当前空间有效成员可访问。可见即允许阅读解析内容和用于问答；写入、原文件下载和分块导出分别授权。</p></div><t-button variant="outline" :loading="busy" @click="load">重新载入</t-button></header>
    <t-alert v-if="error" theme="error" :message="error" />
    <template v-if="policy">
      <p class="fixed-access">创建者：{{ memberName(policy.creator_user_id || '') }}{{ policy.creator_revoked ? '（资格已撤销）' : '' }}。本空间 Owner/Admin 始终拥有全部权限；协作者不能修改知识库名称、配置、删除知识库或转授权。</p>
      <t-popconfirm v-if="policy.creator_revoked && isAdmin" content="恢复创建者的私有可见及管理资格？服务器将重新检查其当前成员角色。" @confirm="restoreCreator"><t-button variant="outline" :disabled="busy">恢复创建者资格</t-button></t-popconfirm>
      <label class="range-label">可见范围
        <select :value="range" :disabled="busy || conflict" @change="changeRange(($event.target as HTMLSelectElement).value)">
          <option value="private">仅创建者和空间管理者</option><option value="selected">指定成员</option><option value="workspace">空间内公开</option>
        </select>
      </label>
      <t-alert v-if="range === 'workspace'" theme="info" message="当前及以后加入的空间成员均可阅读和问答。移除某人的可见勾选只移除单独授权，不会排除空间公开带来的阅读权限。" />
      <p v-else-if="range === 'private'">保存后仅创建者及 Owner/Admin 可见，原有成员单独授权将被移除。</p>
      <template v-if="range !== 'private'">
        <label class="member-search">添加成员<input v-model="query" placeholder="搜索当前空间成员" @input="searchMembers" /></label>
        <div class="member-results"><button v-for="member in candidates" :key="member.user_id" type="button" :disabled="busy || conflict" @click="addMember(member)">{{ memberName(member.user_id) }} · {{ roleLabel(member.role) }} ＋</button><span v-if="!candidates.length">没有可添加的匹配成员</span></div>
        <div v-for="member in policy.members" :key="member.user_id" class="member-card">
          <div class="member-title"><strong>{{ memberName(member.user_id) }}</strong><span>{{ roleLabel(memberRole(member.user_id)) }}</span><button type="button" :disabled="busy || conflict" @click="removeMember(member.user_id)">移除单独授权</button></div>
          <label><input type="checkbox" :checked="member.visible" :disabled="busy || conflict" @change="setVisible(member, ($event.target as HTMLInputElement).checked)" />可见并用于问答</label>
          <div v-if="memberRole(member.user_id) === 'contributor'" class="operation-grid">
            <label v-for="operation in KB_OPERATIONS" :key="operation.value" :title="operation.help"><input type="checkbox" :checked="member.operations.includes(operation.value)" :disabled="busy || conflict" @change="setOperation(member, operation.value, ($event.target as HTMLInputElement).checked)" /><span>{{ operation.label }}<small>{{ operation.help }}</small></span></label>
          </div>
          <p v-else-if="memberRole(member.user_id)">访客只能获得可见权限；停用或不在当前成员名单中的用户不能获得操作权限。</p>
          <p v-else>此成员的角色尚未加载，已有操作保持不变。可搜索此用户 ID 以查看角色；服务端会在保存时重新核验其身份和权限。</p>
        </div>
        <p v-if="!policy.members.length">尚未选择成员。{{ range === 'workspace' ? '空间公开只授予阅读和问答。' : '只有创建者及空间管理者可见。' }}</p>
      </template>
      <p class="operation-note">重新解析需同时拥有“修改文档、修改分块、删除分块”。权限覆盖本库现有及未来内容；撤销不能收回已经展示或下载的内容。</p>
      <div class="panel-actions"><span>权限版本 {{ policy.revision }}</span><t-button :loading="busy" :disabled="conflict" @click="save">保存访问权限</t-button></div>
      <details @toggle="loadEvents"><summary>授权变更记录</summary><p v-if="eventsError">{{ eventsError }}</p><ul><li v-for="(event, i) in events" :key="i">{{ event.created_at }} · {{ event.actor_user_id }} · {{ event.action }} · v{{ event.revision }} · {{ event.outcome }}</li></ul><p v-if="!events.length">暂无变更记录</p></details>
    </template>
  </section>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { get, post, put } from '@/utils/request'
import { announceKBPermissions, kbPermissionEndpoint } from './kb-permissions'
import { KB_OPERATIONS, kbPermissionProblem, kbPolicyInput, type KBPolicy, type KBMember, type KBDirectoryMember } from './kb-permissions-policy'
const props = defineProps<{ kbId: string }>()
const auth = useAuthStore(), policy = ref<KBPolicy | null>(null), directory = ref<KBDirectoryMember[]>([]), query = ref(''), error = ref(''), eventsError = ref(''), busy = ref(false), conflict = ref(false), range = ref('private'), events = ref<any[]>([])
const isAdmin = computed(() => ['admin', 'owner'].includes(auth.currentTenantRole || ''))
const endpoint = () => kbPermissionEndpoint(props.kbId)
const context = () => `${auth.effectiveTenantId}:${auth.currentTenantRole}:${props.kbId}`
let generation = 0, searchGeneration = 0
const names = ref<Record<string, KBDirectoryMember>>({})
const memberName = (id: string) => names.value[id]?.display_name || names.value[id]?.username || id || '待管理者确认'
const memberRole = (id: string) => names.value[id]?.role || ''
const roleLabel = (role: string) => ({ owner: 'Owner', admin: 'Admin', contributor: '编辑', viewer: '访客' }[role] || '角色待核验')
const candidates = computed(() => directory.value.filter(m => !['owner', 'admin'].includes(m.role) && m.user_id !== policy.value?.creator_user_id && !policy.value?.members.some(p => p.user_id === m.user_id)))
async function searchMembers() {
  const request = ++searchGeneration, current = context()
  try { const result: any = await get(endpoint() + '/members', { params: { q: query.value } }); if (request !== searchGeneration || current !== context()) return; directory.value = result.data || []; for (const m of directory.value) names.value[m.user_id] = m } catch (err) { if (request === searchGeneration) error.value = kbPermissionProblem(err) }
}
async function load() {
  const request = ++generation; busy.value = true; error.value = ''; policy.value = null; conflict.value = false
  try { const result: any = await get(endpoint()); if (request !== generation) return; if (!result.success || !result.data) throw new Error('无权管理此知识库。'); policy.value = result.data; range.value = policy.value!.visibility === 'workspace' ? 'workspace' : policy.value!.members.length ? 'selected' : 'private'; await searchMembers() } catch (err) { if (request === generation) error.value = kbPermissionProblem(err) } finally { if (request === generation) busy.value = false }
}
function changeRange(value: string) { range.value = value; if (policy.value) policy.value.visibility = value === 'workspace' ? 'workspace' : 'restricted' }
function addMember(member: KBDirectoryMember) { policy.value?.members.push({ user_id: member.user_id, visible: true, operations: [] }) }
function removeMember(id: string) { if (policy.value) policy.value.members = policy.value.members.filter(m => m.user_id !== id) }
function setVisible(member: KBMember, visible: boolean) { member.visible = visible; if (!visible) member.operations = [] }
function setOperation(member: KBMember, operation: string, enabled: boolean) { member.operations = member.operations.filter(op => op !== operation); if (enabled) { member.visible = true; member.operations.push(operation) } }
async function save() {
  if (!policy.value || busy.value || conflict.value) return
  const current = context(), snapshot = kbPolicyInput(policy.value, range.value, memberRole)
  busy.value = true; error.value = ''
  try { const result: any = await put(endpoint(), snapshot); if (current !== context()) return; if (!result.success) throw new Error(result.message || '保存失败。'); announceKBPermissions(); await load() } catch (err: any) { if (current === context()) { error.value = kbPermissionProblem(err); conflict.value = err?.status === 409; if ([403, 404].includes(err?.status)) { policy.value = null; announceKBPermissions() } } } finally { if (current === context()) busy.value = false }
}
async function restoreCreator() { if (!policy.value || busy.value) return; busy.value = true; try { await post(endpoint() + '/restore-creator', { revision: policy.value.revision }); announceKBPermissions(); await load() } catch (err) { error.value = kbPermissionProblem(err) } finally { busy.value = false } }
async function loadEvents(event: Event) { if (!(event.target as HTMLDetailsElement).open) return; eventsError.value = ''; try { const result: any = await get(endpoint() + '/events'); events.value = result.data || [] } catch (err) { eventsError.value = kbPermissionProblem(err) } }
watch(context, () => { names.value = {}; directory.value = []; events.value = []; void load() }, { immediate: true })
</script>
<style scoped>
.kb-access{padding:20px;max-width:1040px;color:var(--td-text-color-primary)}header,.member-title,.panel-actions{display:flex;align-items:center;justify-content:space-between;gap:16px}h3{margin:0 0 8px}p{line-height:1.7;color:var(--td-text-color-secondary)}header{align-items:flex-start}.range-label,.member-search{display:flex;gap:12px;align-items:center;margin:20px 0}.range-label select,.member-search input{padding:9px 12px;border:1px solid var(--td-component-border);border-radius:6px;background:var(--td-bg-color-container);color:inherit}.member-results{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:16px}.member-results button,.member-title button{background:transparent;border:1px solid var(--td-component-border);border-radius:6px;padding:6px 10px;color:inherit;cursor:pointer}.member-card{padding:16px;margin:12px 0;border:1px solid var(--td-component-border);border-radius:8px}.member-title{margin-bottom:14px}.member-title span{margin-right:auto;color:var(--td-text-color-secondary)}.operation-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:16px;margin-top:16px}.operation-grid label{display:flex;align-items:flex-start;gap:6px}.operation-grid small{display:block;line-height:1.6;color:var(--td-text-color-secondary);margin-top:3px}input[type=checkbox]{accent-color:#078563;margin-right:6px}.panel-actions{border-top:1px solid var(--td-component-border);padding:20px 0}details{margin-top:12px}summary{cursor:pointer}li{line-height:1.8;overflow-wrap:anywhere}.fixed-access,.operation-note{font-size:13px}
</style>
<style scoped>.kb-access{max-height:70vh;overflow:auto}.panel-actions{position:sticky;bottom:-20px;background:var(--td-bg-color-container);z-index:1}.member-search input{min-width:220px}</style>
