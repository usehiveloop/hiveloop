import { createContext } from 'react'

export type ConnectEvent =
  | { type: 'success'; payload: { providerId: string; connectionId: string } }
  | { type: 'integration_success'; payload: { integrationId: string; provider: string } }
  | { type: 'resource_selection'; payload: { integrationId: string; provider: string; resources: Record<string, string[]> } }
  | { type: 'error'; payload: { code: string; message: string; providerId?: string } }
  | { type: 'close' }

export interface ParentEventContextValue {
  sendToParent: (message: ConnectEvent) => void
  isEmbedded: boolean
}

export const ParentEventContext = createContext<ParentEventContextValue>({
  sendToParent: () => {},
  isEmbedded: false,
})
