import { useContext } from 'react'
import { ParentEventContext } from './parentEventContextDef'

export function useParentEvents() {
  return useContext(ParentEventContext)
}
