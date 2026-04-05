"use client"

import { useMemo } from "react"
import { $api } from "@/lib/api/hooks"
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
import { useConnectIntegration } from "../_hooks/use-connect-integration"

interface AddConnectionDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  search: string
  onSearchChange: (value: string) => void
  connectingId: string | null
  onConnect: (integrationId: string) => void
}

export function AddConnectionDialog({
  open,
  onOpenChange,
  search,
  onSearchChange,
  connectingId,
  onConnect,
}: AddConnectionDialogProps) {
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
          onChange={(event) => onSearchChange(event.target.value)}
          autoFocus
        />

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
                    onClick={() => onConnect(integration.id!)}
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
