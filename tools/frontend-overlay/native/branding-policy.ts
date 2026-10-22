export const DEFAULT_SERVICE_NAME = '小纳'
export const BRANDING_ENDPOINT = '/api/v1/mindcreek/branding'
export interface ServiceBranding { name: string; logo_url: string; revision: number }
export function validBrandName(value: string): boolean {
  return value.trim() === value && Array.from(value).length >= 1 && Array.from(value).length <= 64 && !/[\p{Cc}\p{Cf}<>]/u.test(value)
}
export function parseBranding(value: unknown): ServiceBranding {
  const data = value as Partial<ServiceBranding> | null
  if (!data || typeof data.name !== 'string' || !validBrandName(data.name) || !Number.isSafeInteger(data.revision) || Number(data.revision) < 0 || typeof data.logo_url !== 'string') throw new Error('服务外观配置格式无效。')
  // Only serve installation-owned bytes. Never fetch a user-supplied remote URL.
  if (data.logo_url && !/^\/api\/v1\/mindcreek\/branding\/logo\?v=\d+$/.test(data.logo_url)) throw new Error('服务图标地址无效。')
  return data as ServiceBranding
}
export function brandingInput(revision: number, name: string, logo: string | null | undefined) {
  const trimmed = name.trim()
  if (!validBrandName(trimmed)) throw new Error('服务名称需为 1–64 个字符，不能包含控制字符或尖括号。')
  return { revision, name: trimmed, ...(logo === undefined ? {} : { logo_data: logo }) }
}
export function brandingConflict(value: unknown): boolean {
  const error = value as { response?: { status?: number }; status?: number }
  return (error?.response?.status ?? error?.status) === 409
}
export function brandingProblem(value: unknown): string {
  const error = value as { response?: { status?: number; data?: { error?: { code?: string } } }; status?: number; message?: string; code?: string; error?: { code?: string }; data?: { error?: { code?: string } } }
  const status = error?.response?.status ?? error?.status
  const code = error?.response?.data?.error?.code ?? error?.data?.error?.code ?? error?.code ?? error?.error?.code
  if (code === 'branding.image_invalid') return '图片无法处理，请重新上传有效的 PNG、JPEG 或 WebP 图片。'
  if (code === 'branding.input_invalid') return '请检查服务名称和图片是否符合要求，然后重试。'
  if (code === 'branding.unavailable') return '服务外观暂时不可用，请稍后重试。'
  if (status === 409) return '服务外观已被更新，请重新载入后再保存。'
  if (status === 403 || status === 401) return '仅安装时配置的本地管理员可以修改服务外观，请重新登录该账号。'
  if (status === 413) return '图片过大，请选择不超过 2 MiB 的图片。'
  return error?.message || '无法保存服务外观，请重试。'
}
export const enterpriseSourceOptions = [
  { label: '全部来源', value: '' }, { label: '本地上传', value: 'web' },
  { label: 'URL 导入', value: 'url' }, { label: '手动创建', value: 'manual' }, { label: 'API', value: 'api' },
] as const

export function configuredBrandText(text: string, name: string): string { return text.replaceAll('MindCreek', () => name) }
