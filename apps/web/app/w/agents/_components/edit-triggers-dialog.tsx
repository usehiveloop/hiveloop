"use client"

import { useState, useRef, useCallback, useEffect } from "react"
import { AnimatePresence, motion } from "motion/react"
import {
  Dialog,
  DialogContent,
} from "@/components/ui/dialog"
import { $api } from "@/lib/api/hooks"
import type {
  TriggerConfig,
  TriggerConditionsConfig,
} from "./create-agent/types"
import { ConnectionPickerView } from "./create-agent/step-trigger/connection-picker"
import { TriggerPickerView } from "./create-agent/step-trigger/trigger-picker"
import { ConditionBuilderView } from "./create-agent/step-trigger/condition-builder"
import { TriggerListView } from "./edit-triggers/trigger-list-view"
import { TriggerTypePickerView } from "./edit-triggers/trigger-type-picker-view"
import { HttpConfigView } from "./edit-triggers/http-config-view"
import { CronConfigView } from "./edit-triggers/cron-config-view"

export {
  TriggerTypeAvatar,
  triggerDisplayName,
  HttpEndpointPill,
} from "./edit-triggers/trigger-type-display"

type DialogView =
  | "list"
  | "type"
  | "connections"
  | "triggers"
  | "conditions"
  | "http-config"
  | "cron-config"

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
    const order: DialogView[] = [
      "list",
      "type",
      "connections",
      "triggers",
      "conditions",
      "http-config",
      "cron-config",
    ]
    navDirection.current = order.indexOf(nextView) > order.indexOf(view) ? 1 : -1
    setSearch("")
    setView(nextView)
  }

  function handleAddClick() {
    resetFlowState()
    navigateTo("type")
  }

  function handlePickType(triggerType: "webhook" | "http" | "cron") {
    if (triggerType === "webhook") {
      navigateTo("connections")
    } else if (triggerType === "http") {
      navigateTo("http-config")
    } else {
      navigateTo("cron-config")
    }
  }

  function handleSaveHttp(input: { instructions: string; secretKey: string }) {
    onAdd({
      triggerType: "http",
      connectionId: "",
      connectionName: "HTTP trigger",
      provider: "http",
      triggerKeys: [],
      triggerDisplayNames: [],
      conditions: null,
      instructions: input.instructions || undefined,
      secretKey: input.secretKey || undefined,
    })
    resetFlowState()
    navigateTo("list")
  }

  function handleSaveCron(input: { cronSchedule: string; instructions: string }) {
    onAdd({
      triggerType: "cron",
      connectionId: "",
      connectionName: "Cron trigger",
      provider: "cron",
      triggerKeys: [],
      triggerDisplayNames: [],
      conditions: null,
      cronSchedule: input.cronSchedule,
      instructions: input.instructions || undefined,
    })
    resetFlowState()
    navigateTo("list")
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
        triggerType: "webhook",
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
        triggerType: "webhook",
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
        triggerType: "webhook",
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
    if (selectedEvents.size === 0) return
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
    onOpenChange(nextOpen)
    if (!nextOpen) {
      setView("list")
      resetFlowState()
    }
  }

  function backFromTriggers() {
    if (editingIndex !== null) {
      resetFlowState()
      navigateTo("list")
    } else {
      navigateTo("connections")
    }
  }

  const configuringEvent = configuringEventKey
    ? selectedEvents.get(configuringEventKey)
    : null

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="flex h-[600px] flex-col overflow-hidden p-0 sm:max-w-md">
        <div className="flex h-full flex-col overflow-hidden p-6">
          <AnimatePresence mode="wait" custom={navDirection.current}>
            <motion.div
              key={view}
              custom={navDirection.current}
              variants={innerVariants}
              initial="enter"
              animate="center"
              exit="exit"
              transition={{ duration: 0.15, ease: "easeInOut" as const }}
              className="flex h-full flex-col"
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
              {view === "type" && (
                <TriggerTypePickerView
                  onPick={handlePickType}
                  onBack={() => navigateTo("list")}
                />
              )}
              {view === "connections" && (
                <ConnectionPickerView
                  search={search}
                  onSearchChange={setSearch}
                  onPickConnection={handlePickConnection}
                  onBack={() => navigateTo("type")}
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
              {view === "http-config" && (
                <HttpConfigView
                  onSave={handleSaveHttp}
                  onBack={() => navigateTo("type")}
                />
              )}
              {view === "cron-config" && (
                <CronConfigView
                  onSave={handleSaveCron}
                  onBack={() => navigateTo("type")}
                />
              )}
            </motion.div>
          </AnimatePresence>
        </div>
      </DialogContent>
    </Dialog>
  )
}
