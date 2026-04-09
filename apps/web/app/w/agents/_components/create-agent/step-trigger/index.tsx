"use client"

import { useState, useRef } from "react"
import { AnimatePresence, motion } from "motion/react"
import { useCreateAgent } from "../context"
import type { TriggerView, ContextActionConfig, TriggerConditionsConfig, TriggerSelection } from "./types"
import { ChoiceView } from "./choice-view"
import { ConnectionPickerView } from "./connection-picker"
import { TriggerPickerView } from "./trigger-picker"
import { ContextConfigView } from "./recipe-view"

export function StepTrigger() {
  const { goTo } = useCreateAgent()
  const [view, setView] = useState<TriggerView>("choice")
  const [selectedConnection, setSelectedConnection] = useState<{
    id: string
    name: string
    provider: string
  } | null>(null)
  const [selectedTriggerKeys, setSelectedTriggerKeys] = useState<string[]>([])
  const [selectedTriggerNames, setSelectedTriggerNames] = useState<string[]>([])
  const [mergedRefs, setMergedRefs] = useState<Record<string, string>>({})
  const [contextActions, setContextActions] = useState<ContextActionConfig[]>([])
  const [triggerConditions, setTriggerConditions] = useState<TriggerConditionsConfig | undefined>(undefined)
  const [triggerPrompt, setTriggerPrompt] = useState("")
  const [triggerSelection, setTriggerSelection] = useState<TriggerSelection | null>(null)
  const [search, setSearch] = useState("")
  const navDirection = useRef<1 | -1>(1)

  const innerVariants = {
    enter: (direction: number) => ({ x: direction > 0 ? 60 : -60, opacity: 0 }),
    center: { x: 0, opacity: 1 },
    exit: (direction: number) => ({ x: direction > 0 ? -60 : 60, opacity: 0 }),
  }

  function navigateTo(nextView: TriggerView) {
    const order: TriggerView[] = ["choice", "connections", "triggers", "context"]
    navDirection.current = order.indexOf(nextView) > order.indexOf(view) ? 1 : -1
    setSearch("")
    setView(nextView)
  }

  function handlePickConnection(connectionId: string, connectionName: string, provider: string) {
    setSelectedConnection({ id: connectionId, name: connectionName, provider })
    setSelectedTriggerKeys([])
    setSelectedTriggerNames([])
    setMergedRefs({})
    navigateTo("triggers")
  }

  function handleToggleTrigger(triggerKey: string, displayName: string, refs: Record<string, string>) {
    setSelectedTriggerKeys((previous) => {
      if (previous.includes(triggerKey)) {
        setSelectedTriggerNames((names) => names.filter((_, index) => previous[index] !== triggerKey))
        return previous.filter((key) => key !== triggerKey)
      }
      setSelectedTriggerNames((names) => [...names, displayName])
      return [...previous, triggerKey]
    })
    // Merge refs from all selected triggers.
    setMergedRefs((previous) => ({ ...previous, ...refs }))
  }

  function handleConfirmTriggers() {
    setContextActions([])
    navigateTo("context")
  }

  function handleConfirmTrigger(parsedActions: ContextActionConfig[], parsedConditions: TriggerConditionsConfig | undefined, parsedPrompt: string) {
    if (!selectedConnection || selectedTriggerKeys.length === 0) return
    setContextActions(parsedActions)
    setTriggerConditions(parsedConditions)
    setTriggerPrompt(parsedPrompt)
    setTriggerSelection({
      connectionId: selectedConnection.id,
      connectionName: selectedConnection.name,
      provider: selectedConnection.provider,
      triggerKeys: selectedTriggerKeys,
      triggerDisplayNames: selectedTriggerNames,
      refs: mergedRefs,
      contextActions: parsedActions,
      conditions: parsedConditions,
      prompt: parsedPrompt,
    })
    navigateTo("choice")
  }

  function handleRemoveTrigger() {
    setTriggerSelection(null)
    setSelectedConnection(null)
    setSelectedTriggerKeys([])
    setSelectedTriggerNames([])
    setMergedRefs({})
    setContextActions([])
    setTriggerConditions(undefined)
    setTriggerPrompt("")
  }

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
              triggerSelection={triggerSelection}
              onAddTrigger={() => navigateTo("connections")}
              onRemoveTrigger={handleRemoveTrigger}
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
              selectedKeys={selectedTriggerKeys}
              onToggleTrigger={handleToggleTrigger}
              onConfirm={handleConfirmTriggers}
              onBack={() => navigateTo("connections")}
            />
          )}
          {view === "context" && selectedConnection && (
            <ContextConfigView
              provider={selectedConnection.provider}
              triggerDisplayNames={selectedTriggerNames}
              triggerKeys={selectedTriggerKeys}
              refs={mergedRefs}
              contextActions={contextActions}
              conditions={triggerConditions}
              prompt={triggerPrompt}
              onConfirm={handleConfirmTrigger}
              onBack={() => navigateTo("triggers")}
            />
          )}
        </motion.div>
      </AnimatePresence>
    </div>
  )
}

export default StepTrigger
