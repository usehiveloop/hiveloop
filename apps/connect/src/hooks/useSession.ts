import { createWidgetApi } from '../api/client'
import type { components } from '../api/schema'

export type SessionInfo = components['schemas']['sessionInfoResponse']

export function useSession(sessionToken: string | null) {
  const widgetApi = createWidgetApi(sessionToken ?? '')

  return widgetApi.useQuery('get', '/v1/widget/session', undefined, {
    enabled: sessionToken != null,
    retry: false,
    staleTime: Infinity,
  })
}
