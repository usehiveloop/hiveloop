"use client"

import { useCallback, useMemo } from "react"
import { $api } from "@/lib/api/hooks"
import { useConversationStream, type StreamMessage } from "@/hooks/use-conversation-stream"
import { useConversationApprovals } from "@/hooks/use-conversation-approvals"
import { useForge } from "../_context/forge-context"

/**
 * Hook that manages the full context gathering chat flow.
 *
 * Handles three states:
 * 1. Active (gathering_context) — live SSE stream, send messages, show approval
 * 2. Completed (past gathering_context) — load history from events API, read-only
 * 3. No conversation — nothing to show yet
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

  // Load conversation history when context gathering is done.
  const eventsQuery = $api.useQuery(
    "get",
    "/v1/conversations/{convID}/events",
    { params: { path: { convID: contextConversationId ?? "" } } },
    { enabled: isComplete && !!contextConversationId },
  )

  // Parse events into messages for completed conversations.
  const historyMessages: StreamMessage[] = useMemo(() => {
    if (!isComplete || !eventsQuery.data) return []

    const rawEvents = (eventsQuery.data as { data?: Array<{ event_type?: string; event_id?: string; timestamp?: string; data?: unknown }> })?.data ?? []

    const messages: StreamMessage[] = []

    for (const event of rawEvents) {
      const eventType = event.event_type

      // event.data may be a parsed object, a byte array (number[]), or a string.
      let eventData: Record<string, unknown> = {}
      if (event.data && typeof event.data === "object" && !Array.isArray(event.data)) {
        eventData = event.data as Record<string, unknown>
      } else if (Array.isArray(event.data)) {
        try {
          const decoded = String.fromCharCode(...(event.data as number[]))
          eventData = JSON.parse(decoded)
        } catch { /* skip unparseable */ }
      } else if (typeof event.data === "string") {
        try { eventData = JSON.parse(event.data) } catch { /* skip */ }
      }

      if (eventType === "message_received") {
        const content = eventData.content as string
        if (content) {
          messages.push({
            id: event.event_id ?? `msg-${messages.length}`,
            role: "user",
            content,
            timestamp: event.timestamp ?? "",
          })
        }
      } else if (eventType === "response_completed") {
        // full_response may be at top level or nested inside data envelope from webhook.
        let fullResponse = eventData.full_response as string | undefined
        if (!fullResponse && eventData.data && typeof eventData.data === "object") {
          fullResponse = (eventData.data as Record<string, unknown>).full_response as string
        }
        if (fullResponse) {
          messages.push({
            id: event.event_id ?? `resp-${messages.length}`,
            role: "agent",
            content: fullResponse,
            messageId: (eventData.message_id ?? (eventData.data as Record<string, unknown>)?.message_id) as string,
            timestamp: event.timestamp ?? "",
            model: eventData.model as string,
            inputTokens: eventData.input_tokens as number,
            outputTokens: eventData.output_tokens as number,
          })
        }
      }
    }

    return messages
  }, [isComplete, eventsQuery.data])

  // Unified messages — live stream when gathering, history when done.
  const messages = isGathering ? stream.messages : historyMessages

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
    /** Current messages (live or history) */
    messages,
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
    /** Whether history is loading */
    historyLoading: eventsQuery.isLoading,
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
