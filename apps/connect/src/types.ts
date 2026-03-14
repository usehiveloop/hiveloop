import type { components } from './api/schema'

export type Connection = components['schemas']['connectionResponse']

export type View =
  | { type: 'provider-selection' }
  | { type: 'api-key-input'; providerId: string }
  | { type: 'validating'; providerId: string }
  | { type: 'success'; providerId: string }
  | { type: 'error'; providerId: string }
  | { type: 'connected-list' }
  | { type: 'provider-detail'; connection: Connection }
  | { type: 'revoke-confirm'; connection: Connection }
  | { type: 'revoke-success'; providerId: string }
  | { type: 'empty-state' }

export type ThemeMode = 'light' | 'dark' | 'system'
