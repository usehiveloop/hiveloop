"use client"

import { useState, useEffect, useRef, useCallback, useMemo } from "react"
import { AnimatePresence, motion } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
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
import { Skeleton } from "@/components/ui/skeleton"
import { ChoiceCard } from "./create-agent/choice-card"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { useQueryClient, useQueries } from "@tanstack/react-query"
import { toast } from "sonner"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type Agent = components["schemas"]["agentResponse"]
type InConnection = components["schemas"]["inConnectionResponse"]

interface ResourceItem {
  id: string
  name: string
}

type AgentResources = Record<string, Record<string, ResourceItem[]>>

interface ConfigurableResource {
  key: string
  display_name: string
  description: string
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const slideVariants = {
  enter: (direction: number) => ({ x: direction > 0 ? 60 : -60, opacity: 0 }),
  center: { x: 0, opacity: 1 },
  exit: (direction: number) => ({ x: direction > 0 ? -60 : 60, opacity: 0 }),
}

function parseAgentResources(raw: unknown): AgentResources {
  if (!raw || typeof raw !== "object") return {}
  const result: AgentResources = {}
  for (const [connId, resourceTypes] of Object.entries(raw as Record<string, unknown>)) {
    if (!resourceTypes || typeof resourceTypes !== "object") continue
    const parsed: Record<string, ResourceItem[]> = {}
    for (const [resourceKey, items] of Object.entries(resourceTypes as Record<string, unknown>)) {
      if (!Array.isArray(items)) continue
      parsed[resourceKey] = items.filter(
        (item): item is ResourceItem =>
          typeof item === "object" && item !== null && "id" in item && "name" in item,
      )
    }
    result[connId] = parsed
  }
  return result
}

function getConfigurableResources(connection: InConnection): ConfigurableResource[] {
  const raw = (connection as Record<string, unknown>).configurable_resources
  if (!Array.isArray(raw)) return []
  return raw as ConfigurableResource[]
}

// ---------------------------------------------------------------------------
// Back button
// ---------------------------------------------------------------------------

function BackButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors w-fit mb-2"
    >
      <HugeiconsIcon icon={ArrowLeft01Icon} size={14} />
      Back
    </button>
  )
}

// ---------------------------------------------------------------------------
// Step 1: Connection list
// ---------------------------------------------------------------------------

interface ConnectionListViewProps {
  connections: InConnection[]
  getSelectedCount: (connectionId: string) => number
  getOrphanCount: (connectionId: string) => number
  onSelect: (connectionId: string) => void
  onSave: () => void
  saving: boolean
}

function ConnectionListView({ connections, getSelectedCount, getOrphanCount, onSelect, onSave, saving }: ConnectionListViewProps) {
  return (
    <>
      <DialogHeader>
        <DialogTitle>Configure resources</DialogTitle>
        <DialogDescription>Choose which resources each integration can access.</DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {connections.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-3">
            <div className="flex items-center justify-center size-12 rounded-full bg-muted">
              <HugeiconsIcon icon={Plug01Icon} size={20} className="text-muted-foreground" />
            </div>
            <p className="text-sm text-muted-foreground text-center">
              No integrations with configurable resources.
            </p>
          </div>
        ) : (
          connections.map((connection) => {
            const connectionId = connection.id!
            const count = getSelectedCount(connectionId)
            const orphans = getOrphanCount(connectionId)
            const baseDescription = count > 0
              ? `${count} resource${count !== 1 ? "s" : ""} selected`
              : "No resources configured"
            const description = orphans > 0
              ? `${baseDescription} · ${orphans} no longer accessible`
              : baseDescription
            return (
              <ChoiceCard
                key={connectionId}
                logoUrl={`https://connections.ziraloop.com/images/template-logos/${connection.provider ?? ""}.svg`}
                title={connection.display_name ?? connection.provider ?? ""}
                description={description}
                onClick={() => onSelect(connectionId)}
                trailing={
                  orphans > 0 ? (
                    <span className="text-xs font-medium text-amber-700 dark:text-amber-300 bg-amber-500/10 px-2 py-0.5 rounded-full shrink-0">
                      !{orphans}
                    </span>
                  ) : count > 0 ? (
                    <span className="text-xs font-medium text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded-full shrink-0">
                      {count}
                    </span>
                  ) : undefined
                }
              />
            )
          })
        )}
      </div>

      <div className="pt-4 mt-auto">
        <Button className="w-full" onClick={onSave} loading={saving}>
          Save resources
        </Button>
      </div>
    </>
  )
}

