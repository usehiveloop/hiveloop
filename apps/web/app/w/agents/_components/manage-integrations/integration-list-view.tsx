import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowRight01Icon,
  Search01Icon,
  Tick02Icon,
  Plug01Icon,
} from "@hugeicons/core-free-icons"
import {
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { IntegrationLogo } from "@/components/integration-logo"

interface ConnectionItem {
  id?: string
  provider?: string
  display_name?: string
  actions_count?: number
}

interface IntegrationListViewProps {
  connections: ConnectionItem[]
  isLoading: boolean
  search: string
  onSearchChange: (value: string) => void
  selectedIntegrations: Set<string>
  selectedActions: Record<string, Set<string>>
  onOpenDetail: (connectionId: string) => void
  onSave: () => void
}

export function IntegrationListView({
  connections,
  isLoading,
  search,
  onSearchChange,
  selectedIntegrations,
  selectedActions,
  onOpenDetail,
  onSave,
}: IntegrationListViewProps) {
  const selectedCount = selectedIntegrations.size

  return (
    <>
      <DialogHeader>
        <DialogTitle>Manage integrations</DialogTitle>
        <DialogDescription className="mt-2">
          Choose which integrations your agent can access. Only connected integrations are shown.
        </DialogDescription>
      </DialogHeader>

      <div className="relative mt-4">
        <HugeiconsIcon
          icon={Search01Icon}
          size={14}
          className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          placeholder="Search integrations..."
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          className="pl-9 h-9"
        />
      </div>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {isLoading ? (
          Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-[64px] w-full rounded-xl" />
          ))
        ) : connections.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-3">
            {search ? (
              <p className="text-sm text-muted-foreground">
                No integrations found.
              </p>
            ) : (
              <>
                <div className="flex items-center justify-center size-12 rounded-full bg-muted">
                  <HugeiconsIcon
                    icon={Plug01Icon}
                    size={20}
                    className="text-muted-foreground"
                  />
                </div>
                <div className="text-center">
                  <p className="text-sm font-medium text-foreground">
                    No integrations connected
                  </p>
                  <p className="text-xs text-muted-foreground mt-1 max-w-[240px]">
                    Head to the Connections page to connect your first
                    integration, then come back here.
                  </p>
                </div>
              </>
            )}
          </div>
        ) : (
          connections.map((connection) => {
            const connectionId = connection.id ?? ""
            const isSelected = selectedIntegrations.has(connectionId)
            const actionCount = selectedActions[connectionId]?.size ?? 0
            return (
              <button
                key={connectionId}
                type="button"
                onClick={() => onOpenDetail(connectionId)}
                className={`group flex items-start gap-4 w-full rounded-xl p-4 text-left transition-colors cursor-pointer ${
                  isSelected
                    ? "bg-primary/5 border border-primary/20"
                    : "bg-muted/50 hover:bg-muted border border-transparent"
                }`}
              >
                <IntegrationLogo
                  provider={connection.provider ?? ""}
                  size={32}
                  className="shrink-0 mt-0.5"
                />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-semibold text-foreground">
                    {connection.display_name}
                  </p>
                  <p className="text-[13px] text-muted-foreground mt-0.5">
                    {actionCount > 0
                      ? `${actionCount} of ${connection.actions_count ?? 0} actions selected`
                      : `${connection.actions_count ?? 0} actions available`}
                  </p>
                </div>
                {isSelected ? (
                  <HugeiconsIcon
                    icon={Tick02Icon}
                    size={16}
                    className="text-primary shrink-0 mt-0.5"
                  />
                ) : (
                  <HugeiconsIcon
                    icon={ArrowRight01Icon}
                    size={16}
                    className="text-muted-foreground/30 shrink-0 mt-0.5"
                  />
                )}
              </button>
            )
          })
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onSave} className="w-full">
          {selectedCount > 0
            ? `Save with ${selectedCount} integration${selectedCount > 1 ? "s" : ""}`
            : "Save with no integrations"}
        </Button>
      </div>
    </>
  )
}
