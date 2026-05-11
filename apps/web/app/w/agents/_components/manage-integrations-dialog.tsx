"use client"

import { useState, useMemo, useRef, useCallback, useEffect } from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import { AnimatePresence, motion } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Search01Icon,
  Tick02Icon,
  Plug01Icon,
} from "@hugeicons/core-free-icons"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"

/** The shape stored in agent.integrations JSON: connectionId → { actions: string[] } */
export type AgentIntegrations = Record<string, { actions: string[] }>

interface ManageIntegrationsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Current integrations on the agent — used to seed selected state */
  agentIntegrations: AgentIntegrations
  /** Called when the user confirms their selection */
  onSave: (integrations: AgentIntegrations) => void
}

export function ManageIntegrationsDialog({
  open,
  onOpenChange,
  agentIntegrations,
  onSave,
}: ManageIntegrationsDialogProps) {
  const [selectedIntegrations, setSelectedIntegrations] = useState<Set<string>>(
    new Set()
  )
  const [selectedActions, setSelectedActions] = useState<
    Record<string, Set<string>>
  >({})
  const [search, setSearch] = useState("")
  const [detailConnectionId, setDetailConnectionId] = useState<string | null>(
    null
  )
  const [actionSearch, setActionSearch] = useState("")
  const [detailDirection, setDetailDirection] = useState<1 | -1>(1)

  // Seed local state from agentIntegrations when dialog opens
  useEffect(() => {
    if (open) {
      const ids = new Set(Object.keys(agentIntegrations))
      const actions: Record<string, Set<string>> = {}
      for (const [id, config] of Object.entries(agentIntegrations)) {
        actions[id] = new Set(config.actions)
      }
      queueMicrotask(() => {
        setSelectedIntegrations(ids)
        setSelectedActions(actions)
        setSearch("")
        setDetailConnectionId(null)
        setActionSearch("")
      })
    }
  }, [open, agentIntegrations])

  const { data: connectionsData, isLoading } = $api.useQuery(
    "get",
    "/v1/in/connections"
  )
  const connections = useMemo(
    () => connectionsData?.data ?? [],
    [connectionsData]
  )

  const filtered = useMemo(() => {
    if (!search.trim()) return connections
    const query = search.toLowerCase()
    return connections.filter(
      (c) =>
        (c.display_name ?? "").toLowerCase().includes(query) ||
        (c.provider ?? "").toLowerCase().includes(query)
    )
  }, [connections, search])

  const detailConnection = connections.find((c) => c.id === detailConnectionId)

  const toggleIntegration = useCallback((id: string) => {
    setSelectedIntegrations((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
        setSelectedActions((a) => {
          const copy = { ...a }
          delete copy[id]
          return copy
        })
      } else {
        next.add(id)
      }
      return next
    })
  }, [])

  const toggleAction = useCallback(
    (connectionId: string, actionKey: string) => {
      setSelectedActions((prev) => {
        const current = prev[connectionId] ?? new Set<string>()
        const next = new Set(current)
        if (next.has(actionKey)) {
          next.delete(actionKey)
        } else {
          next.add(actionKey)
        }
        return { ...prev, [connectionId]: next }
      })
    },
    []
  )

  function openDetail(connectionId: string) {
    if (!selectedIntegrations.has(connectionId)) {
      toggleIntegration(connectionId)
    }
    setDetailDirection(1)
    setDetailConnectionId(connectionId)
    setActionSearch("")
  }

  function closeDetail() {
    setDetailDirection(-1)
    setDetailConnectionId(null)
    setActionSearch("")
  }

  function removeIntegration(connectionId: string) {
    if (selectedIntegrations.has(connectionId)) {
      toggleIntegration(connectionId)
    }
    closeDetail()
  }

  function handleSave() {
    const result: AgentIntegrations = {}
    for (const id of selectedIntegrations) {
      result[id] = {
        actions: Array.from(selectedActions[id] ?? []),
      }
    }
    onSave(result)
    onOpenChange(false)
  }

  const selectedCount = selectedIntegrations.size

  const innerVariants = {
    enter: (direction: number) => ({ x: direction > 0 ? 60 : -60, opacity: 0 }),
    center: { x: 0, opacity: 1 },
    exit: (direction: number) => ({ x: direction > 0 ? -60 : 60, opacity: 0 }),
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[min(780px,85vh)] flex-col p-6">
        <AnimatePresence mode="wait" custom={detailDirection}>
          {detailConnection && detailConnectionId ? (
            <motion.div
              key={`detail-${detailConnectionId}`}
              custom={detailDirection}
              variants={innerVariants}
              initial="enter"
              animate="center"
              exit="exit"
              transition={{ duration: 0.15, ease: "easeInOut" as const }}
              className="flex h-full flex-col"
            >
              <ActionDetailView
                connection={detailConnection}
                actionSearch={actionSearch}
                onActionSearchChange={setActionSearch}
                selectedActions={
                  selectedActions[detailConnectionId] ?? new Set()
                }
                onToggleAction={(actionKey) =>
                  toggleAction(detailConnectionId, actionKey)
                }
                onBack={closeDetail}
                onRemove={() => removeIntegration(detailConnectionId)}
              />
            </motion.div>
          ) : (
            <motion.div
              key="integration-list"
              custom={detailDirection}
              variants={innerVariants}
              initial="enter"
              animate="center"
              exit="exit"
              transition={{ duration: 0.15, ease: "easeInOut" as const }}
              className="flex h-full flex-col"
            >
              <DialogHeader>
                <DialogTitle>Manage integrations</DialogTitle>
                <DialogDescription className="mt-2">
                  Choose which integrations your agent can access. Only
                  connected integrations are shown.
                </DialogDescription>
              </DialogHeader>

              <div className="relative mt-4">
                <HugeiconsIcon
                  icon={Search01Icon}
                  size={14}
                  className="absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
                />
                <Input
                  placeholder="Search integrations..."
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  className="h-9 pl-9"
                />
              </div>

              <div className="mt-4 flex flex-1 flex-col gap-2 overflow-y-auto">
                {isLoading ? (
                  Array.from({ length: 4 }).map((_, i) => (
                    <Skeleton key={i} className="h-[64px] w-full rounded-xl" />
                  ))
                ) : filtered.length === 0 ? (
                  <div className="flex flex-col items-center justify-center gap-3 py-12">
                    {search ? (
                      <p className="text-sm text-muted-foreground">
                        No integrations found.
                      </p>
                    ) : (
                      <>
                        <div className="flex size-12 items-center justify-center rounded-full bg-muted">
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
                          <p className="mt-1 max-w-[240px] text-xs text-muted-foreground">
                            Head to the Connections page to connect your first
                            integration, then come back here.
                          </p>
                        </div>
                      </>
                    )}
                  </div>
                ) : (
                  filtered.map((connection) => {
                    const connectionId = connection.id ?? ""
                    const isSelected = selectedIntegrations.has(connectionId)
                    const actionCount = selectedActions[connectionId]?.size ?? 0
                    return (
                      <button
                        key={connectionId}
                        type="button"
                        onClick={() => openDetail(connectionId)}
                        className={`group flex w-full cursor-pointer items-start gap-4 rounded-xl p-4 text-left transition-colors ${
                          isSelected
                            ? "border border-primary/20 bg-primary/5"
                            : "border border-transparent bg-muted/50 hover:bg-muted"
                        }`}
                      >
                        <IntegrationLogo
                          provider={connection.provider ?? ""}
                          size={32}
                          className="mt-0.5 shrink-0"
                        />
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-semibold text-foreground">
                            {connection.display_name}
                          </p>
                          <p className="mt-0.5 text-[13px] text-muted-foreground">
                            {actionCount > 0
                              ? `${actionCount} of ${connection.actions_count ?? 0} actions selected`
                              : `${connection.actions_count ?? 0} actions available`}
                          </p>
                        </div>
                        {isSelected ? (
                          <HugeiconsIcon
                            icon={Tick02Icon}
                            size={16}
                            className="mt-0.5 shrink-0 text-primary"
                          />
                        ) : (
                          <HugeiconsIcon
                            icon={ArrowRight01Icon}
                            size={16}
                            className="mt-0.5 shrink-0 text-muted-foreground/30"
                          />
                        )}
                      </button>
                    )
                  })
                )}
              </div>

              <div className="shrink-0 pt-4">
                <Button onClick={handleSave} className="w-full">
                  {selectedCount > 0
                    ? `Save with ${selectedCount} integration${selectedCount > 1 ? "s" : ""}`
                    : "Save with no integrations"}
                </Button>
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </DialogContent>
    </Dialog>
  )
}

// ---------------------------------------------------------------------------
// Action detail sub-view (virtualized action list for a single integration)
// ---------------------------------------------------------------------------

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
    { enabled: !!connection.provider }
  )

  const allActions = useMemo(() => actionsData ?? [], [actionsData])

  const filteredActions = useMemo(() => {
    if (!actionSearch.trim()) return allActions
    const query = actionSearch.toLowerCase()
    return allActions.filter(
      (action) =>
        (action.display_name ?? "").toLowerCase().includes(query) ||
        (action.description ?? "").toLowerCase().includes(query) ||
        (action.key ?? "").toLowerCase().includes(query)
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
            className="-ml-1 flex h-7 w-7 items-center justify-center rounded-lg transition-colors hover:bg-muted"
          >
            <HugeiconsIcon
              icon={ArrowLeft01Icon}
              size={16}
              className="text-muted-foreground"
            />
          </button>
          <div className="flex items-center gap-2.5">
            <IntegrationLogo provider={connection.provider ?? ""} size={20} />
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
          className="absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          placeholder="Search actions..."
          value={actionSearch}
          onChange={(e) => onActionSearchChange(e.target.value)}
          className="h-9 pl-9"
        />
      </div>

      {!isLoading && allActions.length > 0 && (
        <button
          type="button"
          onClick={toggleAll}
          className="mt-3 flex cursor-pointer items-center justify-between px-1 py-2 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          <span>{allSelected ? "Deselect all" : "Select all"}</span>
          <span className="tabular-nums">
            {selectedActions.size}/{allActions.length}
          </span>
        </button>
      )}

      <div ref={parentRef} className="mt-1 flex-1 overflow-y-auto">
        {isLoading ? (
          <div className="flex flex-col pt-[52px]">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="mb-2 h-[60px] w-full rounded-xl" />
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
                    className={`flex w-full cursor-pointer items-start gap-3 rounded-xl p-3 text-left transition-colors ${
                      isSelected
                        ? "border border-primary/20 bg-primary/5"
                        : "border border-transparent bg-muted/50 hover:bg-muted"
                    }`}
                  >
                    <div className="min-w-0 flex-1">
                      <div className="flex min-w-0 items-center gap-2">
                        <span className="truncate text-sm font-medium text-foreground">
                          {action.display_name}
                        </span>
                        <span
                          className={`shrink-0 rounded-full px-1.5 py-0.5 font-mono text-[9px] tracking-[0.5px] uppercase ${
                            action.access === "read"
                              ? "bg-blue-500/10 text-blue-500"
                              : "bg-green-500/10 text-green-500"
                          }`}
                        >
                          {action.access}
                        </span>
                      </div>
                      <p className="mt-0.5 line-clamp-1 text-[12px] text-muted-foreground">
                        {action.description}
                      </p>
                    </div>
                    {isSelected && (
                      <HugeiconsIcon
                        icon={Tick02Icon}
                        size={16}
                        className="mt-0.5 shrink-0 text-primary"
                      />
                    )}
                  </button>
                </div>
              )
            })}
          </div>
        )}
      </div>

      <div className="shrink-0 pt-4">
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
