<template>
  <main class="publishing">
    <header><div><p class="eyebrow">企业网页集成</p><h1>发布智能体</h1><p>将知识问答放到员工工作的网页中。访问者需要登录企业账号。</p></div><t-button variant="outline" @click="$router.push('/platform/agents')">返回智能体</t-button></header>
    <t-alert v-if="error" theme="error" :message="error" />
    <section class="workspace">
      <aside><label for="assistant-agent">选择智能体</label><select id="assistant-agent" v-model="agent" @change="loadChannels"><option value="" disabled>请选择</option><option v-for="a in agents" :key="a.id" :value="a.id">{{ a.name }}</option></select>
        <t-button block :disabled="!agent || busy" @click="edit()">新建发布渠道</t-button>
        <button v-for="c in channels" :key="c.id" class="channel" :class="{selected:current===c.id}" @click="edit(c)"><strong>{{ c.name }}</strong><span>{{ c.enabled ? '已启用' : '已停用' }}</span></button>
        <p v-if="agent&&!channels.length" class="muted">还没有发布渠道。每个渠道可以设置独立的来源网页和调用限额。</p>
      </aside>
      <form v-if="agent" @submit.prevent="save">
        <h2>{{ current ? '渠道设置' : '新建渠道' }}</h2>
        <label>渠道名称<input v-model="form.name" required maxlength="100" placeholder="例如：员工门户助手" /></label>
        <label>允许嵌入的网页来源<textarea v-model="origins" required rows="3" placeholder="https://portal.example.com" /><small>每行一个完整来源，包含协议和端口；不包含路径、末尾斜杠或通配符。</small></label>
        <label>欢迎语<textarea v-model="form.welcome_message" rows="3" maxlength="2000" placeholder="你好，有什么可以帮助你？" /></label>
        <div class="limits"><label>每位员工每分钟<input v-model.number="form.rate_limit_per_minute" type="number" min="1" max="1000" required /></label><label>渠道每日总量（UTC）<input v-model.number="form.rate_limit_per_day" type="number" min="1" max="1000000" required /></label></div>
        <small>新建会话和提问计入限额。知识与智能体权限始终由服务端校验。</small>
        <div class="checks"><label><input v-model="form.enabled" type="checkbox" /> 启用渠道</label><label><input v-model="form.allow_file_upload" type="checkbox" /> 允许对话附件</label></div>
        <div class="actions"><t-button type="submit" :loading="busy">保存设置</t-button><t-button v-if="current" theme="danger" variant="text" :disabled="busy" @click="remove">删除渠道</t-button><span role="status">{{ status }}</span></div>
        <section v-if="current" class="embed-code"><h2>嵌入到网页</h2><p>保存后，将代码放入上述来源网页。代码只包含渠道标识，不包含登录凭证。</p><pre>{{ snippet }}</pre><t-button variant="outline" @click="copy">复制嵌入代码</t-button></section>
      </form>
      <section v-else class="empty">选择一个智能体，配置它的企业网页入口。</section>
    </section>
  </main>
