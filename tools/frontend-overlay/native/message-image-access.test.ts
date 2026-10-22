import assert from 'node:assert/strict'
import test from 'node:test'
import { messageImageAccess } from './message-image-access'
import { buildProtectedFileRequest } from '../utils/protectedFileAccess'
import { protectProviderImageSrcInHTML } from '../utils/security'

test('quick answers and persisted uploaded images use their session/message proxy', () => {
  for (const message of [{ id: 'user-message' }, { id: 'assistant-message' }]) {
    const access = messageImageAccess('session', message)
    assert.ok(access)
    const request = buildProtectedFileRequest('resource://AbCdEfGhIjKlMnOpQrStUv', access)
    assert.equal(request?.url, `/api/v1/sessions/session/messages/${message.id}/files?file_path=resource%3A%2F%2FAbCdEfGhIjKlMnOpQrStUv`)
  }
})

test('the server-confirmed assistant message supersedes the stream placeholder', () => {
  assert.deepEqual(messageImageAccess('session', { id: 'stream-placeholder', assistant_message_id: 'persisted' }), {
    mode: 'message', sessionId: 'session', messageId: 'persisted',
  })
})

test('missing message/session anchors never degrade to tenant-wide file reads', () => {
  assert.equal(messageImageAccess('session', undefined), undefined)
  assert.equal(messageImageAccess('', { id: 'message' }), undefined)
  assert.equal(messageImageAccess('session', { id: '' }), undefined)
  assert.equal(messageImageAccess('session', { id: 42 }), undefined)
  assert.equal(messageImageAccess(42, { id: 'message' }), undefined)
})

test('unregistered legacy image markers show a notice without creating a local image request', () => {
  const html = protectProviderImageSrcInHTML('<p>历史正文<img src="mindcreek-unverified-image" alt="old"></p>')
  assert.match(html, /历史正文/)
  assert.match(html, /role="status"/)
  assert.doesNotMatch(html, /<img\b/)
})
