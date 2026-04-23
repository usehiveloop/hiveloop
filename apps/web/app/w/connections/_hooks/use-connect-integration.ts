"use client"

import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import Nango, { AuthError } from "@nangohq/frontend"
import { toast } from "sonner"
import { api } from "@/lib/api/client"
import { extractErrorMessage } from "@/lib/api/error"

interface ConnectOptions {
  credentials?: Record<string, string>
  params?: Record<string, string>
  installation?: "outbound"
}

export function useConnectIntegration() {
  const queryClient = useQueryClient()
  const [connectingId, setConnectingId] = useState<string | null>(null)

  const mutation = useMutation({
    mutationFn: async ({
      integrationId,
      options,
    }: {
      integrationId: string
      options?: ConnectOptions
    }) => {
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

      const authOptions: Record<string, unknown> = {}
      if (options?.credentials) authOptions.credentials = options.credentials
      if (options?.params) authOptions.params = options.params
      if (options?.installation) authOptions.installation = options.installation

      const authResult = Object.keys(authOptions).length > 0
        ? await nango.auth(providerConfigKey, authOptions)
        : await nango.auth(providerConfigKey)

      const connection = await api.POST("/v1/in/integrations/{id}/connections", {
        params: { path: { id: integrationId } },
        body: { nango_connection_id: authResult.connectionId },
      })

      if (connection.error) throw new Error("Failed to save connection")

      return connection.data
    },
    onMutate: ({ integrationId }) => {
      setConnectingId(integrationId)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/in/connections"] })
      toast.success("Connection added successfully")
    },
    onError: (error) => {
      if (error instanceof AuthError && error.type === "window_closed") return
      toast.error(extractErrorMessage(error, "Connection failed. Please try again."))
    },
    onSettled: () => {
      setConnectingId(null)
    },
  })

  function connect(
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

    mutation.mutate({ integrationId, options }, { onSuccess })
  }

  return { connect, connectingId }
}
