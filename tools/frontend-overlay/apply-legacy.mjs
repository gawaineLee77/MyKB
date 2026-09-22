#!/usr/bin/env node

import {
  appendFileSync,
  cpSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
} from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const repositoryRoot = resolve(scriptDir, '../..')
const frontendRoot = resolve(process.argv[2] || '')
const brandingRoot = resolve(process.argv[3] || resolve(repositoryRoot, 'branding/mindcreek'))
const productRoot = resolve(scriptDir, 'product/mindcreek')
const marker = 'MindCreek Stage 1 product theme'

if (!process.argv[2]) {
  throw new Error('usage: node tools/frontend-overlay/apply.mjs <frontend-copy> [branding-directory]')
}

const brand = JSON.parse(readFileSync(resolve(brandingRoot, 'brand.json'), 'utf8'))

function pathFor(relativePath) {
  return resolve(frontendRoot, relativePath)
}

function read(relativePath) {
  const target = pathFor(relativePath)
  if (!existsSync(target)) throw new Error(`missing upstream anchor file: ${relativePath}`)
  return readFileSync(target, 'utf8')
}

function write(relativePath, content) {
  writeFileSync(pathFor(relativePath), content)
}

function replaceExact(relativePath, before, after, expectedCount = 1) {
  const source = read(relativePath)
  const actualCount = source.split(before).length - 1
  if (actualCount !== expectedCount) {
    throw new Error(
      `${relativePath}: expected ${expectedCount} occurrence(s) of an upstream anchor, found ${actualCount}`,
    )
  }
  write(relativePath, source.split(before).join(after))
}

function replaceRegex(relativePath, pattern, after, expectedCount = 1) {
  const source = read(relativePath)
  const matches = source.match(pattern) || []
  if (matches.length !== expectedCount) {
    throw new Error(
      `${relativePath}: expected ${expectedCount} regex anchor(s), found ${matches.length}`,
    )
  }
  write(relativePath, source.replace(pattern, after))
}

const themePath = 'src/assets/theme/theme.css'
if (read(themePath).includes(marker)) {
  throw new Error('MindCreek overlay has already been applied to this frontend copy')
}

replaceExact('index.html', '<title>WeKnora</title>', `<title>${brand.name}</title>`)
replaceExact(
  'index.html',
  'content="WeKnora是一款基于大语言模型的文档理解与语义检索框架，专为结构复杂、内容异构的文档场景而打造。"',
  `content="${brand.name} 是面向组织内部用户的私有知识库与智能体平台。"`,
)
replaceExact('index.html', './public/favicon.ico', '/mindcreek-favicon.png', 2)
replaceExact(
  'index.html',
  '</head>',
  `    <script>
    // The corporate OAuth service returns to the registered origin root.
    // Forward only its bounded callback fields to the private gateway route
    // before the SPA or automatic SSO retry can start another transaction.
    (function mindcreekOAuthRootCallback() {
      if (window.location.pathname !== '/') return;
      var source = new URLSearchParams(window.location.search);
      if (!source.has('code') && !source.has('error')) return;
      var target = new URL('/api/v1/mindcreek/oidc/callback', window.location.origin);
      ['code', 'error', 'error_description'].forEach(function (name) {
        var value = source.get(name);
        if (value) target.searchParams.set(name, value);
      });
      window.location.replace(target.toString());
    })();
    </script>
</head>`,
)
replaceExact('embed.html', '<title>WeKnora Embed</title>', `<title>${brand.name} Embed</title>`)
replaceExact('embed.html', './public/favicon.ico', '/mindcreek-favicon.png')
replaceExact(
  'nginx.conf',
  '    # API请求代理到后端服务',
  `    # MindCreek Phase 4 hosted MCP facade. Keep the endpoint on the same
    # public origin as the Web UI while the gateway container stays private.
    location = /mcp {
        proxy_pass \${APP_SCHEME}://\${APP_HOST}:\${APP_PORT}/mcp;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }

    # API请求代理到后端服务`,
)

