import { useReducer, useCallback } from 'react'
import type { View, Connection, IntegrationProvider } from '../types'

type Action =
  | { type: 'SELECT_PROVIDER'; providerId: string }
  | { type: 'SELECT_INTEGRATION_PROVIDER'; integration: IntegrationProvider }
  | { type: 'INTEGRATION_SUCCESS' }
  | { type: 'INTEGRATION_ERROR'; error: string }
  | { type: 'INTEGRATION_REQUIRES_RESOURCE_SELECTION'; connectionId: string; nangoConnectionId: string }
  | { type: 'RESOURCE_SELECTION_COMPLETE' }
  | { type: 'RESOURCE_SELECTION_SKIP' }
  | { type: 'SELECT_RESOURCE_TYPE'; resourceType: string }
  | { type: 'SUBMIT_KEY' }
  | { type: 'CONNECTION_SUCCESS'; connectionId: string }
  | { type: 'CONNECTION_ERROR' }
  | { type: 'RETRY' }
  | { type: 'DONE' }
  | { type: 'CANCEL' }
  | { type: 'BACK' }
  | { type: 'VIEW_CONNECTIONS' }
  | { type: 'VIEW_DETAIL'; connection: Connection }
  | { type: 'REVOKE'; connection: Connection }
  | { type: 'CONFIRM_REVOKE' }
  | { type: 'CONNECT_NEW' }
  | { type: 'VIEW_EMPTY' }
  | { type: 'VIEW_INTEGRATION_DETAIL'; integration: IntegrationProvider }
  | { type: 'DISCONNECT_INTEGRATION'; integration: IntegrationProvider }
  | { type: 'CONFIRM_INTEGRATION_DISCONNECT' }
  | { type: 'VIEW_INTEGRATIONS' }
  | { type: 'PROVIDER_CONNECT_SUBMIT' }
  | { type: 'PROVIDER_CONNECT_SUCCESS'; connectionId: string }
  | { type: 'PROVIDER_CONNECT_ERROR' }
  | { type: 'INTEGRATION_CONNECT_START'; integration: IntegrationProvider }
  | { type: 'INTEGRATION_CONNECT_SUCCESS' }
  | { type: 'INTEGRATION_CONNECT_ERROR'; error: string }

export interface ResourceTypeInfo {
  type?: string
  display_name?: string
  description?: string
  icon?: string
}

type Direction = 'forward' | 'back'

interface State {
  current: View
  history: View[]
  direction: Direction
  returnTo: View
}

