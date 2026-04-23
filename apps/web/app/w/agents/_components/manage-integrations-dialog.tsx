"use client"

import { useState, useMemo, useRef, useCallback, useEffect } from "react"
import { AnimatePresence, motion } from "motion/react"
import {
  Dialog,
  DialogContent,
} from "@/components/ui/dialog"
import { $api } from "@/lib/api/hooks"
import { ActionDetailView } from "./manage-integrations/action-detail-view"
import { IntegrationListView } from "./manage-integrations/integration-list-view"

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

const innerVariants = {
  enter: (direction: number) => ({ x: direction > 0 ? 60 : -60, opacity: 0 }),
  center: { x: 0, opacity: 1 },
  exit: (direction: number) => ({ x: direction > 0 ? -60 : 60, opacity: 0 }),
}

export function ManageIntegrationsDialog({
  open,
  onOpenChange,
  agentIntegrations,
  onSave,
}: ManageIntegrationsDialogProps) {
  const [selectedIntegrations, setSelectedIntegrations] = useState<Set<string>>(new Set())
  const [selectedActions, setSelectedActions] = useState<Record<string, Set<string>>>({})
  const [search, setSearch] = useState("")
  const [detailConnectionId, setDetailConnectionId] = useState<string | null>(null)
  const [actionSearch, setActionSearch] = useState("")
  const detailDirection = useRef<1 | -1>(1)

  // Seed local state from agentIntegrations when dialog opens
  useEffect(() => {
    if (open) {
      const ids = new Set(Object.keys(agentIntegrations))
      const actions: Record<string, Set<string>> = {}
      for (const [id, config] of Object.entries(agentIntegrations)) {
        actions[id] = new Set(config.actions)
      }
      setSelectedIntegrations(ids)
      setSelectedActions(actions)
      setSearch("")
      setDetailConnectionId(null)
      setActionSearch("")
    }
  }, [open, agentIntegrations])

  const { data: connectionsData, isLoading } = $api.useQuery("get", "/v1/in/connections")
  const connections = connectionsData?.data ?? []

  const filtered = useMemo(() => {
    if (!search.trim()) return connections
    const query = search.toLowerCase()
    return connections.filter(
      (c) =>
        (c.display_name ?? "").toLowerCase().includes(query) ||
        (c.provider ?? "").toLowerCase().includes(query),
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

  const toggleAction = useCallback((connectionId: string, actionKey: string) => {
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
  }, [])

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

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex flex-col h-[min(780px,85vh)] p-6">
        <AnimatePresence mode="wait" custom={detailDirection.current}>
          {detailConnection && detailConnectionId ? (
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
                selectedActions={selectedActions[detailConnectionId] ?? new Set()}
                onToggleAction={(actionKey) => toggleAction(detailConnectionId, actionKey)}
                onBack={closeDetail}
                onRemove={() => removeIntegration(detailConnectionId)}
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
              <IntegrationListView
                connections={filtered}
                isLoading={isLoading}
                search={search}
                onSearchChange={setSearch}
                selectedIntegrations={selectedIntegrations}
                selectedActions={selectedActions}
                onOpenDetail={openDetail}
                onSave={handleSave}
              />
            </motion.div>
          )}
        </AnimatePresence>
      </DialogContent>
    </Dialog>
  )
}
