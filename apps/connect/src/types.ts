import type { ConnectedProvider } from './data/providers'

export type View =
  | { type: 'provider-selection' }
  | { type: 'api-key-input'; providerId: string }
  | { type: 'validating'; providerId: string }
  | { type: 'success'; providerId: string }
  | { type: 'error'; providerId: string }
  | { type: 'connected-list' }
  | { type: 'provider-detail'; connection: ConnectedProvider }
  | { type: 'revoke-confirm'; connection: ConnectedProvider }
  | { type: 'empty-state' }

export type ThemeMode = 'light' | 'dark' | 'system'
