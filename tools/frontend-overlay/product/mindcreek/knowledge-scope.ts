export interface KnowledgeReference {
  id: string
  knowledge_id?: string
  knowledge_base_id?: string
  knowledge_title?: string
  knowledge_filename?: string
  match_count?: number
}

/** Collapse chunk-level retrieval hits into document-level source cards. */
export function mergeDocumentReferences(references: KnowledgeReference[]): KnowledgeReference[] {
  const merged = new Map<string, KnowledgeReference>()
  for (const reference of references || []) {
    if (!reference?.id) continue
    const title = (reference.knowledge_title || reference.knowledge_filename || '').trim().toLowerCase()
    const documentKey = reference.knowledge_id || title || reference.id
    const key = `${reference.knowledge_base_id || ''}\u0000${documentKey}`
    const existing = merged.get(key)
    if (existing) {
      existing.match_count = (existing.match_count || 1) + 1
      continue
    }
    merged.set(key, { ...reference, match_count: 1 })
  }
  return [...merged.values()]
}
