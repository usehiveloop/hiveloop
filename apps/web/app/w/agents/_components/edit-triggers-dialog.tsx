"use client"

import { useState, useRef, useCallback, useEffect } from "react"
import { AnimatePresence, motion } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  FlashIcon,
  Cancel01Icon,
  Add01Icon,
  Edit02Icon,
} from "@hugeicons/core-free-icons"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"
import type { TriggerConfig, TriggerConditionsConfig } from "./create-agent/types"
import { ConnectionPickerView } from "./create-agent/step-trigger/connection-picker"
import { TriggerPickerView } from "./create-agent/step-trigger/trigger-picker"
import { ConditionBuilderView } from "./create-agent/step-trigger/condition-builder"

type DialogView = "list" | "connections" | "triggers" | "conditions"

interface SelectedEvent {
  key: string
  displayName: string
  refs: Record<string, string>
  conditions: TriggerConditionsConfig | null
}

interface EditTriggersDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  triggers: TriggerConfig[]
  connectionIds: Set<string>
  onAdd: (trigger: TriggerConfig) => void
  onRemove: (index: number) => void
  onUpdate: (index: number, newTriggers: TriggerConfig[]) => void
}

export function EditTriggersDialog({
  open,
  onOpenChange,
  triggers,
  connectionIds,
  onAdd,
  onRemove,
  onUpdate,
}: EditTriggersDialogProps) {
  const [view, setView] = useState<DialogView>("list")
  const [editingIndex, setEditingIndex] = useState<number | null>(null)
  const [selectedConnection, setSelectedConnection] = useState<{
    id: string
    name: string
    provider: string
  } | null>(null)
  const [selectedEvents, setSelectedEvents] = useState<Map<string, SelectedEvent>>(new Map())
  const [configuringEventKey, setConfiguringEventKey] = useState<string | null>(null)
  const [search, setSearch] = useState("")
  const navDirection = useRef<1 | -1>(1)

  const { data: catalogData } = $api.useQuery(
    "get",
    "/v1/catalog/integrations/{id}/triggers",
    { params: { path: { id: selectedConnection?.provider ?? "" } } },
    { enabled: !!selectedConnection?.provider },
  )

  useEffect(() => {
    if (!catalogData) return
    const catalogTriggers = catalogData.triggers ?? []
    setSelectedEvents((previous) => {
      let changed = false
      const next = new Map(previous)
      for (const [key, event] of next) {
        if (Object.keys(event.refs).length > 0) continue
        const catalogTrigger = catalogTriggers.find((candidate) => candidate.key === key)
        const refs = (catalogTrigger as Record<string, unknown> | undefined)?.refs as
          | Record<string, string>
          | undefined
        if (refs && Object.keys(refs).length > 0) {
          next.set(key, { ...event, refs })
          changed = true
        }
      }
      return changed ? next : previous
    })
  }, [catalogData])

  const innerVariants = {
    enter: (direction: number) => ({ x: direction > 0 ? 60 : -60, opacity: 0 }),
    center: { x: 0, opacity: 1 },
    exit: (direction: number) => ({ x: direction > 0 ? -60 : 60, opacity: 0 }),
  }

  function resetFlowState() {
    setSelectedConnection(null)
    setSelectedEvents(new Map())
    setConfiguringEventKey(null)
    setEditingIndex(null)
    setSearch("")
  }

  function navigateTo(nextView: DialogView) {
    const order: DialogView[] = ["list", "connections", "triggers", "conditions"]
    navDirection.current = order.indexOf(nextView) > order.indexOf(view) ? 1 : -1
    setSearch("")
    setView(nextView)
  }

  function handleAddClick() {
    resetFlowState()
    navigateTo("connections")
  }

  function handleEditClick(index: number) {
    const trigger = triggers[index]
    if (!trigger) return
    setEditingIndex(index)
    setSelectedConnection({
      id: trigger.connectionId,
      name: trigger.connectionName,
      provider: trigger.provider,
    })
    const nextEvents = new Map<string, SelectedEvent>()
    trigger.triggerKeys.forEach((key, keyIndex) => {
      nextEvents.set(key, {
        key,
        displayName: trigger.triggerDisplayNames[keyIndex] ?? key,
        refs: {},
        conditions: trigger.conditions,
      })
    })
    setSelectedEvents(nextEvents)
    setConfiguringEventKey(null)
    setSearch("")
    navigateTo("triggers")
  }

  function handlePickConnection(connectionId: string, connectionName: string, provider: string) {
    setSelectedConnection({ id: connectionId, name: connectionName, provider })
    setSelectedEvents(new Map())
    navigateTo("triggers")
  }

  const toggleEvent = useCallback((key: string, displayName: string, refs: Record<string, string>) => {
    setSelectedEvents((previous) => {
      const next = new Map(previous)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.set(key, { key, displayName, refs, conditions: null })
      }
      return next
    })
  }, [])

  const removeEvent = useCallback((key: string) => {
    setSelectedEvents((previous) => {
      const next = new Map(previous)
      next.delete(key)
      return next
    })
  }, [])

  function handleOpenConditions(key: string) {
    setConfiguringEventKey(key)
    navigateTo("conditions")
  }

  function handleConfirmConditions(conditions: TriggerConditionsConfig | null) {
    if (!configuringEventKey) return
    setSelectedEvents((previous) => {
      const next = new Map(previous)
      if (editingIndex !== null) {
        for (const [key, event] of next) {
          next.set(key, { ...event, conditions })
        }
      } else {
        const event = next.get(configuringEventKey)
        if (event) {
          next.set(configuringEventKey, { ...event, conditions })
        }
      }
      return next
    })
    setConfiguringEventKey(null)
    navigateTo("triggers")
  }

  function buildTriggersFromSelection(): TriggerConfig[] {
    if (!selectedConnection) return []
    const events = Array.from(selectedEvents.values())

    // In edit mode, preserve the 1-trigger shape: one conditions set applied to
    // all selected keys. A single backend trigger can't hold per-key filters.
    if (editingIndex !== null) {
      const sharedConditions =
        events.find((event) => event.conditions && event.conditions.conditions.length > 0)
          ?.conditions ?? null
      return [{
        connectionId: selectedConnection.id,
        connectionName: selectedConnection.name,
        provider: selectedConnection.provider,
        triggerKeys: events.map((event) => event.key),
        triggerDisplayNames: events.map((event) => event.displayName),
        conditions: sharedConditions,
      }]
    }

    const withFilters: SelectedEvent[] = []
    const withoutFilters: SelectedEvent[] = []
    for (const event of events) {
      if (event.conditions && event.conditions.conditions.length > 0) {
        withFilters.push(event)
      } else {
        withoutFilters.push(event)
      }
    }
    const result: TriggerConfig[] = []
    if (withoutFilters.length > 0) {
      result.push({
        connectionId: selectedConnection.id,
        connectionName: selectedConnection.name,
        provider: selectedConnection.provider,
        triggerKeys: withoutFilters.map((event) => event.key),
        triggerDisplayNames: withoutFilters.map((event) => event.displayName),
        conditions: null,
      })
    }
    for (const event of withFilters) {
      result.push({
        connectionId: selectedConnection.id,
        connectionName: selectedConnection.name,
        provider: selectedConnection.provider,
        triggerKeys: [event.key],
        triggerDisplayNames: [event.displayName],
        conditions: event.conditions,
      })
    }
    return result
  }

  function handleConfirmSelection() {
    if (!selectedConnection || selectedEvents.size === 0) return
    const built = buildTriggersFromSelection()
    if (editingIndex !== null) {
      onUpdate(editingIndex, built)
    } else {
      for (const trigger of built) onAdd(trigger)
    }
    resetFlowState()
    navigateTo("list")
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      resetFlowState()
      setView("list")
    }
    onOpenChange(nextOpen)
  }

  function backFromTriggers() {
    if (editingIndex !== null) {
      resetFlowState()
      navigateTo("list")
    } else {
      navigateTo("connections")
    }
  }

  const configuringEvent = configuringEventKey ? selectedEvents.get(configuringEventKey) : null

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md h-[600px] overflow-hidden flex flex-col p-0">
        <div className="flex flex-col h-full p-6 overflow-hidden">
          <AnimatePresence mode="wait" custom={navDirection.current}>
            <motion.div
              key={view}
              custom={navDirection.current}
              variants={innerVariants}
              initial="enter"
              animate="center"
              exit="exit"
              transition={{ duration: 0.15, ease: "easeInOut" as const }}
              className="flex flex-col h-full"
            >
              {view === "list" && (
                <TriggerListView
                  triggers={triggers}
                  onAdd={handleAddClick}
                  onEdit={handleEditClick}
                  onRemove={onRemove}
                  onDone={() => handleOpenChange(false)}
                />
              )}
              {view === "connections" && (
                <ConnectionPickerView
                  search={search}
                  onSearchChange={setSearch}
                  onPickConnection={handlePickConnection}
                  onBack={() => { resetFlowState(); navigateTo("list") }}
                  connectionIds={connectionIds}
                />
              )}
              {view === "triggers" && selectedConnection && (
                <TriggerPickerView
                  provider={selectedConnection.provider}
                  connectionName={selectedConnection.name}
                  search={search}
                  onSearchChange={setSearch}
                  selectedEvents={selectedEvents}
                  onToggleEvent={toggleEvent}
                  onRemoveEvent={removeEvent}
                  onConfigureEvent={handleOpenConditions}
                  onConfirm={handleConfirmSelection}
                  onBack={backFromTriggers}
                />
              )}
              {view === "conditions" && configuringEvent && (
                <ConditionBuilderView
                  provider={selectedConnection?.provider ?? ""}
                  triggerDisplayNames={[configuringEvent.displayName]}
                  refs={configuringEvent.refs}
                  initialConditions={configuringEvent.conditions}
                  onConfirm={handleConfirmConditions}
                  onBack={() => { setConfiguringEventKey(null); navigateTo("triggers") }}
                />
              )}
            </motion.div>
          </AnimatePresence>
        </div>
      </DialogContent>
    </Dialog>
  )
}

