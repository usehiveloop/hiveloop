import type { components } from './api/schema'

export type Connection = components['schemas']['connectionResponse']

export type IntegrationProvider = components['schemas']['widgetIntegrationResponse']

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
  | { type: 'integration-selection' }
  | { type: 'integration-auth'; integration: IntegrationProvider }
  | { type: 'integration-success'; integration: IntegrationProvider }
  | { type: 'integration-error'; integration: IntegrationProvider; error: string }
  | { type: 'integration-detail'; integration: IntegrationProvider }
  | { type: 'integration-disconnect-confirm'; integration: IntegrationProvider }

export type ThemeMode = 'light' | 'dark' | 'system'
