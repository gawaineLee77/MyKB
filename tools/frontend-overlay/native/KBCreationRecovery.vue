<template><details class="kb-creation-recovery" @toggle="opened"><summary>知识库创建恢复</summary><p>创建请求超时或结果不确定时，请先核对实际生成的知识库。仅将已核实的知识库 ID 绑定到对应操作；不会重新执行创建。</p><t-alert v-if="error" theme="error" :message="error" /><t-button variant="text" :loading="busy" @click="load">刷新记录</t-button><p v-if="!operations.length">暂无待确认的创建操作</p><div v-for="operation in operations" :key="operation.id" class="recovery-row"><span>{{ operation.created_at }} · 发起者 {{ operation.actor_user_id }}<br />操作 {{ operation.id }}</span><input v-model="targets[operation.id]" placeholder="已核实的知识库 ID" :aria-label="'恢复操作 ' + operation.id" /><t-popconfirm content="确认已核实此知识库属于这次创建操作？服务器会再次核对空间与创建者。" @confirm="resolve(operation.id)"><t-button :disabled="busy || !targets[operation.id]?.trim()">绑定并恢复</t-button></t-popconfirm></div></details></template>
<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { get, post } from '@/utils/request'
import { announceKBPermissions } from './kb-permissions'
import { kbPermissionProblem } from './kb-permissions-policy'
const auth = useAuthStore(), operations = ref<any[]>([]), targets = ref<Record<string, string>>({}), error = ref(''), busy = ref(false)
const endpoint = '/api/v1/mindcreek/kb-creation-operations'
async function load() { busy.value = true; error.value = ''; try { const result: any = await get(endpoint); operations.value = (result.data || []).filter((row: any) => row.state === 'pending') } catch (err) { error.value = kbPermissionProblem(err) } finally { busy.value = false } }
function opened(event: Event) { if ((event.target as HTMLDetailsElement).open) void load() }
async function resolve(id: string) { if (busy.value || !['owner', 'admin'].includes(auth.currentTenantRole || '')) return; busy.value = true; error.value = ''; try { await post(endpoint + '/' + encodeURIComponent(id) + '/resolve', { kb_id: targets.value[id]?.trim() }); announceKBPermissions(); await load() } catch (err) { error.value = kbPermissionProblem(err) } finally { busy.value = false } }
</script>
<style scoped>.kb-creation-recovery{margin:12px 0;padding:12px;border:1px solid var(--td-component-border);border-radius:8px}summary{cursor:pointer}p{color:var(--td-text-color-secondary);line-height:1.6}.recovery-row{display:flex;gap:12px;align-items:center;padding:12px 0;flex-wrap:wrap}.recovery-row span{flex:1;overflow-wrap:anywhere}.recovery-row input{padding:8px;border:1px solid var(--td-component-border);border-radius:6px;background:var(--td-bg-color-container);color:inherit}</style>
