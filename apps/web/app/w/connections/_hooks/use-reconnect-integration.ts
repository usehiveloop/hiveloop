"use client"

import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import Nango, { AuthError } from "@nangohq/frontend"
import { toast } from "sonner"
import { api } from "@/lib/api/client"
import { extractErrorMessage } from "@/lib/api/error"

export function useReconnectIntegration() {
  const queryClient = useQueryClient()
  const [reconnectingId, setReconnectingId] = useState<string | null>(null)

  const mutation = useMutation({
    mutationFn: async ({ connectionId }: { connectionId: string }) => {
      const session = await api.POST("/v1/in/connections/{id}/reconnect-session", {
        params: { path: { id: connectionId } },
      })

      if (session.error) throw new Error("Failed to create reconnect session")

      const { token, provider_config_key: providerConfigKey } =
        session.data as { token: string; provider_config_key: string }

      const nango = new Nango({
        connectSessionToken: token,
        host: process.env.NEXT_PUBLIC_CONNECTIONS_HOST,
      })

      await nango.reconnect(providerConfigKey)
    },
    onMutate: ({ connectionId }) => {
      setReconnectingId(connectionId)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/in/connections"] })
      toast.success("Connection refreshed successfully")
    },
    onError: (error) => {
      if (error instanceof AuthError && error.type === "window_closed") return
      toast.error(extractErrorMessage(error, "Reconnect failed. Please try again."))
    },
    onSettled: () => {
      setReconnectingId(null)
    },
  })

  function reconnect(connectionId: string) {
    mutation.mutate({ connectionId })
  }

  return { reconnect, reconnectingId }
}
