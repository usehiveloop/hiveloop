"use client"

import { useState, useEffect, useRef, useCallback } from "react"
import { AnimatePresence, motion } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Tick02Icon,
  Loading03Icon,
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
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"

type Agent = components["schemas"]["agentResponse"]

interface ResourceItem {
  id: string
  name: string
}

type AgentResources = Record<string, Record<string, ResourceItem[]>>

interface ConfigureResourcesDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  agent: Agent
}

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

export function ConfigureResourcesDialog({
  open,
  onOpenChange,
  agent,
}: ConfigureResourcesDialogProps) {
  const queryClient = useQueryClient()
  const updateAgent = $api.useMutation("put", "/v1/agents/{id}")
  const direction = useRef<1 | -1>(1)

  const [resources, setResources] = useState<AgentResources>({})
  const [activeConnectionId, setActiveConnectionId] = useState<string | null>(null)
  const [activeResourceType, setActiveResourceType] = useState<string | null>(null)

  useEffect(() => {
    if (open) {
      setResources(parseAgentResources(agent.resources))
      setActiveConnectionId(null)
      setActiveResourceType(null)
    }
  }, [open, agent.resources])

  const { data: connectionsData } = $api.useQuery("get", "/v1/in/connections")
  const connections = connectionsData?.data ?? []
  const connectionsById = new Map(connections.filter((c) => c.id).map((c) => [c.id as string, c]))

  const agentConnectionIds = agent.integrations && typeof agent.integrations === "object"
    ? Object.keys(agent.integrations)
    : []
  const agentConnections = agentConnectionIds
    .map((id) => connectionsById.get(id))
    .filter(Boolean) as NonNullable<(typeof connections)[number]>[]

  const activeConnection = activeConnectionId ? connectionsById.get(activeConnectionId) : null
  const configurableResources = (activeConnection as Record<string, unknown> | undefined)?.configurable_resources as
    | { key: string; display_name: string; description: string }[]
    | undefined

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
          [connectionId]: {
            ...connResources,
            [resourceType]: nextItems,
          },
        }
      })
    },
    [],
  )

  function getSelectedCount(connectionId: string): number {
    const connResources = resources[connectionId]
    if (!connResources) return 0
    return Object.values(connResources).reduce((sum, items) => sum + items.length, 0)
  }

  function getTypeSelectedCount(connectionId: string, resourceType: string): number {
    return (resources[connectionId]?.[resourceType] ?? []).length
  }

  function isResourceSelected(connectionId: string, resourceType: string, resourceId: string): boolean {
    return (resources[connectionId]?.[resourceType] ?? []).some((item) => item.id === resourceId)
  }

  function handleSave() {
    if (!agent.id) return

    const cleanedResources: AgentResources = {}
    for (const [connId, types] of Object.entries(resources)) {
      const cleanedTypes: Record<string, ResourceItem[]> = {}
      for (const [typeKey, items] of Object.entries(types)) {
        if (items.length > 0) {
          cleanedTypes[typeKey] = items
        }
      }
      if (Object.keys(cleanedTypes).length > 0) {
        cleanedResources[connId] = cleanedTypes
      }
    }

    updateAgent.mutate(
      {
        params: { path: { id: agent.id } },
        body: { resources: cleanedResources } as never,
      },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents"] })
          toast.success("Resources updated")
          onOpenChange(false)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to save resources"))
        },
      },
    )
  }

  const currentStep = activeResourceType ? "resources" : activeConnectionId ? "types" : "connections"

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex flex-col h-[min(680px,85vh)] sm:max-w-lg p-6">
        <AnimatePresence mode="wait" custom={direction.current}>
          {currentStep === "resources" && activeConnectionId && activeResourceType ? (
            <motion.div
              key={`resources-${activeConnectionId}-${activeResourceType}`}
              custom={direction.current}
              variants={slideVariants}
              initial="enter"
              animate="center"
              exit="exit"
              transition={{ duration: 0.15, ease: "easeInOut" }}
              className="flex flex-col h-full"
            >
              <ResourceInstancesView
                connectionId={activeConnectionId}
                resourceType={activeResourceType}
                selectedItems={resources[activeConnectionId]?.[activeResourceType] ?? []}
                isSelected={(resourceId) => isResourceSelected(activeConnectionId, activeResourceType, resourceId)}
                onToggle={(item) => toggleResource(activeConnectionId, activeResourceType, item)}
                onBack={goBackToResourceTypes}
              />
            </motion.div>
          ) : currentStep === "types" && activeConnectionId && activeConnection ? (
            <motion.div
              key={`types-${activeConnectionId}`}
              custom={direction.current}
              variants={slideVariants}
              initial="enter"
              animate="center"
              exit="exit"
              transition={{ duration: 0.15, ease: "easeInOut" }}
              className="flex flex-col h-full"
            >
              <DialogHeader>
                <button
                  type="button"
                  onClick={goBackToConnections}
                  className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors w-fit mb-2"
                >
                  <HugeiconsIcon icon={ArrowLeft01Icon} size={14} />
                  Back
                </button>
                <div className="flex items-center gap-3">
                  <IntegrationLogo provider={activeConnection.provider ?? ""} size={28} />
                  <div>
                    <DialogTitle>{activeConnection.display_name ?? activeConnection.provider}</DialogTitle>
                    <DialogDescription>Select which resource types to configure</DialogDescription>
                  </div>
                </div>
              </DialogHeader>

              <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
                {!configurableResources || configurableResources.length === 0 ? (
                  <p className="text-sm text-muted-foreground py-8 text-center">
                    No configurable resources for this provider.
                  </p>
                ) : (
                  configurableResources.map((resource) => {
                    const count = getTypeSelectedCount(activeConnectionId, resource.key)
                    return (
                      <button
                        key={resource.key}
                        type="button"
                        onClick={() => openResourceType(resource.key)}
                        className="flex items-center justify-between rounded-xl border border-border px-4 py-3 text-left transition-colors hover:border-primary"
                      >
                        <div className="min-w-0">
                          <p className="text-sm font-medium text-foreground">{resource.display_name}</p>
                          <p className="text-xs text-muted-foreground mt-0.5">{resource.description}</p>
                        </div>
                        <div className="flex items-center gap-2 shrink-0 ml-3">
                          {count > 0 && (
                            <span className="text-xs font-medium text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded-full">
                              {count} selected
                            </span>
                          )}
                          <HugeiconsIcon icon={ArrowRight01Icon} size={14} className="text-muted-foreground" />
                        </div>
                      </button>
                    )
                  })
                )}
              </div>
            </motion.div>
          ) : (
            <motion.div
              key="connections"
              custom={direction.current}
              variants={slideVariants}
              initial="enter"
              animate="center"
              exit="exit"
              transition={{ duration: 0.15, ease: "easeInOut" }}
              className="flex flex-col h-full"
            >
              <DialogHeader>
                <DialogTitle>Configure resources</DialogTitle>
                <DialogDescription>
                  Choose which resources each integration can access.
                </DialogDescription>
              </DialogHeader>

              <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
                {agentConnections.length === 0 ? (
                  <p className="text-sm text-muted-foreground py-8 text-center">
                    This agent has no integrations. Add integrations first.
                  </p>
                ) : (
                  agentConnections.map((connection) => {
                    const connectionId = connection.id as string
                    const count = getSelectedCount(connectionId)
                    return (
                      <button
                        key={connectionId}
                        type="button"
                        onClick={() => openConnection(connectionId)}
                        className="flex items-center justify-between rounded-xl border border-border px-4 py-3 text-left transition-colors hover:border-primary"
                      >
                        <div className="flex items-center gap-3 min-w-0">
                          <IntegrationLogo provider={connection.provider ?? ""} size={28} />
                          <div className="min-w-0">
                            <p className="text-sm font-medium text-foreground truncate">
                              {connection.display_name ?? connection.provider}
                            </p>
                            <p className="text-xs text-muted-foreground">
                              {connection.provider}
                            </p>
                          </div>
                        </div>
                        <div className="flex items-center gap-2 shrink-0 ml-3">
                          {count > 0 && (
                            <span className="text-xs font-medium text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded-full">
                              {count} resources
                            </span>
                          )}
                          <HugeiconsIcon icon={ArrowRight01Icon} size={14} className="text-muted-foreground" />
                        </div>
                      </button>
                    )
                  })
                )}
              </div>

              <div className="pt-4 mt-auto">
                <Button className="w-full" onClick={handleSave} loading={updateAgent.isPending}>
                  Save resources
                </Button>
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </DialogContent>
    </Dialog>
  )
}

