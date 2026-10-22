import assert from 'node:assert/strict'
import test from 'node:test'
import { KB_OPERATIONS, hasKBOperation, canReparseKB, normalizeKBMember, matchesKBFilter, kbPermissionProblem, kbPolicyInput, type KBEffective } from './kb-permissions-policy'
const access = (operations: string[]): KBEffective => ({ visible: true, manage: false, creator: false, visibility: 'restricted', revision: 2, operations })
test('seven independent operations deny unknown/loading/invisible states, including management', () => {
  assert.equal(KB_OPERATIONS.length, 7)
  for (const { value } of KB_OPERATIONS) {
    for (const { value: other } of KB_OPERATIONS) assert.equal(hasKBOperation(access([value]), other), value === other)
    assert.equal(hasKBOperation(null, value), false)
    assert.equal(hasKBOperation({ ...access([value]), visible: false }, value), false)
    assert.equal(hasKBOperation({ ...access([]), manage: true }, value), false)
  }
})
test('reparse requires document update and chunk update/delete, not upload alone', () => {
  assert.equal(canReparseKB(access(['document.create'])), false)
  for (const missing of ['document.update', 'chunk.update', 'chunk.delete']) assert.equal(canReparseKB(access(KB_OPERATIONS.map(op => op.value).filter(op => op !== missing))), false)
  assert.equal(canReparseKB(access(['document.update', 'chunk.update', 'chunk.delete'])), true)
})
test('Viewer and revoked visibility strip writes/download; unknown operations cannot be saved', () => {
  const member = { user_id: 'member', visible: true, operations: [...KB_OPERATIONS.map(op => op.value), 'manage'] }
  assert.deepEqual(normalizeKBMember(member, 'viewer').operations, [])
  assert.deepEqual(normalizeKBMember({ ...member, visible: false }, 'contributor').operations, [])
  assert.equal(normalizeKBMember(member, 'contributor').operations.length, 7)
  assert.equal(normalizeKBMember(member, '').operations.length, 7, 'unresolved existing rows outside the member search page retain grants for server validation')
})
test('list filters use effective creator and visibility; conflicts require reload', () => {
  assert.equal(matchesKBFilter(undefined, 'all'), false)
  assert.equal(matchesKBFilter(access([]), 'shared'), true)
  assert.equal(matchesKBFilter({ ...access([]), creator: true }, 'mine'), true)
  assert.equal(matchesKBFilter({ ...access([]), visibility: 'workspace' }, 'workspace'), true)
  assert.match(kbPermissionProblem({ status: 409 }), /重新载入/)
})
test('policy updates match the strict server input and never submit creator identity; private removes all grants', () => {
  const policy = { kb_id: 'kb', creator_user_id: 'owner', creator_revoked: true, revision: 9, visibility: 'workspace' as const, members: [{ user_id: 'member', visible: true, operations: ['document.create'] }] }
  const input = kbPolicyInput(policy, 'selected', () => 'contributor')
  assert.deepEqual(Object.keys(input).sort(), ['members', 'revision', 'visibility'])
  assert.equal(input.visibility, 'restricted')
  assert.equal(input.revision, 9)
  assert.deepEqual(input.members[0].operations, ['document.create'])
  assert.deepEqual(kbPolicyInput(policy, 'private', () => 'contributor').members, [])
})
