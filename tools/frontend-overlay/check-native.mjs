import { readFileSync, existsSync } from 'node:fs'
import assert from 'node:assert/strict'
const [target, upstream] = process.argv.slice(2)
const read = path => readFileSync(`${target}/${path}`, 'utf8')
for (const path of ['src/views/knowledge/KnowledgeBaseList.vue', 'src/views/knowledge/KnowledgeBase.vue', 'src/views/knowledge/KnowledgeBaseEditorModal.vue', 'src/api/knowledge-base/index.ts', 'src/api/tenant/members.ts', 'src/api/tenant/invitations.ts']) {
  assert.equal(read(path), readFileSync(`${upstream}/${path}`, 'utf8'), `Native module changed unexpectedly: ${path}`)
}
for (const name of ['KnowledgeLibrary.vue', 'CatalogView.vue', 'PublicationDialog.vue', 'NotesWorkspace.vue', 'RAGWorkspace.vue', 'SharingDialog.vue', 'AskWorkspace.vue', 'api.ts', 'contracts.ts']) assert.equal(existsSync(`${target}/src/mindcreek/${name}`), false, `Retired module copied: ${name}`)
assert.match(read('src/router/index.ts'), /nativeDestination/)
assert.match(read('src/router/index.ts'), /\.\.\/views\/knowledge\/KnowledgeBaseList.vue/)
assert.match(read('src/api/chat/streame.ts'), /prepareNativeChat/)
assert.match(read('src/api/chat-history.ts'), /mode: 'keyword'/)
assert.match(read('src/stores/menu.ts'), /!authStore.hasRole\('contributor'\)/)
assert.match(read('src/views/settings/TenantMembers.vue'), /EnterpriseMembers/)
assert.match(read('src/views/agent/AgentEditorModal.vue'), /SUPPORTED_AGENT_TOOLS/)
assert.match(read('src/views/agent/AgentList.vue'), /await refreshModelReadiness\(\)/)
assert.match(read('src/mindcreek/enterprise-entry.ts'), /workspaceHome/)
assert.match(read('src/router/index.ts'), /\/admin\/login/)
assert.match(read('index.html'), /mindcreekOAuthRootCallback/)
assert.match(read('src/main.ts'), /await loadEnterpriseLinks\(\)/)
assert.equal(JSON.parse(read('public/mindcreek-links.json')).enabled, false)
for (const file of ['components/UserMenu.vue', 'views/settings/TenantMembers.vue', 'views/settings/ModelSettings.vue',
  'views/integrations/ApiIntegrationSettings.vue', 'views/knowledge/settings/GraphSettings.vue',
  'views/settings/SystemInfo.vue', 'views/integrations/IntegrationSettingsSection.vue',
  'views/settings/SandboxSettings.vue', 'components/SandboxConfigEditorDrawer.vue', 'views/auth/Login.vue']) {
  assert.match(read('src/' + file), /enterpriseLinks/, file)
  assert.doesNotMatch(read('src/' + file), /https:\/\/github.com\/(Tencent\/WeKnora|gawaineLee77\/MyKB)/, file)
}
assert.match(read('nginx.conf'), /location = \/mindcreek-links.json/)
console.log('R4 native UI boundary, retirement, identity, models, scope and roles verified')
