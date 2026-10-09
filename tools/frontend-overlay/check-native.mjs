import { readFileSync, existsSync } from 'node:fs'
import assert from 'node:assert/strict'
import { applyGraphViewUI } from './apply-native.mjs'
const [target, upstream] = process.argv.slice(2)
const read = path => readFileSync(`${target}/${path}`, 'utf8')
for (const path of ['src/views/knowledge/KnowledgeBaseList.vue', 'src/views/knowledge/KnowledgeBaseEditorModal.vue', 'src/api/knowledge-base/index.ts', 'src/api/tenant/members.ts', 'src/api/tenant/invitations.ts']) {
  assert.equal(read(path), readFileSync(`${upstream}/${path}`, 'utf8'), `Native module changed unexpectedly: ${path}`)
}
// The detail page permits only the reviewed graph tab adapter; all other bytes
// must still match the pinned native page, including its lifecycle/permissions.
const graphPath = 'src/views/knowledge/KnowledgeBase.vue'
let graphExpected = readFileSync(`${upstream}/${graphPath}`, 'utf8')
applyGraphViewUI({
  read(path) { assert.equal(path, graphPath); return graphExpected },
  write(path, value) { assert.equal(path, graphPath); graphExpected = value },
  replaceExact(path, old, next) {
    assert.equal(path, graphPath); assert.equal(graphExpected.split(old).length, 2)
    graphExpected = graphExpected.replace(old, next)
  },
})
assert.equal(read(graphPath), graphExpected, 'Native detail page changed outside the graph tab adapter')
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
for (const path of ['src/views/chat/components/botmsg.vue', 'src/views/chat/components/AgentStreamDisplay.vue']) {
  assert.equal((read(path).match(/<WordExportButton /g) || []).length, 1, 'Single-answer Word export entry')
  assert.match(read(path), /:references="session.knowledge_references"/)
}
assert.equal(JSON.parse(read('package.json')).dependencies.jszip, '3.10.1')
assert.equal(JSON.parse(read('package-lock.json')).packages[''].dependencies.jszip, '3.10.1')
console.log('R4 native UI boundary, retirement, identity, models, scope and roles verified')
