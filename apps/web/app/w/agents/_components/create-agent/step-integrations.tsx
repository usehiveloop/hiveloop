"use client"

import { useState, useMemo, useRef } from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import { AnimatePresence, motion } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, ArrowRight01Icon, Search01Icon, Tick02Icon, Plug01Icon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"
import { useCreateAgent } from "./context"

export function StepIntegrations() {
  const { selectedIntegrations, selectedActions, toggleIntegration, toggleAction, goTo } = useCreateAgent()
  const [search, setSearch] = useState("")
  const [detailConnectionId, setDetailConnectionId] = useState<string | null>(null)
  const [actionSearch, setActionSearch] = useState("")
  const detailDirection = useRef<1 | -1>(1)

  const { data: connectionsData, isLoading } = $api.useQuery("get", "/v1/in/connections")
  const connections = connectionsData?.data ?? []

  const filtered = useMemo(() => {
    let result = connections
    if (search.trim()) {
      const query = search.toLowerCase()
      result = result.filter(
        (connection) =>
          (connection.display_name ?? "").toLowerCase().includes(query) ||
          (connection.provider ?? "").toLowerCase().includes(query),
      )
    }
    return [...result].sort((first, second) => {
      const firstSelected = selectedIntegrations.has(first.id!) ? 0 : 1
      const secondSelected = selectedIntegrations.has(second.id!) ? 0 : 1
      return firstSelected - secondSelected
    })
  }, [connections, search, selectedIntegrations])

  const selectedCount = selectedIntegrations.size
  const detailConnection = connections.find((connection) => connection.id === detailConnectionId)

  const innerVariants = {
    enter: (direction: number) => ({ x: direction > 0 ? 60 : -60, opacity: 0 }),
    center: { x: 0, opacity: 1 },
    exit: (direction: number) => ({ x: direction > 0 ? -60 : 60, opacity: 0 }),
  }

  function openDetail(connectionId: string) {
    if (!selectedIntegrations.has(connectionId)) {
      toggleIntegration(connectionId)
    }
    detailDirection.current = 1
    setDetailConnectionId(connectionId)
    setActionSearch("")
  }

  function closeDetail() {
    detailDirection.current = -1
    setDetailConnectionId(null)
    setActionSearch("")
  }

  function removeIntegration(connectionId: string) {
    if (selectedIntegrations.has(connectionId)) {
      toggleIntegration(connectionId)
    }
    closeDetail()
  }

  return (
    <div className="flex flex-col h-full overflow-hidden">
      <AnimatePresence mode="wait" custom={detailDirection.current}>
        {detailConnection ? (
          <motion.div
            key={`detail-${detailConnectionId}`}
            custom={detailDirection.current}
            variants={innerVariants}
            initial="enter"
            animate="center"
            exit="exit"
            transition={{ duration: 0.15, ease: "easeInOut" as const }}
            className="flex flex-col h-full"
          >
            <ActionDetailView
              connection={detailConnection}
              actionSearch={actionSearch}
              onActionSearchChange={setActionSearch}
              selectedActions={selectedActions[detailConnectionId!] ?? new Set()}
              onToggleAction={(actionKey) => toggleAction(detailConnectionId!, actionKey)}
              onBack={closeDetail}
              onRemove={() => removeIntegration(detailConnectionId!)}
            />
          </motion.div>
        ) : (
          <motion.div
            key="integration-list"
            custom={detailDirection.current}
            variants={innerVariants}
            initial="enter"
            animate="center"
            exit="exit"
            transition={{ duration: 0.15, ease: "easeInOut" as const }}
            className="flex flex-col h-full"
          >
            <DialogHeader>
              <div className="flex items-center gap-2">
                <button type="button" onClick={() => goTo("sandbox")} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
                  <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
                </button>
                <DialogTitle>Connect integrations</DialogTitle>
              </div>
              <DialogDescription className="mt-2">
                Choose which integrations your agent can access. Only connected integrations are shown.
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
                  <Skeleton key={index} className="h-[64px] w-full rounded-xl" />
                ))
              ) : filtered.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-12 gap-3">
                  {search ? (
                    <p className="text-sm text-muted-foreground">No integrations found.</p>
                  ) : (
                    <>
                      <div className="flex items-center justify-center size-12 rounded-full bg-muted">
                        <HugeiconsIcon icon={Plug01Icon} size={20} className="text-muted-foreground" />
                      </div>
                      <div className="text-center">
                        <p className="text-sm font-medium text-foreground">No integrations connected</p>
                        <p className="text-xs text-muted-foreground mt-1 max-w-[240px]">
                          Head to the Connections page to connect your first integration, then come back here.
                        </p>
                      </div>
                    </>
                  )}
                </div>
              ) : (
                filtered.map((connection) => {
                  const isSelected = selectedIntegrations.has(connection.id!)
                  const actionCount = selectedActions[connection.id!]?.size ?? 0
                  return (
                    <button
                      key={connection.id}
                      type="button"
                      onClick={() => openDetail(connection.id!)}
                      className={`group flex items-start gap-4 w-full rounded-xl p-4 text-left transition-colors cursor-pointer ${
                        isSelected ? "bg-primary/5 border border-primary/20" : "bg-muted/50 hover:bg-muted border border-transparent"
                      }`}
                    >
                      <IntegrationLogo provider={connection.provider ?? ""} size={32} className="shrink-0 mt-0.5" />
                      <div className="flex-1 min-w-0">
                        <p className="text-sm font-semibold text-foreground">{connection.display_name}</p>
                        <p className="text-[13px] text-muted-foreground mt-0.5">
                          {actionCount > 0
                            ? `${actionCount} of ${connection.actions_count ?? 0} actions selected`
                            : `${connection.actions_count ?? 0} actions available`}
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
              <Button onClick={() => goTo("trigger")} className="w-full">
                {selectedCount > 0 ? `Continue with ${selectedCount} integration${selectedCount > 1 ? "s" : ""}` : "Skip for now"}
              </Button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

interface ActionDetailViewProps {
  connection: { id?: string; provider?: string; display_name?: string }
  actionSearch: string
  onActionSearchChange: (value: string) => void
  selectedActions: Set<string>
  onToggleAction: (actionKey: string) => void
  onBack: () => void
  onRemove: () => void
}

function ActionDetailView({
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

  const allSelected = allActions.length > 0 && allActions.every((action) => selectedActions.has(action.key!))

  const virtualizer = useVirtualizer({
    count: filteredActions.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 72,
    overscan: 10,
  })

  function toggleAll() {
    allActions.forEach((action) => {
      const isSelected = selectedActions.has(action.key!)
      if (allSelected && isSelected) {
        onToggleAction(action.key!)
      } else if (!allSelected && !isSelected) {
        onToggleAction(action.key!)
      }
    })
  }

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <div className="flex items-center gap-2.5">
            <IntegrationLogo provider={connection.provider ?? ""} size={20} />
            <DialogTitle>{connection.display_name}</DialogTitle>
          </div>
        </div>
        <DialogDescription className="mt-2">
          Select which actions this agent can use. You can always change this later.
        </DialogDescription>
      </DialogHeader>

      <div className="relative mt-4">
        <HugeiconsIcon icon={Search01Icon} size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="Search actions..."
          value={actionSearch}
          onChange={(event) => onActionSearchChange(event.target.value)}
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
          <span className="tabular-nums">{selectedActions.size}/{allActions.length}</span>
        </button>
      )}

      <div ref={parentRef} className="flex-1 overflow-y-auto mt-1">
        {isLoading ? (
          <div className="flex flex-col pt-[52px]">
            {Array.from({ length: 6 }).map((_, index) => (
              <Skeleton key={index} className="h-[60px] w-full rounded-xl mb-2" />
            ))}
          </div>
        ) : filteredActions.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <p className="text-sm text-muted-foreground">No actions found.</p>
          </div>
        ) : (
          // `height` comes from TanStack Virtual — recomputed per render from
          // row count × estimated row size and changes on scroll/filter.
          <div className="relative" style={{ height: virtualizer.getTotalSize() }}>
            {virtualizer.getVirtualItems().map((virtualItem) => {
              const action = filteredActions[virtualItem.index]
              const isSelected = selectedActions.has(action.key!)
              return (
                // `translateY` is the row's live scroll offset from the
                // virtualizer and changes every frame.
                <div
                  key={action.key}
                  className="absolute top-0 left-0 w-full"
                  style={{ transform: `translateY(${virtualItem.start}px)` }}
                >
                  <button
                    type="button"
                    onClick={() => onToggleAction(action.key!)}
                    className={`flex items-start gap-3 w-full rounded-xl p-3 text-left transition-colors cursor-pointer ${
                      isSelected ? "bg-primary/5 border border-primary/20" : "bg-muted/50 hover:bg-muted border border-transparent"
                    }`}
                  >
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 min-w-0">
                        <span className="text-sm font-medium text-foreground truncate">{action.display_name}</span>
                        <span className={`font-mono text-[9px] uppercase tracking-[0.5px] px-1.5 py-0.5 rounded-full shrink-0 ${
                          action.access === "read" ? "bg-blue-500/10 text-blue-500" : "bg-green-500/10 text-green-500"
                        }`}>
                          {action.access}
                        </span>
                      </div>
                      <p className="text-[12px] text-muted-foreground mt-0.5 line-clamp-1">{action.description}</p>
                    </div>
                    {isSelected && (
                      <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0 mt-0.5" />
                    )}
                  </button>
                </div>
              )
            })}
          </div>
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button variant="outline" className="w-full text-destructive hover:text-destructive" onClick={onRemove}>
          Remove integration
        </Button>
      </div>
    </>
  )
}
