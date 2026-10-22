import test from 'node:test'
import assert from 'node:assert/strict'
import { agentConfigurationProblem, destinationForRole, workspaceHome, restrictAgentExecution } from './native-policy'

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
test('saving an agent prevents implicit external tool registration outside its reviewed KB tool scope', () => {
  const config = { allowed_tools: ['knowledge_search'], mcp_selection_mode: 'all', mcp_services: ['external'], skills_selection_mode: 'all', selected_skills: ['skill'], sandbox_config_id: 'sandbox', web_search_enabled: true, memory_enabled: true }
  restrictAgentExecution(config)
  assert.equal(config.mcp_selection_mode, 'none')
  assert.equal(config.skills_selection_mode, 'none')
  assert.deepEqual(config.mcp_services, [])
  assert.deepEqual(config.selected_skills, [])
  assert.equal(config.sandbox_config_id, '')
  assert.equal(config.web_search_enabled, false)
  assert.equal(config.memory_enabled, false)
  assert.deepEqual(config.allowed_tools, ['knowledge_search'])
})
test('Explicit read-only Wiki tools survive while write, arbitrary SQL, history and defaults are rejected', () => {
  assert.equal(agentConfigurationProblem({ agent_mode: 'smart-reasoning', allowed_tools: ['wiki_search', 'wiki_read_page'] }), '')
  for (const tool of ['wiki_write_page', 'wiki_replace_text', 'wiki_rename_page', 'wiki_delete_page', 'wiki_update_issue', 'database_query', 'data_analysis']) assert.notEqual(agentConfigurationProblem({ allowed_tools: [tool] }), '')
  assert.notEqual(agentConfigurationProblem({ agent_mode: 'smart-reasoning', allowed_tools: [] }), '')
  assert.notEqual(agentConfigurationProblem({ allowed_tools: ['search_conversations'] }), '')
  assert.notEqual(agentConfigurationProblem({ memory_enabled: true }), '')
  assert.equal(agentConfigurationProblem({ agent_mode: 'quick-answer', allowed_tools: [] }), '')
})
