"use client"

import { useMemo } from "react"
import { $api } from "@/lib/api/hooks"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowRight01Icon, Loading03Icon } from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { IntegrationLogo } from "@/components/integration-logo"
import type { components } from "@/lib/api/schema"

type ConnectionConfigField = components["schemas"]["ConnectionConfigField"]

const MAX_SUGGESTIONS = 10

function needsForm(integration: components["schemas"]["inIntegrationAvailableResponse"]): boolean {
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

interface ConnectionsEmptyProps {
  connectingId: string | null
  onConnect: (integrationId: string) => void
  onShowAll: () => void
  onShowFormFor: (integrationId: string) => void
}

export function ConnectionsEmpty({ connectingId, onConnect, onShowAll, onShowFormFor }: ConnectionsEmptyProps) {
  const { data, isLoading } = $api.useQuery("get", "/v1/in/integrations/available")

  const integrations = data ?? []

  const suggestions = useMemo(() => {
    const popular = integrations.filter((integration) =>
      (integration.nango_config?.categories ?? []).includes("popular"),
    )

    if (popular.length >= MAX_SUGGESTIONS) {
      return popular.slice(0, MAX_SUGGESTIONS)
    }

    const popularIds = new Set(popular.map((integration) => integration.id))
    const rest = integrations.filter((integration) => !popularIds.has(integration.id))

    return [...popular, ...rest].slice(0, MAX_SUGGESTIONS)
  }, [integrations])

  function handleClick(integration: components["schemas"]["inIntegrationAvailableResponse"]) {
    if (needsForm(integration)) {
      onShowFormFor(integration.id!)
    } else {
      onConnect(integration.id!)
    }
  }

  return (
    <div className="flex flex-col items-center pt-[20vh] pb-24 px-4">
      <div className="text-center mb-8">
        <h2 className="font-heading text-2xl font-semibold text-foreground">
          Connect your first integration
        </h2>
        <p className="text-sm text-muted-foreground mt-2 max-w-sm">
          Give your agents access to the tools you use every day. Pick one to get started.
        </p>
      </div>

      <div className="w-full max-w-sm">
        {isLoading ? (
          <div className="flex flex-col gap-2">
            {Array.from({ length: 6 }).map((_, index) => (
              <Skeleton key={index} className="h-12 w-full rounded-xl" />
            ))}
          </div>
        ) : (
          <div className="flex flex-col gap-1">
            {suggestions.map((integration) => {
              const isConnecting = connectingId === integration.id
              return (
                <button
                  key={integration.id}
                  type="button"
                  disabled={connectingId !== null}
                  className="flex items-center gap-3 rounded-xl px-3 py-3 text-left transition-colors hover:bg-muted cursor-pointer disabled:cursor-not-allowed"
                  onClick={() => handleClick(integration)}
                >
                  <IntegrationLogo provider={integration.provider ?? ""} size={24} />
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-medium truncate">
                      {integration.display_name}
                    </p>
                  </div>
                  {isConnecting ? (
                    <HugeiconsIcon icon={Loading03Icon} className="size-4 animate-spin text-muted-foreground" />
                  ) : (
                    <HugeiconsIcon icon={ArrowRight01Icon} className="size-4 text-muted-foreground" />
                  )}
                </button>
              )
            })}
          </div>
        )}

        <p className="text-sm text-muted-foreground text-center mt-6 mb-4">
          Can&apos;t find the integration you&apos;re looking for?
        </p>
        <Button variant="outline" className="w-full" onClick={onShowAll}>
          See all integrations
        </Button>
      </div>
    </div>
  )
}
