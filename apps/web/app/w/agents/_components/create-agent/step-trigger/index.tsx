"use client"

import { useState, useRef } from "react"
import { AnimatePresence, motion } from "motion/react"
import { useCreateAgent } from "../context"
import type { TriggerView } from "./types"
import type { TriggerConditionsConfig } from "../types"
import { ChoiceView } from "./choice-view"
import { ConnectionPickerView } from "./connection-picker"
import { TriggerPickerView } from "./trigger-picker"
import { ConditionBuilderView } from "./condition-builder"

export function StepTrigger() {
  const { goTo, addTrigger, triggers } = useCreateAgent()
  const [view, setView] = useState<TriggerView>("choice")
  const [selectedConnection, setSelectedConnection] = useState<{
    id: string
    name: string
    provider: string
  } | null>(null)
  // The single event currently being configured (gear icon clicked).
  const [configuringEvent, setConfiguringEvent] = useState<{
    key: string
    displayName: string
    refs: Record<string, string>
  } | null>(null)
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
    navigateTo("triggers")
  }

  function handleConfigureEvent(triggerKey: string, displayName: string, refs: Record<string, string>) {
    setConfiguringEvent({ key: triggerKey, displayName, refs })
    navigateTo("conditions")
  }

  function handleConfirmConditions(conditions: TriggerConditionsConfig | null) {
    if (!selectedConnection || !configuringEvent) return
    addTrigger({
      connectionId: selectedConnection.id,
      connectionName: selectedConnection.name,
      provider: selectedConnection.provider,
      triggerKeys: [configuringEvent.key],
      triggerDisplayNames: [configuringEvent.displayName],
      conditions,
    })
    setConfiguringEvent(null)
    navigateTo("triggers")
  }

  // Trigger keys already configured for the current connection.
  const configuredKeysForConnection = selectedConnection
    ? new Set(
        triggers
          .filter((trigger) => trigger.connectionId === selectedConnection.id)
          .flatMap((trigger) => trigger.triggerKeys)
      )
    : new Set<string>()

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
            />
          )}
          {view === "triggers" && selectedConnection && (
            <TriggerPickerView
              provider={selectedConnection.provider}
              connectionName={selectedConnection.name}
              search={search}
              onSearchChange={setSearch}
              configuredKeys={configuredKeysForConnection}
              onConfigureEvent={handleConfigureEvent}
              onDone={() => navigateTo("choice")}
              onBack={() => navigateTo("connections")}
            />
          )}
          {view === "conditions" && selectedConnection && configuringEvent && (
            <ConditionBuilderView
              provider={selectedConnection.provider}
              triggerDisplayNames={[configuringEvent.displayName]}
              refs={configuringEvent.refs}
              onConfirm={handleConfirmConditions}
              onBack={() => navigateTo("triggers")}
            />
          )}
        </motion.div>
      </AnimatePresence>
    </div>
  )
}

export default StepTrigger
