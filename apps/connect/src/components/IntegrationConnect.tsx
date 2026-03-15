import { useEffect } from 'react'
import { useIntegrationProviders } from '../hooks/useIntegrationProviders'
import type { IntegrationProvider } from '../types'
import { Error } from './Error'
import { Button } from './Button'
import { Footer } from './Footer'
import { IntegrationProviderLogo } from './IntegrationProviderLogo'
import { IconButton } from './IconButton'
import { CloseIcon, SpinnerIcon } from './icons'

interface Props {
  provider: string
  onConnect: (integration: IntegrationProvider) => void
  onViewDetail: (integration: IntegrationProvider) => void
  onClose: () => void
}

export function IntegrationConnect({ provider: providerName, onConnect, onViewDetail, onClose }: Props) {
  const { data: integrations = [], isLoading } = useIntegrationProviders()
  const integration = integrations.find((integration) => integration.unique_key === providerName)
  const name = integration?.display_name || integration?.provider || providerName
  const alreadyConnected = integration?.connection_id != null

  useEffect(() => {
    if (!isLoading && integration && alreadyConnected) {
      onViewDetail(integration)
    }
  }, [isLoading, integration, alreadyConnected]) // eslint-disable-line react-hooks/exhaustive-deps

  if (isLoading || alreadyConnected) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4">
        <SpinnerIcon className="cw-spinner" />
        <Footer />
      </div>
    )
  }

  if (!providerName || !integration) {
    return (
      <Error
        title="Integration not found"
        message={providerName
          ? `The integration "${providerName}" is not available. It may not be configured for this workspace.`
          : 'No integration was specified. Please provide an integrationId parameter.'}
        retryLabel="Close"
        onRetry={onClose}
        onCancel={onClose}
      />
    )
  }

  return (
    <div className="flex flex-col h-full pb-8">
      <div className="flex items-center shrink-0 gap-3">
        <div className="grow text-xl tracking-tight text-cw-heading cw-mobile:font-semibold cw-desktop:font-bold leading-6">
          Connect {name}
        </div>
        <IconButton onClick={onClose} className="cw-mobile:hidden">
          <CloseIcon />
        </IconButton>
      </div>

      <div className="flex flex-col items-center justify-center grow gap-4">
        <IntegrationProviderLogo
          providerName={integration.provider ?? ''}
          className="size-16 rounded-xl"
        />
        <div className="text-base text-cw-heading font-semibold leading-5">{name}</div>
        <div className="text-sm text-center text-cw-secondary leading-normal px-6">
          Connect your {name} account to get started.
        </div>
      </div>

      <Button
        onClick={() => onConnect(integration)}
        className="shrink-0 cw-mobile:rounded-2.5"
      >
        Connect {name}
      </Button>

      <Footer />
    </div>
  )
}
