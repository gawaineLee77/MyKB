import assert from 'node:assert/strict'
import test from 'node:test'
import { createRenderer, h, watch } from 'vue'
import { createWorkspaceRefresh, provideWorkspaceRefresh, useWorkspaceRefresh } from './workspace-refresh'

function deferred() {
  let resolve!: () => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<void>((yes, no) => { resolve = () => yes(); reject = no })
  return { promise, resolve, reject }
}

function controlled() {
  const requests: ReturnType<typeof deferred>[] = []
  const refresh = createWorkspaceRefresh(() => {
    const request = deferred()
    requests.push(request)
    return request.promise
  })
  return { refresh, requests }
}

test('workspace refresh publishes a new generation synchronously before running work', async () => {
  const { refresh, requests } = controlled()
  assert.deepEqual(refresh.state.value, { generation: 0, status: 'idle' })
  const done = refresh.refresh()
  assert.deepEqual(refresh.state.value, { generation: 1, status: 'loading' })
  assert.equal(requests.length, 0)
  await Promise.resolve()
  requests[0].resolve()
  await done
  assert.deepEqual(refresh.state.value, { generation: 1, status: 'ready' })
})

test('a concurrent burst shares one promise and one follow-up without publishing ready early', async () => {
  const { refresh, requests } = controlled()
  const statuses: string[] = []
  const stop = watch(refresh.state, state => statuses.push(state.status), { flush: 'sync' })
  const done = refresh.refresh()
  assert.equal(refresh.refresh(), done)
  assert.equal(refresh.refresh(), done)
  assert.equal(refresh.state.value.generation, 3)
  let settled = false
  void done.then(() => { settled = true })
  await Promise.resolve()
  assert.equal(requests.length, 1)
  requests[0].resolve()
  await Promise.resolve()
  assert.equal(requests.length, 2)
  assert.equal(settled, false)
  assert.equal(statuses.every(status => status === 'loading'), true)
  requests[1].resolve()
  await done
  assert.deepEqual(refresh.state.value, { generation: 3, status: 'ready' })
  assert.equal(statuses.filter(status => status === 'ready').length, 1)
  stop()
})

test('refresh failures settle normally and a later refresh can retry', async () => {
  const { refresh, requests } = controlled()
  const failed = refresh.refresh()
  await Promise.resolve()
  requests[0].reject(new Error('synthetic connection failure'))
  await assert.doesNotReject(failed)
  assert.deepEqual(refresh.state.value, { generation: 1, status: 'error' })
  const retried = refresh.refresh()
  assert.notEqual(retried, failed)
  assert.deepEqual(refresh.state.value, { generation: 2, status: 'loading' })
  await Promise.resolve()
  requests[1].resolve()
  await retried
  assert.deepEqual(refresh.state.value, { generation: 2, status: 'ready' })
})

test('a failure superseded by a queued refresh stays loading until the final outcome', async () => {
  const { refresh, requests } = controlled()
  const statuses: string[] = []
  const stop = watch(refresh.state, state => statuses.push(state.status), { flush: 'sync' })
  const done = refresh.refresh()
  await Promise.resolve()
  assert.equal(refresh.refresh(), done)
  requests[0].reject(new Error('obsolete failure'))
  await Promise.resolve()
  assert.equal(requests.length, 2)
  assert.equal(refresh.state.value.status, 'loading')
  assert.equal(statuses.includes('error'), false)
  requests[1].resolve()
  await done
  assert.equal(refresh.state.value.status, 'ready')
  stop()
})

test('a new change during the follow-up queues one further run without losing the change', async () => {
  const { refresh, requests } = controlled()
  const done = refresh.refresh()
  await Promise.resolve()
  refresh.refresh()
  requests[0].resolve()
  await Promise.resolve()
  assert.equal(requests.length, 2)
  assert.equal(refresh.refresh(), done)
  assert.equal(refresh.refresh(), done)
  requests[1].resolve()
  await Promise.resolve()
  assert.equal(requests.length, 3)
  assert.equal(refresh.state.value.status, 'loading')
  requests[2].resolve()
  await done
  assert.deepEqual(refresh.state.value, { generation: 4, status: 'ready' })
})

test('disposal before the scheduled run prevents work and further refreshes', async () => {
  const { refresh, requests } = controlled()
  const done = refresh.refresh()
  refresh.dispose()
  const disposedState = refresh.state.value
  assert.deepEqual(disposedState, { generation: 2, status: 'idle' })
  await done
  await refresh.refresh()
  refresh.dispose()
  assert.equal(requests.length, 0)
  assert.equal(refresh.state.value, disposedState)
})

test('disposal cancels the queued rerun and ignores a late failure', async () => {
  const { refresh, requests } = controlled()
  const done = refresh.refresh()
  await Promise.resolve()
  assert.equal(refresh.refresh(), done)
  refresh.dispose()
  const disposedState = refresh.state.value
  assert.deepEqual(disposedState, { generation: 3, status: 'idle' })
  requests[0].reject(new Error('late failure'))
  await assert.doesNotReject(done)
  assert.equal(requests.length, 1)
  assert.equal(refresh.state.value, disposedState)
})

test('a synchronous ready observer can start the next flight without the old flight clearing it', async () => {
  const { refresh, requests } = controlled()
  let next: Promise<void> | undefined
  const stop = watch(refresh.state, state => {
    if (state.status === 'ready' && state.generation === 1) next = refresh.refresh()
  }, { flush: 'sync' })
  const first = refresh.refresh()
  await Promise.resolve()
  requests[0].resolve()
  await first
  assert.ok(next)
  assert.notEqual(next, first)
  assert.deepEqual(refresh.state.value, { generation: 2, status: 'loading' })
  assert.equal(refresh.refresh(), next)
  requests[1].resolve()
  await Promise.resolve()
  requests[2].resolve()
  await next
  assert.deepEqual(refresh.state.value, { generation: 3, status: 'ready' })
  stop()
})

test('workspace refresh provides the same reactive coordinator to descendants and is optional', () => {
  type Node = { children: Node[] }
  const node = (): Node => ({ children: [] })
  const renderer = createRenderer<Node, Node>({
    createElement: node,
    createText: node,
    createComment: node,
    insert: (child, parent) => { parent.children.push(child) },
    remove: () => {},
    setText: () => {},
    setElementText: () => {},
    parentNode: () => null,
    nextSibling: () => null,
    patchProp: () => {},
  })
  const coordinator = createWorkspaceRefresh(async () => {})
  let observed: ReturnType<typeof useWorkspaceRefresh>
  const child = { setup() { observed = useWorkspaceRefresh(); return () => null } }
  const app = renderer.createApp({
    setup() { provideWorkspaceRefresh(coordinator); return () => h(child) },
  })
  assert.equal(app.runWithContext(() => useWorkspaceRefresh()), undefined)
  app.mount(node())
  assert.equal(observed, coordinator)
  app.unmount()
  coordinator.dispose()
})
