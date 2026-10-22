import test from 'node:test'
import assert from 'node:assert/strict'
import { agentIconID, agentIconURL, agentIconLabel } from './agent-icon-policy'
const id = 'a'.repeat(32), ref = 'mc-avatar:' + id
test('Agent icons use only product references, never untrusted image URLs', () => {
  for (const value of ['https://example.com/picture.png', 'data:image/svg+xml;base64,aaa', 'mc-avatar:../../resource', 'mc-avatar:' + 'A'.repeat(32), 'mc-avatar:' + 'b'.repeat(31)]) {
    assert.equal(agentIconID(value), '')
    assert.equal(agentIconURL(value), '')
    assert.equal(agentIconLabel(value), '')
  }
  assert.equal(agentIconID(ref), id)
  assert.equal(agentIconURL(ref), `/api/v1/mindcreek/agent-icons/${id}`)
  assert.equal(agentIconURL(ref, 'agent?&'), `/api/v1/mindcreek/agent-icons/${id}?agent_id=agent%3F%26`)
  assert.equal(agentIconLabel(ref), '')
  assert.equal(agentIconLabel('🤖'), '🤖')
})
