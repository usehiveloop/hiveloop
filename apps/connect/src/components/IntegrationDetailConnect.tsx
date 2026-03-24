import { useEffect } from 'react'
import { useIntegrationProviders } from '../hooks/useIntegrationProviders'
import type { IntegrationProvider } from '../types'
import { Error } from './Error'
import { Footer } from './Footer'
import { SpinnerIcon } from './icons'

interface Props {
  integrationId: string
  onReady: (integration: IntegrationProvider) => void
  onClose: () => void
}

export function IntegrationDetailConnect({ integrationId, onReady, onClose }: Props) {
  const { data: integrations = [], isLoading } = useIntegrationProviders()
  const integration = integrations.find((i) => i.unique_key === integrationId)
  const isConnected = integration?.connection_id != null

  useEffect(() => {
    if (!isLoading && integration && isConnected) {
      onReady(integration)
    }
  }, [isLoading, integration, isConnected]) // eslint-disable-line react-hooks/exhaustive-deps

  if (isLoading || isConnected) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4">
        <SpinnerIcon className="cw-spinner" />
        <Footer />
      </div>
    )
  }

  if (!integrationId || !integration) {
    return (
      <Error
        title="Integration not found"
        message={integrationId
          ? `The integration "${integrationId}" is not available. It may not be configured for this workspace.`
          : 'No integration was specified. Please provide an integrationId parameter.'}
        retryLabel="Close"
        onRetry={onClose}
        onCancel={onClose}
      />
    )
  }

  return (
    <Error
      title="Integration not connected"
      message={`The integration "${integration.display_name || integrationId}" is not currently connected.`}
      retryLabel="Close"
      onRetry={onClose}
      onCancel={onClose}
    />
  )
}
