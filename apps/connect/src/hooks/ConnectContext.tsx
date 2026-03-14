import { useState, useCallback, type ReactNode } from 'react'
import { ConnectContext } from './connectContextDef'

interface PendingConnection {
  providerId: string
  apiKey: string
  label: string
}

interface ProviderProps {
  sessionId: string | null
  preview: boolean
  children: ReactNode
}

export function ConnectProvider({ sessionId, preview, children }: ProviderProps) {
  const [pending, setPendingState] = useState<PendingConnection | null>(null)
  const setPending = useCallback((p: PendingConnection | null) => setPendingState(p), [])

  return (
    <ConnectContext.Provider value={{ sessionId, preview, pending, setPending }}>
      {children}
    </ConnectContext.Provider>
  )
}
