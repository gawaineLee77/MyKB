// Product-owned, exact-anchor adapters. The pinned upstream copy stays untouched.
export function applyEnterpriseSourceFilter({ replaceExact: exact, replaceRegex: regex }) {
  const page = 'src/views/knowledge/KnowledgeBase.vue'
  exact(page, '<script setup lang="ts">', '<script setup lang="ts">\nimport { enterpriseSourceOptions } from \'@/mindcreek/branding-policy\';')
  regex(page, /const sourceOptions = computed\(\(\) => \[[\s\S]*?\n\]\);/g, 'const sourceOptions = computed(() => [...enterpriseSourceOptions]);')
}

export function applyProductBranding({ read, write, replaceExact: exact, replaceRegex: regex }) {
  applyEnterpriseSourceFilter({ replaceExact: exact, replaceRegex: regex })
  exact('src/App.vue', '<script setup lang="ts">', '<script setup lang="ts">\nimport { useBrandingRuntime } from \'@/mindcreek/branding\';\nuseBrandingRuntime();')
  // Update the public shell before the SPA fetch returns. Runtime data wins.
  regex('index.html', /<title>[^<]*<\/title>/g, '<title>小纳</title>')
  regex('index.html', /content="MindCreek 是面向组织内部用户的私有知识库与智能体平台。"/g, 'content="小纳 是面向组织内部用户的私有知识库与智能体平台。"')

  const menu = 'src/components/menu.vue'
  exact(menu, '<script setup lang="ts">', '<script setup lang="ts">\nimport { brandLogo, brandName } from \'@/mindcreek/branding\';')
  exact(menu, '<img class="logo mindcreek-logo-image" src="@/assets/img/mindcreek-mark.png" alt="">', '<img class="logo mindcreek-logo-image" :src="brandLogo" alt="">')
  exact(menu, '<span class="mindcreek-wordmark">MindCreek</span>', '<span class="mindcreek-wordmark" :title="brandName">{{ brandName }}</span>')
  write(menu, read(menu) + '\n<style scoped> .mindcreek-wordmark{min-width:0;max-width:160px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.mindcreek-logo-image{flex-shrink:0;object-fit:contain!important;filter:none!important} </style>\n')

  for (const file of ['src/mindcreek/SSOLogin.vue', 'src/mindcreek/AdminLogin.vue', 'src/views/auth/Login.vue']) {
    exact(file, '<script setup lang="ts">', '<script setup lang="ts">\nimport { brandLogo, brandName, brandText } from \'@/mindcreek/branding\';')
    exact(file, 'src="@/assets/img/mindcreek-mark.png"', ':src="brandLogo"')
  }
  exact('src/mindcreek/SSOLogin.vue', '<p class="eyebrow">MindCreek</p>', '<p class="eyebrow">{{ brandName }}</p>')
  exact('src/mindcreek/SSOLogin.vue', '{{ copy.policy }}', '{{ brandText(copy.policy) }}')
  exact('src/mindcreek/AdminLogin.vue', '<h1>{{ zh ?', '<p class="brand-name">{{ brandName }}</p>\n      <h1>{{ zh ?')
  write('src/mindcreek/AdminLogin.vue', read('src/mindcreek/AdminLogin.vue') + '\n<style scoped>.logo{object-fit:contain}.brand-name{font-weight:600;font-size:18px;color:inherit;overflow-wrap:anywhere}</style>\n')
  exact('src/views/auth/Login.vue', 'aria-label="MindCreek"', ':aria-label="brandName"')
  exact('src/views/auth/Login.vue', '<span class="mindcreek-wordmark">MindCreek</span>', '<span class="mindcreek-wordmark">{{ brandName }}</span>')
  // Welcome copy is rendered as text, so a configured name never becomes HTML
  // or a Vue-i18n message expression.
  for (const key of ['auth.firstTime', 'auth.registerSubtitle']) {
    const file = 'src/views/auth/Login.vue', marker = `$t('${key}')`
    if (read(file).includes(marker)) write(file, read(file).replaceAll(marker, `brandText(${marker})`))
  }
  exact('src/views/creatChat/creatChat.vue', '<script setup lang="ts">', '<script setup lang="ts">\nimport { brandText } from \'@/mindcreek/branding\';')
  exact('src/views/creatChat/creatChat.vue', "$t('createChat.title')", "brandText($t('createChat.title'))")
  exact('src/components/SpotlightGuide.vue', '<script setup lang="ts">', '<script setup lang="ts">\nimport { brandText } from \'@/mindcreek/branding\';')
  exact('src/components/SpotlightGuide.vue', 'const stepTitle = computed(() => t(`${props.stepI18nPrefix}.${step.value.key}.title`))', 'const stepTitle = computed(() => brandText(t(`${props.stepI18nPrefix}.${step.value.key}.title`)))')
  exact('src/components/SpotlightGuide.vue', 'const stepDesc = computed(() => t(`${props.stepI18nPrefix}.${step.value.key}.desc`))', 'const stepDesc = computed(() => brandText(t(`${props.stepI18nPrefix}.${step.value.key}.desc`)))')
  for (const file of ['src/mindcreek/ManagedModelSettings.vue', 'src/mindcreek/AdvancedModelSettings.vue']) {
    exact(file, '<script setup lang="ts">', '<script setup lang="ts">\nimport { brandName } from \'@/mindcreek/branding\';')
    exact(file, '>MindCreek', '>{{ brandName }}')
  }
  const entry = 'src/mindcreek/enterprise-entry.ts'
  write(entry, "import { setBrandingAdmin } from './branding'\n" + read(entry))
  exact(entry, "    if (!response.success || !data?.user || !data.mindcreek) throw new Error('Identity unavailable')", "    if (!response.success || !data?.user || !data.mindcreek) { setBrandingAdmin(false); throw new Error('Identity unavailable') }\n    setBrandingAdmin(data.mindcreek.local_admin === true)")
  exact(entry, '  if (!auth.token && !localStorage.getItem(\'weknora_token\')) {', '  if (!auth.token && !localStorage.getItem(\'weknora_token\')) {\n    setBrandingAdmin(false)')
  const settings = 'src/views/settings/Settings.vue'
  exact(settings, '<script setup lang="ts">', '<script setup lang="ts">\nimport ServiceBranding from \'@/mindcreek/ServiceBranding.vue\';\nimport { canManageServiceBranding } from \'@/mindcreek/branding\';')
  exact(settings, 'const canSeeSection = (key: string): boolean => {', "const canSeeSection = (key: string): boolean => {\n  if (key === 'service-branding') return canManageServiceBranding.value")
  exact(settings, "  const all: NavItem[] = [", "  const all: NavItem[] = [\n    { key: 'service-branding', icon: 'palette', label: '服务外观' },")
  exact(settings, "items: pickItems(['system-global', 'runtime-queues', 'platform-api-keys', 'system-audit-log']),", "items: pickItems(['service-branding', 'system-global', 'runtime-queues', 'platform-api-keys', 'system-audit-log']),")
  exact(settings, '                  <!-- 常规设置 -->', '                  <div v-if="currentSection === \'service-branding\' && canManageServiceBranding" class="section"><ServiceBranding /></div>\n                  <!-- 常规设置 -->')
}
