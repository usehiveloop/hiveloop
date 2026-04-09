"use client"

import { createContext, useContext } from "react"
import type { components } from "@/lib/api/schema"

type ForgeRunResponse = components["schemas"]["forgeGetRunResponse"]
type AgentResponse = components["schemas"]["agentResponse"]

interface ForgeContextValue {
  agent: AgentResponse | undefined
  agentLoading: boolean
  forge: ForgeRunResponse | undefined
  forgeLoading: boolean
}

const ForgeContext = createContext<ForgeContextValue | null>(null)

export function ForgeProvider({
  agent,
  agentLoading,
  forge,
  forgeLoading,
  children,
}: ForgeContextValue & { children: React.ReactNode }) {
  return (
    <ForgeContext.Provider value={{ agent, agentLoading, forge, forgeLoading }}>
      {children}
    </ForgeContext.Provider>
  )
}

export function useForge() {
  const context = useContext(ForgeContext)
  if (!context) {
    throw new Error("useForge must be used within a ForgeProvider")
  }
  return context
}
