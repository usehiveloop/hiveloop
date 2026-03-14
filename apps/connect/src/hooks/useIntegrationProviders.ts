import { createWidgetApi } from '../api/client'
import { useConnect } from './useConnect'

export function useIntegrationProviders() {
  const { sessionId } = useConnect()

  const widgetApi = createWidgetApi(sessionId ?? '')
  return widgetApi.useQuery('get', '/v1/widget/integrations', undefined, {
    enabled: sessionId != null,
    staleTime: 5 * 60 * 1000,
  })
}