replaceExact(
  'src/views/auth/Login.vue',
  `    <a href="https://github.com/Tencent/WeKnora" target="_blank" class="header-logo" :title="$t('common.github')">\n      <img src="@/assets/img/weknora.png" alt="WeKnora" class="logo-image" />\n    </a>`,
  `    <a href="/" class="header-logo mindcreek-brand" aria-label="${brand.name}">\n      <img src="@/assets/img/mindcreek-mark.png" alt="" class="logo-image mindcreek-logo-image" />\n      <span class="mindcreek-wordmark">${brand.name}</span>\n    </a>`,
)
replaceRegex(
  'src/views/auth/Login.vue',
  /      <a href="https:\/\/weknora\.weixin\.qq\.com"[\s\S]*?      <\/a>\n\n/g,
  '',
)
replaceExact(
  'src/views/auth/Login.vue',
  'href="https://github.com/Tencent/WeKnora"',
  `href="${brand.repositoryUrl}"`,
)
replaceExact(
  'src/components/menu.vue',
  '                <img class="logo" src="@/assets/img/weknora.png" alt="">',
  `                <img class="logo mindcreek-logo-image" src="@/assets/img/mindcreek-mark.png" alt="">\n                <span class="mindcreek-wordmark">${brand.name}</span>`,
)

replaceExact(
  'src/router/index.ts',
  `        {
          path: "knowledge-bases",
          name: "knowledgeBaseList",
          component: () => import("../views/knowledge/KnowledgeBaseList.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },`,
  `        {
          path: "knowledge-bases",
          name: "knowledgeBaseList",
          component: () => import("@/mindcreek/KnowledgeLibrary.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        // MindCreek Phase 5 product modules. Their source lives outside the upstream tree.
        {
          path: "mindcreek/create",
          name: "mindcreekCreateKnowledgeSpace",
          component: () => import("@/mindcreek/CreateKnowledgeSpace.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "mindcreek/notes/:kbId",
          name: "mindcreekNotesWorkspace",
          component: () => import("@/mindcreek/NotesWorkspace.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "mindcreek/rag/:kbId",
          name: "mindcreekRAGWorkspace",
          component: () => import("@/mindcreek/RAGWorkspace.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "mindcreek/ask",
          name: "mindcreekAskWorkspace",
          component: () => import("@/mindcreek/AskWorkspace.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "mindcreek/settings/models",
          name: "mindcreekAdvancedModelSettings",
          component: () => import("@/mindcreek/AdvancedModelSettings.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },`,
)
replaceExact(
  'src/router/index.ts',
  'component: () => import("../views/auth/Login.vue")',
  'component: () => import("@/mindcreek/AuthEntry.vue")',
  2,
)
replaceExact(
  'src/components/UserMenu.vue',
  `  // 跳转到登录页
  router.push('/login')`,
  `  // Gate B closes both the local and corporate sessions.
  window.location.assign(localStorage.getItem('mindcreek_auth_kind') === 'local-admin' ? '/admin/login' : '/api/v1/mindcreek/oidc/logout')`,
)
replaceExact(
  'src/components/menu.vue',
  `        MessagePlugin.success(t('menu.logoutSuccess'));
        router.push('/login');
        return;`,
  `        MessagePlugin.success(t('menu.logoutSuccess'));
        window.location.assign(localStorage.getItem('mindcreek_auth_kind') === 'local-admin' ? '/admin/login' : '/api/v1/mindcreek/oidc/logout');
        return;`,
)

// R2 adds identity-only pages while leaving the native resource UI untouched.
replaceExact('src/router/index.ts', "import { createRouter, createWebHistory } from 'vue-router'", "import { createRouter, createWebHistory } from 'vue-router'\nimport { enterpriseDestination } from '@/mindcreek/enterprise-entry'")
replaceExact('src/router/index.ts', 'component: () => import("../views/auth/WorkspaceOnboarding.vue")', 'component: () => import("@/mindcreek/EmployeeOnboarding.vue")')
replaceExact('src/router/index.ts', '  // Lite：硬刷新后若落在默认首页，恢复本次会话中最后访问的 /platform 子路径', `  const enterpriseTarget = await enterpriseDestination(to.path)
  if (enterpriseTarget !== undefined) { if (enterpriseTarget === null) next(); else next(enterpriseTarget); return }

  // Lite：硬刷新后若落在默认首页，恢复本次会话中最后访问的 /platform 子路径`)
