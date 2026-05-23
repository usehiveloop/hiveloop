"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  CheckmarkCircle02Icon,
  Delete02Icon,
  MoreHorizontalIcon,
  Plug01Icon,
  RefreshIcon,
  Search01Icon,
} from "@hugeicons/core-free-icons"
import { CredentialsForm } from "@/app/w-old/connections/_components/credentials-form"
import { useConnectIntegration } from "@/app/w-old/connections/_hooks/use-connect-integration"
import { useReconnectIntegration } from "@/app/w-old/connections/_hooks/use-reconnect-integration"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { IntegrationLogo } from "@/components/integration-logo"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent } from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

type Integration = components["schemas"]["inIntegrationAvailableResponse"]
type Connection = components["schemas"]["inConnectionResponse"]
type ConnectionConfigField = components["schemas"]["ConnectionConfigField"]

interface ConnectOptions {
  credentials?: Record<string, string>
  params?: Record<string, string>
  installation?: "outbound"
}

function needsConnectionForm(integration: Integration): boolean {
  const authMode = integration.nango_config?.auth_mode
  const installation = integration.nango_config?.installation
  if (authMode === "API_KEY" || authMode === "BASIC") return true
  if (installation === "outbound") return true

  const connectionConfig = integration.nango_config?.connection_config as
    | Record<string, ConnectionConfigField>
    | undefined

  return Boolean(
    connectionConfig &&
    Object.values(connectionConfig).some((field) => !field.automated)
  )
}

function providerLabel(integration: Integration): string {
  return integration.display_name ?? integration.provider ?? "Integration"
}

