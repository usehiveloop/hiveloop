"use client"

import { useCallback } from "react"
import { toast } from "sonner"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"

// Sends a chat message into a live conversation. Backend returns 202 and the
// response streams via SSE — this hook is fire-and-forget; the
// useConversationEventStream subscriber on the same page handles incoming
// events.
export function useSendConversationMessage(convId: string | null) {
  const queryClient = useQueryClient()
  const mutation = $api.useMutation("post", "/v1/conversations/{convID}/messages")

  const send = useCallback(
    (content: string) =>
      new Promise<boolean>((resolve) => {
        const trimmed = content.trim()
        if (!convId || !trimmed) {
          resolve(false)
          return
        }
        mutation.mutate(
          {
            params: { path: { convID: convId } },
            body: { content: trimmed } as never,
          },
          {
            onSuccess: () => {
              // Refresh the rendered messages list so the user's message
              // appears even before the SSE round-trip lands.
              queryClient.invalidateQueries({
                queryKey: ["get", "/v1/conversations/{convID}/messages", { params: { path: { convID: convId } } }],
              })
              resolve(true)
            },
            onError: (error) => {
              toast.error(extractErrorMessage(error, "Failed to send message"))
              resolve(false)
            },
          },
        )
      }),
    [convId, mutation, queryClient],
  )

  return { send, isSending: mutation.isPending }
}