function reducer(state: State, action: Action): State {
  const push = (view: View): State => ({
    ...state,
    current: view,
    history: [...state.history, state.current],
    direction: 'forward',
  })
  const pop = (): State => {
    const history = [...state.history]
    const previous = history.pop()
    return { ...state, current: previous ?? state.returnTo, history, direction: 'back' }
  }
  /** Clear history but preserve returnTo. */
  const reset = (view: View): State => ({
    ...state,
    current: view,
    history: [],
    direction: 'forward',
  })
  /** Navigate to a new root and set it as returnTo. */
  const resetHome = (view: View): State => ({
    ...state,
    current: view,
    history: [],
    direction: 'forward',
    returnTo: view,
  })

  switch (action.type) {
    case 'SELECT_PROVIDER':
      return push({ type: 'api-key-input', providerId: action.providerId })
    case 'SELECT_INTEGRATION_PROVIDER':
      return push({ type: 'integration-auth', integration: action.integration })
    case 'INTEGRATION_SUCCESS': {
      const current = state.current
      if (current.type !== 'integration-auth' && current.type !== 'integration-resource-selection') return state
      return reset({ type: 'integration-success', integration: current.integration })
    }
    case 'INTEGRATION_ERROR': {
      const current = state.current
      if (current.type !== 'integration-auth' && current.type !== 'integration-resource-selection') return state
      return reset({ type: 'integration-error', integration: current.integration, error: action.error })
    }
    case 'INTEGRATION_REQUIRES_RESOURCE_SELECTION': {
      const current = state.current
      if (current.type !== 'integration-auth') return state
      return push({
        type: 'integration-resource-selection',
        integration: current.integration,
        connectionId: action.connectionId,
        nangoConnectionId: action.nangoConnectionId,
      })
    }
    case 'RESOURCE_SELECTION_COMPLETE':
    case 'RESOURCE_SELECTION_SKIP': {
      const current = state.current
      if (current.type !== 'integration-resource-selection') return state
      return reset({ type: 'resource-selection-success', integration: current.integration })
    }
    case 'SELECT_RESOURCE_TYPE': {
      const current = state.current
      if (current.type !== 'integration-detail') return state
      // Navigate to resource selection for the selected resource type
      // Requires connection_id and nango_connection_id from the integration
      const integration = current.integration
      if (!integration.connection_id || !integration.nango_connection_id) return state
      return push({
        type: 'integration-resource-selection',
        integration: integration,
        connectionId: integration.connection_id,
        nangoConnectionId: integration.nango_connection_id,
      })
    }
    case 'SUBMIT_KEY': {
      const current = state.current
      if (current.type !== 'api-key-input') return state
      return push({ type: 'validating', providerId: current.providerId })
    }
    case 'CONNECTION_SUCCESS': {
      const current = state.current
      if (current.type !== 'validating') return state
      return reset({ type: 'success', providerId: current.providerId, connectionId: action.connectionId })
    }
    case 'CONNECTION_ERROR': {
      const current = state.current
      if (current.type !== 'validating') return state
      return reset({ type: 'error', providerId: current.providerId })
    }
    case 'RETRY': {
      const current = state.current
      if (current.type !== 'error') return state
      return reset({ type: 'api-key-input', providerId: current.providerId })
    }
    case 'DONE':
    case 'CANCEL':
      return resetHome(state.returnTo)
    case 'BACK':
      return pop()
    case 'VIEW_CONNECTIONS':
      return resetHome({ type: 'connected-list' })
    case 'VIEW_DETAIL':
      return push({ type: 'provider-detail', connection: action.connection })
    case 'REVOKE':
      return push({ type: 'revoke-confirm', connection: action.connection })
    case 'CONFIRM_REVOKE': {
      const c = state.current
      if (c.type !== 'revoke-confirm') return state
      return reset({ type: 'revoke-success', providerId: c.connection.provider_id ?? '' })
    }
    case 'CONNECT_NEW':
      return push({ type: 'provider-selection' })
    case 'VIEW_EMPTY':
      return resetHome({ type: 'empty-state' })
    case 'VIEW_INTEGRATION_DETAIL':
      return push({ type: 'integration-detail', integration: action.integration })
    case 'DISCONNECT_INTEGRATION':
      return push({ type: 'integration-disconnect-confirm', integration: action.integration })
    case 'CONFIRM_INTEGRATION_DISCONNECT': {
      const c = state.current
      if (c.type !== 'integration-disconnect-confirm') return state
      return reset({ type: 'integration-selection' })
    }
    case 'VIEW_INTEGRATIONS':
      return reset({ type: 'integration-selection' })
    case 'PROVIDER_CONNECT_SUBMIT': {
      const current = state.current
      if (current.type !== 'provider-connect') return state
      return push({ type: 'validating', providerId: current.providerId })
    }
    case 'PROVIDER_CONNECT_SUCCESS': {
      const current = state.current
      if (current.type !== 'validating') return state
      return reset({ type: 'success', providerId: current.providerId, connectionId: action.connectionId })
    }
    case 'PROVIDER_CONNECT_ERROR': {
      const current = state.current
      if (current.type !== 'validating') return state
      return reset({ type: 'error', providerId: current.providerId })
    }
    case 'INTEGRATION_CONNECT_START': {
      const current = state.current
      if (current.type !== 'integration-connect') return state
      return push({ type: 'integration-auth', integration: action.integration })
    }
    case 'INTEGRATION_CONNECT_SUCCESS': {
      const current = state.current
      if (current.type !== 'integration-auth') return state
      return reset({ type: 'integration-success', integration: current.integration })
    }
    case 'INTEGRATION_CONNECT_ERROR': {
      const current = state.current
      if (current.type !== 'integration-auth') return state
      return reset({ type: 'integration-error', integration: current.integration, error: action.error })
    }
    default:
      return state
  }
}

export type { Action }

export function useWidget(initialView?: View) {
  const initial = initialView ?? { type: 'provider-selection' }
  const [state, dispatch] = useReducer(reducer, {
    current: initial,
    history: [],
    direction: 'forward',
    returnTo: initial,
  })
  const navigate = useCallback((action: Action) => dispatch(action), [])
  return { view: state.current, direction: state.direction, canGoBack: state.history.length > 0, returnTo: state.returnTo, navigate }
}
