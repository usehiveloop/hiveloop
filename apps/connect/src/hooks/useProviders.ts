import { $api, createWidgetApi } from '../api/client'
import type { components } from '../api/schema'
import { useConnect } from './useConnect'

export type ProviderSummary = components['schemas']['internal_handler.providerSummary']

export function useProviders() {
  const { sessionId, preview } = useConnect()

  if (!preview && sessionId) {
    const widgetApi = createWidgetApi(sessionId)
    return widgetApi.useQuery('get', '/v1/widget/providers', undefined, {
      staleTime: 5 * 60 * 1000,
    })
  }

  return $api.useQuery('get', '/v1/providers', undefined, {
    staleTime: 5 * 60 * 1000,
  })
}
