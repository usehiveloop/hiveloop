import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { IntegrationProvider } from '../types'
import { useConnect } from '../hooks/useConnect'
import { createWidgetFetchClient } from '../api/client'
import { Button } from './Button'
import { Footer } from './Footer'
import { WarningIcon } from './icons'

interface Props {
  integration: IntegrationProvider
  onConfirm: () => void
  onCancel: () => void
}

export function IntegrationDisconnectConfirm({ integration, onConfirm, onCancel }: Props) {
  const { sessionId } = useConnect()
  const queryClient = useQueryClient()
  const name = integration.display_name || integration.provider || ''

  const mutation = useMutation({
    mutationFn: async () => {
      const client = createWidgetFetchClient(sessionId!)
      await client.DELETE('/v1/widget/integrations/{id}/connections/{connectionId}', {
        params: { path: { id: integration.id!, connectionId: integration.connection_id! } },
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['get', '/v1/widget/integrations'] })
      onConfirm()
    },
  })

  return (
    <div className="flex flex-col items-center justify-center h-full py-7 px-12 gap-3">
      <div className="flex items-center justify-center rounded-full bg-cw-error-bg shrink-0 size-14">
        <WarningIcon />
      </div>
      <div className="text-xl text-cw-heading font-bold leading-6">
        Disconnect {name}?
      </div>
      <div className="text-sm text-center leading-normal text-cw-secondary">
        This will revoke the connection. You can reconnect later if needed.
      </div>
      <div className="flex flex-col w-full mt-3 gap-2.5">
        <Button variant="danger" onClick={() => mutation.mutate()} loading={mutation.isPending}>
          Yes, disconnect
        </Button>
        <Button variant="secondary" onClick={onCancel} disabled={mutation.isPending}>
          Cancel
        </Button>
      </div>
      <Footer />
    </div>
  )
}
