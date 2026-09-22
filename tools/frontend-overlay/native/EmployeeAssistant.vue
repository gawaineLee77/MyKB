<template>
  <main class="employee-assistant">
    <header><div><span class="eyebrow">MINDCREEK · 企业小助手</span><h1>{{ channel?.name || '知识随时可用' }}</h1></div>
      <button v-if="channel" type="button" @click="logout">退出助手</button>
    </header>
    <section v-if="!channel" class="welcome">
      <div class="mark">M</div><h2>使用企业账号继续</h2>
      <p>登录后，即可使用你有权访问的知识与智能体。</p>
      <t-button theme="primary" size="large" :disabled="!hostReady" @click="login">企业登录</t-button>
      <p v-if="error" role="alert" class="error">{{ error }}</p>
      <p class="hint">登录将在独立窗口中完成。</p>
    </section>
    <template v-else>
      <nav><select aria-label="历史对话" :value="sessionId" :disabled="replying" @change="selectHistory(($event.target as HTMLSelectElement).value)"><option value="">新对话</option><option v-for="(s,index) in sessions" :key="s.session_id" :value="s.session_id">对话 {{ sessions.length-index }}</option></select><button :disabled="replying" @click="newConversation">新建对话</button></nav>
      <section ref="scrollContainer" class="messages">
        <p v-if="messages.length===0" class="greeting">{{ channel.welcome_message || '你好，有什么可以帮助你？' }}</p>
        <article v-for="(message,index) in messages" :key="String(message.id || index)">
          <EmbedUserMessage v-if="message.role==='user'" :content="String(message.content || '')" :images="(message.images || []) as any" :attachments="(message.attachments || []) as any" :employee-session-id="sessionId" :employee-message-id="String(message.id || '')" />
          <EmbedBotMessage v-else :content="String(message.content || '')" :session="message as any" :session-id="sessionId" :embed-channel-id="assistantChannel" />
        </article>
        <p v-if="loading" class="hint">正在查询知识…</p>
      </section>
      <div class="composer"><p v-if="error" role="alert" class="error">{{ error }}</p>
        <button v-if="interrupted" @click="reconnect">重新连接回答</button>
        <EmbedInputField placeholder="向企业知识小助手提问" :is-replying="replying" :show-file-upload-toggle="channel.allow_file_upload" :show-web-search-toggle="false" @send-msg="send" @stop-generation="stop" />
        <p class="hint">仅使用当前账号可访问的知识。重要结论请核对来源。</p>
      </div>
    </template>
    <ChatReferencesDrawer embedded-mode />
  </main>
