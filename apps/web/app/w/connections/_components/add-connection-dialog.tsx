"use client"

import { useMemo, useState, useCallback, useEffect } from "react"
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
import { IntegrationLogo } from "@/components/integration-logo"
import { CredentialsForm } from "./credentials-form"
import type { components } from "@/lib/api/schema"

type Integration = components["schemas"]["inIntegrationAvailableResponse"]
type ConnectionConfigField = components["schemas"]["ConnectionConfigField"]

interface ConnectOptions {
  credentials?: Record<string, string>
  params?: Record<string, string>
  installation?: "outbound"
}

interface AddConnectionDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  search: string
  onSearchChange: (value: string) => void
  connectingId: string | null
  onConnect: (integrationId: string, options?: ConnectOptions) => void
  preSelectedIntegrationId?: string | null
  onPreSelectedClear?: () => void
}

function needsForm(integration: Integration): boolean {
  const authMode = integration.nango_config?.auth_mode
  const installation = integration.nango_config?.installation

  if (authMode === "API_KEY" || authMode === "BASIC") return true
  if (installation === "outbound") return true

  const connectionConfig = integration.nango_config?.connection_config as
    | Record<string, ConnectionConfigField>
    | undefined
  if (connectionConfig) {
    const hasRequiredFields = Object.values(connectionConfig).some(
      (field) => !field.automated,
    )
    if (hasRequiredFields) return true
  }

  return false
}

export function AddConnectionDialog({
  open,
  onOpenChange,
  search,
  onSearchChange,
  connectingId,
  onConnect,
  preSelectedIntegrationId,
  onPreSelectedClear,
}: AddConnectionDialogProps) {
  const [selectedIntegration, setSelectedIntegration] = useState<Integration | null>(null)

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

  // Handle pre-selected integration from empty state
  useEffect(() => {
    if (preSelectedIntegrationId && integrations.length > 0 && !selectedIntegration) {
      const found = integrations.find((i) => i.id === preSelectedIntegrationId)
      if (found && needsForm(found)) {
        setSelectedIntegration(found)
      }
    }
  }, [preSelectedIntegrationId, integrations, selectedIntegration])

  const filtered = useMemo(() => {
    if (!search.trim()) return integrations
    const query = search.toLowerCase()
    return integrations.filter(
      (integration) =>
        (integration.display_name ?? "").toLowerCase().includes(query) ||
        (integration.provider ?? "").toLowerCase().includes(query),
    )
  }, [integrations, search])

  function handleIntegrationClick(integration: Integration) {
    if (needsForm(integration)) {
      setSelectedIntegration(integration)
    } else {
      onConnect(integration.id!)
    }
  }

  function handleFormSubmit(
    credentials: Record<string, string> | undefined,
    params: Record<string, string>,
    installation?: "outbound",
  ) {
    if (!selectedIntegration?.id) return

    const options: ConnectOptions = {}
    if (credentials) options.credentials = credentials
    if (Object.keys(params).length > 0) options.params = params
    if (installation) options.installation = installation

    onConnect(
      selectedIntegration.id,
      Object.keys(options).length > 0 ? options : undefined,
    )
  }

  const handleBack = useCallback(() => {
    setSelectedIntegration(null)
    onPreSelectedClear?.()
  }, [onPreSelectedClear])

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setSelectedIntegration(null)
      onPreSelectedClear?.()
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        {selectedIntegration ? (
          <CredentialsForm
            integration={selectedIntegration}
            onSubmit={handleFormSubmit}
            onBack={handleBack}
            isSubmitting={connectingId === selectedIntegration.id}
          />
        ) : (
          <>
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
                        onClick={() => handleIntegrationClick(integration)}
                      >
                        <IntegrationLogo provider={integration.provider ?? ""} size={24} />
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
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
