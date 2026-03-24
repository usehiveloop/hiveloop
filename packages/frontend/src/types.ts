export type ThemeOption = 'light' | 'dark' | 'system'

export type ConnectScreen =
  | 'provider-selection'
  | 'integration-selection'
  | 'connected-list'
  | 'provider-connect'
  | 'integration-connect'
  | 'integration-detail'

export type ConnectErrorCode =
  | 'session_invalid'
  | 'session_expired'
  | 'connection_failed'
  | 'integration_failed'
  | 'unknown_error'

export type ConnectEvent =
  | { type: 'success'; payload: SuccessPayload }
  | { type: 'integration_success'; payload: IntegrationSuccessPayload }
  | { type: 'resource_selection'; payload: ResourceSelectionPayload }
  | { type: 'error'; payload: ErrorPayload }
  | { type: 'close' }

export interface SuccessPayload {
  providerId: string
  connectionId: string
}

export interface IntegrationSuccessPayload {
  integrationId: string
  connectionId: string
}

export interface ResourceSelectionPayload {
  integrationId: string
  resources: Record<string, string[]>
}

export interface ErrorPayload {
  code: ConnectErrorCode
  message: string
  providerId?: string
}

export interface LLMVaultConnectConfig {
  baseURL?: string
  theme?: ThemeOption
}

export interface ConnectOpenOptions {
  sessionToken: string
  screen?: ConnectScreen
  providerId?: string
  integrationId?: string
  preview?: boolean
  onSuccess?: (payload: SuccessPayload) => void
  onIntegrationSuccess?: (payload: IntegrationSuccessPayload) => void
  onResourceSelection?: (payload: ResourceSelectionPayload) => void
  onError?: (payload: ErrorPayload) => void
  onClose?: () => void
  onEvent?: (event: ConnectEvent) => void
}
