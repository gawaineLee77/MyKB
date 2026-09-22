import test from 'node:test'
import assert from 'node:assert/strict'
import { agentConfigurationProblem, destinationForRole, workspaceHome } from './native-policy'

test('Viewer direct management links redirect, conversation and account links remain', () => {
  for (const path of ['/knowledgeBase', '/platform/knowledge-bases/kb', '/platform/agents', '/platform/organizations']) assert.equal(destinationForRole('viewer', path), '/platform/creatChat')
  assert.equal(destinationForRole('viewer', '/platform/settings', 'members'), '/platform/creatChat')
  for (const path of ['/platform/creatChat', '/platform/chat/one', '/platform/retired']) assert.equal(destinationForRole('viewer', path), undefined)
  assert.equal(destinationForRole('viewer', '/platform/settings', 'userprofile'), undefined)
})
test('Contributor preserves native management navigation without assuming write permission', () => {
  for (const role of ['contributor', 'admin', 'owner']) {
    assert.equal(workspaceHome(role), '/platform/knowledge-bases')
    assert.equal(destinationForRole(role, '/platform/knowledge-bases/another-creators-kb'), undefined)
  }
})
test('Explicit native Wiki tools survive while unavailable history, memory and defaults are rejected', () => {
  assert.equal(agentConfigurationProblem({ agent_mode: 'smart-reasoning', allowed_tools: ['wiki_search', 'wiki_write_page'] }), '')
  assert.notEqual(agentConfigurationProblem({ agent_mode: 'smart-reasoning', allowed_tools: [] }), '')
  assert.notEqual(agentConfigurationProblem({ allowed_tools: ['search_conversations'] }), '')
  assert.notEqual(agentConfigurationProblem({ memory_enabled: true }), '')
  assert.equal(agentConfigurationProblem({ agent_mode: 'quick-answer', allowed_tools: [] }), '')
})
