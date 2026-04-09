"use client"

import { useState, useMemo, useRef, lazy, Suspense } from "react"
import { AnimatePresence, motion } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Search01Icon,
  Tick02Icon,
  Notification03Icon,
  Cancel01Icon,
  FlashIcon,
} from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"
import { useCreateAgent } from "./context"
import { RecipeEditor } from "./recipe-editor"

type TriggerView = "choice" | "connections" | "triggers" | "context"

interface ContextActionConfig {
  as: string
  action: string
  actionDisplayName: string
  ref?: string
  params?: Record<string, string>
  optional?: boolean
}

interface TriggerSelection {
  connectionId: string
  connectionName: string
  provider: string
  triggerKeys: string[]
  triggerDisplayNames: string[]
  refs: Record<string, string>
  contextActions: ContextActionConfig[]
  prompt: string
}

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

  function handleConfirmTrigger(parsedActions: ContextActionConfig[], parsedPrompt: string) {
    if (!selectedConnection || selectedTriggerKeys.length === 0) return
    setContextActions(parsedActions)
    setTriggerPrompt(parsedPrompt)
    setTriggerSelection({
      connectionId: selectedConnection.id,
      connectionName: selectedConnection.name,
      provider: selectedConnection.provider,
      triggerKeys: selectedTriggerKeys,
      triggerDisplayNames: selectedTriggerNames,
      refs: mergedRefs,
      contextActions: parsedActions,
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

/* ────────────────────────────────────────
   View 1: Choice — add trigger or skip
   ──────────────────────────────────────── */

interface ChoiceViewProps {
  triggerSelection: TriggerSelection | null
  onAddTrigger: () => void
  onRemoveTrigger: () => void
  onContinue: () => void
  onBack: () => void
}

function ChoiceView({ triggerSelection, onAddTrigger, onRemoveTrigger, onContinue, onBack }: ChoiceViewProps) {
  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Webhook trigger</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Optionally configure a webhook event that automatically starts this agent.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3 mt-6 flex-1">
        {triggerSelection ? (
          <div className="rounded-xl border border-primary/20 bg-primary/5 p-4">
            <div className="flex items-start gap-3">
              <IntegrationLogo provider={triggerSelection.provider} size={32} className="shrink-0 mt-0.5" />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-semibold text-foreground">
                  {triggerSelection.triggerKeys.length} event{triggerSelection.triggerKeys.length > 1 ? "s" : ""}
                </p>
                <p className="text-[13px] text-muted-foreground mt-0.5">
                  {triggerSelection.triggerDisplayNames.slice(0, 3).join(", ")}
                  {triggerSelection.triggerDisplayNames.length > 3 && ` +${triggerSelection.triggerDisplayNames.length - 3} more`}
                </p>
                <p className="text-[12px] text-muted-foreground mt-0.5">
                  via {triggerSelection.connectionName}
                  {triggerSelection.contextActions.length > 0 && ` · ${triggerSelection.contextActions.length} context action${triggerSelection.contextActions.length > 1 ? "s" : ""}`}
                </p>
              </div>
              <button type="button" onClick={onRemoveTrigger} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-destructive/10 transition-colors">
                <HugeiconsIcon icon={Cancel01Icon} size={14} className="text-destructive" />
              </button>
            </div>
          </div>
        ) : (
          <>
            <button type="button" onClick={onAddTrigger} className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer border border-transparent">
              <div className="flex items-center justify-center h-10 w-10 rounded-lg bg-primary/10 shrink-0">
                <HugeiconsIcon icon={FlashIcon} size={20} className="text-primary" />
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-sm font-semibold text-foreground">Add a trigger</p>
                <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">
                  Start this agent automatically when a webhook event fires — like a new issue, PR, or message.
                </p>
              </div>
              <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
            </button>
            <div className="flex items-center gap-3 px-4 py-2">
              <div className="h-px flex-1 bg-border" />
              <span className="text-xs text-muted-foreground">or</span>
              <div className="h-px flex-1 bg-border" />
            </div>
            <div className="px-4 py-2">
              <p className="text-sm text-muted-foreground text-center">Skip this step to create a manually-triggered agent.</p>
            </div>
          </>
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onContinue} className="w-full">
          {triggerSelection ? "Continue with trigger" : "Skip for now"}
        </Button>
      </div>
    </>
  )
}

/* ────────────────────────────────────────
   View 2: Connection picker
   ──────────────────────────────────────── */

interface ConnectionPickerViewProps {
  search: string
  onSearchChange: (value: string) => void
  onPickConnection: (connectionId: string, connectionName: string, provider: string) => void
  onBack: () => void
}

function ConnectionPickerView({ search, onSearchChange, onPickConnection, onBack }: ConnectionPickerViewProps) {
  const { selectedIntegrations } = useCreateAgent()
  const { data: connectionsData, isLoading } = $api.useQuery("get", "/v1/in/connections")
  const allConnections = connectionsData?.data ?? []
  const connections = useMemo(
    () => allConnections.filter((connection) => selectedIntegrations.has(connection.id!)),
    [allConnections, selectedIntegrations],
  )

  const filtered = useMemo(() => {
    if (!search.trim()) return connections
    const query = search.toLowerCase()
    return connections.filter(
      (connection) =>
        (connection.display_name ?? "").toLowerCase().includes(query) ||
        (connection.provider ?? "").toLowerCase().includes(query),
    )
  }, [connections, search])

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Choose connection</DialogTitle>
        </div>
        <DialogDescription className="mt-2">Pick which integration connection this trigger listens on.</DialogDescription>
      </DialogHeader>

      <div className="relative mt-4">
        <HugeiconsIcon icon={Search01Icon} size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
        <Input placeholder="Search connections..." value={search} onChange={(event) => onSearchChange(event.target.value)} className="pl-9 h-9" />
      </div>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {isLoading ? (
          Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-[64px] w-full rounded-xl" />)
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-3">
            <p className="text-sm text-muted-foreground">No connections found.</p>
          </div>
        ) : (
          filtered.map((connection) => (
            <button
              key={connection.id}
              type="button"
              onClick={() => onPickConnection(connection.id!, connection.display_name ?? connection.provider ?? "", connection.provider ?? "")}
              className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer border border-transparent"
            >
              <IntegrationLogo provider={connection.provider ?? ""} size={32} className="shrink-0 mt-0.5" />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-semibold text-foreground">{connection.display_name}</p>
                <p className="text-[13px] text-muted-foreground mt-0.5">{connection.provider}</p>
              </div>
              <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
            </button>
          ))
        )}
      </div>
    </>
  )
}

/* ────────────────────────────────────────
   View 3: Trigger picker (multi-select)
   ──────────────────────────────────────── */

interface TriggerPickerViewProps {
  provider: string
  connectionName: string
  search: string
  onSearchChange: (value: string) => void
  selectedKeys: string[]
  onToggleTrigger: (triggerKey: string, displayName: string, refs: Record<string, string>) => void
  onConfirm: () => void
  onBack: () => void
}

function TriggerPickerView({ provider, connectionName, search, onSearchChange, selectedKeys, onToggleTrigger, onConfirm, onBack }: TriggerPickerViewProps) {
  const { data: triggersData, isLoading } = $api.useQuery(
    "get",
    "/v1/catalog/integrations/{id}/triggers",
    { params: { path: { id: provider } } },
    { enabled: !!provider },
  )

  const triggers = useMemo(() => {
    if (!triggersData || !Array.isArray(triggersData)) return []
    return triggersData
  }, [triggersData])

  const filtered = useMemo(() => {
    if (!search.trim()) return triggers
    const query = search.toLowerCase()
    return triggers.filter(
      (trigger) =>
        (trigger.display_name ?? "").toLowerCase().includes(query) ||
        (trigger.description ?? "").toLowerCase().includes(query) ||
        (trigger.key ?? "").toLowerCase().includes(query),
    )
  }, [triggers, search])

  const selectedSet = new Set(selectedKeys)

  const grouped = useMemo(() => {
    const groups: Record<string, typeof filtered> = {}
    for (const trigger of filtered) {
      const resourceType = trigger.resource_type || "other"
      if (!groups[resourceType]) groups[resourceType] = []
      groups[resourceType].push(trigger)
    }
    // Sort selected triggers to top within each group.
    for (const group of Object.values(groups)) {
      group.sort((first, second) => {
        const firstSelected = selectedSet.has(first.key ?? "") ? 0 : 1
        const secondSelected = selectedSet.has(second.key ?? "") ? 0 : 1
        return firstSelected - secondSelected
      })
    }
    return groups
  }, [filtered, selectedSet])

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <div className="flex items-center gap-2.5">
            <IntegrationLogo provider={provider} size={20} />
            <DialogTitle>Pick triggers</DialogTitle>
          </div>
        </div>
        <DialogDescription className="mt-2">
          Choose which webhook events start this agent. You can select multiple.
        </DialogDescription>
      </DialogHeader>

      <div className="relative mt-4">
        <HugeiconsIcon icon={Search01Icon} size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
        <Input placeholder="Search triggers..." value={search} onChange={(event) => onSearchChange(event.target.value)} className="pl-9 h-9" />
      </div>

      <div className="flex flex-col gap-1 mt-4 flex-1 overflow-y-auto">
        {isLoading ? (
          Array.from({ length: 6 }).map((_, index) => <Skeleton key={index} className="h-[56px] w-full rounded-xl" />)
        ) : Object.keys(grouped).length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-3">
            <div className="flex items-center justify-center size-12 rounded-full bg-muted">
              <HugeiconsIcon icon={Notification03Icon} size={20} className="text-muted-foreground" />
            </div>
            <p className="text-sm text-muted-foreground">No triggers available for this provider.</p>
          </div>
        ) : (
          Object.entries(grouped).map(([resourceType, resourceTriggers]) => (
            <div key={resourceType} className="mb-3">
              <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground px-1 mb-1.5">{resourceType}</p>
              <div className="flex flex-col gap-1">
                {resourceTriggers.map((trigger) => {
                  const isSelected = selectedSet.has(trigger.key ?? "")
                  return (
                    <button
                      key={trigger.key}
                      type="button"
                      onClick={() => onToggleTrigger(trigger.key ?? "", trigger.display_name ?? "", (trigger as any).refs ?? {})}
                      className={`flex items-start gap-3 w-full rounded-xl p-3 text-left transition-colors cursor-pointer ${
                        isSelected ? "bg-primary/5 border border-primary/20" : "bg-muted/50 hover:bg-muted border border-transparent"
                      }`}
                    >
                      <div className="flex items-center justify-center h-6 w-6 rounded-md bg-amber-500/10 shrink-0 mt-0.5">
                        <HugeiconsIcon icon={FlashIcon} size={12} className="text-amber-500" />
                      </div>
                      <div className="flex-1 min-w-0">
                        <p className="text-sm font-medium text-foreground">{trigger.display_name}</p>
                        <p className="text-[12px] text-muted-foreground mt-0.5 line-clamp-1">{trigger.description}</p>
                      </div>
                      {isSelected && <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0 mt-0.5" />}
                    </button>
                  )
                })}
              </div>
            </div>
          ))
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onConfirm} disabled={selectedKeys.length === 0} className="w-full">
          {selectedKeys.length > 0
            ? `Continue with ${selectedKeys.length} event${selectedKeys.length > 1 ? "s" : ""}`
            : "Select at least one event"}
        </Button>
      </div>
    </>
  )
}

/* ────────────────────────────────────────
   View 4: Context recipe (YAML editor)
   ──────────────────────────────────────── */

interface ContextConfigViewProps {
  provider: string
  triggerDisplayNames: string[]
  triggerKeys: string[]
  refs: Record<string, string>
  contextActions: ContextActionConfig[]
  prompt: string
  onConfirm: (actions: ContextActionConfig[], prompt: string) => void
  onBack: () => void
}

interface ParsedRecipe {
  context: ContextActionConfig[]
  prompt: string
}

function recipeToYaml(actions: ContextActionConfig[], prompt: string, triggerKeys: string[], refs: Record<string, string>): string {
  const lines: string[] = []

  // Context section.
  if (actions.length === 0) {
    lines.push("context:")
    lines.push("  # - as: issue")
    lines.push("  #   action: issues_get")
    lines.push("  #   ref: issue")
  } else {
    lines.push("context:")
    for (const action of actions) {
      lines.push(`  - as: ${action.as}`)
      lines.push(`    action: ${action.action}`)
      if (action.ref) lines.push(`    ref: ${action.ref}`)
      if (action.params && Object.keys(action.params).length > 0) {
        lines.push("    params:")
        for (const [key, value] of Object.entries(action.params)) {
          const stringValue = String(value)
          if (stringValue.includes("{{") || stringValue.includes(" ")) {
            lines.push(`      ${key}: "${stringValue}"`)
          } else {
            lines.push(`      ${key}: ${stringValue}`)
          }
        }
      }
      if (action.optional) lines.push("    optional: true")
    }
  }

  lines.push("")

  // Prompt section.
  if (prompt) {
    lines.push("prompt: |")
    for (const promptLine of prompt.split("\n")) {
      lines.push(`  ${promptLine}`)
    }
  } else {
    // Generate starter prompt from trigger context.
    const eventNames = triggerKeys.map((key) => key.split(".").map((word) => word.charAt(0).toUpperCase() + word.slice(1)).join(" ")).join(" / ")
    const refNames = Object.keys(refs)
    const hasIssue = refNames.includes("issue_number")
    const hasPR = refNames.includes("pull_number")

    lines.push("prompt: |")
    lines.push(`  You are an automation agent for $refs.repository.`)
    lines.push("")
    lines.push(`  ## Triggering Event`)
    lines.push(`  ${eventNames}`)
    lines.push("")
    if (hasIssue) {
      lines.push("  ## Issue")
      lines.push("  **{{$issue.title}}**")
      lines.push("  {{$issue.body}}")
      lines.push("")
    }
    if (hasPR) {
      lines.push("  ## Pull Request")
      lines.push("  **{{$pr.title}}**")
      lines.push("  {{$pr.body}}")
      lines.push("")
    }
    lines.push("  Analyze the event and take appropriate action.")
  }

  return lines.join("\n") + "\n"
}

function parseRecipeYaml(yamlText: string): ParsedRecipe | null {
  try {
    const actions: ContextActionConfig[] = []
    const lines = yamlText.split("\n")
    let currentAction: Partial<ContextActionConfig> | null = null
    let inParams = false
    let section: "none" | "context" | "prompt" = "none"
    let inPromptBlock = false
    const promptLines: string[] = []

    function flushAction() {
      if (currentAction?.as && currentAction?.action) {
        actions.push({ as: currentAction.as, action: currentAction.action, actionDisplayName: currentAction.action, ref: currentAction.ref, params: currentAction.params, optional: currentAction.optional })
      }
      currentAction = null
      inParams = false
    }

    for (const rawLine of lines) {
      const line = rawLine.trimEnd()
      const trimmed = line.trim()

      if (trimmed === "context:") { flushAction(); section = "context"; inPromptBlock = false; continue }
      if (trimmed.startsWith("prompt:")) {
        flushAction()
        section = "prompt"
        inPromptBlock = true
        const inlineValue = trimmed.slice("prompt:".length).trim()
        if (inlineValue && inlineValue !== "|") promptLines.push(inlineValue)
        continue
      }

      if (section === "prompt" && inPromptBlock) {
        if (line.startsWith("  ")) { promptLines.push(line.slice(2)); continue }
        if (trimmed === "") { promptLines.push(""); continue }
        inPromptBlock = false
        continue
      }

      if (section !== "context") continue
      if (trimmed === "" || trimmed.startsWith("#")) continue

      if (trimmed.startsWith("- as:")) {
        flushAction()
        currentAction = { as: trimmed.replace("- as:", "").trim() }
        continue
      }

      if (!currentAction) continue

      if (trimmed.startsWith("action:")) { currentAction.action = trimmed.replace("action:", "").trim(); inParams = false }
      else if (trimmed.startsWith("ref:")) { currentAction.ref = trimmed.replace("ref:", "").trim(); inParams = false }
      else if (trimmed.startsWith("optional:")) { currentAction.optional = trimmed.replace("optional:", "").trim() === "true"; inParams = false }
      else if (trimmed === "params:") { currentAction.params = {}; inParams = true }
      else if (inParams && trimmed.includes(":")) {
        const colonIndex = trimmed.indexOf(":")
        const paramKey = trimmed.slice(0, colonIndex).trim()
        let paramValue = trimmed.slice(colonIndex + 1).trim()
        if ((paramValue.startsWith('"') && paramValue.endsWith('"')) || (paramValue.startsWith("'") && paramValue.endsWith("'"))) {
          paramValue = paramValue.slice(1, -1)
        }
        if (!currentAction.params) currentAction.params = {}
        currentAction.params[paramKey] = paramValue
      }
    }

    flushAction()
    while (promptLines.length > 0 && promptLines[promptLines.length - 1].trim() === "") promptLines.pop()

    return { context: actions, prompt: promptLines.join("\n") }
  } catch {
    return null
  }
}

function ContextConfigView({
  provider,
  triggerDisplayNames,
  triggerKeys,
  refs,
  contextActions,
  prompt,
  onConfirm,
  onBack,
}: ContextConfigViewProps) {
  const [yamlText, setYamlText] = useState(() => recipeToYaml(contextActions, prompt, triggerKeys, refs))
  const [parseError, setParseError] = useState<string | null>(null)

  // Fetch schema paths for autocomplete.
  const { data: schemaPathsData } = $api.useQuery(
    "get",
    "/v1/catalog/integrations/{id}/schema-paths",
    { params: { path: { id: provider } } },
    { enabled: !!provider },
  )

  // Fetch read actions for this provider (for action key autocomplete).
  const { data: readActionsData } = $api.useQuery(
    "get",
    "/v1/catalog/integrations/{id}/actions",
    { params: { path: { id: provider }, query: { access: "read" } } },
    { enabled: !!provider },
  )

  const refNames = useMemo(() => Object.keys(refs), [refs])

  const actionPaths = useMemo(() => {
    const rawActions = (schemaPathsData as any)?.actions ?? {}
    const result: Record<string, Array<{ path: string; type: string }>> = {}
    for (const [actionKey, actionData] of Object.entries(rawActions)) {
      const paths = (actionData as any)?.paths as Array<{ path: string; type: string }> | undefined
      if (paths) result[actionKey] = paths
    }
    return result
  }, [schemaPathsData])

  const actionKeysForEditor = useMemo(() => {
    if (!readActionsData || !Array.isArray(readActionsData)) return []
    return readActionsData.map((action) => ({
      key: action.key ?? "",
      displayName: action.display_name ?? "",
      access: action.access ?? "",
      resourceType: action.resource_type ?? "",
    }))
  }, [allActionsData])

  function handleConfirm() {
    const parsed = parseRecipeYaml(yamlText)
    if (!parsed) {
      setParseError("Could not parse YAML. Check your formatting.")
      return
    }
    setParseError(null)
    onConfirm(parsed.context, parsed.prompt)
  }

  const refsList = refNames.map((refName) => `$refs.${refName}`).join(", ")

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Trigger recipe</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Define the context to gather and the prompt for your agent.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3 mt-4 flex-1 min-h-0">
        <RecipeEditor
          value={yamlText}
          onChange={(newValue) => { setYamlText(newValue); setParseError(null) }}
          refNames={refNames}
          actionPaths={actionPaths}
          actionKeys={actionKeysForEditor}
        />

        {parseError && (
          <p className="text-xs text-destructive px-1">{parseError}</p>
        )}

        <div className="rounded-lg bg-muted/50 px-3 py-2 shrink-0">
          <p className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider mb-1">Available refs</p>
          <p className="text-[11px] font-mono text-muted-foreground leading-relaxed break-all">{refsList}</p>
        </div>
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={handleConfirm} className="w-full">
          Confirm trigger
        </Button>
      </div>
    </>
  )
}
