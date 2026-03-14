import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { Connection } from '../types'
import { useProviders } from '../hooks/useProviders'
import { useConnect } from '../hooks/useConnect'
import { createWidgetFetchClient } from '../api/client'
import { Button } from './Button'
import { Footer } from './Footer'
import { WarningIcon } from './icons'

interface Props {
  connection: Connection
  onConfirm: () => void
  onCancel: () => void
}

export function RevokeConfirm({ connection, onConfirm, onCancel }: Props) {
  const { data: providers = [] } = useProviders()
  const { sessionId } = useConnect()
  const queryClient = useQueryClient()
  const provider = providers.find((p) => p.id === connection.provider_id)

  const mutation = useMutation({
    mutationFn: async () => {
      const client = createWidgetFetchClient(sessionId!)
      await client.DELETE('/v1/widget/connections/{id}', {
        params: { path: { id: connection.id! } },
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['widget', 'connections'] })
      onConfirm()
    },
  })

  return (
    <div className="flex flex-col items-center justify-center h-full py-7 px-12 gap-3">
      <div className="flex items-center justify-center rounded-full bg-cw-error-bg shrink-0 size-14">
        <WarningIcon />
      </div>
      <div className="text-xl text-cw-heading font-bold leading-6">
        Revoke {provider?.name ?? connection.provider_name ?? connection.provider_id}?
      </div>
      <div className="text-sm text-center leading-normal text-cw-secondary">
        This will permanently revoke the API key. Any apps using this connection will stop working.
      </div>
      <div className="flex flex-col w-full mt-3 gap-2.5">
        <Button variant="danger" onClick={() => mutation.mutate()} loading={mutation.isPending}>
          Yes, revoke access
        </Button>
        <Button variant="secondary" onClick={onCancel} disabled={mutation.isPending}>
          Cancel
        </Button>
      </div>
      <Footer />
    </div>
  )
}
