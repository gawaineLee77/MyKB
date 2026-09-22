<template>
  <section class="enterprise-members" data-testid="enterprise-members">
    <div class="operations">
      <t-button variant="outline" @click="batchOpen = !batchOpen">批量添加员工</t-button>
      <t-button variant="outline" @click="openTransfer">移交空间所有者</t-button>
    </div>
    <t-alert v-if="error" theme="error" :message="error" />
    <div v-if="batchOpen" class="batch-panel">
      <h3>批量添加员工</h3>
      <p>目标空间：<strong>{{ tenantName }}</strong>。仅添加已通过企业登录开户的员工，已有成员的角色保持不变。</p>
      <label class="field">员工邮箱
        <textarea v-model="input" :disabled="busy" placeholder="每行一个邮箱，或粘贴含 email / 邮箱列的 CSV" rows="5" />
      </label>
      <label class="file">导入 CSV <input type="file" accept=".csv,text/csv" :disabled="busy" @change="loadFile" /></label>
      <label class="field">新增成员角色
        <select v-model="role" :disabled="busy"><option value="viewer">Viewer · 访客</option><option value="contributor">Contributor · 编辑者</option><option value="admin">Admin · 管理员</option></select>
      </label>
      <div class="operations"><t-button :loading="busy" @click="preview">预览成员</t-button><t-button v-if="rows.length" theme="primary" :disabled="busy || !previewCurrent" @click="run">{{ hasRun ? '核对并重试失败项' : '确认添加' }}</t-button></div>
      <p v-if="summary" role="status">{{ summary }}</p>
      <p v-if="rows.length && !previewCurrent">输入或角色已变化，请重新预览。</p>
      <div class="result-scroll" v-if="rows.length"><table><thead><tr><th>员工邮箱</th><th>当前角色</th><th>结果</th></tr></thead><tbody><tr v-for="row in rows" :key="row.email"><td>{{ row.email }}</td><td>{{ row.role || '—' }}</td><td>{{ labels[row.state] || row.state }}<small v-if="row.code">{{ row.code }}</small></td></tr></tbody></table></div>
    </div>
    <t-dialog v-model:visible="transferOpen" header="移交空间所有者" :footer="false" :close-on-overlay-click="false" width="560px">
      <p>空间：<strong>{{ tenantName }}</strong></p>
      <p>当前所有者：{{ currentName }}。完成移交后你成为空间 Admin，平台管理员身份保持不变。</p>
      <label class="field">目标所有者<select v-model="target" :disabled="busy || Boolean(intent)"><option value="">请选择已有成员</option><option v-for="member in targets" :key="member.user_id" :value="member.user_id">{{ member.username || member.email }} · {{ member.role }}</option></select></label>
      <p v-if="stage === 'demote'">第一步已完成：目标成员已成为 Owner。确认第二步后，你将成为 Admin。</p>
      <p v-if="stage === 'complete'" role="status">所有权移交已完成。</p>
      <p v-if="stage === 'conflict'">成员或角色已变化，无法继续。请重新查询核对。</p>
      <t-alert v-if="transferError" theme="error" :message="transferError" />
      <div class="operations">
        <t-button variant="outline" :disabled="busy" @click="refreshTransfer">重新查询角色</t-button>
        <t-button v-if="stage === 'conflict'" variant="outline" :disabled="busy" @click="resetTransfer">结束本次移交，保留现有角色</t-button>
        <t-button v-if="stage === 'promote' || stage === 'demote'" theme="primary" :disabled="!target || busy" @click="advance">{{ stage === 'promote' ? '第一步：确认目标成为 Owner' : '第二步：确认自己成为 Admin' }}</t-button>
      </div>
    </t-dialog>
  </section>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import type { TenantMember } from '@/api/tenant/members'
