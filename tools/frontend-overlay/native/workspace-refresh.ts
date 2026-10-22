import { inject, provide, shallowRef, type InjectionKey, type ShallowRef } from 'vue'

export type WorkspaceRefreshStatus = 'idle' | 'loading' | 'ready' | 'error'
export interface WorkspaceRefreshState {
  generation: number
  status: WorkspaceRefreshStatus
}
export interface WorkspaceRefresh {
  state: ShallowRef<WorkspaceRefreshState>
  refresh(): Promise<void>
  dispose(): void
}

// All consumers observe the generation change before any network work starts.
// Requests arriving during a refresh share its promise and queue one follow-up.
export function createWorkspaceRefresh(run: () => Promise<void>): WorkspaceRefresh {
  const state = shallowRef<WorkspaceRefreshState>({ generation: 0, status: 'idle' })
  const settled = Promise.resolve()
  let inflight: Promise<void> | undefined
  let pending = false
  let disposed = false

  async function drain(): Promise<void> {
    while (!disposed) {
      let status: WorkspaceRefreshStatus = 'ready'
      try { await run() } catch { status = 'error' }
      if (disposed) break
      if (pending) {
        pending = false
        continue
      }
      // Release before publishing the terminal state: a synchronous observer
      // may request another refresh, which must receive a new active promise.
      inflight = undefined
      state.value = { generation: state.value.generation, status }
      return
    }
    inflight = undefined
  }

  function refresh(): Promise<void> {
    if (disposed) return settled
    let request = inflight
    if (request) {
      pending = true
    } else {
      request = Promise.resolve().then(drain)
      inflight = request
    }
    state.value = { generation: state.value.generation + 1, status: 'loading' }
    return request
  }

  function dispose(): void {
    if (disposed) return
    disposed = true
    pending = false
    state.value = { generation: state.value.generation + 1, status: 'idle' }
  }

  return { state, refresh, dispose }
}

const workspaceRefreshKey: InjectionKey<WorkspaceRefresh> = Symbol('mindcreek-workspace-refresh')
export function provideWorkspaceRefresh(value: WorkspaceRefresh): void {
  provide(workspaceRefreshKey, value)
}
export function useWorkspaceRefresh(): WorkspaceRefresh | undefined {
  return inject(workspaceRefreshKey, undefined)
}
