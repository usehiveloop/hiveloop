"use client"

import { useState, useEffect, useRef, useCallback, useMemo } from "react"
import { AnimatePresence, motion } from "motion/react"
import {
  Dialog,
  DialogContent,
} from "@/components/ui/dialog"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { useQueryClient, useQueries } from "@tanstack/react-query"
import { toast } from "sonner"
import { extractErrorMessage } from "@/lib/api/error"
import { ConnectionListView } from "./configure-resources/connection-list-view"
import { ResourceTypeListView } from "./configure-resources/resource-type-view"
import { ResourceInstanceListView } from "./configure-resources/resource-instance-view"
import {
  type Agent,
  type AgentResources,
  type InConnection,
  type ResourceItem,
  getConfigurableResources,
  parseAgentResources,
  slideVariants,
} from "./configure-resources/types"

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
      { params: { path: { id: agent?.id as string } }, body: { resources: cleanedResources } as never },
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
      <DialogContent className="sm:max-w-md h-195 overflow-hidden flex flex-col">
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
