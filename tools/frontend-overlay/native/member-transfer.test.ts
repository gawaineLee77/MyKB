import test from 'node:test'
import assert from 'node:assert/strict'
import { transferState } from './member-transfer'
const intent = { tenant: 42, from: 'old', to: 'new' }
const member = (user_id: string, role: string, status = 'active') => ({ user_id, role, status }) as any
test('Transfer resumes by observed roles after either step, never guesses ownership', () => {
  assert.equal(transferState(intent, [member('old', 'owner'), member('new', 'viewer')]), 'promote')
  assert.equal(transferState(intent, [member('old', 'owner'), member('new', 'owner')]), 'demote')
  assert.equal(transferState(intent, [member('old', 'admin'), member('new', 'owner')]), 'complete')
  assert.equal(transferState(intent, [member('old', 'admin'), member('new', 'viewer')]), 'conflict')
  assert.equal(transferState(intent, [member('old', 'viewer'), member('new', 'owner')]), 'conflict')
  assert.equal(transferState(intent, [member('old', 'admin', 'suspended'), member('new', 'owner')]), 'conflict')
  assert.equal(transferState(intent, [member('old', 'owner'), member('new', 'viewer', 'suspended')]), 'conflict')
  assert.equal(transferState(intent, [member('old', 'owner')]), 'conflict')
})