</template>
<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, reactive, ref } from 'vue'
import { fetchEventSource } from '@microsoft/fetch-event-source'
import EmbedInputField from '@/components/EmbedInputField.vue'
import EmbedBotMessage from '@/views/embed/EmbedBotMessage.vue'
import EmbedUserMessage from '@/views/embed/EmbedUserMessage.vue'
import ChatReferencesDrawer from '@/components/ChatReferencesDrawer.vue'
import { provideChatReferencesDrawer } from '@/composables/useChatReferencesDrawer'
import { useChatStreamHandler } from '@/composables/useChatStreamHandler'
import { fileToDataURI } from '@/utils/embedFile'
import { assistantChannel, assistantOrigin, assistantHeaders, assistantNativeURL, assistantJSON, assistantFetch, setAssistantSession, setAssistantToken } from './assistant-context'
provideChatReferencesDrawer()
const channel = ref<any>(null), error = ref(''), sessions = ref<Array<{session_id:string}>>([]), sessionId = ref('')
const messages = reactive<Record<string,unknown>[]>([]), loading = ref(false), replying = ref(false)
const assistantMessageId = ref(''), fullContent = ref(''), scrollContainer = ref<HTMLElement|null>(null)
const hostReady = ref(window.parent===window), interrupted = ref(false)
let popup: Window|null = null, nonce = '', controller: AbortController|undefined, loginTimeout: ReturnType<typeof setTimeout>|undefined
const scroll = () => { void nextTick(() => { if(scrollContainer.value) scrollContainer.value.scrollTop=scrollContainer.value.scrollHeight }) }
const handler = useChatStreamHandler({ messagesList: messages, loading, isReplying: replying, currentAssistantMessageId: assistantMessageId, fullContent,
  isAgentStreamSession: () => channel.value?.agent_mode !== 'quick-answer', scrollToBottom:scroll,
  onError: message => { error.value=message }, preserveIncompleteStreamReactive:true,
})
function denied() { controller?.abort(); channel.value=null; messages.splice(0); setAssistantToken(''); replying.value=false; error.value='登录已过期或访问权限已变更，请重新登录。' }
function logout() { window.location.reload() }
async function authenticated(token:string) {
  setAssistantToken(token);error.value=''
  try { channel.value=(await assistantJSON('config')).data; await loadSessions() }
  catch(e) { channel.value=null;error.value=e instanceof Error?e.message:'无法访问此助手' }
}
function login() {
  error.value='';nonce=Array.from(crypto.getRandomValues(new Uint8Array(32)),x=>x.toString(16).padStart(2,'0')).join('')
  popup=window.open(`/assistant-login?nonce=${nonce}`,'_blank','popup,width=520,height=720')
  if (!popup) error.value='浏览器阻止了登录弹窗，请允许此站点打开弹窗后重试。'
  clearTimeout(loginTimeout)
  loginTimeout=setTimeout(()=>{nonce='';if(!channel.value)error.value='登录窗口已超时，请重新登录。'},10*60_000)
}
function message(event:MessageEvent) {
  if(event.source===window.parent&&event.origin===assistantOrigin&&event.data?.type==='mindcreek-assistant-host'){hostReady.value=true;return}
  if(event.source!==popup||event.origin!==window.location.origin||!nonce||event.data?.nonce!==nonce)return
  if(event.data.type==='mindcreek-assistant-ready') popup?.postMessage({type:'mindcreek-assistant-token-request',nonce},window.location.origin)
  if(event.data.type==='mindcreek-assistant-token'&&typeof event.data.token==='string'){nonce='';clearTimeout(loginTimeout);void authenticated(event.data.token)}
}
async function loadSessions(){sessions.value=(await assistantJSON('sessions')).data}
function newConversation(){controller?.abort();sessionId.value='';setAssistantSession('');messages.splice(0);error.value='';interrupted.value=false}
async function selectHistory(id:string){newConversation();if(!id)return;sessionId.value=id;setAssistantSession(id);try{const result=await assistantJSON(`/api/v1/messages/${id}/load?limit=100`);await handler.handleMsgList(result.data || [])}catch(e){error.value=String(e)}}
async function stream(path:string,body?:unknown){
  controller?.abort();controller=new AbortController();interrupted.value=false
  try {
    await fetchEventSource(assistantNativeURL(path),{method:body===undefined?'GET':'POST',credentials:'omit',headers:{...assistantHeaders(),'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body),signal:controller.signal,openWhenHidden:true,
      async onopen(response){if(!response.ok){if([401,403].includes(response.status))denied();throw new Error(`请求失败（${response.status}）`)}if(!response.headers.get('content-type')?.includes('text/event-stream'))throw new Error('服务未返回对话流')},
      onmessage(event){if(event.data&&event.data!=='[DONE]'){handler.processStreamChunk(JSON.parse(event.data));scroll()}},
      onerror(err){throw err}, // Retries are explicit; the gateway reauthorizes every reconnect.
    })
    if(replying.value&&!controller.signal.aborted){interrupted.value=true;error.value='回答连接已中断，可以重新连接。'}
  } catch(e){if(!controller.signal.aborted){error.value=String(e);interrupted.value=true}}
  finally{loading.value=false;replying.value=false}
}
async function send(query:string,imageFiles:File[]=[],files:File[]=[]){
  if(replying.value||!channel.value)return
  error.value='';replying.value=true;loading.value=true;handler.prepareForNewOutgoingMessage()
  try {
    if(!sessionId.value){sessionId.value=(await assistantJSON('sessions',{title:query.slice(0,80)})).data.id;setAssistantSession(sessionId.value);await loadSessions()}
    const images=await Promise.all(imageFiles.map(async f=>({data:String(await fileToDataURI(f))})))
    const uploads=await Promise.all(files.map(async f=>({data:String(await fileToDataURI(f)),file_name:f.name,file_size:f.size})))
    messages.push({role:'user',content:query,images:images.map(v=>({url:v.data})),attachments:uploads,created_at:new Date().toISOString()});scroll()
    const endpoint=channel.value.agent_mode==='quick-answer'?'knowledge-chat':'agent-chat'
    await stream(`/api/v1/${endpoint}/${sessionId.value}`,{query,images,attachment_uploads:uploads})
  }catch(e){error.value=String(e);replying.value=false;loading.value=false}
}
async function reconnect(){if(!assistantMessageId.value){error.value='未收到回答标识，请打开历史对话后重试。';return}replying.value=true;await stream(`/api/v1/sessions/continue-stream/${sessionId.value}?message_id=${encodeURIComponent(assistantMessageId.value)}`)}
async function stop(){controller?.abort();replying.value=false;loading.value=false;handler.markInFlightAssistantStopped();try{if(assistantMessageId.value)await assistantFetch(`/api/v1/sessions/${sessionId.value}/stop`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({message_id:assistantMessageId.value})})}catch(e){error.value=String(e)}}
onMounted(()=>{window.addEventListener('message',message);window.addEventListener('mindcreek-assistant-denied',denied);if(window.parent!==window)window.parent.postMessage({type:'mindcreek-assistant-ready-for-host'},assistantOrigin)})
onUnmounted(()=>{controller?.abort();clearTimeout(loginTimeout);setAssistantToken('');window.removeEventListener('message',message);window.removeEventListener('mindcreek-assistant-denied',denied)})
</script>
<style scoped>
.employee-assistant{height:100dvh;display:flex;flex-direction:column;background:var(--td-bg-color-container,#fff);color:var(--td-text-color-primary,#183a36);overflow:hidden}
header{padding:20px 24px;border-bottom:1px solid #e6efeb;display:flex;justify-content:space-between;align-items:center}.eyebrow{font-size:10px;letter-spacing:1.8px;color:#2c8172}h1{font-size:20px;margin:6px 0 0}button,select{font:inherit;cursor:pointer;border:1px solid #d8e7e1;border-radius:8px;background:transparent;padding:7px 12px;color:inherit}button:disabled{opacity:.5;cursor:default}
.welcome{margin:auto;padding:36px;text-align:center;max-width:460px}.mark{display:grid;place-items:center;background:#147c66;color:#fff;width:64px;height:64px;border-radius:20px;margin:auto;font-size:30px}.welcome h2{font-size:24px;margin:24px 0 12px}.welcome p{line-height:1.8;color:#667e77}.welcome .t-button{margin:18px 0}
nav{display:flex;gap:10px;padding:12px 20px}select{flex:1}.messages{flex:1;overflow:auto;padding:8px 22px 24px}.messages article{margin:16px 0}.greeting{line-height:1.8;color:#496c62;padding:20px;background:#f2f8f5;border-radius:14px}.composer{padding:10px 18px 12px;border-top:1px solid #e6efeb}.hint{font-size:11px;color:#81938d;text-align:center;margin:10px 0 0}.error{font-size:13px;color:#b42318;overflow-wrap:anywhere}
</style>
