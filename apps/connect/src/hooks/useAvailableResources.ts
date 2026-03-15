import { useQuery } from '@tanstack/react-query'
import { useConnect } from './useConnect'
import { createWidgetFetchClient } from '../api/client'
import type { DiscoveryResult } from '../types'

export function useAvailableResources(
  integrationId: string,
  resourceType: string,
  nangoConnectionId: string
) {
  const { sessionId } = useConnect()

  return useQuery<DiscoveryResult, string>({
    queryKey: [
      'get',
      '/v1/widget/integrations/{id}/resources/{type}/available',
      integrationId,
      resourceType,
      nangoConnectionId,
    ],
    queryFn: async () => {
      if (!sessionId) {
        throw new Error('No session token available')
      }

      const client = createWidgetFetchClient(sessionId)

      const { data, error } = await client.GET(
        '/v1/widget/integrations/{id}/resources/{type}/available',
        {
          params: {
            path: { id: integrationId, type: resourceType },
            query: { nango_connection_id: nangoConnectionId },
          },
        }
      )

      if (error) {
        throw typeof error === 'string' ? error : 'Failed to fetch resources'
      }

      return data as DiscoveryResult
    },
    enabled: !!sessionId && !!integrationId && !!resourceType && !!nangoConnectionId,
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}
