// Exact-anchor adapters for the pinned v0.8.0 copy; never writes the submodule.
export const retainedModules = [
  'AuthEntry.vue', 'SSOLogin.vue', 'AdminLogin.vue', 'EmployeeOnboarding.vue',
  'InstallationStatus.vue', 'enterprise-auth.ts', 'enterprise-entry.ts',
  'model-selection.ts', 'model-selection.test.ts', 'api-problem.ts', 'api-problem.test.ts',
]

export function applyNativeUI({ read, write, replaceExact: exact, replaceRegex: regex }) {
  for (const pattern of [/    location = \/weknora-widget\.js \{[\s\S]*?\n    \}/g, /    location = \/embed\.html \{[\s\S]*?\n    \}/g, /    location \^~ \/embed\/ \{[\s\S]*?\n    \}/g]) {
    regex('nginx.conf', pattern, match => match.slice(0, match.indexOf('{') + 1) + ' return 404; }')
  }
  write('src/utils/request.ts', "import { nativeProblem } from '@/mindcreek/native-problems'\n" + read('src/utils/request.ts'))
  exact('src/utils/request.ts', '      message: errorMessage,', "      message: nativeProblem(data, errorMessage || '请求失败'),")
  const router = 'src/router/index.ts'
  exact(router, "import { enterpriseDestination } from '@/mindcreek/enterprise-entry'", "import { enterpriseDestination } from '@/mindcreek/enterprise-entry'\nimport { nativeDestination } from '@/mindcreek/native-navigation'")
  exact(router, '  // Lite：硬刷新后若落在默认首页，恢复本次会话中最后访问的 /platform 子路径', `  const nativeTarget = nativeDestination(to.path, to.query)
  if (nativeTarget) { next(nativeTarget); return }

  // Lite：硬刷新后若落在默认首页，恢复本次会话中最后访问的 /platform 子路径`)
  exact(router, '        {\n          path: "tenant",', `        {
          path: "retired", component: () => import('@/mindcreek/RetiredPage.vue'),
          meta: { requiresAuth: true, requiresInit: true }
        },
        {
          path: "mindcreek/:pathMatch(.*)*", redirect: '/platform/retired'
        },
        {
          path: "tenant",`)
  // Viewer direct links land in chat; the backend retains native resource rights.
  const menu = 'src/stores/menu.ts'
  exact(menu, '    return menuArr.filter(item => {', `    return menuArr.filter(item => {
      if (!authStore.hasRole('contributor') && !['creatChat', 'logout'].includes(item.path)) return false`)
  exact('src/components/menu.vue', "@click=\"router.push('/platform/knowledge-bases')\"", "@click=\"router.push(authStore.hasRole('contributor') ? '/platform/knowledge-bases' : '/platform/creatChat')\"")
  const settings = 'src/views/settings/Settings.vue'
  exact(settings, "const canSeeSection = (key: string): boolean => {", `const canSeeSection = (key: string): boolean => {
  if (!authStore.hasRole('contributor') && !['general', 'userprofile'].includes(key)) return false`)
  exact(settings, "  'weknoracloud',", "  'weknoracloud',\n  'chathistory',\n  'ollama',")
  const userMenu = 'src/components/UserMenu.vue'
  exact(userMenu, 'v-if="!authStore.isLiteMode" class="menu-item" @click="handleQuickNav(\'tenant\')"', 'v-if="!authStore.isLiteMode && authStore.hasRole(\'contributor\')" class="menu-item" @click="handleQuickNav(\'tenant\')"')
  exact('src/views/settings/UserProfile.vue', '<script setup lang="ts">', '<script setup lang="ts">\nimport { isLocalAdminSession } from \'@/mindcreek/enterprise-auth\'')
  exact('src/views/settings/UserProfile.vue', '  () => userInfo.value?.preferences?.oidc_only_login === true,', '  () => !isLocalAdminSession(),')
  // Retain native model management, with defaults displayed separately.
  exact('src/mindcreek/enterprise-entry.ts', "import { get, post } from '@/utils/request'", "import { get, post } from '@/utils/request'\nimport { workspaceHome } from './native-policy'")
  exact('src/mindcreek/enterprise-entry.ts', "return '/platform/knowledge-bases'", "return workspaceHome(auth.currentTenantRole)")
  exact('src/api/chat-history.ts', "return post('/api/v1/messages/search', data)", "return post('/api/v1/messages/search', { ...data, mode: 'keyword' })")
  exact('src/stores/deploymentCapabilities.ts', '  const isSupported = (key?: DeploymentCapabilityKey) => {', `  const isSupported = (key?: DeploymentCapabilityKey) => {
    if (key && ['integrations.im', 'integrations.embed', 'settings.mcp', 'settings.websearch', 'settings.sandbox', 'settings.sandbox.docker'].includes(key)) return false`)
  regex('src/stores/chatResources.ts', /  async function ensureWebSearchProviders\(force = false\): Promise<void> \{[\s\S]*?\n  \}/g, `  async function ensureWebSearchProviders(_force = false): Promise<void> {
    webSearchProviders.value = []
    loadedAt.value.webSearchProviders = Date.now()
  }`)
  regex('src/stores/chatResources.ts', /  async function ensureSandboxConfigs\(force = false\): Promise<void> \{[\s\S]*?\n  \}/g, `  async function ensureSandboxConfigs(_force = false): Promise<void> {
    sandboxConfigs.value = []
    loadedAt.value.sandboxConfigs = Date.now()
  }`)
  regex('src/components/menu.vue', /const loadSessionOriginMeta = async \(\) => \{[\s\S]*?\n\};/g, `const loadSessionOriginMeta = async () => {
    imPlatforms.value = [];
    embedChannelNames.value = {};
};`)
  regex('src/components/Input-field.vue', /const loadMCPServices = async \(\) => \{[\s\S]*?\n\};/g, `const loadMCPServices = async () => { mcpServices.value = []; };`)

  exact('src/views/agent/AgentList.vue', 'const { loaded: modelsReadyLoaded, isReadyForAgent } = useTenantModelReadiness()', 'const { loaded: modelsReadyLoaded, isReadyForAgent, refresh: refreshModelReadiness } = useTenantModelReadiness()')
  exact('src/views/agent/AgentList.vue', 'const handleCreateAgent = () => {', `const handleCreateAgent = async () => {
  try { await refreshModelReadiness() } catch { MessagePlugin.error('模型列表加载失败，请重试。'); return }`)
  const agent = 'src/views/agent/AgentEditorModal.vue'
  exact(agent, '  return allTools.value.map(tool => {', '  return allTools.value.filter(tool => SUPPORTED_AGENT_TOOLS.has(tool.value)).map(tool => {')
  exact(agent, '<script setup lang="ts">', `<script setup lang="ts">\nimport { SUPPORTED_AGENT_TOOLS, agentConfigurationProblem } from '@/mindcreek/native-policy';`)
  regex(agent, /                    <div class="setting-row">\n                      <div class="setting-info">\n                        <label>\{\{ \$t\('agent.editor.memoryEnabled'\)[\s\S]*?<\/div>\n                    <\/div>/g, '')
  exact(agent, '    memory_enabled: true,', '    memory_enabled: false,')
  exact(agent, 'if (agentData.config.memory_enabled == null) agentData.config.memory_enabled = true;', 'if (agentData.config.memory_enabled == null) agentData.config.memory_enabled = false;')
  exact(agent, "  return agentTypePresets.value.map(p => ({", "  return agentTypePresets.value.filter(p => !p.config?.allowed_tools?.some((name: string) => !SUPPORTED_AGENT_TOOLS.has(name))).map(p => ({")
  exact(agent, '<div class="setting-row" data-guide="agent-create-name">', `<t-alert v-if="formData.config.memory_enabled" theme="warning" message="此安装未启用记忆。">
                      <template #operation><t-button variant="text" @click="formData.config.memory_enabled = false">关闭记忆</t-button></template>
                    </t-alert>
                    <t-alert v-if="formData.config.allowed_tools.some((name: string) => !SUPPORTED_AGENT_TOOLS.has(name))" theme="warning" message="当前配置包含不支持的工具。">
                      <template #operation><t-button variant="text" @click="formData.config.allowed_tools = formData.config.allowed_tools.filter((name: string) => SUPPORTED_AGENT_TOOLS.has(name))">移除不支持的工具</t-button></template>
                    </t-alert>
                    <div class="setting-row" data-guide="agent-create-name">`)
  exact(agent, 'const handleSave = async () => {', `const handleSave = async () => {
  const problem = agentConfigurationProblem(formData.value.config);
  if (problem) { MessagePlugin.error(problem); currentSection.value = 'tools'; return; }`)
  // Server fills absent models. Explicit selections are kept and validated there.
  exact(agent, `  if (!formData.value.config.model_id) {
    MessagePlugin.error(t('agent.editor.modelRequired'));
    currentSection.value = 'model';
    return;
  }`, '')
  // Scope verification occurs before the native SSE call, including reconnects.
  const stream = 'src/api/chat/streame.ts'
  exact(stream, "import { ref, onUnmounted } from 'vue';", "import { ref, onUnmounted } from 'vue';\nimport { prepareNativeChat, streamFailure } from '@/mindcreek/native-scope';")
  exact(stream, '      lastStreamRequest.value = {', `      if (!embedToken && params.method === 'POST') await prepareNativeChat(postBody);
      lastStreamRequest.value = {`)
  exact(stream, '          if (!res.ok) throw new Error(`HTTP ${res.status}`);', '          if (!res.ok) throw new Error(await streamFailure(res));')
  // Settings embeds the native roster and invitations plus enterprise operations.
  exact('src/views/settings/TenantMembers.vue', '<div class="tenant-members">', '<div class="tenant-members">\n    <EnterpriseMembers v-if="canManage" @changed="loadMembers" />')
  exact('src/views/settings/TenantMembers.vue', '<script setup lang="ts">', '<script setup lang="ts">\nimport EnterpriseMembers from \'@/mindcreek/EnterpriseMembers.vue\'')
  // Keep only authenticated media routes; public capability URLs are not a fallback.
  exact('src/views/platform/index.vue', '<NewUserGuide />', '<NewUserGuide v-if="authStore.hasRole(\'contributor\')" />')
  // The settings route already renders Settings; mounting the global copy too
  // duplicates dialogs, requests and resumable member operations on a deep link.
  exact('src/views/platform/index.vue', '<Settings />', '<Settings v-if="route.path !== \'/platform/settings\'" />')
  exact('src/views/platform/index.vue', '<GlobalCommandPalette />', '<GlobalCommandPalette v-if="authStore.hasRole(\'contributor\')" />\n        <EmployeeHistorySearch v-else />')
  exact('src/views/platform/index.vue', "import GlobalCommandPalette from '@/components/GlobalCommandPalette.vue'", "import GlobalCommandPalette from '@/components/GlobalCommandPalette.vue'\nimport EmployeeHistorySearch from '@/mindcreek/EmployeeHistorySearch.vue'")
  exact('src/views/platform/index.vue', "import Menu from '@/components/menu.vue'", "import Menu from '@/components/menu.vue'\nimport { useAuthStore } from '@/stores/auth'\nimport { useNativeWorkspaceContext } from '@/mindcreek/native-context'")
  exact('src/views/platform/index.vue', 'const route = useRoute();', 'const authStore = useAuthStore();\nuseNativeWorkspaceContext();\nconst route = useRoute();')
}
