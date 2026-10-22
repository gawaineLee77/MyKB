import { readFileSync, existsSync } from 'node:fs'
import assert from 'node:assert/strict'
import { applyGraphViewUI } from './apply-native.mjs'
import { applyKnowledgeBaseACL } from './apply-kb-acl.mjs'
import { applyEnterpriseSourceFilter } from './apply-branding.mjs'
import { applyScopedImageAccess } from './apply-image-access.mjs'
import { applyWordExport } from './apply-word-export.mjs'
import { applyChatLinks } from './apply-chat-links.mjs'
const [target, upstream] = process.argv.slice(2)
const read = path => readFileSync(`${target}/${path}`, 'utf8')
for (const path of ['src/api/tenant/members.ts', 'src/api/tenant/invitations.ts']) {
  assert.equal(read(path), readFileSync(`${upstream}/${path}`, 'utf8'), `Native module changed unexpectedly: ${path}`)
}
// All knowledge UI changes are reproduced by explicit product adapters.
const expected = new Map()
const adapter = {
  read(path) { if (!expected.has(path)) expected.set(path, readFileSync(`${upstream}/${path}`, 'utf8')); return expected.get(path) },
  write(path, value) { expected.set(path, value) },
  replaceExact(path, old, next, count = 1) { const source = this.read(path); assert.equal(source.split(old).length - 1, count, path); this.write(path, source.split(old).join(next)) },
  replaceRegex(path, pattern, next, count = 1) { const source = this.read(path); assert.equal((source.match(pattern) || []).length, count, path); this.write(path, source.replace(pattern, next)) },
}
const bound = Object.fromEntries(Object.entries(adapter).map(([name, fn]) => [name, fn.bind(adapter)]))
applyGraphViewUI(bound)
applyKnowledgeBaseACL(bound)
applyEnterpriseSourceFilter(bound)
applyScopedImageAccess(bound)
applyWordExport(bound)
applyChatLinks(bound)
// The employee assistant adapts the same protected image helper afterward.
const securityPath = 'src/utils/security.ts'
adapter.write(securityPath, "import { assistantActive, assistantNativeURL, assistantHeaders } from '../mindcreek/assistant-context'\n" + adapter.read(securityPath))
adapter.replaceExact(securityPath, '    const { url: requestURL, headers } = request;', '    const requestURL = assistantActive ? assistantNativeURL(request.url) : request.url;\n    const headers = assistantActive ? assistantHeaders() : request.headers;')
adapter.replaceExact(securityPath, "            credentials: 'include',", "            credentials: assistantActive ? 'omit' : 'include',")
// request.ts also carries the identity, native-problem and assistant adapters.
expected.delete('src/utils/request.ts')
assert.match(read('src/utils/request.ts'), /data.error.code.startsWith\('kb_acl\.'\)/)
for (const [path, source] of expected) assert.equal(read(path), source, `Knowledge UI changed outside reviewed adapters: ${path}`)
for (const name of ['KnowledgeLibrary.vue', 'CatalogView.vue', 'PublicationDialog.vue', 'NotesWorkspace.vue', 'RAGWorkspace.vue', 'SharingDialog.vue', 'AskWorkspace.vue', 'api.ts', 'contracts.ts']) assert.equal(existsSync(`${target}/src/mindcreek/${name}`), false, `Retired module copied: ${name}`)
assert.match(read('src/router/index.ts'), /nativeDestination/)
assert.match(read('src/router/index.ts'), /\.\.\/views\/knowledge\/KnowledgeBaseList.vue/)
assert.match(read('src/api/chat/streame.ts'), /prepareNativeChat/)
assert.match(read('src/api/chat-history.ts'), /mode: 'keyword'/)
assert.match(read('src/stores/menu.ts'), /!authStore.hasRole\('contributor'\)/)
assert.match(read('src/views/settings/TenantMembers.vue'), /EnterpriseMembers/)
assert.match(read('src/views/settings/TenantMembers.vue'), /canManageMember\(currentRole, row.role\)/)
assert.match(read('src/config/integrations.ts'), /api: 'admin'/)
assert.match(read('src/views/settings/TenantInfo.vue'), /canEditTenant = computed\(\(\) => authStore.hasRole\('admin'\)\)/)
assert.match(read('src/views/integrations/ApiIntegrationSettings.vue'), /v-if="isWorkspaceOwner" class="settings-band principal-section"/)
assert.match(read('src/views/integrations/ApiIntegrationSettings.vue'), /if \(!isWorkspaceOwner.value\) return/)
assert.match(read('src/views/integrations/ApiIntegrationSettings.vue'), /editingRestricted.value/)
assert.match(read('src/mindcreek/EnterpriseMembers.vue'), /auth.currentTenantRole === 'owner'/)
assert.doesNotMatch(read('src/mindcreek/WorkspaceAdminDelegation.vue'), /refresh_token|refreshToken/)

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
