import { createContext } from 'react'

interface PendingConnection {
  providerId: string
  apiKey: string
  label: string
}

export interface ConnectContextValue {
  sessionId: string | null
  preview: boolean
  pending: PendingConnection | null
  setPending: (pending: PendingConnection | null) => void
}

export const ConnectContext = createContext<ConnectContextValue>({
  sessionId: null,
  preview: true,
  pending: null,
  setPending: () => {},
})
