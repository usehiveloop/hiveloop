export const popularProviderIds = ['openai', 'anthropic', 'google']

export interface ConnectedProvider {
  id: string
  providerId: string
  label: string
  connectedAt: string
  status: 'active' | 'revoked'
  maskedKey: string
  requests: number
}

export const mockConnectedProviders: ConnectedProvider[] = [
  { id: '1', providerId: 'openai', label: 'Production', connectedAt: 'Mar 2, 2026', status: 'active', maskedKey: 'sk-••••••••Mx', requests: 1247 },
  { id: '2', providerId: 'anthropic', label: 'Team API Key', connectedAt: 'Mar 5, 2026', status: 'active', maskedKey: 'sk-ant-••••••Wq', requests: 892 },
  { id: '3', providerId: 'google', label: 'Testing', connectedAt: 'Mar 7, 2026', status: 'revoked', maskedKey: 'AI••••••••3k', requests: 45 },
]