</template>
<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { DialogPlugin } from 'tdesign-vue-next'
import { get, post, put, del } from '@/utils/request'
import { useAuthStore } from '@/stores/auth'
const auth = useAuthStore(), agent = ref(''), agents = ref<any[]>([]), channels = ref<any[]>([]), current = ref('')
const error = ref(''), status = ref(''), busy = ref(false), origins = ref('')
const defaults = () => ({name:'', enabled:true, welcome_message:'', rate_limit_per_minute:20, rate_limit_per_day:1000, allow_file_upload:false})
const form = reactive(defaults()), prefix='/api/v1/mindcreek/assistant'
const snippet = computed(() => `<script src="${window.location.origin}/mindcreek-assistant.js" data-space="${auth.effectiveTenantId}" data-channel="${current.value}" defer><\/script>`)
function edit(c?:any){current.value=c?.id || '';Object.assign(form,defaults(),c ? Object.fromEntries(Object.keys(defaults()).map(k=>[k,c[k]])) : {});origins.value=(c?.allowed_origins || []).join('\n');status.value=''}
function problem(e:any){error.value=e?.message || '操作失败，请重试。'}
async function loadChannels(){error.value='';edit();try{channels.value=(await get(`${prefix}/agents/${agent.value}/channels`)).data || []}catch(e){channels.value=[];problem(e)}}
async function save(){busy.value=true;error.value='';status.value='';try{const body={...form,allowed_origins:origins.value.split(/\n/).map(x=>x.trim()).filter(Boolean)};const result=current.value?await put(`${prefix}/channels/${current.value}`,body):await post(`${prefix}/agents/${agent.value}/channels`,body);await loadChannels();edit(result.data);status.value='已保存'}catch(e){problem(e)}finally{busy.value=false}}
function remove(){const dialog=DialogPlugin.confirm({header:'删除发布渠道',body:'删除后，此渠道的小助手和历史对话入口将无法继续访问。',confirmBtn:'删除',onConfirm:async()=>{dialog.hide();busy.value=true;try{await del(`${prefix}/channels/${current.value}`);await loadChannels()}catch(e){problem(e)}finally{busy.value=false}}})}
async function copy(){try{await navigator.clipboard.writeText(snippet.value);status.value='代码已复制'}catch{status.value='无法自动复制，请手动选择代码。'}}
onMounted(async()=>{try{const cfg=await get('/api/v1/mindcreek/auth/config');if(!cfg.assistant_enabled)throw new Error('此安装尚未启用企业网页小助手。');if(!auth.hasRole('admin'))throw new Error('仅空间所有者和管理员可以管理发布渠道。');const result=await get('/api/v1/agents');agents.value=result.data || [];if(agents.value.length){agent.value=agents.value[0].id;await loadChannels()}}catch(e){problem(e)}})
</script>
<style scoped>
.publishing{padding:36px;max-width:1280px;margin:auto;color:var(--td-text-color-primary)}header{display:flex;justify-content:space-between;gap:24px;margin-bottom:28px;align-items:center}h1{font-size:28px;margin:6px 0 12px}header p,.muted,.empty,small,.embed-code p{color:var(--td-text-color-secondary);line-height:1.7}.eyebrow{font-size:12px;letter-spacing:2px;color:#16856c!important}.workspace{display:grid;grid-template-columns:260px 1fr;border:1px solid var(--td-component-border);border-radius:16px;overflow:hidden;margin-top:20px}aside{padding:24px;background:var(--td-bg-color-secondarycontainer)}form,.empty{padding:28px}label{display:block;font-weight:500;font-size:14px;margin-bottom:20px}input:not([type=checkbox]),textarea,select{box-sizing:border-box;width:100%;display:block;margin-top:8px;border:1px solid var(--td-component-border);border-radius:7px;padding:10px;font:inherit;background:var(--td-bg-color-container);color:inherit}small{display:block;font-size:12px;font-weight:400;margin-top:7px}.limits{display:grid;grid-template-columns:1fr 1fr;gap:20px}.checks{display:flex;gap:24px;margin-top:22px}.checks label{display:flex;gap:7px;align-items:center}.channel{width:100%;text-align:left;display:flex;justify-content:space-between;padding:14px 10px;background:transparent;border:0;border-radius:8px;margin-top:12px;color:inherit;cursor:pointer}.channel span{font-size:12px;color:var(--td-text-color-secondary)}.channel.selected{background:#dcefe7;color:#13614e}.actions{display:flex;gap:12px;align-items:center}.actions span{font-size:12px;color:#16856c}h2{font-size:18px;margin-top:0}.embed-code{margin-top:32px;padding-top:24px;border-top:1px solid var(--td-component-border)}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:var(--td-bg-color-secondarycontainer);border-radius:8px;padding:18px;font-size:12px;line-height:1.8}@media(max-width:800px){.publishing{padding:20px}.workspace{grid-template-columns:1fr}.limits{grid-template-columns:1fr}header{align-items:flex-start}}
</style>