replaceExact('src/router/index.ts', '      path: "/login",', `      path: "/admin/login",
      name: "localAdminLogin",
      component: () => import("@/mindcreek/AdminLogin.vue"),
      meta: { requiresAuth: false, requiresInit: false, requiresTenant: false }
    },
    {
      path: "/admin/setup",
      name: "enterpriseInstallation",
      component: () => import("@/mindcreek/InstallationStatus.vue"),
      meta: { requiresAuth: true, requiresInit: false, requiresTenant: false }
    },
    {
      path: "/login",`)
replaceExact('src/router/index.ts', "  const authStore = useAuthStore()\n\n  // OIDC", `  const authStore = useAuthStore()
  if (to.path === '/admin/login') { next(); return }
  if (to.path === '/admin/setup') {
    if (authStore.token || localStorage.getItem('weknora_token')) next(); else next('/admin/login')
    return
  }

  // OIDC`)
replaceExact('src/api/auth/index.ts', "import { post, get, put } from '@/utils/request'", "import { post, get, put } from '@/utils/request'\nimport { authEndpoint } from '@/mindcreek/enterprise-auth'")
for (const action of ['refresh', 'logout', 'change-password']) {
  replaceExact('src/api/auth/index.ts', `post('/api/v1/auth/${action}',`, `post(authEndpoint('${action}'),`)
}
replaceExact('src/utils/request.ts', "const PUBLIC_AUTH_PATHS = [", "const PUBLIC_AUTH_PATHS = ['/mindcreek/admin/auth/login', '/mindcreek/admin/auth/refresh', ")
replaceExact('src/utils/request.ts', "  if (window.location.pathname === '/login') return;", "  if (window.location.pathname === '/login' || window.location.pathname === '/admin/login') return;")
replaceExact('src/utils/request.ts', "  window.location.href = '/login';", "  window.location.href = localStorage.getItem('mindcreek_auth_kind') === 'local-admin' ? '/admin/login' : '/login';")
replaceExact('src/App.vue', '  if (response.refresh_token) {', "  localStorage.setItem('mindcreek_auth_kind', 'corporate')\n  if (response.refresh_token) {")

if (!read('src/utils/request.ts').includes('export function patch<T = any>')) {
  replaceExact(
    'src/utils/request.ts',
    `export function put<T = any>(url: string, data = {}, config?: any): Promise<T> {
  return instance.put<T>(url, data, config) as unknown as Promise<T>;
}

export function del<T = any>(url: string, data?: any): Promise<T> {`,
    `export function put<T = any>(url: string, data = {}, config?: any): Promise<T> {
  return instance.put<T>(url, data, config) as unknown as Promise<T>;
}

export function patch<T = any>(url: string, data = {}, config?: any): Promise<T> {
  return instance.patch<T>(url, data, config) as unknown as Promise<T>;
}

export function del<T = any>(url: string, data?: any): Promise<T> {`,
  )
}

replaceExact(
  'src/views/settings/ModelSettings.vue',
  `    </div>

    <t-tabs v-model="activeTypeFilter" class="model-type-tabs" data-guide="settings-models">`,
  `    </div>

    <ManagedModelSettings />

    <t-tabs v-model="activeTypeFilter" class="model-type-tabs" data-guide="settings-models">`,
)

const agentEditorPath = 'src/views/agent/AgentEditorModal.vue'
const sandboxDependency = read(agentEditorPath).includes('chatResources.ensureSandboxConfigs()')
  ? '      chatResources.ensureSandboxConfigs(),\n'
  : ''
replaceExact(
  agentEditorPath,
  `    await Promise.all([
      chatResources.ensureModels(),
      chatResources.ensureKnowledgeBases(),
      chatResources.ensureWebSearchProviders(),
${sandboxDependency}      editorResources.prefetchAgentEditorDeps(),
    ]);`,
  `    // Knowledge-base selection is essential; optional editor dependencies
    // must not blank the list when one auxiliary endpoint is unavailable.
    const dependencyResults = await Promise.allSettled([
      chatResources.ensureModels(),
      // A knowledge space may have been created after the shell cache was populated.
      // Agent scope selection must always show the current authorized list.
      chatResources.ensureKnowledgeBases(true),
      // MindCreek excludes upstream Web Search, MCP services, and skill sandboxes.
      editorResources.prefetchAgentEditorDeps(),
    ]);
    dependencyResults.forEach((result, index) => {
      if (result.status === 'rejected') {
        console.warn('[AgentEditor] dependency unavailable', { index, error: result.reason });
      }
    });`,
)

