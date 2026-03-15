import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useConnect } from './useConnect'
import { createWidgetFetchClient } from '../api/client'

export function useUpdateConnectionResources(
  connectionId: string,
  integrationId: string,
  callbacks: {
    onSuccess: () => void
    onError: (error: string) => void
  }
) {
  const { sessionId } = useConnect()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (resources: Record<string, string[]>) => {
      if (!sessionId) {
        throw new Error('No session token available')
      }

      const client = createWidgetFetchClient(sessionId)

      const { error } = await client.PATCH(
        '/v1/widget/integrations/{id}/connections/{connectionId}',
        {
          params: {
            path: { id: integrationId, connectionId },
          },
          body: { resources },
        }
      )

      if (error) {
        throw typeof error === 'string' ? error : 'Failed to update resources'
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['get', '/v1/widget/integrations'] })
      callbacks.onSuccess()
    },
    onError: (err) => {
      callbacks.onError(err instanceof Error ? err.message : 'Failed to save resources')
    },
  })
}
