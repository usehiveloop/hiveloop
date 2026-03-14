import { useReducer, useCallback } from 'react'
import type { View, Connection } from '../types'

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
  | { type: 'VIEW_DETAIL'; connection: Connection }
  | { type: 'REVOKE'; connection: Connection }
  | { type: 'CONFIRM_REVOKE' }
  | { type: 'CONNECT_NEW' }
  | { type: 'VIEW_EMPTY' }

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
  return { view: state.current, direction: state.direction, canGoBack: state.history.length > 0, navigate }
}
