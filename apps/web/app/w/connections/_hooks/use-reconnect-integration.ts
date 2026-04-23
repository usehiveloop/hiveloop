"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import Nango, { AuthError } from "@nangohq/frontend"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"

export function useReconnectIntegration() {
  const queryClient = useQueryClient()
  const [reconnectingId, setReconnectingId] = useState<string | null>(null)

  // Typed $api mutation sequenced with a Nango reconnect() call — composed
  // rather than wrapped in a raw useMutation + api.POST.
  const createReconnectSession = $api.useMutation(
    "post",
    "/v1/in/connections/{id}/reconnect-session",
  )

  async function reconnect(connectionId: string) {
    setReconnectingId(connectionId)
    try {
      const session = await createReconnectSession.mutateAsync({
        params: { path: { id: connectionId } },
      })

      const { token, provider_config_key: providerConfigKey } =
        session as { token: string; provider_config_key: string }

      const nango = new Nango({
        connectSessionToken: token,
        host: process.env.NEXT_PUBLIC_CONNECTIONS_HOST,
      })

      await nango.reconnect(providerConfigKey)

      queryClient.invalidateQueries({ queryKey: ["get", "/v1/in/connections"] })
      toast.success("Connection refreshed successfully")
    } catch (error) {
      if (error instanceof AuthError && error.type === "window_closed") return
      toast.error(extractErrorMessage(error, "Reconnect failed. Please try again."))
    } finally {
      setReconnectingId(null)
    }
  }

  return { reconnect, reconnectingId }
}
