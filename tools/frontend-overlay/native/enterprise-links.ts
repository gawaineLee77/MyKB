import { readonly, shallowRef } from 'vue'

export const enterpriseLinkKeys = [
  'help', 'project', 'rbac', 'builtinModels', 'api', 'knowledgeGraph',
  'migration', 'feedback', 'integration', 'sandbox',
] as const
export type EnterpriseLinkKey = typeof enterpriseLinkKeys[number]
export type EnterpriseLinks = Record<EnterpriseLinkKey, string>

function hiddenLinks(): EnterpriseLinks {
  return Object.fromEntries(enterpriseLinkKeys.map(key => [key, ''])) as EnterpriseLinks
}

// Deployment-owned URLs only. Relative links must start at the same origin's
// root; never accept protocol-relative URLs, credentials or executable schemes.
export function safeEnterpriseURL(value: unknown): string {
  if (typeof value !== 'string') return ''
  const url = value.trim()
  if (!url || /[\u0000-\u0020\u007f\\]/u.test(url)) return ''
  if (url.startsWith('/') && !url.startsWith('//')) return url
  try {
    const parsed = new URL(url)
    if (!/^https?:\/\//i.test(url) || !['http:', 'https:'].includes(parsed.protocol)
      || parsed.username || parsed.password) return ''
    return parsed.href
  } catch {
    return ''
  }
}

export function parseEnterpriseLinks(value: unknown): EnterpriseLinks {
  const links = hiddenLinks()
  if (!value || typeof value !== 'object' || Array.isArray(value)) return links
  const config = value as Record<string, unknown>
  if (config.version !== 1 || config.enabled !== true || !config.links
    || typeof config.links !== 'object' || Array.isArray(config.links)) return links
  const input = config.links as Record<string, unknown>
  for (const key of enterpriseLinkKeys) links[key] = safeEnterpriseURL(input[key])
  return links
}

const state = shallowRef<EnterpriseLinks>(hiddenLinks())
export const enterpriseLinks = readonly(state)

export async function loadEnterpriseLinks(fetcher: typeof fetch = fetch): Promise<void> {
  state.value = hiddenLinks()
  const controller = new AbortController()
  let timer: ReturnType<typeof setTimeout> | undefined
  try {
    const config = await Promise.race([
      fetcher('/mindcreek-links.json', {
        cache: 'no-store', credentials: 'same-origin', redirect: 'error', signal: controller.signal,
      }).then(response => response.ok ? response.json() : null),
      new Promise<null>(resolve => {
        timer = setTimeout(() => { controller.abort(); resolve(null) }, 2000)
      }),
    ])
    state.value = parseEnterpriseLinks(config)
  } catch {
    // An absent/malformed config hides links without blocking login or chat.
  } finally {
    clearTimeout(timer)
  }
}

export function openEnterpriseLink(key: EnterpriseLinkKey): void {
  const url = state.value[key]
  if (url) window.open(url, '_blank', 'noopener,noreferrer')
}
