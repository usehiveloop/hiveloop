"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import Nango, { AuthError } from "@nangohq/frontend"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"

interface ConnectOptions {
  credentials?: Record<string, string>
  params?: Record<string, string>
  installation?: "outbound"
}

export function useConnectIntegration() {
  const queryClient = useQueryClient()
  const [connectingId, setConnectingId] = useState<string | null>(null)

  // Two typed $api mutations are composed into a single connect flow: create a
  // Nango connect-session, run the Nango auth popup, then persist the resulting
  // connection. mutateAsync lets us sequence them with the external SDK call in
  // between without falling back to a raw useMutation + api.POST.
  const createSession = $api.useMutation(
    "post",
    "/v1/in/integrations/{id}/connect-session",
  )
  const saveConnection = $api.useMutation(
    "post",
    "/v1/in/integrations/{id}/connections",
  )

  async function connect(
    integrationId: string,
    optionsOrOnSuccess?: ConnectOptions & { onSuccess?: () => void } | (() => void),
  ) {
    let options: ConnectOptions | undefined
    let onSuccess: (() => void) | undefined

    if (typeof optionsOrOnSuccess === "function") {
      onSuccess = optionsOrOnSuccess
    } else if (optionsOrOnSuccess) {
      const { onSuccess: onSuccessFn, ...rest } = optionsOrOnSuccess
      onSuccess = onSuccessFn
      if (Object.keys(rest).length > 0) options = rest
    }

    setConnectingId(integrationId)
    try {
      const session = await createSession.mutateAsync({
        params: { path: { id: integrationId } },
      })

      const { token, provider_config_key: providerConfigKey } =
        session as { token: string; provider_config_key: string }

      const nango = new Nango({
        connectSessionToken: token,
        host: process.env.NEXT_PUBLIC_CONNECTIONS_HOST,
      })

      const authOptions: Record<string, unknown> = {}
      if (options?.credentials) authOptions.credentials = options.credentials
      if (options?.params) authOptions.params = options.params
      if (options?.installation) authOptions.installation = options.installation

      const authResult = Object.keys(authOptions).length > 0
        ? await nango.auth(providerConfigKey, authOptions)
        : await nango.auth(providerConfigKey)

      await saveConnection.mutateAsync({
        params: { path: { id: integrationId } },
        body: { nango_connection_id: authResult.connectionId } as never,
      })

      queryClient.invalidateQueries({ queryKey: ["get", "/v1/in/connections"] })
      toast.success("Connection added successfully")
      onSuccess?.()
    } catch (error) {
      if (error instanceof AuthError && error.type === "window_closed") return
      toast.error(extractErrorMessage(error, "Connection failed. Please try again."))
    } finally {
      setConnectingId(null)
    }
  }

  return { connect, connectingId }
}
