export const MANAGED_CHAT_MODEL_ID = 'builtin-mindcreek-chat'

export type ChatModelChoice = {
  id?: string
  type?: string
  status?: string
  is_default?: boolean
}

// Preserve an available user choice, otherwise prefer MindCreek's managed
// default. This also repairs stale localStorage selections after an operator
// rotates or removes a workspace override.
export function chooseChatModel(
  models: ChatModelChoice[],
  lastSelectedID = '',
  currentSelectedID = '',
): string {
  const available = models.filter(model =>
    model.type === 'KnowledgeQA'
    && typeof model.id === 'string'
    && model.id.length > 0
    && model.status !== 'inactive',
  )
  const byID = new Set(available.map(model => model.id))
  if (lastSelectedID && byID.has(lastSelectedID)) return lastSelectedID
  if (currentSelectedID && byID.has(currentSelectedID)) return currentSelectedID
  return available.find(model => model.id === MANAGED_CHAT_MODEL_ID)?.id
    || available.find(model => model.is_default)?.id
    || available[0]?.id
    || ''
}
