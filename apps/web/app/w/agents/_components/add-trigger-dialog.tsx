"use client"

import { useState, useRef, useCallback } from "react"
import { AnimatePresence, motion } from "motion/react"
import { Dialog, DialogContent } from "@/components/ui/dialog"
import type { TriggerConfig, TriggerConditionsConfig } from "./create-agent/types"
import type { TriggerView } from "./create-agent/step-trigger/types"
import { ConnectionPickerView } from "./create-agent/step-trigger/connection-picker"
import { TriggerPickerView } from "./create-agent/step-trigger/trigger-picker"
import { ConditionBuilderView } from "./create-agent/step-trigger/condition-builder"

interface SelectedEvent {
  key: string
  displayName: string
  refs: Record<string, string>
  conditions: TriggerConditionsConfig | null
}

interface AddTriggerDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onAdd: (trigger: TriggerConfig) => void
}

export function AddTriggerDialog({ open, onOpenChange, onAdd }: AddTriggerDialogProps) {
  const [view, setView] = useState<TriggerView>("connections")
  const [selectedConnection, setSelectedConnection] = useState<{
    id: string
    name: string
    provider: string
  } | null>(null)
  const [selectedEvents, setSelectedEvents] = useState<Map<string, SelectedEvent>>(new Map())
  const [configuringEventKey, setConfiguringEventKey] = useState<string | null>(null)
  const [search, setSearch] = useState("")
  const navDirection = useRef<1 | -1>(1)

  const innerVariants = {
    enter: (direction: number) => ({ x: direction > 0 ? 60 : -60, opacity: 0 }),
    center: { x: 0, opacity: 1 },
    exit: (direction: number) => ({ x: direction > 0 ? -60 : 60, opacity: 0 }),
  }

  function reset() {
    setView("connections")
    setSelectedConnection(null)
    setSelectedEvents(new Map())
    setConfiguringEventKey(null)
    setSearch("")
  }

  function navigateTo(nextView: TriggerView) {
    const order: TriggerView[] = ["choice", "connections", "triggers", "conditions"]
    navDirection.current = order.indexOf(nextView) > order.indexOf(view) ? 1 : -1
    setSearch("")
    setView(nextView)
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
      const event = next.get(configuringEventKey)
      if (event) {
        next.set(configuringEventKey, { ...event, conditions })
      }
      return next
    })
    setConfiguringEventKey(null)
    navigateTo("triggers")
  }

  function handleConfirmSelection() {
    if (!selectedConnection || selectedEvents.size === 0) return

    const withFilters: SelectedEvent[] = []
    const withoutFilters: SelectedEvent[] = []

    for (const event of selectedEvents.values()) {
      if (event.conditions && event.conditions.conditions.length > 0) {
        withFilters.push(event)
      } else {
        withoutFilters.push(event)
      }
    }

    if (withoutFilters.length > 0) {
      onAdd({
        connectionId: selectedConnection.id,
        connectionName: selectedConnection.name,
        provider: selectedConnection.provider,
        triggerKeys: withoutFilters.map((event) => event.key),
        triggerDisplayNames: withoutFilters.map((event) => event.displayName),
        conditions: null,
      })
    }

    for (const event of withFilters) {
      onAdd({
        connectionId: selectedConnection.id,
        connectionName: selectedConnection.name,
        provider: selectedConnection.provider,
        triggerKeys: [event.key],
        triggerDisplayNames: [event.displayName],
        conditions: event.conditions,
      })
    }

    reset()
    onOpenChange(false)
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) reset()
    onOpenChange(nextOpen)
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
              {view === "connections" && (
                <ConnectionPickerView
                  search={search}
                  onSearchChange={setSearch}
                  onPickConnection={handlePickConnection}
                  onBack={() => handleOpenChange(false)}
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
                  onBack={() => navigateTo("connections")}
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
