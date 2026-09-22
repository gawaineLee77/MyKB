import Papa from 'papaparse'

export type BatchRole = 'viewer' | 'contributor' | 'admin'
export interface BatchRow { email: string; state: string; role?: string; user_id?: string; code?: string }
export function parseMemberEmails(text: string): { emails: string[]; duplicates: number; invalid: string[] } {
  if (new TextEncoder().encode(text).byteLength > 131072) throw new Error('导入内容不能超过 128 KiB。')
  const csv = Papa.parse<string[]>(text.replace(/^\uFEFF/, ''), { skipEmptyLines: 'greedy' })
  // Plain newline/semicolon lists are supported as well as an email CSV column.
  const header = csv.data[0] || []
  const column = header.findIndex(value => /^(email|邮箱|企业邮箱)$/i.test(value.trim()))
  if (csv.errors.some(e => e.code !== 'UndetectableDelimiter')) throw new Error('CSV 格式不正确，请检查引号和列。')
  const values = column >= 0 ? csv.data.slice(1).map(row => row[column] || '') : csv.data.flatMap(row => row.flatMap(v => v.split(/[;；\s]+/)))
  const emails: string[] = [], invalid: string[] = [], seen = new Set<string>()
  let duplicates = 0
  for (const raw of values) {
    const value = raw.trim().toLowerCase()
    if (!value) continue
    if (!/^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/.test(value) || value.length > 254) { invalid.push(value.slice(0, 254)); continue }
    if (seen.has(value)) { duplicates++; continue }
    seen.add(value); emails.push(value)
    if (emails.length > 500) throw new Error('每批最多 500 个邮箱，请拆分导入。')
  }
  return { emails, duplicates, invalid }
}

export interface BatchAPI {
  preview(emails: string[]): Promise<BatchRow[]>
  add(email: string, role: BatchRole): Promise<void>
}

// Each run rereads live membership first. Existing members are never updated.
// Uncertain writes are reconciled and reported, never automatically replayed.
export async function runMemberBatch(rows: BatchRow[], role: BatchRole, api: BatchAPI, changed: () => void = () => {}): Promise<void> {
  const candidates = rows.filter(row => !['added', 'existing', 'invalid'].includes(row.state))
  if (!candidates.length) return
  const current = await api.preview(candidates.map(row => row.email))
  if (current.length !== candidates.length) throw new Error('预览结果不完整，请刷新后重试。')
  for (const row of candidates) {
    const fresh = current.find(item => item.email === row.email)
    if (!fresh) throw new Error('成员结果不完整。')
    Object.assign(row, fresh)
    if (row.state !== 'ready') { changed(); continue }
    row.state = 'adding'; changed()
    try {
      await api.add(row.email, role)
      row.state = 'added'; row.role = role; row.code = undefined
    } catch (error: any) {
      row.state = 'unknown'; row.code = error?.error?.code || error?.code || 'member.result_unknown'
      try {
        const result = (await api.preview([row.email]))[0]
        if (result?.state === 'existing') Object.assign(row, result)
        else if (error?.status >= 400 && error?.status < 500) {
          row.state = result?.state === 'unregistered' ? 'unregistered' : error.status === 409 ? 'conflict' : 'failed'
        }
      } catch { /* Keep unknown until an explicit retry can recheck. */ }
    }
    changed()
  }
}

// Persist only keyed digests and outcome codes, never addresses or credentials.
export async function batchJournal(user: string, tenant: number, role: BatchRole, rows: BatchRow[]) {
  const entries = await Promise.all(rows.map(async row => ({
    id: Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(`${user}:${tenant}:${row.email}`)))).map(v => v.toString(16).padStart(2, '0')).join(''),
    state: row.state, code: row.code,
  })))
  return { version: 1, tenant, role, entries }
}
