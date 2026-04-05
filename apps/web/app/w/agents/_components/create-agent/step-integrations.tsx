"use client"

import { useState, useMemo } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, ArrowRight01Icon, Search01Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"

interface StepIntegrationsProps {
  selectedIntegrations: Set<string>
  onToggleIntegration: (connectionId: string) => void
  onBack: () => void
  onNext: () => void
}

export function StepIntegrations({
  selectedIntegrations,
  onToggleIntegration,
  onBack,
  onNext,
}: StepIntegrationsProps) {
  const [search, setSearch] = useState("")

  const { data: connectionsData, isLoading } = $api.useQuery("get", "/v1/in/connections")
  const connections = connectionsData?.data ?? []

  const filtered = useMemo(() => {
    if (!search.trim()) return connections
    const query = search.toLowerCase()
    return connections.filter(
      (connection) =>
        (connection.display_name ?? "").toLowerCase().includes(query) ||
        (connection.provider ?? "").toLowerCase().includes(query),
    )
  }, [connections, search])

  const selectedCount = selectedIntegrations.size

  return (
    <div className="flex flex-col h-full overflow-hidden">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Connect integrations</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Choose which integrations your agent can access. Only integrations you&apos;ve already connected are shown.
        </DialogDescription>
      </DialogHeader>

      <div className="relative mt-4">
        <HugeiconsIcon icon={Search01Icon} size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="Search integrations..."
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          className="pl-9 h-9"
        />
      </div>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {isLoading ? (
          Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-[72px] w-full rounded-xl" />
          ))
        ) : filtered.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <p className="text-sm text-muted-foreground">
              {search ? "No integrations found." : "No connected integrations. Connect one first from the Connections page."}
            </p>
          </div>
        ) : (
          filtered.map((connection) => {
            const isSelected = selectedIntegrations.has(connection.id!)
            return (
              <button
                key={connection.id}
                type="button"
                onClick={() => onToggleIntegration(connection.id!)}
                className={`group flex items-start gap-4 w-full rounded-xl p-4 text-left transition-colors cursor-pointer ${
                  isSelected ? "bg-primary/5 border border-primary/20" : "bg-muted/50 hover:bg-muted border border-transparent"
                }`}
              >
                <IntegrationLogo provider={connection.provider ?? ""} size={32} className="shrink-0 mt-0.5" />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-semibold text-foreground">{connection.display_name}</p>
                  <p className="text-[13px] text-muted-foreground mt-0.5">
                    {connection.actions_count ?? 0} actions available
                  </p>
                </div>
                {isSelected ? (
                  <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0 mt-0.5" />
                ) : (
                  <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
                )}
              </button>
            )
          })
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onNext} className="w-full">
          {selectedCount > 0 ? `Continue with ${selectedCount} integration${selectedCount > 1 ? "s" : ""}` : "Skip for now"}
        </Button>
      </div>
    </div>
  )
}
