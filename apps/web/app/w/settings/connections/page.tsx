"use client"

import * as React from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { SettingsShell } from "@/components/settings-shell"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { AddConnectionDialog } from "@/app/w/connections/_components/add-connection-dialog"
import { useConnectIntegration } from "@/app/w/connections/_hooks/use-connect-integration"
import { useReconnectIntegration } from "@/app/w/connections/_hooks/use-reconnect-integration"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  Alert02Icon,
  Delete02Icon,
  Loading03Icon,
  MoreHorizontalCircle01Icon,
  Plug01Icon,
  RefreshIcon,
} from "@hugeicons/core-free-icons"
import type { components } from "@/lib/api/schema"

type Connection = components["schemas"]["inConnectionResponse"]

interface ConnectOptions {
  credentials?: Record<string, string>
  params?: Record<string, string>
  installation?: "outbound"
}

function formatDate(iso?: string) {
  if (!iso) return "—"
  return new Date(iso).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

export default function Page() {
  const queryClient = useQueryClient()

  const [addOpen, setAddOpen] = React.useState(false)
  const [dialogSearch, setDialogSearch] = React.useState("")
  const [preSelectedId, setPreSelectedId] = React.useState<string | null>(null)
  const [disconnecting, setDisconnecting] = React.useState<Connection | null>(null)

  const { data, isLoading } = $api.useQuery("get", "/v1/in/connections")
  const connections = data?.data ?? []

  const { connect, connectingId } = useConnectIntegration()
  const { reconnect, reconnectingId } = useReconnectIntegration()

  const deleteConnection = $api.useMutation("delete", "/v1/in/connections/{id}")

  const needsAttention = connections.filter(
    (c) => c.webhook_configured === false
  ).length

  function handleConnect(integrationId: string, options?: ConnectOptions) {
    connect(integrationId, {
      ...options,
      onSuccess: () => {
        setAddOpen(false)
        setDialogSearch("")
        setPreSelectedId(null)
      },
    })
  }

  function handleDisconnect() {
    if (!disconnecting?.id) return
    deleteConnection.mutate(
      { params: { path: { id: disconnecting.id } } },
      {
        onSuccess: () => {
          toast.success(
            `${disconnecting.display_name ?? "Connection"} disconnected`
          )
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/in/connections"],
          })
          setDisconnecting(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to disconnect"))
          setDisconnecting(null)
        },
      }
    )
  }

  return (
    <SettingsShell
      title="Connections"
      description={
        isLoading
          ? "Loading connections…"
          : connections.length === 0
            ? "No connections yet."
            : needsAttention > 0
              ? `${connections.length} connections, ${needsAttention} need attention.`
              : `${connections.length} connection${connections.length !== 1 ? "s" : ""}.`
      }
      action={
        <Button size="sm" className="h-8" onClick={() => setAddOpen(true)}>
          <HugeiconsIcon
            icon={Add01Icon}
            strokeWidth={2}
            className="size-4"
            data-icon="inline-start"
          />
          Add connection
        </Button>
      }
      dividers={false}
    >
      <section>
        {isLoading ? (
          <ul className="divide-y divide-border/60 rounded-lg border border-border/60">
            {Array.from({ length: 3 }).map((_, i) => (
              <li key={i} className="flex items-center gap-3 px-3.5 py-2.5">
                <Skeleton className="size-6 rounded-md" />
                <div className="flex-1 space-y-1.5">
                  <Skeleton className="h-3.5 w-40 rounded" />
                  <Skeleton className="h-3 w-28 rounded" />
                </div>
                <Skeleton className="size-7 rounded-md" />
              </li>
            ))}
          </ul>
        ) : connections.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-border/60 px-6 py-12 text-center">
            <div className="flex size-10 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <HugeiconsIcon
                icon={Plug01Icon}
                strokeWidth={2}
                className="size-4"
              />
            </div>
            <p className="max-w-xs text-[13px] text-muted-foreground">
              No connections yet. Add one to give your agents access to external
              services.
            </p>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setAddOpen(true)}
            >
              <HugeiconsIcon
                icon={Add01Icon}
                strokeWidth={2}
                className="size-4"
                data-icon="inline-start"
              />
              Add connection
            </Button>
          </div>
        ) : (
          <ul className="divide-y divide-border/60 rounded-lg border border-border/60">
            {connections.map((connection) => {
              const isReconnecting = reconnectingId === connection.id
              const isConnecting = connectingId === connection.in_integration_id
              const needsWebhook = connection.webhook_configured === false
              return (
                <li
                  key={connection.id}
                  className="flex items-center gap-3 px-3.5 py-2.5 transition-colors hover:bg-muted/40"
                >
                  <IntegrationLogo
                    provider={connection.provider ?? ""}
                    size={24}
                  />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-[13px] font-medium">
                      {connection.display_name ?? connection.provider}
                    </p>
                    <p className="truncate text-[12px] text-muted-foreground">
                      Connected {formatDate(connection.created_at)}
                    </p>
                  </div>

                  {needsWebhook ? (
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className="inline-flex items-center gap-1 text-[12px] text-amber-600 dark:text-amber-400">
                            <HugeiconsIcon
                              icon={Alert02Icon}
                              strokeWidth={2}
                              className="size-3.5"
                            />
                            Webhook
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>
                          Webhook delivery isn't fully configured for this
                          connection.
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  ) : isReconnecting || isConnecting ? (
                    <span className="inline-flex items-center gap-1 text-[12px] text-muted-foreground">
                      <HugeiconsIcon
                        icon={Loading03Icon}
                        strokeWidth={2}
                        className="size-3.5 animate-spin"
                      />
                      Working
                    </span>
                  ) : (
                    <span className="text-[12px] text-muted-foreground">
                      Connected
                    </span>
                  )}

                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={
                        <button
                          type="button"
                          aria-label={`Actions for ${connection.display_name ?? connection.provider}`}
                          className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground"
                        />
                      }
                    >
                      <HugeiconsIcon
                        icon={MoreHorizontalCircle01Icon}
                        strokeWidth={2}
                        className="size-4"
                      />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="min-w-44">
                      <DropdownMenuItem
                        onClick={() =>
                          connection.id && reconnect(connection.id)
                        }
                        disabled={isReconnecting}
                      >
                        <HugeiconsIcon
                          icon={isReconnecting ? Loading03Icon : RefreshIcon}
                          strokeWidth={2}
                          className={isReconnecting ? "animate-spin" : ""}
                        />
                        Reconnect
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        onClick={() => setDisconnecting(connection)}
                      >
                        <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
                        Disconnect
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </li>
              )
            })}
          </ul>
        )}
      </section>

      <AddConnectionDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        search={dialogSearch}
        onSearchChange={setDialogSearch}
        connectingId={connectingId}
        onConnect={handleConnect}
        preSelectedIntegrationId={preSelectedId}
        onPreSelectedClear={() => setPreSelectedId(null)}
      />

      <ConfirmDialog
        open={disconnecting !== null}
        onOpenChange={(open) => {
          if (!open) setDisconnecting(null)
        }}
        title="Disconnect integration"
        description={`This will remove "${disconnecting?.display_name ?? disconnecting?.provider ?? "this connection"}" and any agents using it will lose access. This action cannot be undone.`}
        confirmText={disconnecting?.display_name ?? disconnecting?.provider ?? ""}
        confirmLabel="Disconnect"
        destructive
        loading={deleteConnection.isPending}
        onConfirm={handleDisconnect}
      />
    </SettingsShell>
  )
}
