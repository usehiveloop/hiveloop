"use client"

import { useState, useRef, useCallback } from "react"
import { AnimatePresence, motion } from "motion/react"
import { useCreateAgent } from "../context"
import type { TriggerView } from "./types"
import type { TriggerConditionsConfig } from "../types"
import { ChoiceView } from "./choice-view"
import { ConnectionPickerView } from "./connection-picker"
import { TriggerPickerView } from "./trigger-picker"
import { ConditionBuilderView } from "./condition-builder"

// Tracks a selected event and its optional per-event filters.
interface SelectedEvent {
  key: string
  displayName: string
  refs: Record<string, string>
  conditions: TriggerConditionsConfig | null
}

export function StepTrigger() {
  const { goTo, addTrigger, selectedIntegrations } = useCreateAgent()
  const [view, setView] = useState<TriggerView>("choice")
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

    // Events with filters get their own trigger each.
    // Events without filters get grouped into one trigger.
    const withFilters: SelectedEvent[] = []
    const withoutFilters: SelectedEvent[] = []

    for (const event of selectedEvents.values()) {
      if (event.conditions && event.conditions.conditions.length > 0) {
        withFilters.push(event)
      } else {
        withoutFilters.push(event)
      }
    }

    // One grouped trigger for all filterless events.
    if (withoutFilters.length > 0) {
      addTrigger({
        connectionId: selectedConnection.id,
        connectionName: selectedConnection.name,
        provider: selectedConnection.provider,
        triggerKeys: withoutFilters.map((event) => event.key),
        triggerDisplayNames: withoutFilters.map((event) => event.displayName),
        conditions: null,
      })
    }

    // One trigger per filtered event.
    for (const event of withFilters) {
      addTrigger({
        connectionId: selectedConnection.id,
        connectionName: selectedConnection.name,
        provider: selectedConnection.provider,
        triggerKeys: [event.key],
        triggerDisplayNames: [event.displayName],
        conditions: event.conditions,
      })
    }

    setSelectedEvents(new Map())
    setSelectedConnection(null)
    navigateTo("choice")
  }

  const configuringEvent = configuringEventKey ? selectedEvents.get(configuringEventKey) : null

  return (
    <div className="flex flex-col h-full overflow-hidden">
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
          {view === "choice" && (
            <ChoiceView
              onAddTrigger={() => navigateTo("connections")}
              onContinue={() => goTo("llm-key")}
              onBack={() => goTo("integrations")}
            />
          )}
          {view === "connections" && (
            <ConnectionPickerView
              search={search}
              onSearchChange={setSearch}
              onPickConnection={handlePickConnection}
              onBack={() => navigateTo("choice")}
              connectionIds={selectedIntegrations}
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
  )
}

export default StepTrigger
