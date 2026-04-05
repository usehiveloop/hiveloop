"use client"

import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import Nango, { AuthError } from "@nangohq/frontend"
import { toast } from "sonner"
import { api } from "@/lib/api/client"

export function useConnectIntegration() {
  const queryClient = useQueryClient()
  const [connectingId, setConnectingId] = useState<string | null>(null)

  const mutation = useMutation({
    mutationFn: async (integrationId: string) => {
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

      const authResult = await nango.auth(providerConfigKey)

      const connection = await api.POST("/v1/in/integrations/{id}/connections", {
        params: { path: { id: integrationId } },
        body: { nango_connection_id: authResult.connectionId } as never,
      })

      if (connection.error) throw new Error("Failed to save connection")

      return connection.data
    },
    onMutate: (integrationId) => {
      setConnectingId(integrationId)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/in/connections"] })
      toast.success("Connection added successfully")
    },
    onError: (error) => {
      if (error instanceof AuthError && error.type === "window_closed") return
      toast.error("Connection failed. Please try again.")
    },
    onSettled: () => {
      setConnectingId(null)
    },
  })

  function connect(integrationId: string, onSuccess?: () => void) {
    mutation.mutate(integrationId, { onSuccess })
  }

  return { connect, connectingId }
}