replaceExact(
  agentEditorPath,
  `  items.push({ key: 'websearch', icon: 'internet', label: t('agent.editor.webSearchConfig') });
  items.push({ key: 'multimodal', icon: 'attach', label: t('agentEditor.imageUpload.navLabel') });
  // Agent 模式能力
  if (isAgentMode.value) {
    items.push({ key: 'tools', icon: 'tools', label: t('agent.editor.toolsConfig') });
    items.push({ key: 'mcp', icon: 'server', label: t('agentEditor.mcp.label') });
    items.push({ key: 'skills', icon: SKILL_ICON, label: t('agent.editor.skillsConfig') });
  }`,
  `  // Retain upstream declarations for upgrade-contract tests, but keep the
  // excluded capabilities unreachable in the MindCreek product navigation.
  const mindCreekExternalAgentToolsEnabled = false;
  if (mindCreekExternalAgentToolsEnabled) {
    items.push({ key: 'websearch', icon: 'internet', label: t('agent.editor.webSearchConfig') });
  }
  items.push({ key: 'multimodal', icon: 'attach', label: t('agentEditor.imageUpload.navLabel') });
  if (isAgentMode.value) {
    items.push({ key: 'tools', icon: 'tools', label: t('agent.editor.toolsConfig') });
    if (mindCreekExternalAgentToolsEnabled) {
      items.push({ key: 'mcp', icon: 'server', label: t('agentEditor.mcp.label') });
      items.push({ key: 'skills', icon: SKILL_ICON, label: t('agent.editor.skillsConfig') });
    }
  }`,
)

replaceExact(
  'src/stores/editorResources.ts',
  `  /** 智能体编辑器打开时预取的依赖（不含 IM channels / 单 KB shares） */
  async function prefetchAgentEditorDeps(force = false): Promise<void> {
    await Promise.all([
      ensureMcpServices(force),
      ensureAgentTypePresets(force),`,
  `  /** MindCreek excludes upstream MCP-service and skill-sandbox configuration. */
  async function prefetchAgentEditorDeps(force = false): Promise<void> {
    await Promise.all([
      ensureAgentTypePresets(force),`,
)

replaceExact(
  'src/views/settings/Settings.vue',
  `const SYSTEM_ADMIN_SECTIONS = SYSTEM_ADMIN_SETTINGS_SECTIONS

const normalizeSettingsSection = (section: string) => {`,
  `const SYSTEM_ADMIN_SECTIONS = SYSTEM_ADMIN_SETTINGS_SECTIONS

// Presentation follows the authoritative Gateway deny policy. These modules
// stay in the upstream source tree but are not part of the MindCreek product.
const MINDCREEK_HIDDEN_SETTINGS_SECTIONS = new Set([
  'weknoracloud',
  'websearch',
  'memory',
  'mymemory',
  'envvars',
  'sandbox',
  'skills',
  'mcp',
  'integration-im',
  'integration-embed',
  'integration-chrome',
  'integration-claw',
])

const normalizeSettingsSection = (section: string) => {`,
)
replaceExact(
  'src/views/settings/Settings.vue',
  `const isSectionSupported = (key: string): boolean => {
  if (isIntegrationSection(key)) {`,
  `const isSectionSupported = (key: string): boolean => {
  if (MINDCREEK_HIDDEN_SETTINGS_SECTIONS.has(key)) return false
  if (isIntegrationSection(key)) {`,
)

