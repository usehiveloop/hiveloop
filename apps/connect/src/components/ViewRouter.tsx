import { useEffect } from 'react'
import type { View } from '../types'
import type { Action } from '../hooks/useWidget'
import { useParentEvents } from '../hooks/useParentEvents'
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
import { IntegrationResourceSelection } from './IntegrationResourceSelection'
import { IntegrationDetail } from './IntegrationDetail'
import { IntegrationDisconnectConfirm } from './IntegrationDisconnectConfirm'
import { IntegrationSuccess } from './IntegrationSuccess'
import { ResourceSelectionSuccess } from './ResourceSelectionSuccess'

interface Props {
  view: View
  canGoBack: boolean
  navigate: (action: Action) => void
  onClose: () => void
}

export function ViewRouter({ view, canGoBack, navigate, onClose }: Props) {
  const { sendToParent } = useParentEvents()

  // Send error events to parent
  useEffect(() => {
    if (view.type === 'error') {
      sendToParent({
        type: 'error',
        payload: { code: 'connection_failed', message: 'Connection failed', providerId: view.providerId },
      })
    }
    if (view.type === 'integration-error') {
      sendToParent({
        type: 'error',
        payload: { code: 'integration_failed', message: view.error || 'Integration failed' },
      })
    }
  }, [view, sendToParent])

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
          onSuccess={(connectionId) => navigate({ type: 'CONNECTION_SUCCESS', connectionId })}
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
          connectionId={view.connectionId}
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
    case 'integration-resource-selection':
      return (
        <IntegrationResourceSelection
          integration={view.integration}
          connectionId={view.connectionId}
          nangoConnectionId={view.nangoConnectionId}
          navigate={navigate}
          onBack={() => navigate({ type: 'VIEW_INTEGRATION_DETAIL', integration: view.integration })}
          onClose={onClose}
        />
      )
    case 'integration-success':
      return (
        <IntegrationSuccess
          integration={view.integration}
          onDone={() => navigate({ type: 'DONE' })}
          onManage={() => navigate({ type: 'VIEW_INTEGRATION_DETAIL', integration: view.integration })}
        />
      )
    case 'resource-selection-success':
      return (
        <ResourceSelectionSuccess
          integration={view.integration}
          onDone={() => navigate({ type: 'VIEW_INTEGRATION_DETAIL', integration: view.integration })}
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
          onBack={() => navigate({ type: 'VIEW_INTEGRATIONS' })}
          onClose={onClose}
          onSelectResource={(resource) => navigate({ type: 'SELECT_RESOURCE_TYPE', resourceType: resource.type ?? '' })}
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