interface ResourceInstancesViewProps {
  connectionId: string
  resourceType: string
  selectedItems: ResourceItem[]
  isSelected: (resourceId: string) => boolean
  onToggle: (item: ResourceItem) => void
  onBack: () => void
}

function ResourceInstancesView({
  connectionId,
  resourceType,
  isSelected,
  onToggle,
  onBack,
}: ResourceInstancesViewProps) {
  const { data, isLoading } = $api.useQuery("get", "/v1/connections/{id}/resources/{type}", {
    params: { path: { id: connectionId, type: resourceType } },
  })

  const resourceItems: ResourceItem[] = ((data as Record<string, unknown> | undefined)?.resources as ResourceItem[] | undefined) ?? []

  return (
    <>
      <DialogHeader>
        <button
          type="button"
          onClick={onBack}
          className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors w-fit mb-2"
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} size={14} />
          Back
        </button>
        <DialogTitle className="capitalize">{resourceType.replace(/_/g, " ")}s</DialogTitle>
        <DialogDescription>Select which {resourceType.replace(/_/g, " ")}s this agent can access</DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-1.5 mt-4 flex-1 overflow-y-auto">
        {isLoading ? (
          Array.from({ length: 5 }).map((_, index) => (
            <Skeleton key={index} className="h-[44px] w-full rounded-xl" />
          ))
        ) : resourceItems.length === 0 ? (
          <p className="text-sm text-muted-foreground py-8 text-center">
            No {resourceType.replace(/_/g, " ")}s found.
          </p>
        ) : (
          resourceItems.map((item) => {
            const selected = isSelected(item.id)
            return (
              <button
                key={item.id}
                type="button"
                onClick={() => onToggle(item)}
                className={`flex items-center justify-between rounded-xl border px-4 py-2.5 text-left transition-colors ${
                  selected
                    ? "border-emerald-500/30 bg-emerald-500/5"
                    : "border-border hover:border-primary"
                }`}
              >
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-foreground truncate">{item.name}</p>
                  {item.id !== item.name && (
                    <p className="text-[11px] text-muted-foreground font-mono truncate">{item.id}</p>
                  )}
                </div>
                {selected && (
                  <HugeiconsIcon icon={Tick02Icon} size={16} className="text-emerald-600 dark:text-emerald-400 shrink-0 ml-2" />
                )}
              </button>
            )
          })
        )}
      </div>
    </>
  )
}
