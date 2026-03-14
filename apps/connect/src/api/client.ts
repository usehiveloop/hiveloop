import createFetchClient, { type Middleware } from 'openapi-fetch'
import createClient from 'openapi-react-query'
import type { paths } from './schema'

const API_URL = import.meta.env.VITE_API_URL || 'https://api.dev.llmvault.dev'

let onUnauthorized: (() => void) | null = null

export function setOnUnauthorized(cb: (() => void) | null) {
  onUnauthorized = cb
}

const throwOnError: Middleware = {
  async onResponse({ response }) {
    if (!response.ok) {
      if (response.status === 401) {
        onUnauthorized?.()
      }
      const body = await response.clone().json().catch(() => ({}))
      throw new Error(body.error ?? `${response.status} ${response.statusText}`)
    }
  },
}

const fetchClient = createFetchClient<paths>({ baseUrl: API_URL })
fetchClient.use(throwOnError)

/** React Query client (for use in components) */
export const $api = createClient(fetchClient)

/** Raw openapi-fetch client (for server-side or non-hook usage) */
export { fetchClient }

export function createWidgetFetchClient(sessionToken: string) {
  const client = createFetchClient<paths>({
    baseUrl: API_URL,
    headers: { Authorization: `Bearer ${sessionToken}` },
  })
  client.use(throwOnError)
  return client
}

export function createWidgetApi(sessionToken: string) {
  return createClient(createWidgetFetchClient(sessionToken))
}
