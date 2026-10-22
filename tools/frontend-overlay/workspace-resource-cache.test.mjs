import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { stripTypeScriptTypes } from 'node:module'
import test from 'node:test'
import { applyWorkspaceResourceCache } from './apply-workspace-resource-cache.mjs'

let source = readFileSync(new URL('../../upstream/weknora/frontend/src/stores/chatResources.ts', import.meta.url), 'utf8')
applyWorkspaceResourceCache({
  replaceExact(path, before, after) {
    assert.equal(path, 'src/stores/chatResources.ts')
    assert.equal(source.split(before).length - 1, 1, `unique overlay anchor: ${before}`)
    source = source.replace(before, after)
  },
})
// Execute the actual transformed store. Only framework and API dependencies are
// replaced, allowing controlled network response ordering without a browser.
source = stripTypeScriptTypes(source.replace(/^import[\s\S]*?from ['"][^'"]+['"]\r?\n/gm, ''))
source = source.replace('export const useChatResourcesStore', 'const useChatResourcesStore')
const createStore = new Function('deps', `
  const { defineStore, ref, computed, watch, listKnowledgeBases,
    getKnowledgeBaseById, listModels, useOrganizationStore,
    getCurrentLanguage } = deps;
  ${source}
  return useChatResourcesStore();
`)

function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

function setup() {
  const kb = [], detail = [], models = [], shared = []
  const store = createStore({
    defineStore: (_name, factory) => factory,
    ref: value => ({ value }),
    computed: fn => ({ get value() { return fn() } }),
    watch: () => {},
    listKnowledgeBases: () => { const request = deferred(); kb.push(request); return request.promise },
    getKnowledgeBaseById: () => { const request = deferred(); detail.push(request); return request.promise },
    listModels: () => { const request = deferred(); models.push(request); return request.promise },
    useOrganizationStore: () => ({
      fetchSharedKnowledgeBases: () => shared.length ? shared.shift().promise : Promise.resolve(),
    }),
    getCurrentLanguage: () => 'zh-CN',
  })
  const reset = (...keys) => {
    store.invalidate(...keys)
    // The workspace permission refresh synchronously clears displayed lists.
    store.rawKnowledgeBases.value = []
  }
  return { store, reset, kb, detail, models, shared }
}

const oldKB = { id: 'revoked-kb', summary_model_id: 'chat', embedding_model_id: 'embed' }
const currentKB = { id: 'visible-kb', summary_model_id: 'chat', embedding_model_id: 'embed' }

test('a failed or malformed successful HTTP response is not cached as an empty knowledge list', async () => {
  for (const response of [{ success: false, data: [] }, { success: true, data: null }]) {
    const { store, kb } = setup()
    const request = store.fetchKnowledgeBasesForList()
    const rejected = assert.rejects(request, /Knowledge list unavailable/)
    kb[0].resolve(response)
    await rejected
    assert.equal(store.isFresh('knowledgeBases'), false)
  }
})

for (const keys of [[], ['knowledgeBases']]) {
  test(`KB list ignores a response preceding ${keys.length ? 'scoped' : 'full'} invalidation`, async () => {
    const { store, reset, kb } = setup()
    const old = store.fetchKnowledgeBasesForList()
    reset(...keys)
    const fresh = store.fetchKnowledgeBasesForList()
    kb[0].resolve({ data: [oldKB] })
    assert.deepEqual(await old, [])
    assert.deepEqual(store.rawKnowledgeBases.value, [])
    const joined = store.fetchKnowledgeBasesForList()
    assert.equal(kb.length, 2, 'old finally must not discard the new request')
    kb[1].resolve({ data: [currentKB] })
    assert.deepEqual(await fresh, [currentKB])
    assert.deepEqual(await joined, [currentKB])
    assert.deepEqual(store.rawKnowledgeBases.value, [currentKB])
  })
}

test('older forced KB refresh cannot overwrite a newer result', async () => {
  const { store, kb } = setup()
  const old = store.fetchKnowledgeBasesForList()
  const fresh = store.fetchKnowledgeBasesForList(undefined, true)
  kb[1].resolve({ data: [currentKB] })
  await fresh
  kb[0].resolve({ data: [oldKB] })
  assert.deepEqual(await old, [currentKB])
  assert.deepEqual(store.rawKnowledgeBases.value, [currentKB])
})

test('stale KB callers do not receive cached rows retained by a scoped invalidation', async () => {
  const { store, kb } = setup()
  const loaded = store.fetchKnowledgeBasesForList()
  kb[0].resolve({ data: [oldKB] })
  await loaded
  const pending = store.fetchKnowledgeBasesForList(undefined, true)
  store.invalidate('knowledgeBases')
  kb[1].resolve({ data: [oldKB] })
  assert.deepEqual(await pending, [])
})

test('KB list caller cannot receive revoked data while shared metadata was loading', async () => {
  const { store, reset, kb, shared } = setup()
  const sharedRequest = deferred()
  shared.push(sharedRequest)
  const old = store.fetchKnowledgeBasesForList()
  kb[0].resolve({ data: [oldKB] })
  await Promise.resolve()
  assert.deepEqual(store.rawKnowledgeBases.value, [oldKB])
  reset('knowledgeBases')
  sharedRequest.resolve()
  assert.deepEqual(await old, [])
})

test('creator-filtered KB responses are discarded after permission invalidation', async () => {
  const { store, reset, kb } = setup()
  const old = store.fetchKnowledgeBasesForList({ creator: 'mine' })
  reset('knowledgeBases')
  kb[0].resolve({ data: [oldKB] })
  assert.deepEqual(await old, [])
})

for (const kind of ['agent', 'detail']) {
  test(`${kind} KB cache discards old response and preserves newer in-flight request`, async () => {
    const { store, reset, kb, detail } = setup()
    const requests = kind === 'agent' ? kb : detail
    const fetch = () => kind === 'agent'
      ? store.ensureAgentKnowledgeBases('synthetic-agent')
      : store.fetchKnowledgeBaseById('synthetic-kb')
    const old = fetch()
    if (kind === 'agent') reset('knowledgeBases')
    else store.invalidateKnowledgeBaseDetail('synthetic-kb')
    const fresh = fetch()
    requests[0].resolve({ data: kind === 'agent' ? [oldKB] : oldKB })
    assert.deepEqual(await old, kind === 'agent' ? [] : null)
    const joined = fetch()
    assert.equal(requests.length, 2)
    const expected = kind === 'agent' ? [currentKB] : currentKB
    requests[1].resolve({ data: expected })
    assert.deepEqual(await fresh, expected)
    assert.deepEqual(await joined, expected)
    assert.deepEqual(await fetch(), expected)
    assert.equal(requests.length, 2, 'only the valid response can seed the cache')
  })
}

test('a previous model request cannot remove its replacement from the shared request map', async () => {
  const { store, models } = setup()
  const old = store.ensureModels()
  store.invalidate('models')
  const fresh = store.ensureModels()
  models[0].resolve([{ id: 'old-model', type: 'KnowledgeQA' }])
  await old
  assert.deepEqual(store.allModels.value, [])
  const joined = store.ensureModels()
  assert.equal(models.length, 2)
  models[1].resolve([{ id: 'current-model', type: 'KnowledgeQA' }])
  await Promise.all([fresh, joined])
  assert.deepEqual(store.allModels.value, [{ id: 'current-model', type: 'KnowledgeQA' }])
})
