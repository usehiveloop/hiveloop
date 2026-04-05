"use client"

import { useState, useMemo, useCallback } from "react"
import Nango, { AuthError } from "@nangohq/frontend"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { HugeiconsIcon } from "@hugeicons/react"
import { Loading03Icon, ArrowRight01Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import Image from "next/image"

export function AddConnectionDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [search, setSearch] = useState("")
  const [connectingId, setConnectingId] = useState<string | null>(null)
  const [error, setError] = useState("")

  const { data, isLoading } = $api.useQuery(
    "get",
    "/v1/in/integrations/available",
    {},
    { enabled: open },
  )

  const { data: connectionsData } = $api.useQuery(
    "get",
    "/v1/in/connections",
    {},
    { enabled: open },
  )

  const connectedIntegrationIds = useMemo(() => {
    const connections = connectionsData?.data ?? []
    return new Set(connections.map((connection) => connection.in_integration_id))
  }, [connectionsData])

  const integrations = data ?? []

  const filtered = useMemo(() => {
    if (!search.trim()) return integrations
    const query = search.toLowerCase()
    return integrations.filter(
      (integration) =>
        (integration.display_name ?? "").toLowerCase().includes(query) ||
        (integration.provider ?? "").toLowerCase().includes(query),
    )
  }, [integrations, search])

  const handleConnect = useCallback(async (integrationId: string) => {
    setConnectingId(integrationId)
    setError("")

    try {
      const sessionResponse = await api.POST(
        "/v1/in/integrations/{id}/connect-session",
        { params: { path: { id: integrationId } } },
      )

      if (sessionResponse.error) {
        setError("Failed to start connection")
        setConnectingId(null)
        return
      }

      const { token, provider_config_key: providerConfigKey } =
        sessionResponse.data as { token: string; provider_config_key: string }

      const nango = new Nango({
        connectSessionToken: token,
        host: process.env.NEXT_PUBLIC_CONNECTIONS_HOST,
      })

      const authResult = await nango.auth(providerConfigKey)

      await api.POST("/v1/in/integrations/{id}/connections", {
        params: { path: { id: integrationId } },
        body: { nango_connection_id: authResult.connectionId } as never,
      })

      onOpenChange(false)
      setSearch("")
      setConnectingId(null)
    } catch (thrown) {
      if (thrown instanceof AuthError && thrown.type === "window_closed") {
        setConnectingId(null)
        return
      }
      setError("Connection failed. Please try again.")
      setConnectingId(null)
    }
  }, [onOpenChange])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Add connection</DialogTitle>
          <DialogDescription>
            Choose an integration to connect to your workspace.
          </DialogDescription>
        </DialogHeader>

        <Input
          placeholder="Search integrations..."
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          autoFocus
        />

        {error && (
          <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            {error}
          </div>
        )}

        <ScrollArea className="h-80">
          {isLoading ? (
            <div className="flex flex-col gap-2 pr-4">
              {Array.from({ length: 6 }).map((_, index) => (
                <Skeleton key={index} className="h-14 w-full rounded-xl" />
              ))}
            </div>
          ) : filtered.length === 0 ? (
            <div className="flex items-center justify-center h-full">
              <p className="text-sm text-muted-foreground">
                {search ? "No integrations found." : "No integrations available."}
              </p>
            </div>
          ) : (
            <div className="flex flex-col gap-1">
              {filtered.map((integration) => {
                const isConnecting = connectingId === integration.id
                const isConnected = connectedIntegrationIds.has(integration.id)
                const isDisabled = isConnected || connectingId !== null
                return (
                  <button
                    key={integration.id}
                    type="button"
                    disabled={isDisabled}
                    className="flex items-center gap-3 rounded-xl px-3 py-3 text-left transition-colors hover:bg-muted cursor-pointer disabled:cursor-not-allowed"
                    onClick={() => handleConnect(integration.id!)}
                  >
                    {integration.nango_config?.logo ? (
                      <Image
                        src={integration.nango_config.logo}
                        alt={integration.display_name || "app connection"}
                        className="size-6 rounded-lg object-contain"
                        width={24}
                        height={24}
                      />
                    ) : (
                      <div className="size-8 rounded-lg bg-muted flex items-center justify-center text-xs font-medium text-muted-foreground">
                        {(integration.display_name ?? "?").charAt(0).toUpperCase()}
                      </div>
                    )}
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium truncate">
                        {integration.display_name}
                      </p>
                    </div>
                    {isConnected ? (
                      <HugeiconsIcon icon={Tick02Icon} className="size-4 text-green-500" />
                    ) : isConnecting ? (
                      <HugeiconsIcon icon={Loading03Icon} className="size-4 animate-spin text-muted-foreground" />
                    ) : (
                      <HugeiconsIcon icon={ArrowRight01Icon} className="size-4 text-muted-foreground" />
                    )}
                  </button>
                )
              })}
            </div>
          )}
        </ScrollArea>
      </DialogContent>
    </Dialog>
  )
}
