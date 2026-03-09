import { useReducer, useCallback } from 'react'
import type { View } from '../types'
import type { ConnectedProvider } from '../data/providers'

type Action =
  | { type: 'SELECT_PROVIDER'; providerId: string }
  | { type: 'SUBMIT_KEY' }
  | { type: 'CONNECTION_SUCCESS' }
  | { type: 'CONNECTION_ERROR' }
  | { type: 'RETRY' }
  | { type: 'DONE' }
  | { type: 'CANCEL' }
  | { type: 'BACK' }
  | { type: 'VIEW_CONNECTIONS' }
  | { type: 'VIEW_DETAIL'; connection: ConnectedProvider }
  | { type: 'REVOKE'; connection: ConnectedProvider }
  | { type: 'CONFIRM_REVOKE' }
  | { type: 'CONNECT_NEW' }
  | { type: 'VIEW_EMPTY' }

type Direction = 'forward' | 'back'

interface State {
  current: View
  history: View[]
  direction: Direction
}

function reducer(state: State, action: Action): State {
  const push = (view: View): State => ({
    current: view,
    history: [...state.history, state.current],
    direction: 'forward',
  })
  const pop = (): State => {
    const history = [...state.history]
    const previous = history.pop()
    return { current: previous ?? { type: 'provider-selection' }, history, direction: 'back' }
  }
  const reset = (view: View): State => ({
    current: view,
    history: [],
    direction: 'forward',
  })

  switch (action.type) {
    case 'SELECT_PROVIDER':
      return push({ type: 'api-key-input', providerId: action.providerId })
    case 'SUBMIT_KEY': {
      const c = state.current
      if (c.type !== 'api-key-input') return state
      return push({ type: 'validating', providerId: c.providerId })
    }
    case 'CONNECTION_SUCCESS': {
      const c = state.current
      if (c.type !== 'validating') return state
      return reset({ type: 'success', providerId: c.providerId })
    }
    case 'CONNECTION_ERROR': {
      const c = state.current
      if (c.type !== 'validating') return state
      return reset({ type: 'error', providerId: c.providerId })
    }
    case 'RETRY': {
      const c = state.current
      if (c.type !== 'error') return state
      return reset({ type: 'api-key-input', providerId: c.providerId })
    }
    case 'DONE':
    case 'CANCEL':
      return reset({ type: 'provider-selection' })
    case 'BACK':
      return pop()
    case 'VIEW_CONNECTIONS':
      return reset({ type: 'connected-list' })
    case 'VIEW_DETAIL':
      return push({ type: 'provider-detail', connection: action.connection })
    case 'REVOKE':
      return push({ type: 'revoke-confirm', connection: action.connection })
    case 'CONFIRM_REVOKE':
      return reset({ type: 'connected-list' })
    case 'CONNECT_NEW':
      return push({ type: 'provider-selection' })
    case 'VIEW_EMPTY':
      return reset({ type: 'empty-state' })
    default:
      return state
  }
}

export type { Action }

export function useWidget(initialView?: View) {
  const [state, dispatch] = useReducer(reducer, {
    current: initialView ?? { type: 'provider-selection' },
    history: [],
    direction: 'forward',
  })
  const navigate = useCallback((action: Action) => dispatch(action), [])
  return { view: state.current, direction: state.direction, navigate }
}