// ---------------------------------------------------------------------------
// Step 2: Resource type list
// ---------------------------------------------------------------------------

interface ResourceTypeListViewProps {
  connection: InConnection
  resourceTypes: ConfigurableResource[]
  getTypeSelectedCount: (resourceType: string) => number
  onSelect: (resourceType: string) => void
  onBack: () => void
}

function ResourceTypeListView({ connection, resourceTypes, getTypeSelectedCount, onSelect, onBack }: ResourceTypeListViewProps) {
  return (
    <>
      <DialogHeader>
        <BackButton onClick={onBack} />
        <DialogTitle>{connection.display_name ?? connection.provider}</DialogTitle>
        <DialogDescription>Choose resource types to scope</DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {resourceTypes.map((resource) => {
          const count = getTypeSelectedCount(resource.key)
          return (
            <ChoiceCard
              key={resource.key}
              title={resource.display_name}
              description={resource.description}
              onClick={() => onSelect(resource.key)}
              trailing={
                count > 0 ? (
                  <span className="text-xs font-medium text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded-full shrink-0">
                    {count} selected
                  </span>
                ) : undefined
              }
            />
          )
        })}
      </div>
    </>
  )
}

// ---------------------------------------------------------------------------
// Step 3: Resource instance multi-select
// ---------------------------------------------------------------------------

interface ResourceInstanceListViewProps {
  connectionId: string
  resourceType: string
  selectedItems: ResourceItem[]
  isSelected: (resourceId: string) => boolean
  onToggle: (item: ResourceItem) => void
  onBack: () => void
}

function ResourceInstanceListView({ connectionId, resourceType, selectedItems, isSelected, onToggle, onBack }: ResourceInstanceListViewProps) {
  const { data, isLoading } = $api.useQuery("get", "/v1/in/connections/{id}/resources/{type}", {
    params: { path: { id: connectionId, type: resourceType } },
  })

  const items: ResourceItem[] = ((data as Record<string, unknown> | undefined)?.resources as ResourceItem[] | undefined) ?? []
  const label = resourceType.replace(/_/g, " ")

  // Orphans: items selected in the agent's state but not returned by the live
  // item list. Only computed once the live list has resolved to avoid a false
  // warning during load.
  const reachableIds = useMemo(() => new Set(items.map((item) => item.id)), [items])
  const orphans = useMemo(
    () => (isLoading ? [] : selectedItems.filter((item) => !reachableIds.has(item.id))),
    [isLoading, selectedItems, reachableIds],
  )

  return (
    <>
      <DialogHeader>
        <BackButton onClick={onBack} />
        <DialogTitle className="capitalize">{label}s</DialogTitle>
        <DialogDescription>Select which {label}s this agent can access</DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {orphans.length > 0 && (
          <div className="rounded-xl border border-amber-300/60 bg-amber-500/10 px-3 py-2.5">
            <p className="text-xs font-medium text-amber-800 dark:text-amber-200">
              {orphans.length} {label}{orphans.length !== 1 ? "s" : ""} no longer accessible
            </p>
            <p className="text-[11px] text-amber-800/80 dark:text-amber-200/80 mt-0.5">
              These will be removed from this agent when you save.
            </p>
            <ul className="mt-2 space-y-0.5">
              {orphans.map((item) => (
                <li key={item.id} className="font-mono text-[11px] text-amber-900 dark:text-amber-100">
                  {item.id}
                </li>
              ))}
            </ul>
          </div>
        )}
        {isLoading ? (
          Array.from({ length: 5 }).map((_, index) => (
            <Skeleton key={index} className="h-[52px] w-full rounded-xl" />
          ))
        ) : items.length === 0 ? (
          <p className="text-sm text-muted-foreground py-8 text-center">
            No {label}s found.
          </p>
        ) : (
          items.map((item) => {
            const selected = isSelected(item.id)
            return (
              <ChoiceCard
                key={item.id}
                title={item.name}
                description={item.id !== item.name ? item.id : ""}
                onClick={() => onToggle(item)}
                trailing={
                  selected ? (
                    <HugeiconsIcon icon={Tick02Icon} size={16} className="text-emerald-600 dark:text-emerald-400 shrink-0" />
                  ) : (
                    <span className="h-4 w-4 rounded-full border border-border shrink-0" />
                  )
                }
              />
            )
          })
        )}
      </div>
    </>
  )
}

// ---------------------------------------------------------------------------
// Main dialog
// ---------------------------------------------------------------------------

