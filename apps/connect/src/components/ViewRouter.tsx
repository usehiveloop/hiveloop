import type { View } from '../types'
import type { Action } from '../hooks/useWidget'
import { ProviderSelection } from './ProviderSelection'
import { IntegrationProviderSelection } from './IntegrationProviderSelection'
import { ApiKeyInput } from './ApiKeyInput'
import { Validating } from './Validating'
import { Success } from './Success'
import { Error } from './Error'
import { ConnectedList } from './ConnectedList'
import { ProviderDetail } from './ProviderDetail'
import { RevokeConfirm } from './RevokeConfirm'
import { EmptyState } from './EmptyState'
import { IntegrationAuth } from './IntegrationAuth'
import { IntegrationDetail } from './IntegrationDetail'
import { IntegrationDisconnectConfirm } from './IntegrationDisconnectConfirm'

interface Props {
  view: View
  canGoBack: boolean
  navigate: (action: Action) => void
  onClose: () => void
}

export function ViewRouter({ view, canGoBack, navigate, onClose }: Props) {
  switch (view.type) {
    case 'provider-selection':
      return (
        <ProviderSelection
          onSelect={(providerId) => navigate({ type: 'SELECT_PROVIDER', providerId })}
          onBack={canGoBack ? () => navigate({ type: 'BACK' }) : undefined}
          onClose={onClose}
        />
      )
    case 'integration-selection':
      return (
        <IntegrationProviderSelection
          onSelect={(integration) => navigate({ type: 'SELECT_INTEGRATION_PROVIDER', integration })}
          onViewDetail={(integration) => navigate({ type: 'VIEW_INTEGRATION_DETAIL', integration })}
          onBack={canGoBack ? () => navigate({ type: 'BACK' }) : undefined}
          onClose={onClose}
        />
      )
    case 'api-key-input':
      return (
        <ApiKeyInput
          providerId={view.providerId}
          onSubmit={() => navigate({ type: 'SUBMIT_KEY' })}
          onSuccess={() => navigate({ type: 'CONNECTION_SUCCESS' })}
          onError={() => navigate({ type: 'CONNECTION_ERROR' })}
          onBack={() => navigate({ type: 'BACK' })}
          onClose={onClose}
        />
      )
    case 'validating':
      return <Validating providerId={view.providerId} />
    case 'success':
      return (
        <Success
          providerId={view.providerId}
          onDone={() => navigate({ type: 'DONE' })}
        />
      )
    case 'error':
      return (
        <Error
          onRetry={() => navigate({ type: 'RETRY' })}
          onCancel={onClose}
        />
      )
    case 'connected-list':
      return (
        <ConnectedList
          onViewDetail={(connection) => navigate({ type: 'VIEW_DETAIL', connection })}
          onConnectNew={() => navigate({ type: 'CONNECT_NEW' })}
          onClose={onClose}
        />
      )
    case 'provider-detail':
      return (
        <ProviderDetail
          connection={view.connection}
          onRevoke={() => navigate({ type: 'REVOKE', connection: view.connection })}
          onBack={() => navigate({ type: 'BACK' })}
          onClose={onClose}
        />
      )
    case 'revoke-confirm':
      return (
        <RevokeConfirm
          connection={view.connection}
          onConfirm={() => navigate({ type: 'CONFIRM_REVOKE' })}
          onCancel={() => navigate({ type: 'BACK' })}
        />
      )
    case 'revoke-success':
      return (
        <Success
          providerId={view.providerId}
          title="Access revoked"
          message="The API key has been permanently revoked and can no longer be used."
          doneLabel="Back to providers"
          onDone={() => navigate({ type: 'VIEW_CONNECTIONS' })}
        />
      )
    case 'empty-state':
      return (
        <EmptyState
          onConnect={() => navigate({ type: 'CONNECT_NEW' })}
          onClose={onClose}
        />
      )
    case 'integration-auth':
      return (
        <IntegrationAuth
          integration={view.integration}
          navigate={navigate}
          onBack={canGoBack ? () => navigate({ type: 'BACK' }) : undefined}
          onClose={onClose}
        />
      )
    case 'integration-success':
      return (
        <Success
          providerId={view.integration.provider ?? ''}
          title="Connected"
          message={`${view.integration.display_name ?? view.integration.provider} has been connected successfully.`}
          onDone={() => navigate({ type: 'DONE' })}
        />
      )
    case 'integration-error':
      return (
        <Error
          title="Connection failed"
          message={view.error || 'Something went wrong while connecting. Please try again.'}
          onRetry={() => navigate({ type: 'BACK' })}
          onCancel={onClose}
        />
      )
    case 'integration-detail':
      return (
        <IntegrationDetail
          integration={view.integration}
          onDisconnect={() => navigate({ type: 'DISCONNECT_INTEGRATION', integration: view.integration })}
          onBack={() => navigate({ type: 'BACK' })}
          onClose={onClose}
        />
      )
    case 'integration-disconnect-confirm':
      return (
        <IntegrationDisconnectConfirm
          integration={view.integration}
          onConfirm={() => navigate({ type: 'CONFIRM_INTEGRATION_DISCONNECT' })}
          onCancel={() => navigate({ type: 'BACK' })}
        />
      )
  }
}
