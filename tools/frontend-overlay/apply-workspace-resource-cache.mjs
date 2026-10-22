// Permission-sensitive caches must discard responses started before a reset.
// Apply only to the generated frontend copy of the pinned upstream store.
export function applyWorkspaceResourceCache({ replaceExact: exact }) {
  const store = 'src/stores/chatResources.ts'
  exact(store, '  let kbAllGen = 0', `  let kbAllGen = 0
  let kbCacheEpoch = 0`)
  exact(store, `    const p = loader().finally(() => {
      inflight.delete(key)
    })`, `    const p = loader().finally(() => {
      if (inflight.get(key) === p) inflight.delete(key)
    })`)
  exact(store, `    if (creator !== 'all') {
      const res: any = await listKnowledgeBases({ creator })
      return res?.data && Array.isArray(res.data) ? res.data : []
    }`, `    if (creator !== 'all') {
      const epoch = kbCacheEpoch
      const res: any = await listKnowledgeBases({ creator })
      if (epoch !== kbCacheEpoch) return []
      if (res?.success === false || !Array.isArray(res?.data)) throw new Error('Knowledge list unavailable')
      return res?.data && Array.isArray(res.data) ? res.data : []
    }`)
  exact(store, `        const res: any = await listKnowledgeBases()
        const data = res?.data && Array.isArray(res.data) ? res.data : []
        rawKnowledgeBases.value = data`, `        const res: any = await listKnowledgeBases()
        if (kbAllGen !== gen) return epoch === kbCacheEpoch ? rawKnowledgeBases.value : []
        if (res?.success === false || !Array.isArray(res?.data)) throw new Error('Knowledge list unavailable')
        const data = res.data
        rawKnowledgeBases.value = data`)
  exact(store, `    const gen = ++kbAllGen
    kbAllInflight =`, `    const gen = ++kbAllGen
    const epoch = kbCacheEpoch
    kbAllInflight =`)
  exact(store, `        await orgStore.fetchSharedKnowledgeBases({ force })
        return data`, `        await orgStore.fetchSharedKnowledgeBases({ force })
        if (epoch !== kbCacheEpoch) return []
        return kbAllGen === gen ? data : rawKnowledgeBases.value`)
  exact(store, `    const p = (async () => {
      try {
        const res: any = await listKnowledgeBases({
          agent_id: agentId,`, `    let p!: Promise<any[]>
    p = (async () => {
      try {
        const res: any = await listKnowledgeBases({
          agent_id: agentId,`)
  exact(store, `        const list = res?.data && Array.isArray(res.data) ? res.data : []
        agentKbCache.set(cacheKey, { at: Date.now(), data: list })`, `        const list = res?.data && Array.isArray(res.data) ? res.data : []
        if (agentKbInflight.get(cacheKey) !== p) return agentKbCache.get(cacheKey)?.data ?? []
        agentKbCache.set(cacheKey, { at: Date.now(), data: list })`)
  exact(store, `      } finally {
        agentKbInflight.delete(cacheKey)
      }`, `      } finally {
        if (agentKbInflight.get(cacheKey) === p) agentKbInflight.delete(cacheKey)
      }`)
  exact(store, `    const p = (async () => {
      try {
        const res: any = await getKnowledgeBaseById(kbId)
        const data = res?.data ?? null`, `    let p!: Promise<any | null>
    p = (async () => {
      try {
        const res: any = await getKnowledgeBaseById(kbId)
        const data = res?.data ?? null
        if (kbDetailInflight.get(kbId) !== p) return kbDetailCache.get(kbId)?.data ?? null`)
  exact(store, `      } finally {
        kbDetailInflight.delete(kbId)
      }`, `      } finally {
        if (kbDetailInflight.get(kbId) === p) kbDetailInflight.delete(kbId)
      }`)
  exact(store, `    if (keys.length === 0) {
      loadedAt.value = {}`, `    if (keys.length === 0) {
      kbAllGen++
      kbCacheEpoch++
      loadedAt.value = {}`)
  exact(store, `    if (keys.includes('knowledgeBases')) {
      agentKbCache.clear()`, `    if (keys.includes('knowledgeBases')) {
      kbAllGen++
      kbCacheEpoch++
      agentKbCache.clear()`)
}
