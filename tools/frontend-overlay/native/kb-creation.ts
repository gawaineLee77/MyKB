import { post } from '@/utils/request'
// Keep an uncertain attempt across page refreshes. The server owns the saga;
// the browser stores only an opaque key and a digest, never document content.
export async function createPrivateKnowledgeBase(data: Record<string, unknown>) {
  const tenant = localStorage.getItem('weknora_selected_tenant_id') || ''
  const user = JSON.parse(localStorage.getItem('weknora_user') || '{}')?.id || ''
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(JSON.stringify(data)))
  const fingerprint = Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('')
  const slot = `mindcreek-kb-create:${tenant}:${user}:${fingerprint}`
  const key = sessionStorage.getItem(slot) || crypto.randomUUID()
  sessionStorage.setItem(slot, key)
  const response = await post('/api/v1/knowledge-bases', data, { headers: { 'Idempotency-Key': key } })
  if (response?.success) sessionStorage.removeItem(slot)
  return response
}
