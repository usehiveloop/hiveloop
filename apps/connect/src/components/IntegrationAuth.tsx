import { useEffect, useRef } from 'react'
import type { IntegrationProvider } from '../types'
import type { Action } from '../hooks/useWidget'
import { useNangoAuth } from '../hooks/useNangoAuth'
import { useParentEvents } from '../hooks/useParentEvents'
import { IntegrationProviderLogo } from './IntegrationProviderLogo'
import { Footer } from './Footer'
import { IconButton } from './IconButton'
import { BackIcon, CloseIcon, SpinnerIcon } from './icons'

interface Props {
  integration: IntegrationProvider
  navigate: (action: Action) => void
  onBack?: () => void
  onClose: () => void
}

export function IntegrationAuth({ integration, navigate, onBack, onClose }: Props) {
  const { sendToParent } = useParentEvents()
  const mutation = useNangoAuth(integration.id ?? '', {
    onSuccess: () => {
      sendToParent({
        type: 'integration_success',
        payload: { integrationId: integration.id ?? '', provider: integration.provider ?? '' },
      })
      navigate({ type: 'INTEGRATION_SUCCESS' })
    },
    onError: (error) => navigate({ type: 'INTEGRATION_ERROR', error }),
  })
  const triggered = useRef(false)

  useEffect(() => {
    if (!triggered.current) {
      triggered.current = true
      mutation.mutate()
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="flex flex-col h-full pb-8">
      <div className="flex items-center shrink-0 gap-3">
        {onBack && (
          <IconButton onClick={onBack}>
            <BackIcon />
          </IconButton>
        )}
        <div className="grow text-xl tracking-tight text-cw-heading cw-mobile:font-semibold cw-desktop:font-bold leading-6">
          Connect
        </div>
        <IconButton onClick={onClose}>
          <CloseIcon />
        </IconButton>
      </div>

      <div className="flex flex-col items-center justify-center grow gap-4">
        <IntegrationProviderLogo
          providerName={integration.provider ?? ''}
          className="size-16 rounded-xl"
        />
        <div className="flex items-center gap-2.5">
          <SpinnerIcon className="cw-spinner" />
          <div className="text-sm text-cw-secondary">
            Connecting to {integration.display_name ?? integration.provider}...
          </div>
        </div>
      </div>

      <Footer />
    </div>
  )
}