export default function ConnectionsPage() {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState("")
  const [formIntegration, setFormIntegration] = useState<Integration | null>(
    null
  )
  const [disconnecting, setDisconnecting] = useState<Connection | null>(null)

  const integrationsQuery = $api.useQuery(
    "get",
    "/v1/in/integrations/available"
  )
  const connectionsQuery = $api.useQuery("get", "/v1/in/connections")
  const deleteConnection = $api.useMutation("delete", "/v1/in/connections/{id}")
  const { connect, connectingId } = useConnectIntegration()
  const { reconnect, reconnectingId } = useReconnectIntegration()

  const integrations = integrationsQuery.data ?? []
  const connections = connectionsQuery.data?.data ?? []

  const connectionsByIntegrationId = useMemo(() => {
    const map = new Map<string, Connection>()
    for (const connection of connections) {
      if (connection.in_integration_id) {
        map.set(connection.in_integration_id, connection)
      }
    }
    return map
  }, [connections])

  const filteredIntegrations = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return integrations

    return integrations.filter((integration) => {
      return (
        providerLabel(integration).toLowerCase().includes(query) ||
        (integration.provider ?? "").toLowerCase().includes(query)
      )
    })
  }, [integrations, search])

  const isLoading = integrationsQuery.isLoading || connectionsQuery.isLoading

  function handleConnect(integration: Integration, options?: ConnectOptions) {
    if (!integration.id) return
    connect(integration.id, {
      ...options,
      onSuccess: () => setFormIntegration(null),
    })
  }

  function handleCardAction(integration: Integration) {
    const connection = integration.id
      ? connectionsByIntegrationId.get(integration.id)
      : undefined
    if (connection) return
    if (needsConnectionForm(integration)) {
      setFormIntegration(integration)
      return
    }
    handleConnect(integration)
  }

  function handleFormSubmit(
    credentials: Record<string, string> | undefined,
    params: Record<string, string>,
    installation?: "outbound"
  ) {
    const integration = formIntegration
    if (!integration?.id) return

    const options: ConnectOptions = {}
    if (credentials) options.credentials = credentials
    if (Object.keys(params).length > 0) options.params = params
    if (installation) options.installation = installation

    handleConnect(
      integration,
      Object.keys(options).length > 0 ? options : undefined
    )
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
    <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-7">
      <div className="flex flex-col gap-5">
        <div className="max-w-2xl">
          <h1 className="font-heading text-3xl font-normal tracking-[-0.02em] text-foreground md:text-4xl">
            Connections
          </h1>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            Connect the tools Hivy can work with across your workspace.
          </p>
        </div>

        <div className="relative">
          <HugeiconsIcon
            icon={Search01Icon}
            className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search integrations"
            className="h-11 rounded-md bg-card pl-9"
          />
        </div>
      </div>

      {isLoading ? (
        <IntegrationSkeletonGrid />
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {filteredIntegrations.map((integration) => {
            const connection = integration.id
              ? connectionsByIntegrationId.get(integration.id)
              : undefined
            const isConnected = Boolean(connection)
            const isConnecting = connectingId === integration.id
            const isReconnecting = reconnectingId === connection?.id
            const isBusy = isConnecting || isReconnecting
            const label = providerLabel(integration)

            return (
              <div
                key={integration.id ?? integration.provider ?? label}
                className={cn(
                  "group relative flex min-h-18 items-center gap-3 rounded-md border border-border bg-card p-4 text-left transition-colors hover:border-muted-foreground/25 hover:bg-muted/20",
                  !isConnected &&
                    "cursor-pointer focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/30"
                )}
                role={isConnected ? undefined : "button"}
                tabIndex={isConnected ? undefined : 0}
                onClick={() => handleCardAction(integration)}
                onKeyDown={(event) => {
                  if (isConnected) return
                  if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault()
                    handleCardAction(integration)
                  }
                }}
              >
                <IntegrationLogo
                  provider={integration.provider ?? ""}
                  size={32}
                />

                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 items-center gap-2">
                    <h2 className="truncate text-sm font-medium text-foreground">
                      {label}
                    </h2>
                    {isConnected ? (
                      <HugeiconsIcon
                        icon={CheckmarkCircle02Icon}
                        className="size-4 ml-2 shrink-0 text-emerald-600"
                        aria-label="Connected"
                      />
                    ) : null}
                  </div>
                </div>

                {isConnected ? (
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      className="flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30 focus-visible:outline-none"
                      aria-label={`${label} options`}
                      onClick={(event) => event.stopPropagation()}
                    >
                      <HugeiconsIcon
                        icon={MoreHorizontalIcon}
                        className="size-4"
                      />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" sideOffset={6}>
                      <DropdownMenuItem
                        disabled={isReconnecting}
                        onClick={(event) => {
                          event.stopPropagation()
                          if (connection?.id) reconnect(connection.id)
                        }}
                      >
                        <HugeiconsIcon
                          icon={RefreshIcon}
                          className={cn(
                            "size-4 text-muted-foreground",
                            isReconnecting && "animate-spin"
                          )}
                        />
                        Reconnect
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        variant="destructive"
                        disabled={deleteConnection.isPending}
                        onClick={(event) => {
                          event.stopPropagation()
                          if (connection) setDisconnecting(connection)
                        }}
                      >
                        <HugeiconsIcon icon={Delete02Icon} className="size-4" />
                        Delete connection
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                ) : (
                  <Button
                    type="button"
                    variant="secondary"
                    loading={isBusy}
                    disabled={connectingId !== null}
                    onClick={(event) => {
                      event.stopPropagation()
                      handleCardAction(integration)
                    }}
                  >
                    Connect
                  </Button>
                )}
              </div>
            )
          })}
        </div>
      )}

      {!isLoading && filteredIntegrations.length === 0 ? (
        <div className="flex h-40 flex-col items-center justify-center gap-2 rounded-md border border-dashed border-border text-sm text-muted-foreground">
          <HugeiconsIcon icon={Plug01Icon} className="size-5" />
          No integrations found
        </div>
      ) : null}

      <Dialog
        open={formIntegration !== null}
        onOpenChange={(open) => {
          if (!open) setFormIntegration(null)
        }}
      >
        <DialogContent className="sm:max-w-md">
          {formIntegration ? (
            <CredentialsForm
              integration={formIntegration}
              onSubmit={handleFormSubmit}
              onBack={() => setFormIntegration(null)}
              isSubmitting={connectingId === formIntegration.id}
            />
          ) : null}
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={disconnecting !== null}
        onOpenChange={(open) => {
          if (!open) setDisconnecting(null)
        }}
        title="Disconnect integration"
        description={`Disconnect ${disconnecting?.display_name ?? "this integration"} from Hivy? Workspace access will be revoked immediately.`}
        confirmLabel="Disconnect"
        destructive
        loading={deleteConnection.isPending}
        onConfirm={handleDisconnect}
      />
    </div>
  )
}

function IntegrationSkeletonGrid() {
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
      {Array.from({ length: 9 }).map((_, index) => (
        <div
          key={index}
          className="flex min-h-22 items-center gap-3 rounded-md border border-border bg-card p-4"
        >
          <Skeleton className="h-10 w-10 rounded-md" />
          <div className="min-w-0 flex-1 space-y-2">
            <Skeleton className="h-4 w-32" />
            <Skeleton className="h-3 w-20" />
          </div>
          <Skeleton className="h-8 w-20 rounded-md" />
        </div>
      ))}
    </div>
  )
}
