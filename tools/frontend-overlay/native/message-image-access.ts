import type { ProtectedFileAccessContext } from '../utils/protectedFileAccess'

// A request/stream ID is not a persisted message authorization anchor. Wait for
// the server's message ID; never fall back to the tenant-wide /files endpoint.
export function messageImageAccess(
  sessionID: unknown,
  message: { assistant_message_id?: unknown; id?: unknown } | undefined,
): ProtectedFileAccessContext | undefined {
  const sessionId = typeof sessionID === 'string' ? sessionID.trim() : ''
  const value = message?.assistant_message_id || message?.id
  const messageId = typeof value === 'string' ? value.trim() : ''
  if (!sessionId || !messageId) return undefined
  return { mode: 'message', sessionId, messageId }
}
