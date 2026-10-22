import assert from 'node:assert/strict'
import test from 'node:test'
import { assignableWorkspaceRoles, canManageMember, canManageWorkspace, allowedKeyCapabilities, keyScopeProblem, keyCreatorLabel } from './workspace-admin-policy'

test('Admin manages ordinary members but cannot change Owner or assign Owner', () => {
  for (const role of ['owner', 'admin', 'contributor', 'viewer', '', null]) {
    assert.equal(canManageWorkspace(role), role === 'owner' || role === 'admin')
    for (const target of ['owner', 'admin', 'contributor', 'viewer']) assert.equal(canManageMember(role, target), role === 'owner' || (role === 'admin' && target !== 'owner'))
  }
  assert.equal(canManageMember('admin', 'unknown'), false)
  assert.deepEqual(assignableWorkspaceRoles('admin'), ['admin', 'contributor', 'viewer'])
  assert.ok(assignableWorkspaceRoles('owner').includes('owner'))
  assert.deepEqual(assignableWorkspaceRoles('viewer'), [])
})
test('Admin key capabilities cannot grant member/config/history powers and require explicit knowledge scope', () => {
  for (const capability of ['manage_members', 'manage_tenant_settings', 'message_history', 'manage_spaces', 'full_access']) assert.equal(allowedKeyCapabilities('admin').has(capability), false)
  assert.equal(keyScopeProblem('admin', ['retrieve', 'chat'], ['approved-kb']), '')
  assert.notEqual(keyScopeProblem('admin', ['read_agents'], []), '')
  assert.notEqual(keyScopeProblem('admin', ['manage_members'], ['approved-kb']), '')
  assert.notEqual(keyScopeProblem('contributor', ['retrieve'], ['approved-kb']), '')
  assert.equal(keyScopeProblem('owner', ['manage_members'], []), '')
})
test('Owner can distinguish personal, other-member and unknown historical key ownership', () => {
  assert.equal(keyCreatorLabel({ creator_user_id: 'u1', ownership: 'self' }), 'u1（我）')
  assert.equal(keyCreatorLabel({ creator_user_id: 'u2', ownership: 'other' }), 'u2')
  assert.equal(keyCreatorLabel({ ownership: 'legacy' }), '历史密钥 · 归属不明')
})
