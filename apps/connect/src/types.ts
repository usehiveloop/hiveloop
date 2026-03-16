import type { components } from './api/schema'

export type Connection = components['schemas']['connectionResponse']

export type IntegrationProvider = components['schemas']['widgetIntegrationResponse']

export type IntegrationResource = components['schemas']['widgetResourceResponse']

export type AvailableResource = components['schemas']['github_com_llmvault_llmvault_internal_resources.AvailableResource']

export type DiscoveryResult = components['schemas']['github_com_llmvault_llmvault_internal_resources.DiscoveryResult']

export type View =
  | { type: 'provider-selection' }
  | { type: 'api-key-input'; providerId: string }
  | { type: 'validating'; providerId: string }
  | { type: 'success'; providerId: string; connectionId: string }
  | { type: 'error'; providerId: string }
  | { type: 'connected-list' }
  | { type: 'provider-detail'; connection: Connection }
  | { type: 'revoke-confirm'; connection: Connection }
  | { type: 'revoke-success'; providerId: string }
  | { type: 'empty-state' }
  | { type: 'integration-selection' }
  | { type: 'integration-auth'; integration: IntegrationProvider }
  | { type: 'integration-resource-selection'; integration: IntegrationProvider; connectionId: string; nangoConnectionId: string }
  | { type: 'integration-success'; integration: IntegrationProvider }
  | { type: 'resource-selection-success'; integration: IntegrationProvider }
  | { type: 'integration-error'; integration: IntegrationProvider; error: string }
  | { type: 'integration-detail'; integration: IntegrationProvider }
  | { type: 'integration-disconnect-confirm'; integration: IntegrationProvider }
  | { type: 'provider-connect'; providerId: string }
  | { type: 'integration-connect'; provider: string }

export type ThemeMode = 'light' | 'dark' | 'system'
