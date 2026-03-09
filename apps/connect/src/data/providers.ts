export interface Provider {
  id: string
  name: string
  models: string
  colorClass: string
}

export const providers: Provider[] = [
  { id: 'openai', name: 'OpenAI', models: 'GPT-4o, GPT-4, o1, o3', colorClass: 'bg-cw-provider-openai' },
  { id: 'anthropic', name: 'Anthropic', models: 'Claude 4, Sonnet, Haiku', colorClass: 'bg-cw-provider-anthropic' },
  { id: 'google-gemini', name: 'Google Gemini', models: 'Gemini 2.5, 2.0, 1.5', colorClass: 'bg-cw-provider-google-gemini' },
  { id: 'mistral', name: 'Mistral', models: 'Mistral Large, Medium, Small', colorClass: 'bg-cw-provider-mistral' },
  { id: 'groq', name: 'Groq', models: 'LLaMA 3, Mixtral', colorClass: 'bg-cw-provider-groq' },
  { id: 'deepseek', name: 'DeepSeek', models: 'DeepSeek V3, R1', colorClass: 'bg-cw-provider-deepseek' },
  { id: 'cohere', name: 'Cohere', models: 'Command R+, Embed', colorClass: 'bg-cw-provider-cohere' },
  { id: 'perplexity', name: 'Perplexity', models: 'Sonar, Sonar Pro', colorClass: 'bg-cw-provider-perplexity' },
]

export const popularProviderIds = ['openai', 'anthropic', 'google-gemini']

/** Console URLs where users find their API keys */
export const consoleUrls: Record<string, string> = {
  openai: 'platform.openai.com/api-keys',
  anthropic: 'console.anthropic.com/settings/keys',
  'google-gemini': 'aistudio.google.com/apikey',
  mistral: 'console.mistral.ai/api-keys',
  groq: 'console.groq.com/keys',
  deepseek: 'platform.deepseek.com/api_keys',
  cohere: 'dashboard.cohere.com/api-keys',
  perplexity: 'perplexity.ai/settings/api',
}

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
  { id: '3', providerId: 'google-gemini', label: 'Testing', connectedAt: 'Mar 7, 2026', status: 'revoked', maskedKey: 'AI••••••••3k', requests: 45 },
]
