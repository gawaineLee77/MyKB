// This marker chooses an endpoint/landing page only. The gateway independently
// verifies the installed admin ID and live native privileges on every request.
const AUTH_KIND = 'mindcreek_auth_kind'
export const isLocalAdminSession = () => localStorage.getItem(AUTH_KIND) === 'local-admin'
export const markLocalAdminSession = () => localStorage.setItem(AUTH_KIND, 'local-admin')
export const markCorporateSession = () => localStorage.setItem(AUTH_KIND, 'corporate')
export const loginPath = () => isLocalAdminSession() ? '/admin/login' : '/login'
export const logoutPath = () => isLocalAdminSession() ? '/admin/login' : '/api/v1/mindcreek/oidc/logout'
export const authEndpoint = (action: 'refresh' | 'logout' | 'change-password') =>
  isLocalAdminSession() ? `/api/v1/mindcreek/admin/auth/${action}` : `/api/v1/auth/${action}`

let mode: Promise<boolean> | undefined
export const enterpriseMode = () => mode ??= fetch('/api/v1/mindcreek/auth/config', { credentials: 'same-origin' })
  .then(async response => response.status === 404 ? false : response.ok ? Boolean((await response.json()).enterprise_enabled) : true)
  .catch(() => true)
