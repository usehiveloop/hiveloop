import { http, HttpResponse } from 'msw'

const API_URL = 'https://api.dev.ziraloop.com'

export const mockProviders = [
  { id: 'openai', name: 'OpenAI', doc: 'https://platform.openai.com/api-keys', model_count: 45 },
  { id: 'anthropic', name: 'Anthropic', doc: 'https://console.anthropic.com/keys', model_count: 12 },
  { id: 'google', name: 'Google', doc: 'https://aistudio.google.com/apikey', model_count: 20 },
  { id: 'mistral', name: 'Mistral', model_count: 8 },
  { id: 'cohere', name: 'Cohere', model_count: 5 },
]

export const mockSession = {
  id: 'sess-001',
  identity_id: 'ident-001',
  external_id: 'user-ext-1',
  allowed_integrations: [],
  permissions: [],
  expires_at: new Date(Date.now() + 3600_000).toISOString(),
}

export const mockConnections = [
  {
    id: 'conn-001',
    label: 'Production',
    provider_id: 'openai',
    provider_name: 'OpenAI',
    base_url: 'https://api.openai.com',
    auth_scheme: 'bearer',
    created_at: '2026-03-01T12:00:00Z',
  },
]

export const handlers = [
  // Public providers
  http.get(`${API_URL}/v1/providers`, () => {
    return HttpResponse.json(mockProviders)
  }),

  // Widget providers (session-filtered)
  http.get(`${API_URL}/v1/widget/providers`, () => {
    return HttpResponse.json(mockProviders)
  }),

  // Session validation
  http.get(`${API_URL}/v1/widget/session`, () => {
    return HttpResponse.json(mockSession)
  }),

  // List connections
  http.get(`${API_URL}/v1/widget/connections`, () => {
    return HttpResponse.json({ data: mockConnections, has_more: false })
  }),

  // Create connection
  http.post(`${API_URL}/v1/widget/connections`, async ({ request }) => {
    const body = await request.json() as Record<string, string>
    const provider = mockProviders.find((p) => p.id === body.provider_id)
    return HttpResponse.json(
      {
        id: 'conn-new',
        label: body.label || provider?.name || body.provider_id,
        provider_id: body.provider_id,
        provider_name: provider?.name ?? body.provider_id,
        base_url: 'https://api.example.com',
        auth_scheme: 'bearer',
        created_at: new Date().toISOString(),
      },
      { status: 201 },
    )
  }),

  // Delete connection
  http.delete(`${API_URL}/v1/widget/connections/:id`, () => {
    return HttpResponse.json({ status: 'deleted' })
  }),
]
