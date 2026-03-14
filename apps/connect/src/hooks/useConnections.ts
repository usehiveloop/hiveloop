import { useQuery } from '@tanstack/react-query'
import { createWidgetFetchClient } from '../api/client'
import { useConnect } from './useConnect'

export function useConnections() {
  const { sessionId, preview } = useConnect()

  return useQuery({
    queryKey: ['widget', 'connections', sessionId],
    queryFn: async () => {
      const client = createWidgetFetchClient(sessionId!)
      const { data } = await client.GET('/v1/widget/connections')
      return data?.data ?? []
    },
    enabled: !preview && sessionId != null,
  })
}
