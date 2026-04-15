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
    mutationFn: async ({ integrationId }: { integrationId: string }) => {
      const session = await api.POST("/v1/in/integrations/{id}/connect-session", {
        params: { path: { id: integrationId } },
      })

      if (session.error) throw new Error("Failed to create session")

      const { token, provider_config_key: providerConfigKey } =
        session.data as { token: string; provider_config_key: string }

      const nango = new Nango({
        connectSessionToken: token,
        host: process.env.NEXT_PUBLIC_CONNECTIONS_HOST,
      })

      await nango.reconnect(providerConfigKey)
    },
    onMutate: ({ integrationId }) => {
      setReconnectingId(integrationId)
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

  function reconnect(integrationId: string) {
    mutation.mutate({ integrationId })
  }

  return { reconnect, reconnectingId }
}
