const REFERENCE = /^mc-avatar:([a-f0-9]{32})$/
export function agentIconID(value?: string): string { return REFERENCE.exec(value || '')?.[1] || '' }
export function agentIconURL(value?: string, agentID?: string): string {
  const id = agentIconID(value)
  return id ? `/api/v1/mindcreek/agent-icons/${id}${agentID ? `?agent_id=${encodeURIComponent(agentID)}` : ''}` : ''
}
// Legacy native emoji/icon labels are text, never arbitrary remote image URLs.
export function agentIconLabel(value?: string): string {
  if (!value || value.startsWith('mc-avatar') || value.length > 64 || /[:/<>]/.test(value)) return ''
  return value
}
