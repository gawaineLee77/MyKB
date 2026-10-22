import assert from 'node:assert/strict'
import test from 'node:test'
import { createRenderer, h, nextTick, ref } from 'vue'
import { useKBListRefresh } from './kb-list-refresh'
import { createWorkspaceRefresh, provideWorkspaceRefresh } from './workspace-refresh'

const renderer = createRenderer<any, any>({
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  insert() {}, remove() {}, setText() {}, setElementText() {}, patchProp() {},
  parentNode: () => null, nextSibling: () => null,
})
function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (reason: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const visible = { visible: true, manage: false, creator: false, visibility: 'restricted' as const, revision: 1, operations: [] }
const rows = (id: string) => ({ items: [{ id }], organizationItems: [] as { knowledge_base: { id: string } }[] })
function setup(options: { load?: any; access?: any; run?: () => Promise<void> } = {}) {
  const identity = ref('user:tenant:contributor:all')
  const coordinator = createWorkspaceRefresh(options.run || (async () => {}))
  let list!: ReturnType<typeof useKBListRefresh<{ id: string }, { knowledge_base: { id: string } }>>
  const Child = { setup() {
    list = useKBListRefresh({ identity: () => identity.value, load: options.load || (async () => rows('visible')), access: options.access || (async () => visible) })
    return () => null
  } }
  const app = renderer.createApp({ setup() { provideWorkspaceRefresh(coordinator); return () => h(Child) } })
  app.mount({})
  return { list, identity, coordinator, close: () => { coordinator.dispose(); app.unmount() } }
}

test('a focus refresh cannot let an old empty result replace the new verified list', async () => {
  const old = deferred<ReturnType<typeof rows>>()
  let calls = 0
  const s = setup({ load: () => ++calls === 1 ? old.promise : Promise.resolve(rows('current')) })
  const pending = s.list.refresh()
  await s.coordinator.refresh()
  await nextTick(); await new Promise(resolve => setTimeout(resolve, 0))
  assert.deepEqual(s.list.items.value, [{ id: 'current' }])
  old.resolve({ items: [], organizationItems: [] })
  await pending
  assert.deepEqual(s.list.items.value, [{ id: 'current' }])
  assert.equal(s.list.loading.value, false)
  s.close()
})

test('late ACL success cannot resurrect a revoked knowledge base', async () => {
  const acl = deferred<typeof visible>()
  let calls = 0
  const s = setup({ load: () => Promise.resolve(++calls === 1 ? rows('revoked') : { items: [], organizationItems: [] }), access: () => acl.promise })
  const old = s.list.refresh()
  await nextTick()
  await s.list.refresh(true)
  acl.resolve(visible)
  await old
  assert.deepEqual(s.list.items.value, [])
  assert.deepEqual(s.list.access.value, {})
  assert.equal(s.list.ready.value, true)
  s.close()
})

test('pending ACL and failed ACL do not count as a successfully empty list', async () => {
  const acl = deferred<typeof visible>()
  const s = setup({ access: () => acl.promise })
  const pending = s.list.refresh()
  await nextTick()
  assert.equal(s.list.loading.value, true)
  assert.equal(s.list.ready.value, false)
  acl.reject({ status: 503 })
  await pending
  assert.equal(s.list.loading.value, false)
  assert.match(s.list.error.value, /核验/)
  assert.deepEqual(s.list.items.value, [])
  s.close()
})

test('only freshly verified rows survive a partial ACL outage; denial stays hidden', async () => {
  const s = setup({ load: async () => ({ items: ['allowed', 'denied', 'deleted', 'unavailable'].map(id => ({ id })), organizationItems: [] }), access: async (id: string) => {
    if (id === 'allowed') return visible
    throw { status: id === 'denied' ? 403 : id === 'deleted' ? 404 : 503 }
  } })
  await s.list.refresh()
  assert.deepEqual(s.list.items.value, [{ id: 'allowed' }])
  assert.ok(s.list.error.value)
  s.close()
})

test('confirmed denials and a genuine empty response have no transport error', async () => {
  const s = setup({ access: async () => { throw { status: 403 } } })
  await s.list.refresh()
  assert.deepEqual(s.list.items.value, [])
  assert.equal(s.list.error.value, '')
  assert.equal(s.list.ready.value, true)
  s.close()
})

test('a failed list has an error and retry revalidates workspace before recovering', async () => {
  let fail = true, checks = 0
  const s = setup({ run: async () => { checks++ }, load: async () => {
    if (fail) throw { status: 503 }
    return rows('recovered')
  } })
  await s.list.refresh()
  assert.equal(s.list.ready.value, false)
  assert.ok(s.list.error.value)
  fail = false
  await s.list.retry()
  await nextTick(); await new Promise(resolve => setTimeout(resolve, 0))
  assert.equal(checks, 1)
  assert.deepEqual(s.list.items.value, [{ id: 'recovered' }])
  assert.equal(s.list.error.value, '')
  s.close()
})

test('scope, account and role changes discard late rows and organization responses', async () => {
  for (const identity of ['user:tenant:contributor:other-org', 'other-user:tenant:contributor:all', 'user:tenant:viewer:all']) {
    const old = deferred<ReturnType<typeof rows>>()
    let calls = 0
    const s = setup({ load: () => ++calls === 1 ? old.promise : Promise.resolve(rows('new-context')) })
    const pending = s.list.refresh()
    s.identity.value = identity
    await nextTick(); await new Promise(resolve => setTimeout(resolve, 0))
    old.resolve({ items: [{ id: 'old' }], organizationItems: [{ knowledge_base: { id: 'old-shared' } }] })
    await pending
    assert.deepEqual(s.list.items.value, [{ id: 'new-context' }])
    assert.deepEqual(s.list.organizationItems.value, [])
    s.close()
  }
})

test('unmounted lists cannot publish late results', async () => {
  const old = deferred<ReturnType<typeof rows>>()
  const s = setup({ load: () => old.promise })
  const pending = s.list.refresh()
  s.close()
  old.resolve(rows('late'))
  await pending
  assert.deepEqual(s.list.items.value, [])
  assert.equal(s.list.ready.value, false)
})
