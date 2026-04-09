"use client"

import { useState, useMemo, useRef, useCallback } from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import { AnimatePresence, motion } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Search01Icon,
  Tick02Icon,
  Notification03Icon,
  Cancel01Icon,
  Add01Icon,
  Delete02Icon,
  FlashIcon,
  Edit02Icon,
} from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"
import { useCreateAgent } from "./context"

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

  function handleConfirmTrigger() {
    if (!selectedConnection || selectedTriggerKeys.length === 0) return
    setTriggerSelection({
      connectionId: selectedConnection.id,
      connectionName: selectedConnection.name,
      provider: selectedConnection.provider,
      triggerKeys: selectedTriggerKeys,
      triggerDisplayNames: selectedTriggerNames,
      refs: mergedRefs,
      contextActions,
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
              refs={mergedRefs}
              contextActions={contextActions}
              onSetContextActions={setContextActions}
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

  const grouped = useMemo(() => {
    const groups: Record<string, typeof filtered> = {}
    for (const trigger of filtered) {
      const resourceType = trigger.resource_type || "other"
      if (!groups[resourceType]) groups[resourceType] = []
      groups[resourceType].push(trigger)
    }
    return groups
  }, [filtered])

  const selectedSet = new Set(selectedKeys)

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
   View 4: Context recipe builder
   ──────────────────────────────────────── */

interface ContextConfigViewProps {
  provider: string
  triggerDisplayNames: string[]
  refs: Record<string, string>
  contextActions: ContextActionConfig[]
  onSetContextActions: (actions: ContextActionConfig[]) => void
  onConfirm: () => void
  onBack: () => void
}

function ContextConfigView({
  provider,
  triggerDisplayNames,
  refs,
  contextActions,
  onSetContextActions,
  onConfirm,
  onBack,
}: ContextConfigViewProps) {
  const [showActionPicker, setShowActionPicker] = useState(false)
  const [actionSearch, setActionSearch] = useState("")
  const [editingIndex, setEditingIndex] = useState<number | null>(null)
  const pickerScrollRef = useRef<HTMLDivElement>(null)

  // Fetch the provider's integration detail (includes resources with ref_bindings).
  const { data: integrationData } = $api.useQuery(
    "get",
    "/v1/catalog/integrations/{id}",
    { params: { path: { id: provider } } },
    { enabled: !!provider },
  )

  const resources = integrationData?.resources ?? {}

  // Fetch read actions for this provider.
  const { data: actionsData, isLoading } = $api.useQuery(
    "get",
    "/v1/catalog/integrations/{id}/actions",
    { params: { path: { id: provider }, query: { access: "read" } } },
    { enabled: !!provider },
  )
  const readActions = actionsData ?? []

  const filteredReadActions = useMemo(() => {
    if (!actionSearch.trim()) return readActions
    const query = actionSearch.toLowerCase()
    return readActions.filter(
      (action) =>
        (action.display_name ?? "").toLowerCase().includes(query) ||
        (action.description ?? "").toLowerCase().includes(query) ||
        (action.key ?? "").toLowerCase().includes(query),
    )
  }, [readActions, actionSearch])

  const selectedActionKeys = new Set(contextActions.map((contextAction) => contextAction.action))

  const virtualizer = useVirtualizer({
    count: filteredReadActions.length,
    getScrollElement: () => pickerScrollRef.current,
    estimateSize: () => 52,
    overscan: 10,
  })

  // Check if an action can be auto-resolved via ref_bindings.
  const getAutoRef = useCallback((actionKey: string): string | undefined => {
    const action = readActions.find((readAction) => readAction.key === actionKey)
    if (!action?.resource_type) return undefined
    const resource = resources[action.resource_type] as any
    if (!resource?.ref_bindings) return undefined
    // Check that all ref_binding values exist in our refs.
    const bindings = resource.ref_bindings as Record<string, string>
    const allRefsAvailable = Object.values(bindings).every((refValue: string) => {
      const refName = refValue.replace("$refs.", "")
      return refName in refs
    })
    return allRefsAvailable ? action.resource_type : undefined
  }, [readActions, resources, refs])

  function handleAddAction(actionKey: string, actionDisplayName: string) {
    if (selectedActionKeys.has(actionKey)) return
    const autoRef = getAutoRef(actionKey)
    const asName = actionKey.replace(/\./g, "_")
    onSetContextActions([
      ...contextActions,
      {
        as: asName,
        action: actionKey,
        actionDisplayName,
        ref: autoRef,
        params: autoRef ? undefined : {},
      },
    ])
  }

  function handleRemoveAction(index: number) {
    onSetContextActions(contextActions.filter((_, actionIndex) => actionIndex !== index))
    if (editingIndex === index) setEditingIndex(null)
  }

  function handleUpdateParam(index: number, paramKey: string, paramValue: string) {
    const updated = [...contextActions]
    const action = { ...updated[index] }
    action.params = { ...(action.params ?? {}), [paramKey]: paramValue }
    updated[index] = action
    onSetContextActions(updated)
  }

  // Available template variables for the param input hints.
  const availableVars = useMemo(() => {
    const variables: Array<{ label: string; value: string; source: string }> = []
    // $refs from the trigger.
    for (const refName of Object.keys(refs)) {
      variables.push({ label: refName, value: `$refs.${refName}`, source: "trigger ref" })
    }
    // {{step.field}} from prior context actions — we show the step name.
    for (const contextAction of contextActions) {
      variables.push({ label: contextAction.as, value: `{{${contextAction.as}}}`, source: "context step" })
    }
    return variables
  }, [refs, contextActions])

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Context recipe</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          When {triggerDisplayNames.length === 1 ? (
            <span className="font-medium text-foreground">{triggerDisplayNames[0]}</span>
          ) : (
            <span className="font-medium text-foreground">{triggerDisplayNames.length} events</span>
          )} fire, these read actions run first to gather context for the agent.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {/* Current context actions */}
        {contextActions.length > 0 && (
          <div className="flex flex-col gap-1.5 mb-2">
            {contextActions.map((contextAction, index) => (
              <div key={`${contextAction.action}-${index}`} className="rounded-xl bg-primary/5 border border-primary/20 p-3">
                <div className="flex items-center gap-3">
                  <span className="flex items-center justify-center h-5 w-5 rounded-md bg-primary/10 text-[10px] font-bold text-primary shrink-0">
                    {index + 1}
                  </span>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-foreground truncate">{contextAction.actionDisplayName}</p>
                    <div className="flex items-center gap-2 mt-0.5">
                      <span className="text-[11px] text-muted-foreground font-mono">{contextAction.as}</span>
                      {contextAction.ref && (
                        <span className="text-[10px] bg-green-500/10 text-green-600 px-1.5 py-0.5 rounded-full">
                          auto: ref={contextAction.ref}
                        </span>
                      )}
                      {!contextAction.ref && (
                        <span className="text-[10px] bg-amber-500/10 text-amber-600 px-1.5 py-0.5 rounded-full">
                          {Object.keys(contextAction.params ?? {}).length > 0 ? "custom params" : "needs params"}
                        </span>
                      )}
                    </div>
                  </div>
                  <button
                    type="button"
                    onClick={() => setEditingIndex(editingIndex === index ? null : index)}
                    className="flex items-center justify-center h-6 w-6 rounded-md hover:bg-muted transition-colors shrink-0"
                  >
                    <HugeiconsIcon icon={Edit02Icon} size={12} className="text-muted-foreground" />
                  </button>
                  <button
                    type="button"
                    onClick={() => handleRemoveAction(index)}
                    className="flex items-center justify-center h-6 w-6 rounded-md hover:bg-destructive/10 transition-colors shrink-0"
                  >
                    <HugeiconsIcon icon={Delete02Icon} size={12} className="text-destructive" />
                  </button>
                </div>

                {/* Param editor (expanded) */}
                {editingIndex === index && !contextAction.ref && (
                  <div className="mt-3 pt-3 border-t border-primary/10">
                    <p className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider mb-2">Parameters</p>
                    {Object.entries(contextAction.params ?? {}).map(([paramKey, paramValue]) => (
                      <div key={paramKey} className="flex items-center gap-2 mb-1.5">
                        <span className="text-[11px] font-mono text-muted-foreground w-20 shrink-0 truncate">{paramKey}</span>
                        <Input
                          value={paramValue}
                          onChange={(event) => handleUpdateParam(index, paramKey, event.target.value)}
                          className="h-7 text-xs font-mono"
                          placeholder={`$refs.${paramKey} or {{step.field}}`}
                        />
                      </div>
                    ))}
                    <button
                      type="button"
                      onClick={() => handleUpdateParam(index, "", "")}
                      className="flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground mt-1"
                    >
                      <HugeiconsIcon icon={Add01Icon} size={10} /> Add parameter
                    </button>
                    {availableVars.length > 0 && (
                      <div className="mt-2 pt-2 border-t border-primary/10">
                        <p className="text-[10px] text-muted-foreground mb-1">Available variables:</p>
                        <div className="flex flex-wrap gap-1">
                          {availableVars.map((variable) => (
                            <span key={variable.value} className="text-[10px] font-mono bg-muted px-1.5 py-0.5 rounded text-muted-foreground">
                              {variable.value}
                            </span>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                )}

                {/* Auto-resolved ref info */}
                {editingIndex === index && contextAction.ref && (
                  <div className="mt-3 pt-3 border-t border-primary/10">
                    <p className="text-[11px] text-muted-foreground">
                      Parameters auto-filled from <span className="font-mono">{contextAction.ref}</span> resource ref_bindings. No manual configuration needed.
                    </p>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}

        {/* Add action button / picker */}
        {!showActionPicker ? (
          <button
            type="button"
            onClick={() => { setShowActionPicker(true); setActionSearch("") }}
            className="flex items-center gap-2 w-full rounded-xl border border-dashed border-muted-foreground/20 p-3 text-left transition-colors hover:bg-muted/50 cursor-pointer"
          >
            <HugeiconsIcon icon={Add01Icon} size={14} className="text-muted-foreground" />
            <span className="text-sm text-muted-foreground">Add a context action</span>
          </button>
        ) : (
          <div className="flex flex-col gap-2 rounded-xl border border-border p-3">
            <div className="flex items-center justify-between">
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Read actions</p>
              <button type="button" onClick={() => setShowActionPicker(false)} className="text-xs text-muted-foreground hover:text-foreground transition-colors">
                Done
              </button>
            </div>
            <div className="relative">
              <HugeiconsIcon icon={Search01Icon} size={12} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground" />
              <Input placeholder="Search read actions..." value={actionSearch} onChange={(event) => setActionSearch(event.target.value)} className="pl-8 h-8 text-xs" autoFocus />
            </div>
            <div ref={pickerScrollRef} className="max-h-[240px] overflow-y-auto">
              {isLoading ? (
                <div className="flex flex-col gap-1">
                  {Array.from({ length: 3 }).map((_, index) => <Skeleton key={index} className="h-[48px] w-full rounded-lg" />)}
                </div>
              ) : filteredReadActions.length === 0 ? (
                <p className="text-xs text-muted-foreground py-4 text-center">No read actions found.</p>
              ) : (
                <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
                  {virtualizer.getVirtualItems().map((virtualItem) => {
                    const action = filteredReadActions[virtualItem.index]
                    const isAlreadyAdded = selectedActionKeys.has(action.key!)
                    const autoRef = getAutoRef(action.key!)
                    return (
                      <div key={action.key} style={{ position: "absolute", top: 0, left: 0, width: "100%", transform: `translateY(${virtualItem.start}px)` }}>
                        <button
                          type="button"
                          disabled={isAlreadyAdded}
                          onClick={() => handleAddAction(action.key!, action.display_name ?? action.key!)}
                          className={`flex items-start gap-2 w-full rounded-lg p-2 text-left transition-colors ${isAlreadyAdded ? "opacity-40 cursor-not-allowed" : "hover:bg-muted cursor-pointer"}`}
                        >
                          <div className="flex-1 min-w-0">
                            <div className="flex items-center gap-1.5">
                              <p className="text-xs font-medium text-foreground truncate">{action.display_name}</p>
                              {autoRef && (
                                <span className="text-[9px] bg-green-500/10 text-green-600 px-1 py-0.5 rounded shrink-0">auto</span>
                              )}
                            </div>
                            <p className="text-[11px] text-muted-foreground line-clamp-1">{action.description}</p>
                          </div>
                          {isAlreadyAdded ? (
                            <HugeiconsIcon icon={Tick02Icon} size={12} className="text-primary shrink-0 mt-0.5" />
                          ) : (
                            <HugeiconsIcon icon={Add01Icon} size={12} className="text-muted-foreground shrink-0 mt-0.5" />
                          )}
                        </button>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          </div>
        )}

        {contextActions.length === 0 && !showActionPicker && (
          <div className="flex items-center justify-center py-6">
            <p className="text-sm text-muted-foreground text-center max-w-[260px]">
              Context actions are optional. The agent will still receive the raw webhook payload.
            </p>
          </div>
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onConfirm} className="w-full">
          {contextActions.length > 0
            ? `Confirm with ${contextActions.length} context action${contextActions.length > 1 ? "s" : ""}`
            : "Confirm trigger"}
        </Button>
      </div>
    </>
  )
}
