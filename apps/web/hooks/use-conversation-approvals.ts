"use client"

import { useCallback } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"

/**
 * Hook for managing tool approval requests in a conversation.
 *
 * Returns handlers to list, approve, and deny pending tool approvals.
 * The approve/deny handlers optimistically update the UI and handle errors.
 */
export function useConversationApprovals(conversationId: string | null) {
  const queryClient = useQueryClient()

  const approvalsQuery = $api.useQuery(
    "get",
    "/v1/conversations/{convID}/approvals",
    { params: { path: { convID: conversationId ?? "" } } },
    { enabled: conversationId !== null, refetchInterval: 3000 },
  )

  const resolveMutation = $api.useMutation(
    "post",
    "/v1/conversations/{convID}/approvals/{requestID}",
  )

  const approve = useCallback(
    (requestId: string, options?: { onSuccess?: () => void; onError?: (error: string) => void }) => {
      if (!conversationId) return

      resolveMutation.mutate(
        {
          params: { path: { convID: conversationId, requestID: requestId } },
          body: { decision: "approve" },
        },
        {
          onSuccess: () => {
            queryClient.invalidateQueries({
              queryKey: ["get", "/v1/conversations/{convID}/approvals"],
            })
            options?.onSuccess?.()
          },
          onError: (error) => {
            options?.onError?.(extractErrorMessage(error, "Failed to approve"))
          },
        },
      )
    },
    [conversationId, resolveMutation, queryClient],
  )

  const deny = useCallback(
    (requestId: string, options?: { onSuccess?: () => void; onError?: (error: string) => void }) => {
      if (!conversationId) return

      resolveMutation.mutate(
        {
          params: { path: { convID: conversationId, requestID: requestId } },
          body: { decision: "deny" },
        },
        {
          onSuccess: () => {
            queryClient.invalidateQueries({
              queryKey: ["get", "/v1/conversations/{convID}/approvals"],
            })
            options?.onSuccess?.()
          },
          onError: (error) => {
            options?.onError?.(extractErrorMessage(error, "Failed to deny"))
          },
        },
      )
    },
    [conversationId, resolveMutation, queryClient],
  )

  return {
    /** Pending approval requests */
    approvals: (approvalsQuery.data ?? []) as {
      id: string
      tool_name: string
      arguments: unknown
      status: string
      created_at: string
    }[],
    /** Whether approvals are loading */
    loading: approvalsQuery.isLoading,
    /** Whether a resolve (approve/deny) is in progress */
    resolving: resolveMutation.isPending,
    /** Approve a pending tool call */
    approve,
    /** Deny a pending tool call */
    deny,
  }
}
