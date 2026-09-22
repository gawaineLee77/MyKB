// Employee assistant adapters applied only to the disposable frontend copy.
export const assistantNginx = `    # Employee assistant: CSP binds the frame to its declared parent origin.
    location = /_mindcreek_assistant_policy {
        internal;
        proxy_pass \${APP_SCHEME}://\${APP_HOST}:\${APP_PORT}/api/v1/mindcreek/assistant-frame-policy;
        proxy_pass_request_body off;
        proxy_set_header Content-Length "";
        proxy_set_header X-Original-URI $request_uri;
    }
    location ^~ /assistant/ {
        root /usr/share/nginx/html;
        auth_request /_mindcreek_assistant_policy;
        auth_request_set $assistant_csp $upstream_http_content_security_policy;
        add_header Content-Security-Policy $assistant_csp always;
        add_header X-Content-Type-Options "nosniff" always;
        add_header Referrer-Policy "no-referrer" always;
        add_header Cache-Control "no-store" always;
        rewrite ^ /index.html break;
    }
    location = /mindcreek-assistant.js {
        root /usr/share/nginx/html;
        add_header Cache-Control "no-cache" always;
        add_header X-Content-Type-Options "nosniff" always;
        try_files $uri =404;
    }

`

export function applyEmployeeAssistant({read,write,replaceExact:exact}) {
  exact('src/components/EmbedInputField.vue', ':placeholder="t(\'input.placeholder\')"', ':placeholder="placeholder || t(\'input.placeholder\')"')
  exact('src/components/EmbedInputField.vue', '  isReplying: boolean', '  isReplying: boolean\n  placeholder?: string')
  exact('nginx.conf','    location = /weknora-widget.js { return 404; }',assistantNginx+'    location = /weknora-widget.js { return 404; }')
  exact('src/router/index.ts','  const authStore = useAuthStore()',`  if (to.path === '/assistant-login' || to.path.startsWith('/assistant/')) { next(); return }
  const authStore = useAuthStore()`)
  exact('src/router/index.ts','      path: "/admin/login",',`      path: '/assistant/:tenant/:channel', component: () => import('@/mindcreek/EmployeeAssistant.vue')
    },
    { path: '/assistant-login', component: () => import('@/mindcreek/AssistantLogin.vue') },
    {
      path: "/admin/login",`)
  exact('src/router/index.ts', '          path: "retired",', `          path: 'assistant-publishing', component: () => import('@/mindcreek/AssistantPublishing.vue'), meta: { requiresAuth:true, requiresInit:true }
        },
        {
          path: "retired",`)
  exact('src/views/agent/AgentList.vue','<script setup lang="ts">',"<script setup lang=\"ts\">\nimport AssistantPublishButton from '@/mindcreek/AssistantPublishButton.vue'")
  exact('src/views/agent/AgentList.vue','<h2 style="--wails-draggable: drag">{{ $t(\'agent.title\') }}</h2>', '<h2 style="--wails-draggable: drag">{{ $t(\'agent.title\') }}</h2><AssistantPublishButton />')
  exact('src/mindcreek/enterprise-entry.ts',"import { workspaceHome } from './native-policy'","import { workspaceHome } from './native-policy'\nimport { pendingAssistantLogin } from './assistant-context'")
  exact('src/mindcreek/enterprise-entry.ts',"    if (['/', '/login',", "    const assistantLogin = pendingAssistantLogin()\n    if (assistantLogin && path !== '/assistant-login') return assistantLogin\n    if (['/', '/login',")
  for(const path of ['src/main.ts','src/App.vue','src/utils/request.ts','src/utils/security.ts','src/views/embed/EmbedBotMessage.vue']) {
    const imports=`import { assistantActive, assistantNativeURL, assistantHeaders } from '${path === 'src/utils/security.ts' ? '../mindcreek/assistant-context' : '@/mindcreek/assistant-context'}'\n`
    if(path.endsWith('.vue'))exact(path,'<script setup lang="ts">','<script setup lang="ts">\n'+imports)
    else write(path,imports+read(path))
  }
  exact('src/main.ts','  if (localStorage.getItem("weknora_token")) {','  if (!assistantActive && localStorage.getItem("weknora_token")) {')
  exact('src/App.vue','  if (invitationPollTimer || !authStore.isLoggedIn) return','  if (assistantActive || invitationPollTimer || !authStore.isLoggedIn) return')
  exact('src/App.vue','onMounted(() => {','onMounted(() => {\n  if (assistantActive) return')
  exact('src/utils/request.ts','  (config) => {',`  (config) => {
    if (assistantActive) {
      config.url = assistantNativeURL(config.url || '');
      Object.assign(config.headers, assistantHeaders());
      config.withCredentials = false;
      return config;
    }`)
  exact('src/utils/request.ts','  async (error: any) => {',`  async (error: any) => {
    if (assistantActive) {
      if ([401,403].includes(error.response?.status)) window.dispatchEvent(new Event('mindcreek-assistant-denied'));
      return Promise.reject(error);
    }`)
  exact('src/views/embed/EmbedBotMessage.vue','    props.embedChannelId && props.embedToken',`    assistantActive && props.sessionId && (props.session as any)?.id
      ? { mode:'message', sessionId:props.sessionId, messageId:String((props.session as any).id) }
      : props.embedChannelId && props.embedToken`)
  exact('src/utils/security.ts','    const { url: requestURL, headers } = request;',`    const requestURL = assistantActive ? assistantNativeURL(request.url) : request.url;
    const headers = assistantActive ? assistantHeaders() : request.headers;`)
  exact('src/utils/security.ts',"            credentials: 'include',","            credentials: assistantActive ? 'omit' : 'include',")
  exact('src/views/embed/EmbedUserMessage.vue','<script setup lang="ts">',"<script setup lang=\"ts\">\nimport { assistantActive } from '@/mindcreek/assistant-context'")
  exact('src/views/embed/EmbedUserMessage.vue','    embedToken?: string','    embedToken?: string\n    employeeSessionId?: string\n    employeeMessageId?: string')
  exact('src/views/embed/EmbedUserMessage.vue','  if (!props.embedChannelId || !props.embedToken) return',`  if (assistantActive) {
    if (props.employeeSessionId && props.employeeMessageId) await hydrateProtectedFileImages(containerRef.value, {mode:'message',sessionId:props.employeeSessionId,messageId:props.employeeMessageId})
    return
  }
  if (!props.embedChannelId || !props.embedToken) return`)
}
