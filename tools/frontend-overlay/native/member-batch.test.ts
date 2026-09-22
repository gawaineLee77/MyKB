import test from 'node:test'
import assert from 'node:assert/strict'
import { parseMemberEmails, runMemberBatch, batchJournal, type BatchRow } from './member-batch'

test('CSV quoted columns, BOM, repeated addresses and invalid input', () => {
  assert.deepEqual(parseMemberEmails('\uFEFF姓名,email\n"张,三",A@example.invalid\n甲,a@example.invalid\n错误,invalid'), { emails: ['a@example.invalid'], duplicates: 1, invalid: ['invalid'] })
  assert.throws(() => parseMemberEmails('x'.repeat(131073)))
  assert.throws(() => parseMemberEmails('员'.repeat(44000)))
  assert.throws(() => parseMemberEmails(Array.from({ length: 501 }, (_, i) => `${i}@example.invalid`).join('\n')))
})
test('Existing Owner retained, unknown account not created, and only failed items retried', async () => {
  const rows: BatchRow[] = [{ email: 'owner@example.invalid', state: 'ready' }, { email: 'new@example.invalid', state: 'ready' }, { email: 'missing@example.invalid', state: 'ready' }]
  const calls: string[] = []
  const api = { preview: async (emails: string[]) => emails.map(email => ({ email, state: email.startsWith('owner') ? 'existing' : email.startsWith('missing') ? 'unregistered' : 'ready', role: email.startsWith('owner') ? 'owner' : undefined })), add: async (email: string) => { calls.push(email) } }
  await runMemberBatch(rows, 'viewer', api)
  await runMemberBatch(rows, 'admin', api)
  assert.deepEqual(calls, ['new@example.invalid'])
  assert.equal(rows[0].role, 'owner')
})
test('Lost POST response is reconciled without a second write', async () => {
  const rows: BatchRow[] = [{ email: 'one@example.invalid', state: 'ready' }]
  let writes = 0
  await runMemberBatch(rows, 'viewer', { preview: async () => [{ email: rows[0].email, state: writes ? 'existing' : 'ready', role: writes ? 'admin' : undefined }], add: async () => { writes++; throw new Error('lost response') } })
  assert.equal(writes, 1); assert.equal(rows[0].state, 'existing'); assert.equal(rows[0].role, 'admin')
})
test('Failure to reconcile stays unknown and journal contains no addresses', async () => {
  const rows: BatchRow[] = [{ email: 'one@example.invalid', state: 'ready' }]
  let reads = 0
  await runMemberBatch(rows, 'viewer', { preview: async () => { if (++reads > 1) throw new Error('offline'); return [...rows] }, add: async () => { throw new Error('offline') } })
  assert.equal(rows[0].state, 'unknown')
  const journal = JSON.stringify(await batchJournal('synthetic-user', 42, 'viewer', rows))
  assert.equal(journal.includes('@'), false); assert.equal(journal.includes('one'), false)
})