interface TriggerListViewProps {
  triggers: TriggerConfig[]
  onAdd: () => void
  onEdit: (index: number) => void
  onRemove: (index: number) => void
  onDone: () => void
}

function TriggerListView({ triggers, onAdd, onEdit, onRemove, onDone }: TriggerListViewProps) {
  return (
    <>
      <DialogHeader>
        <DialogTitle>Edit triggers</DialogTitle>
        <DialogDescription className="mt-2">
          Add, edit, or remove webhook events that invoke this agent.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {triggers.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-3 text-center">
            <div className="flex items-center justify-center size-12 rounded-full bg-muted">
              <HugeiconsIcon icon={FlashIcon} size={20} className="text-muted-foreground" />
            </div>
            <p className="text-sm text-muted-foreground max-w-xs">
              No triggers configured. Add one to invoke this agent automatically on webhook events.
            </p>
          </div>
        ) : (
          triggers.map((trigger, index) => (
            <div
              key={`${trigger.connectionId}-${index}`}
              className="flex items-start gap-3 rounded-xl border border-border bg-muted/50 p-3"
            >
              <IntegrationLogo provider={trigger.provider} size={28} className="shrink-0 mt-0.5" />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-foreground">{trigger.connectionName}</p>
                <div className="flex flex-wrap gap-1 mt-1">
                  {trigger.triggerDisplayNames.map((displayName, keyIndex) => (
                    <Badge
                      key={`${displayName}-${keyIndex}`}
                      variant="secondary"
                      className="text-[10px] font-mono"
                    >
                      {displayName}
                    </Badge>
                  ))}
                </div>
                {trigger.conditions && trigger.conditions.conditions.length > 0 && (
                  <p className="text-[11px] text-muted-foreground mt-1">
                    {trigger.conditions.conditions.length} filter
                    {trigger.conditions.conditions.length !== 1 ? "s" : ""} ({trigger.conditions.mode})
                  </p>
                )}
              </div>
              <div className="flex items-center gap-1 shrink-0">
                <button
                  type="button"
                  onClick={() => onEdit(index)}
                  className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors"
                  title="Edit"
                >
                  <HugeiconsIcon icon={Edit02Icon} size={14} className="text-muted-foreground" />
                </button>
                <button
                  type="button"
                  onClick={() => onRemove(index)}
                  className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-destructive/10 transition-colors"
                  title="Remove"
                >
                  <HugeiconsIcon icon={Cancel01Icon} size={14} className="text-destructive" />
                </button>
              </div>
            </div>
          ))
        )}

        <button
          type="button"
          onClick={onAdd}
          className="group flex items-center gap-3 w-full rounded-xl bg-muted/50 p-3 text-left transition-colors hover:bg-muted cursor-pointer border border-transparent mt-1"
        >
          <HugeiconsIcon icon={Add01Icon} size={16} className="text-muted-foreground shrink-0" />
          <span className="text-sm text-muted-foreground">Add trigger</span>
        </button>
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onDone} className="w-full">Done</Button>
      </div>
    </>
  )
}
