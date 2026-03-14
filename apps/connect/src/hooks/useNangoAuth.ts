import { useMutation, useQueryClient } from '@tanstack/react-query'
import Nango from '@nangohq/frontend'
import { useConnect } from './useConnect'
import { createWidgetFetchClient } from '../api/client'

const INTEGRATIONS_API = import.meta.env.VITE_INTEGRATIONS_API || 'https://integrations.dev.llmvault.dev'

export function useNangoAuth(integrationId: string, callbacks: {
  onSuccess: () => void
  onError: (error: string) => void
}) {
  const { sessionId } = useConnect()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async () => {
      if (!sessionId) {
        throw new Error('No session token available')
      }

      const client = createWidgetFetchClient(sessionId)

      // Step 1: Get connect session token
      const { data: sessionData } = await client.POST(
        '/v1/widget/integrations/{id}/connect-session',
        { params: { path: { id: integrationId } } }
      )

      const token = sessionData?.token
      const providerConfigKey = sessionData?.provider_config_key
      if (!token || !providerConfigKey) {
        throw new Error('Failed to create connect session')
      }

      // Step 2: Initialize Nango and trigger auth
      const nango = new Nango({ connectSessionToken: token, host: INTEGRATIONS_API })
      const result = await nango.auth(providerConfigKey, { detectClosedAuthWindow: true })

      // Step 3: Store the connection
      await client.POST('/v1/widget/integrations/{id}/connections', {
        params: { path: { id: integrationId } },
        body: { nango_connection_id: result.connectionId },
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['get', '/v1/widget/integrations'] })
      callbacks.onSuccess()
    },
    onError: (err) => {
      callbacks.onError(err instanceof Error ? err.message : 'Connection failed')
    },
  })
}
