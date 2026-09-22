// Documentation navigation is deployment configuration, independent of RBAC.
// Apply to the disposable product frontend copy, never to upstream sources.
export function applyEnterpriseLinks({ read, write, replaceExact: exact, replaceRegex: regex }) {
  const inject = (file, opener = false) => exact(file, '<script setup lang="ts">',
    `<script setup lang="ts">\nimport { enterpriseLinks${opener ? ', openEnterpriseLink' : ''} } from '@/mindcreek/enterprise-links'`)
  const main = 'src/main.ts'
  write(main, `import { loadEnterpriseLinks } from '@/mindcreek/enterprise-links';\n` + read(main))
  exact(main, 'async function bootstrap() {', 'async function bootstrap() {\n  await loadEnterpriseLinks();')
  exact('nginx.conf', '    # 前端静态文件', `    # Public non-secret link policy: no SPA fallback or stale browser cache.
    location = /mindcreek-links.json {
        root /usr/share/nginx/html;
        default_type application/json;
        add_header Cache-Control "no-store" always;
        add_header X-Content-Type-Options "nosniff" always;
        try_files /mindcreek-runtime/links.json /mindcreek-links.json =404;
    }

    # 前端静态文件`)

  const menu = 'src/components/UserMenu.vue'
  inject(menu, true)
  exact(menu, '<div class="menu-item" @click="openDocs">', '<div v-if="enterpriseLinks.help" class="menu-item" @click="openDocs">')
  exact(menu, '<div class="menu-divider"></div>\n        <div v-if="enterpriseLinks.help"', '<div v-if="enterpriseLinks.help || enterpriseLinks.project" class="menu-divider"></div>\n        <div v-if="enterpriseLinks.help"')
  exact(menu, '<div class="menu-item" :title="$t(\'common.githubStarTip\')" @click="openGithub">', '<div v-if="enterpriseLinks.project" class="menu-item" @click="openGithub">')
  exact(menu, '<t-icon name="logo-github" class="menu-icon" />', '<t-icon name="link" class="menu-icon" />')
  exact(menu, "{{ $t('common.github') }}", "{{ $t('general.projectHome') }}")
  exact(menu, '            <t-icon name="star-filled" class="menu-github-star-icon" size="16px" aria-hidden="true" />\n', '')
  exact(menu, "window.open('https://github.com/Tencent/WeKnora/tree/main/docs', '_blank')", "openEnterpriseLink('help')")
  exact(menu, "window.open('https://github.com/Tencent/WeKnora', '_blank')", "openEnterpriseLink('project')")

  for (const [file, key, url] of [
    ['src/views/settings/TenantMembers.vue', 'rbac', 'https://github.com/Tencent/WeKnora/blob/main/docs/RBAC%E8%AF%B4%E6%98%8E.md'],
    ['src/views/settings/ModelSettings.vue', 'builtinModels', 'https://github.com/Tencent/WeKnora/blob/main/docs/BUILTIN_MODELS.md'],
    ['src/views/integrations/IntegrationSettingsSection.vue', 'integration', 'https://github.com/Tencent/WeKnora/blob/main/docs/IM%E9%9B%86%E6%88%90%E5%BC%80%E5%8F%91%E6%96%87%E6%A1%A3.md'],
  ]) {
    inject(file)
    exact(file, `href="${url}"`, `v-if="enterpriseLinks.${key}" :href="enterpriseLinks.${key}"`)
  }
  const api = 'src/views/integrations/ApiIntegrationSettings.vue'
  inject(api, true)
  exact(api, '<div class="row row--doc">', '<div v-if="enterpriseLinks.api" class="row row--doc">')
  exact(api, '<a class="doc-link" @click="openApiDoc">', '<a v-if="enterpriseLinks.api" class="doc-link" :href="enterpriseLinks.api" target="_blank" rel="noopener noreferrer" @click.prevent="openApiDoc">')
  exact(api, "window.open('https://github.com/Tencent/WeKnora/blob/main/docs/api/README.md', '_blank')", "openEnterpriseLink('api')")

  const graph = 'src/views/knowledge/settings/GraphSettings.vue'
  inject(graph, true)
  exact(graph, '<t-link class="graph-guide-link"', '<t-link v-if="enterpriseLinks.knowledgeGraph" class="graph-guide-link"')
  exact(graph, `const graphGuideUrl =
  import.meta.env.VITE_KG_GUIDE_URL ||
  'https://github.com/Tencent/WeKnora/blob/main/docs/KnowledgeGraph.md'\n\n`, '')
  exact(graph, "window.open(graphGuideUrl, '_blank', 'noopener')", "openEnterpriseLink('knowledgeGraph')")

  const info = 'src/views/settings/SystemInfo.vue'
  inject(info)
  exact(info, '<div class="migration-error-actions">', '<div v-if="enterpriseLinks.migration || enterpriseLinks.feedback" class="migration-error-actions">')
  exact(info, ':href="troubleshootingDocsURL"', 'v-if="enterpriseLinks.migration" :href="enterpriseLinks.migration"')
  exact(info, '<span class="migration-error-actions-sep">', '<span v-if="enterpriseLinks.migration && enterpriseLinks.feedback" class="migration-error-actions-sep">')
  exact(info, ':href="reportIssueURL"', 'v-if="enterpriseLinks.feedback" :href="enterpriseLinks.feedback"')
  // Internal feedback URLs receive no automatic error/environment query data.
  regex(info, /const troubleshootingDocsURL =[\s\S]*?\n\}\)\n\n\/\/ Methods/g, '// Methods')

  for (const [file, name, count] of [
    ['src/views/settings/SandboxSettings.vue', 'sandboxGuideUrl', 1],
    ['src/components/SandboxConfigEditorDrawer.vue', 'clusterGuideUrl', 2],
  ]) {
    inject(file)
    exact(file, `:href="${name}"`, 'v-if="enterpriseLinks.sandbox" :href="enterpriseLinks.sandbox"', count)
    exact(file, `const ${name} = 'https://github.com/Tencent/WeKnora/blob/main/docs/sandbox-cluster.md'\n`, '')
  }
  const login = 'src/views/auth/Login.vue'
  inject(login)
  regex(login, /<a href="[^"]+" target="_blank" class="header-link"/g,
    '<a v-if="enterpriseLinks.project" :href="enterpriseLinks.project" target="_blank" rel="noopener noreferrer" class="header-link"')

  for (const [locale, label] of [['zh-CN', '项目主页'], ['en-US', 'Project home'], ['ko-KR', '프로젝트 홈'], ['ru-RU', 'Главная страница проекта']]) {
    exact(`src/i18n/locales/${locale}.ts`, '  general: {', `  general: {\n    projectHome: '${label}',`)
  }
  for (const [locale, text] of [
    ['zh-CN', '启动时数据库迁移未成功完成，部分表或索引可能未创建，会导致 Wiki、知识图谱等功能异常。请联系部署管理员处理。'],
    ['en-US', 'The startup database migration did not complete successfully. Missing tables or indexes may affect Wiki, the knowledge graph and other features. Contact your deployment administrator.'],
    ['ko-KR', '시작 시 데이터베이스 마이그레이션이 완료되지 않았습니다. 누락된 테이블이나 인덱스로 인해 Wiki 및 지식 그래프 기능에 문제가 발생할 수 있습니다. 배포 관리자에게 문의하세요.'],
    ['ru-RU', 'Миграция базы данных при запуске не завершена. Отсутствие таблиц или индексов может повлиять на Wiki и граф знаний. Обратитесь к администратору развёртывания.'],
  ]) regex(`src/i18n/locales/${locale}.ts`, /^    dbMigrationFailedDesc: '[^\n]*',/gm, `    dbMigrationFailedDesc: '${text}',`)
}
