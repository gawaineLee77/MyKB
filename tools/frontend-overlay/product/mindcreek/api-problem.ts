export interface APIProblem {
  code?: string
  message: string
}

function record(value: unknown): Record<string, any> | undefined {
  return value !== null && typeof value === 'object' ? value as Record<string, any> : undefined
}

// request.ts rejects a flattened API body, while a few callers and tests may
// still provide an Axios-shaped error. Normalize both without depending on
// transport-specific details.
export function apiProblem(value: unknown): APIProblem {
  const outer = record(value)
  const response = record(outer?.response)
  const body = record(response?.data) ?? outer
  const error = record(body?.error)
  const code = typeof error?.code === 'string'
    ? error.code
    : typeof body?.code === 'string' ? body.code : undefined
  const message = typeof error?.message === 'string'
    ? error.message
    : typeof body?.message === 'string'
      ? body.message
      : value instanceof Error ? value.message : String(value ?? 'Unknown error')
  return { code, message }
}