replaceExact(
  'src/components/UserMenu.vue',
  `const canManageSkills = computed(() =>
  authStore.canAccessAllTenants ||
  authStore.isSystemAdmin ||
  authStore.hasRole(SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE.skills),
)`,
  `// Skill catalog and sandbox execution are outside the MindCreek release.
const canManageSkills = computed(() => false)`,
)
replaceExact(
  'src/views/settings/ModelSettings.vue',
  `import ModelDebugDrawer from '@/components/ModelDebugDrawer.vue'`,
  `import ModelDebugDrawer from '@/components/ModelDebugDrawer.vue'
import ManagedModelSettings from '@/mindcreek/ManagedModelSettings.vue'`,
)
replaceExact(
  'src/views/settings/ModelSettings.vue',
  `    const models = await listModels()
    allModels.value = models`,
  `    const models = await listModels()
    const managedIDs = new Set(['builtin-mindcreek-chat', 'builtin-mindcreek-embedding', 'builtin-mindcreek-rerank', 'builtin-mindcreek-vlm'])
    allModels.value = models.filter(model => !model.id || !managedIDs.has(model.id))`,
)

const inputFieldPath = 'src/components/Input-field.vue'
const mentionImport = read(inputFieldPath).match(/^import .* from '@\/types\/mention';$/m)?.[0]
if (!mentionImport) {
  throw new Error(`${inputFieldPath}: missing mention type import anchor`)
}
replaceExact(
  inputFieldPath,
  mentionImport,
  `${mentionImport}
import { chooseChatModel } from '@/mindcreek/model-selection';`,
)
replaceExact(
  'src/components/Input-field.vue',
  `const selectedModelId = computed({
  get: () => settingsStore.conversationModels.selectedChatModelId || '',
  set: (val: string) => settingsStore.updateConversationModels({ selectedChatModelId: val })
});`,
  `const selectedModelId = computed({
  get: () => settingsStore.conversationModels.selectedChatModelId || '',
  set: (val: string) => settingsStore.updateConversationModels({
    summaryModelId: val,
    selectedChatModelId: val,
  })
});`,
)
replaceExact(
  'src/components/Input-field.vue',
  `const ensureModelSelection = () => {
  if (selectedModelId.value) {
    return;
  }
  const lastPick = readLastChatModelID();
  if (lastPick) {
    selectedModelId.value = lastPick;
    return;
  }
  if (availableModels.value.length > 0) {
    selectedModelId.value = availableModels.value[0].id || '';
  }
};`,
  `const ensureModelSelection = () => {
  // Do not clear a stored choice before the async model list arrives. Once it
  // does, repair stale selections and prefer MindCreek's managed default.
  if (availableModels.value.length === 0) return;
  const choice = chooseChatModel(
    availableModels.value,
    readLastChatModelID(),
    settingsStore.conversationModels.selectedChatModelId,
  );
  if (!choice) return;
  if (
    settingsStore.conversationModels.selectedChatModelId !== choice
    || settingsStore.conversationModels.summaryModelId !== choice
  ) {
    settingsStore.updateConversationModels({
      summaryModelId: choice,
      selectedChatModelId: choice,
    });
  }
};`,
)
replaceExact(
  'src/components/Input-field.vue',
  `  if (settingsStore.selectedAgentSourceTenantId) return
  const currentId = settingsStore.selectedAgentId || BUILTIN_QUICK_ANSWER_ID
  if (!disabledOwnAgentIds.value.includes(currentId)) return

  const isEnabled = (id: string) =>
    agents.value.some(a => a.id === id) && !disabledOwnAgentIds.value.includes(id)

  let fallback: CustomAgent | undefined
  if (isEnabled(BUILTIN_SMART_REASONING_ID)) {`,
  `  if (settingsStore.selectedAgentSourceTenantId) return
  const currentId = settingsStore.selectedAgentId || BUILTIN_QUICK_ANSWER_ID
  const currentExists = agents.value.some(a => a.id === currentId)
  if (currentExists && !disabledOwnAgentIds.value.includes(currentId)) return

  const isEnabled = (id: string) =>
    agents.value.some(a => a.id === id) && !disabledOwnAgentIds.value.includes(id)

  let fallback: CustomAgent | undefined
  if (!currentExists && isEnabled(BUILTIN_QUICK_ANSWER_ID)) {
    // The UI used to render a quick-answer fallback while retaining a deleted
    // agent ID in the request, which made otherwise valid KB chats return 404.
    fallback = agents.value.find(a => a.id === BUILTIN_QUICK_ANSWER_ID)
  } else if (isEnabled(BUILTIN_SMART_REASONING_ID)) {`,
)
replaceExact(
  'src/components/Input-field.vue',
  `  if (kbId && !selectedKbIds.value.includes(kbId)) {
    settingsStore.addKnowledgeBase(kbId);
  }`,
  `  if (kbId) {
    // A KB-specific route is an exact scope, not an addition to stale global state.
    settingsStore.selectKnowledgeBases([kbId]);
  }`,
)
replaceExact(
  'src/components/Input-field.vue',
  `  if (newKbId && typeof newKbId === 'string' && !selectedKbIds.value.includes(newKbId)) {
    settingsStore.addKnowledgeBase(newKbId);
  }`,
  `  if (newKbId && typeof newKbId === 'string') {
    settingsStore.selectKnowledgeBases([newKbId]);
  }`,
)
const translations = {
  'src/i18n/locales/en-US.ts': [
    ['Welcome to WeKnora', `Welcome to ${brand.name}`],
    [
      'Everything starts here: upload documents, web pages or FAQs and WeKnora parses and indexes them automatically. Click here to open knowledge bases.',
      `Everything starts here: add documents, web pages or FAQs and ${brand.name} prepares them for reliable retrieval. Click here to open knowledge bases.`,
    ],
    ['New to WeKnora?', `New to ${brand.name}?`],
    [
      'RAG Q&A, ReAct Agent and Wiki — an LLM-powered enterprise knowledge framework',
      'Private notes, reliable RAG and governed knowledge sharing for your organization',
    ],
    ['Create your account and start using WeKnora', `Create your account and start using ${brand.name}`],
    ['Hi, I am WeKnora — your knowledge, within reach', `Hi, I am ${brand.name} — your knowledge, within reach`],
    [
      'LLM-Powered Enterprise Knowledge Framework',
      'Private knowledge that flows where work happens',
    ],
    [
      'RAG retrieval, agentic reasoning and Wiki knowledge bases — so your documents are truly understood and put to work',
      'Personal notes, reliable RAG and governed sharing in one self-hosted workspace',
    ],
  ],
  'src/i18n/locales/zh-CN.ts': [
    ['欢迎使用 WeKnora', `欢迎使用 ${brand.name}`],
    [
      '知识库是一切的起点：上传文档、网页或 FAQ，WeKnora 会自动解析并建立索引。点击这里进入知识库。',
      `知识库是一切的起点：添加文档、网页或 FAQ，${brand.name} 会为可靠检索完成处理。点击这里进入知识库。`,
    ],
    ['首次使用 WeKnora？', `首次使用 ${brand.name}？`],
    [
      'RAG 问答、ReAct 智能体与 Wiki 知识库，大模型驱动的企业级知识框架',
      '面向组织的私人笔记、可靠 RAG 与受控知识共享',
    ],
    ['创建账户并开始使用 WeKnora', `创建账户并开始使用 ${brand.name}`],
    ['Hi，我是 WeKnora，让你的知识触手可及', `Hi，我是 ${brand.name}，让你的知识触手可及`],
    ['大模型驱动的企业级知识框架', '让组织知识持续汇聚、可靠流动'],
    [
      'RAG 检索、智能体推理、Wiki 知识库，让文档真正被理解和运用',
      '在一个私有化工作空间中统一管理个人笔记、可靠 RAG 与受控共享',
    ],
  ],
}

for (const [relativePath, replacements] of Object.entries(translations)) {
  for (const [before, after] of replacements) replaceExact(relativePath, before, after)
}

mkdirSync(pathFor('src/assets/img'), { recursive: true })
mkdirSync(pathFor('public'), { recursive: true })
mkdirSync(pathFor('src/mindcreek'), { recursive: true })
cpSync(productRoot, pathFor('src/mindcreek'), { recursive: true })
copyFileSync(resolve(brandingRoot, 'assets/mindcreek-mark-ui.png'), pathFor('src/assets/img/mindcreek-mark.png'))
copyFileSync(resolve(brandingRoot, 'assets/mindcreek-favicon.png'), pathFor('public/mindcreek-favicon.png'))
appendFileSync(pathFor(themePath), `\n\n${readFileSync(resolve(brandingRoot, 'theme.css'), 'utf8')}\n`)

console.log(`Applied ${brand.name} branding and Phase 5 product modules to ${frontendRoot}`)
