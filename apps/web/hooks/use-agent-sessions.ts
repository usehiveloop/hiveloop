"use client"

import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"

export type AgentSession = components["schemas"]["conversationResponse"]

/**
 * Hook for listing sessions (conversations) belonging to an agent.
 *
 * Pass `null` to disable the query.
 */
export function useAgentSessions(agentId: string | null) {
  const query = $api.useQuery(
    "get",
    "/v1/agents/{agentID}/conversations",
    { params: { path: { agentID: agentId ?? "" } } },
    { enabled: agentId !== null },
  )

  return {
    sessions: (query.data?.data ?? []) as AgentSession[],
    isLoading: query.isLoading,
    error: query.error,
    refetch: query.refetch,
  }
}