interface ConfigureResourcesDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  agent: Agent | null
}

export function ConfigureResourcesDialog({ open, onOpenChange, agent: agentProp }: ConfigureResourcesDialogProps) {
  // Keep a ref to the last non-null agent so the dialog can animate out with stale data
  const lastAgent = useRef<Agent | null>(null)
  if (agentProp) lastAgent.current = agentProp
  const agent = agentProp ?? lastAgent.current

  const queryClient = useQueryClient()
  const updateAgent = $api.useMutation("put", "/v1/agents/{id}")
  const direction = useRef<1 | -1>(1)

  const [resources, setResources] = useState<AgentResources>({})
  const [activeConnectionId, setActiveConnectionId] = useState<string | null>(null)
  const [activeResourceType, setActiveResourceType] = useState<string | null>(null)

  // Reset state when dialog opens
  useEffect(() => {
    if (open && agent) {
      setResources(parseAgentResources(agent.resources))
      setActiveConnectionId(null)
      setActiveResourceType(null)
    }
  }, [open, agent?.resources])

  // Load connections
  const { data: connectionsData } = $api.useQuery("get", "/v1/in/connections")
  const allConnections = (connectionsData?.data ?? []) as InConnection[]
  const connectionsById = new Map(allConnections.filter((c) => c.id).map((c) => [c.id!, c]))

  // Eagerly fetch the live item list for every (connection, resource_type) pair the
  // agent currently has in state. We use this for two things:
  //   1. Render an orphan banner in the per-type view (items selected in state
  //      but no longer returned by the connection's live list).
  //   2. Strip orphans from the payload at save time so they disappear from the
  //      agent config — guaranteeing the warning only ever shows once.
  //
  // If a fetch hasn't resolved yet, we leave that type untouched on save (better
  // than nuking legitimate selections on a transient network error).
  const statePairs = useMemo(() => {
    const pairs: Array<{ connId: string; resourceType: string }> = []
    for (const [connId, types] of Object.entries(resources)) {
      for (const resourceType of Object.keys(types)) {
        pairs.push({ connId, resourceType })
      }
    }
    return pairs
  }, [resources])

  const reachabilityQueries = useQueries({
    queries: statePairs.map(({ connId, resourceType }) => ({
      queryKey: ["resources-reachability", connId, resourceType],
      queryFn: async () => {
        const res = await api.GET("/v1/in/connections/{id}/resources/{type}", {
          params: { path: { id: connId, type: resourceType } },
        })
        if (res.error) throw res.error
        return res.data
      },
      enabled: open,
      staleTime: 60_000,
    })),
  })

  const reachableSets = useMemo(() => {
    const sets: Record<string, Record<string, Set<string>>> = {}
    statePairs.forEach(({ connId, resourceType }, index) => {
      const query = reachabilityQueries[index]
      if (!query?.data) return
      const items = ((query.data as { resources?: ResourceItem[] }).resources ?? [])
      sets[connId] = sets[connId] ?? {}
      sets[connId][resourceType] = new Set(items.map((i) => i.id))
    })
    return sets
  }, [statePairs, reachabilityQueries])

  // Only connections the agent uses AND that have configurable resources
  const agentConnectionIds = agent?.integrations && typeof agent.integrations === "object"
    ? Object.keys(agent.integrations)
    : []
  const configurableConnections = agentConnectionIds
    .map((id) => connectionsById.get(id))
    .filter((connection): connection is InConnection =>
      !!connection && getConfigurableResources(connection).length > 0,
    )

  const activeConnection = activeConnectionId ? connectionsById.get(activeConnectionId) ?? null : null
  const activeResourceTypes = activeConnection ? getConfigurableResources(activeConnection) : []

  // Selection
  const toggleResource = useCallback(
    (connectionId: string, resourceType: string, item: ResourceItem) => {
      setResources((prev) => {
        const connResources = prev[connectionId] ?? {}
        const items = connResources[resourceType] ?? []
        const exists = items.some((existing) => existing.id === item.id)
        const nextItems = exists
          ? items.filter((existing) => existing.id !== item.id)
          : [...items, item]
        return {
          ...prev,
          [connectionId]: { ...connResources, [resourceType]: nextItems },
        }
      })
    },
    [],
  )

  if (!agent) return null

  // Navigation
  function openConnection(connectionId: string) {
    direction.current = 1
    setActiveConnectionId(connectionId)
    setActiveResourceType(null)
  }

  function openResourceType(resourceType: string) {
    direction.current = 1
    setActiveResourceType(resourceType)
  }

  function goBackToConnections() {
    direction.current = -1
    setActiveConnectionId(null)
    setActiveResourceType(null)
  }

  function goBackToResourceTypes() {
    direction.current = -1
    setActiveResourceType(null)
  }

  function getSelectedCount(connectionId: string): number {
    const connResources = resources[connectionId]
    if (!connResources) return 0
    return Object.values(connResources).reduce((sum, items) => sum + items.length, 0)
  }

  // Counts items selected in state for this connection that are NOT returned by
  // the live resource list. Returns 0 until reachability resolves (safer to
  // show no warning than a false positive on a slow fetch).
  function getOrphanCount(connectionId: string): number {
    const connState = resources[connectionId]
    const connReachable = reachableSets[connectionId]
    if (!connState || !connReachable) return 0
    let count = 0
    for (const [resourceType, items] of Object.entries(connState)) {
      const reachable = connReachable[resourceType]
      if (!reachable) continue
      for (const item of items) if (!reachable.has(item.id)) count++
    }
    return count
  }

  function getTypeSelectedCount(connectionId: string, resourceType: string): number {
    return (resources[connectionId]?.[resourceType] ?? []).length
  }

  function isResourceSelected(connectionId: string, resourceType: string, resourceId: string): boolean {
    return (resources[connectionId]?.[resourceType] ?? []).some((item) => item.id === resourceId)
  }

  // Save
  function handleSave() {
    if (!agent?.id) return

    // Strip orphans — items present in state but not in the live reachability
    // set for (connection, type). When reachability hasn't resolved for a type,
    // leave its items as-is (better than dropping valid data on a transient
    // network error). This is what makes the orphan banner a one-shot: save
    // once with orphans visible → next open has nothing to warn about.
    const cleanedResources: AgentResources = {}
    for (const [connId, types] of Object.entries(resources)) {
      const cleanedTypes: Record<string, ResourceItem[]> = {}
      for (const [typeKey, items] of Object.entries(types)) {
        const reachable = reachableSets[connId]?.[typeKey]
        const filtered = reachable ? items.filter((item) => reachable.has(item.id)) : items
        if (filtered.length > 0) cleanedTypes[typeKey] = filtered
      }
      if (Object.keys(cleanedTypes).length > 0) cleanedResources[connId] = cleanedTypes
    }

    updateAgent.mutate(
      { params: { path: { id: agent!.id } }, body: { resources: cleanedResources } as never },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents"] })
          toast.success("Resources updated")
          onOpenChange(false)
        },
        onError: (error) => toast.error(extractErrorMessage(error, "Failed to save resources")),
      },
    )
  }

  const currentStep = activeResourceType ? "instances" : activeConnectionId ? "types" : "connections"

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md h-[780px] overflow-hidden flex flex-col">
        <AnimatePresence mode="wait" custom={direction.current}>
          <motion.div
            key={currentStep === "instances" ? `inst-${activeConnectionId}-${activeResourceType}` : currentStep === "types" ? `types-${activeConnectionId}` : "connections"}
            custom={direction.current}
            variants={slideVariants}
            initial="enter"
            animate="center"
            exit="exit"
            transition={{ duration: 0.2, ease: "easeInOut" }}
            className="flex flex-col h-full"
          >
            {currentStep === "instances" && activeConnectionId && activeResourceType ? (
              <ResourceInstanceListView
                connectionId={activeConnectionId}
                resourceType={activeResourceType}
                selectedItems={resources[activeConnectionId]?.[activeResourceType] ?? []}
                isSelected={(resourceId) => isResourceSelected(activeConnectionId, activeResourceType, resourceId)}
                onToggle={(item) => toggleResource(activeConnectionId, activeResourceType, item)}
                onBack={goBackToResourceTypes}
              />
            ) : currentStep === "types" && activeConnectionId && activeConnection ? (
              <ResourceTypeListView
                connection={activeConnection}
                resourceTypes={activeResourceTypes}
                getTypeSelectedCount={(resourceType) => getTypeSelectedCount(activeConnectionId, resourceType)}
                onSelect={openResourceType}
                onBack={goBackToConnections}
              />
            ) : (
              <ConnectionListView
                connections={configurableConnections}
                getSelectedCount={getSelectedCount}
                getOrphanCount={getOrphanCount}
                onSelect={openConnection}
                onSave={handleSave}
                saving={updateAgent.isPending}
              />
            )}
          </motion.div>
        </AnimatePresence>
      </DialogContent>
    </Dialog>
  )
}
