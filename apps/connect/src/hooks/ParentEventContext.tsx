import type { ReactNode } from 'react'
import { ParentEventContext } from './parentEventContextDef'
import type { ConnectEvent } from './parentEventContextDef'

interface Props {
  sendToParent: (message: ConnectEvent) => void
  isEmbedded: boolean
  children: ReactNode
}

export function ParentEventProvider({ sendToParent, isEmbedded, children }: Props) {
  return (
    <ParentEventContext.Provider value={{ sendToParent, isEmbedded }}>
      {children}
    </ParentEventContext.Provider>
  )
}
