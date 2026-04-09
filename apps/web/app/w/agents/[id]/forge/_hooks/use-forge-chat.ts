"use client"

import { useCallback } from "react"
import { $api } from "@/lib/api/hooks"
import { useConversationStream } from "@/hooks/use-conversation-stream"
import { useConversationApprovals } from "@/hooks/use-conversation-approvals"
import { useForge } from "../_context/forge-context"

/**
 * Hook that manages the context gathering chat flow.
 *
 * Handles two states:
 * 1. Active (gathering_context) — live SSE stream, send messages, show approval
 * 2. Completed (past gathering_context) — read-only, no stream
 */
export function useForgeChat() {
  const { forge } = useForge()

  const runStatus = forge?.run?.status
  const contextConversationId = forge?.run?.context_conversation_id ?? null
  const isGathering = runStatus === "gathering_context"
  const isComplete = !!runStatus && runStatus !== "gathering_context"

  // Live stream — only connect when actively gathering context.
  const stream = useConversationStream(isGathering ? contextConversationId : null)

  // Approvals — only poll when actively gathering context.
  const approvals = useConversationApprovals(isGathering ? contextConversationId : null)

  // Send message mutation.
  const sendMutation = $api.useMutation("post", "/v1/conversations/{convID}/messages")

  // Send a message to the context gatherer.
  const sendMessage = useCallback(
    (content: string) => {
      if (!contextConversationId || !isGathering) return

      stream.addUserMessage(content)
      sendMutation.mutate({
        params: { path: { convID: contextConversationId } },
        body: { content },
      })
    },
    [contextConversationId, isGathering, stream, sendMutation],
  )

  // Find pending start_forge approval.
  const startForgeApproval = approvals.approvals.find(
    (approval) => approval.tool_name === "start_forge",
  )

  // Approve the start_forge tool call.
  const approveForge = useCallback(
    (options?: { onSuccess?: () => void; onError?: (error: string) => void }) => {
      if (!startForgeApproval) return
      approvals.approve(startForgeApproval.id, options)
    },
    [startForgeApproval, approvals],
  )

  return {
    /** Current messages from live stream */
    messages: stream.messages,
    /** Whether SSE stream is connected */
    connected: stream.connected,
    /** Whether stream is connecting */
    connecting: stream.connecting,
    /** Whether the agent is currently streaming a response */
    isStreaming: stream.isStreaming,
    /** Whether we're actively gathering context */
    isGathering,
    /** Whether context gathering is done */
    isComplete,
    /** Send a message to the context gatherer */
    sendMessage,
    /** Pending start_forge approval (if context gatherer called the tool) */
    startForgeApproval,
    /** Approve the start_forge tool call to begin eval design */
    approveForge,
    /** Whether approval is being resolved */
    approving: approvals.resolving,
    /** Stream error */
    error: stream.error,
  }
}
