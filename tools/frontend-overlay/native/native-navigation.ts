import { useAuthStore } from '@/stores/auth'
import { destinationForRole } from './native-policy'
export function nativeDestination(path: string, query: Record<string, unknown>) {
  return destinationForRole(useAuthStore().currentTenantRole, path, query.section)
}
