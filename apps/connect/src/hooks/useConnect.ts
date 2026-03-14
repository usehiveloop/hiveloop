import { useContext } from 'react'
import { ConnectContext } from './connectContextDef'

export function useConnect() {
  return useContext(ConnectContext)
}