import { batchJournal, parseMemberEmails, runMemberBatch, type BatchRole, type BatchRow } from './member-batch'
import { addBatchMember, previewMembers, readMembers, transferState, transferStep, type TransferIntent } from './member-operations'
const emit = defineEmits<{ (event: 'changed'): void }>()
const auth = useAuthStore()
const tenant = Number(auth.effectiveTenantId), user = auth.user?.id || ''
const tenantName = computed(() => auth.selectedTenantName || auth.tenant?.name || String(tenant))
const error = ref(''), busy = ref(false), input = ref(''), batchOpen = ref(false), role = ref<BatchRole>('viewer')
const rows = ref<BatchRow[]>([]), summary = ref(''), previewText = ref(''), previewRole = ref(''), hasRun = ref(false)
const previewCurrent = computed(() => previewText.value === input.value && previewRole.value === role.value)
const labels: Record<string, string> = { ready: '待添加', existing: '已有成员，保留原角色', unregistered: '尚未开户或邮箱冲突', invalid: '邮箱无效', adding: '正在添加', added: '添加成功', failed: '添加失败', conflict: '冲突，请核对', unknown: '结果未知，重试前先核对' }
function assertContext() { if (Number(auth.effectiveTenantId) !== tenant || auth.currentTenantRole !== 'owner' || auth.user?.id !== user) throw new Error('空间或角色已变化，请刷新页面。') }
async function loadFile(event: Event) { const file = (event.target as HTMLInputElement).files?.[0]; if (!file) return; if (file.size > 131072) { error.value = 'CSV 不能超过 128 KiB。'; return }; input.value = await file.text() }
async function preview() {
  busy.value = true; error.value = ''
  try { assertContext(); const parsed = parseMemberEmails(input.value); if (!parsed.emails.length) throw new Error('请输入至少一个有效邮箱。'); rows.value = await previewMembers(tenant, parsed.emails); rows.value.push(...parsed.invalid.map(email => ({ email, state: 'invalid' }))); summary.value = `有效邮箱 ${parsed.emails.length} 个，已去重 ${parsed.duplicates} 个，无效 ${parsed.invalid.length} 个。`; previewText.value = input.value; previewRole.value = role.value; hasRun.value = false } catch (e: any) { error.value = e.message || '预览失败'; rows.value = [] } finally { busy.value = false }
}
async function run() {
  if (!previewCurrent.value || busy.value) return
  busy.value = true; error.value = ''
  try {
    assertContext(); hasRun.value = true
    await runMemberBatch(rows.value, role.value, { preview: emails => { assertContext(); return previewMembers(tenant, emails) }, add: (email, fixedRole) => { assertContext(); return addBatchMember(tenant, email, fixedRole) } })
    sessionStorage.setItem(`mindcreek:r4:batch:${user}:${tenant}`, JSON.stringify(await batchJournal(user, tenant, role.value, rows.value)))
    summary.value = `处理完成：成功 ${rows.value.filter(r => r.state === 'added').length}，已有成员 ${rows.value.filter(r => r.state === 'existing').length}。其余结果见下表。`
    emit('changed')
  } catch (e: any) { error.value = e.message || '添加失败' } finally { busy.value = false }
}
const transferOpen = ref(false), transferError = ref(''), members = ref<TenantMember[]>([]), target = ref('')
const intent = ref<TransferIntent | null>(null), stage = ref<'promote' | 'demote' | 'complete' | 'conflict'>('promote')
const transferKey = `mindcreek:r4:transfer:${user}:${tenant}`
const targets = computed(() => members.value.filter(m => m.user_id !== user && m.status === 'active'))
const currentName = computed(() => members.value.find(m => m.user_id === user)?.username || auth.user?.username || user)
async function refreshTransfer() { transferError.value = ''; try { members.value = await readMembers(tenant); if (intent.value) stage.value = transferState(intent.value, members.value); if (stage.value === 'complete') sessionStorage.removeItem(transferKey) } catch (e: any) { transferError.value = e.message; stage.value = 'conflict' } }
async function openTransfer() { transferOpen.value = true; await refreshTransfer() }
function resetTransfer() { sessionStorage.removeItem(transferKey); intent.value = null; target.value = ''; stage.value = 'promote'; transferError.value = ''; transferOpen.value = false }
async function advance() {
  busy.value = true; transferError.value = ''
  try {
    assertContext()
    if (!intent.value) { intent.value = { tenant, from: user, to: target.value }; sessionStorage.setItem(transferKey, JSON.stringify(intent.value)) }
    if (stage.value !== 'promote' && stage.value !== 'demote') return
    stage.value = await transferStep(intent.value, stage.value)
    if (stage.value === 'complete') { sessionStorage.removeItem(transferKey); emit('changed'); await auth.refreshFromAuthMe() }
    else await refreshTransfer()
  } catch (e: any) { transferError.value = e.message || '结果未知，请重新查询角色。' } finally { busy.value = false }
}
onMounted(() => { try { const saved = JSON.parse(sessionStorage.getItem(transferKey) || 'null'); if (saved?.tenant === tenant && saved?.from === user && typeof saved.to === 'string') { intent.value = saved; target.value = saved.to; void openTransfer() } } catch { sessionStorage.removeItem(transferKey) } })
</script>
<style scoped>
.enterprise-members { margin-bottom: 20px; }.operations { display: flex; gap: 10px; margin: 16px 0; flex-wrap: wrap; }.batch-panel { margin-top: 14px; border: 1px solid var(--td-component-border); border-radius: 10px; padding: 20px; background: var(--td-bg-color-container); }.field { display: flex; flex-direction: column; gap: 8px; margin: 16px 0; }.field textarea,.field select { padding: 10px; border: 1px solid var(--td-component-border); border-radius: 6px; background: var(--td-bg-color-container); color: var(--td-text-color-primary); font: inherit; }.result-scroll { max-height: 360px; overflow: auto; }table { width: 100%; border-collapse: collapse; }th,td { padding: 10px; text-align: left; border-bottom: 1px solid var(--td-component-border); }small { display: block; color: var(--td-text-color-secondary); }p { line-height: 1.7; margin: 14px 0; }
</style>
