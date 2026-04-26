"use client"

import { useMemo, useRef } from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  Search01Icon,
  Tick02Icon,
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
import { $api } from "@/lib/api/hooks"

interface ActionDetailViewProps {
  connection: { id?: string; provider?: string; display_name?: string }
  actionSearch: string
  onActionSearchChange: (value: string) => void
  selectedActions: Set<string>
  onToggleAction: (actionKey: string) => void
  onBack: () => void
  onRemove: () => void
}

export function ActionDetailView({
  connection,
  actionSearch,
  onActionSearchChange,
  selectedActions,
  onToggleAction,
  onBack,
  onRemove,
}: ActionDetailViewProps) {
  const parentRef = useRef<HTMLDivElement>(null)

  const { data: actionsData, isLoading } = $api.useQuery(
    "get",
    "/v1/catalog/integrations/{id}/actions",
    { params: { path: { id: connection.provider ?? "" } } },
    { enabled: !!connection.provider },
  )

  const allActions = actionsData ?? []

  const filteredActions = useMemo(() => {
    if (!actionSearch.trim()) return allActions
    const query = actionSearch.toLowerCase()
    return allActions.filter(
      (action) =>
        (action.display_name ?? "").toLowerCase().includes(query) ||
        (action.description ?? "").toLowerCase().includes(query) ||
        (action.key ?? "").toLowerCase().includes(query),
    )
  }, [allActions, actionSearch])

  const allSelected =
    allActions.length > 0 &&
    allActions.every((action) => selectedActions.has(action.key ?? ""))

  const virtualizer = useVirtualizer({
    count: filteredActions.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 72,
    overscan: 10,
  })

  function toggleAll() {
    for (const action of allActions) {
      const key = action.key ?? ""
      const isSelected = selectedActions.has(key)
      if (allSelected && isSelected) {
        onToggleAction(key)
      } else if (!allSelected && !isSelected) {
        onToggleAction(key)
      }
    }
  }

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onBack}
            className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1"
          >
            <HugeiconsIcon
              icon={ArrowLeft01Icon}
              size={16}
              className="text-muted-foreground"
            />
          </button>
          <div className="flex items-center gap-2.5">
            <IntegrationLogo
              provider={connection.provider ?? ""}
              size={20}
            />
            <DialogTitle>{connection.display_name}</DialogTitle>
          </div>
        </div>
        <DialogDescription className="mt-2">
          Select which actions this agent can use.
        </DialogDescription>
      </DialogHeader>

      <div className="relative mt-4">
        <HugeiconsIcon
          icon={Search01Icon}
          size={14}
          className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          placeholder="Search actions..."
          value={actionSearch}
          onChange={(e) => onActionSearchChange(e.target.value)}
          className="pl-9 h-9"
        />
      </div>

      {!isLoading && allActions.length > 0 && (
        <button
          type="button"
          onClick={toggleAll}
          className="flex items-center justify-between px-1 py-2 mt-3 text-xs font-medium text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
        >
          <span>{allSelected ? "Deselect all" : "Select all"}</span>
          <span className="tabular-nums">
            {selectedActions.size}/{allActions.length}
          </span>
        </button>
      )}

      <div ref={parentRef} className="flex-1 overflow-y-auto mt-1">
        {isLoading ? (
          <div className="flex flex-col pt-[52px]">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-[60px] w-full rounded-xl mb-2" />
            ))}
          </div>
        ) : filteredActions.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <p className="text-sm text-muted-foreground">No actions found.</p>
          </div>
        ) : (
          <div
            style={{
              height: virtualizer.getTotalSize(),
              position: "relative",
            }}
          >
            {virtualizer.getVirtualItems().map((virtualItem) => {
              const action = filteredActions[virtualItem.index]
              const actionKey = action.key ?? ""
              const isSelected = selectedActions.has(actionKey)
              return (
                <div
                  key={actionKey}
                  style={{
                    position: "absolute",
                    top: 0,
                    left: 0,
                    width: "100%",
                    transform: `translateY(${virtualItem.start}px)`,
                  }}
                >
                  <button
                    type="button"
                    onClick={() => onToggleAction(actionKey)}
                    className={`flex items-start gap-3 w-full rounded-xl p-3 text-left transition-colors cursor-pointer ${
                      isSelected
                        ? "bg-primary/5 border border-primary/20"
                        : "bg-muted/50 hover:bg-muted border border-transparent"
                    }`}
                  >
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 min-w-0">
                        <span className="text-sm font-medium text-foreground truncate">
                          {action.display_name}
                        </span>
                        <span
                          className={`font-mono text-[9px] uppercase tracking-[0.5px] px-1.5 py-0.5 rounded-full shrink-0 ${
                            action.access === "read"
                              ? "bg-blue-500/10 text-blue-500"
                              : "bg-green-500/10 text-green-500"
                          }`}
                        >
                          {action.access}
                        </span>
                      </div>
                      <p className="text-[12px] text-muted-foreground mt-0.5 line-clamp-1">
                        {action.description}
                      </p>
                    </div>
                    {isSelected && (
                      <HugeiconsIcon
                        icon={Tick02Icon}
                        size={16}
                        className="text-primary shrink-0 mt-0.5"
                      />
                    )}
                  </button>
                </div>
              )
            })}
          </div>
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button
          variant="outline"
          className="w-full text-destructive hover:text-destructive"
          onClick={onRemove}
        >
          Remove integration
        </Button>
      </div>
    </>
  )
}
