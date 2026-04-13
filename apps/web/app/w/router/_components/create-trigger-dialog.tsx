"use client"

import { useState, useMemo, useCallback } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Search01Icon,
  Tick02Icon,
  FlashIcon,
} from "@hugeicons/core-free-icons"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Label } from "@/components/ui/label"
import { IntegrationLogo } from "@/components/integration-logo"
import {
  MOCK_CONNECTIONS,
  PROVIDER_TRIGGERS,
  PROVIDER_WEBHOOK_CONFIG,
  type MockTrigger,
  type MockConnection,
} from "./mock-data"
import { apiUrl } from "@/lib/api/client"

type Step = "connection" | "events" | "mode" | "webhook"

interface CreateTriggerDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (trigger: MockTrigger) => void
}

export function CreateTriggerDialog({ open, onOpenChange, onCreated }: CreateTriggerDialogProps) {
  const [step, setStep] = useState<Step>("connection")
  const [selectedConnection, setSelectedConnection] = useState<MockConnection | null>(null)
  const [selectedKeys, setSelectedKeys] = useState<string[]>([])
  const [routingMode, setRoutingMode] = useState<"rule" | "triage">("rule")
  const [enrichCrossRefs, setEnrichCrossRefs] = useState(false)
  const [searchConnection, setSearchConnection] = useState("")
  const [searchTrigger, setSearchTrigger] = useState("")

  function reset() {
    setStep("connection")
    setSelectedConnection(null)
    setSelectedKeys([])
    setRoutingMode("rule")
    setEnrichCrossRefs(false)
    setSearchConnection("")
    setSearchTrigger("")
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) reset()
    onOpenChange(nextOpen)
  }

  function pickConnection(connection: MockConnection) {
    setSelectedConnection(connection)
    setStep("events")
  }

  function toggleTriggerKey(key: string) {
    setSelectedKeys((previous) =>
      previous.includes(key)
        ? previous.filter((existingKey) => existingKey !== key)
        : [...previous, key]
    )
  }

  const requiresWebhookConfig = selectedConnection
    ? PROVIDER_WEBHOOK_CONFIG[selectedConnection.provider]?.webhookUrlRequired === true
    : false

  const createTrigger = useCallback(() => {
    if (!selectedConnection) return
    const trigger: MockTrigger = {
      id: `trigger-${Date.now()}`,
      connectionId: selectedConnection.id,
      provider: selectedConnection.provider,
      connectionName: selectedConnection.displayName,
      triggerKeys: selectedKeys,
      routingMode,
      enabled: true,
      enrichCrossReferences: enrichCrossRefs,
      rules: [],
    }
    onCreated(trigger)
  }, [selectedConnection, selectedKeys, routingMode, enrichCrossRefs, onCreated])

  function handleCreate() {
    if (requiresWebhookConfig) {
      createTrigger()
      setStep("webhook")
    } else {
      createTrigger()
      handleOpenChange(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md h-[680px] overflow-hidden flex flex-col" showCloseButton={step === "connection" || step === "webhook"}>
        {step === "connection" && (
          <ConnectionStep
            search={searchConnection}
            onSearchChange={setSearchConnection}
            onPick={pickConnection}
          />
        )}
        {step === "events" && selectedConnection && (
          <EventsStep
            connection={selectedConnection}
            search={searchTrigger}
            onSearchChange={setSearchTrigger}
            selectedKeys={selectedKeys}
            onToggle={toggleTriggerKey}
            onBack={() => setStep("connection")}
            onContinue={() => setStep("mode")}
          />
        )}
        {step === "mode" && (
          <ModeStep
            routingMode={routingMode}
            onModeChange={setRoutingMode}
            enrichCrossRefs={enrichCrossRefs}
            onEnrichChange={setEnrichCrossRefs}
            onBack={() => setStep("events")}
            onCreate={handleCreate}
          />
        )}
        {step === "webhook" && selectedConnection && (
          <WebhookStep
            connection={selectedConnection}
            onDone={() => handleOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

// --- Step 1: Connection ---

interface ConnectionStepProps {
  search: string
  onSearchChange: (value: string) => void
  onPick: (connection: MockConnection) => void
}

function ConnectionStep({ search, onSearchChange, onPick }: ConnectionStepProps) {
  const filtered = useMemo(() => {
    if (!search.trim()) return MOCK_CONNECTIONS
    const query = search.toLowerCase()
    return MOCK_CONNECTIONS.filter(
      (connection) =>
        connection.displayName.toLowerCase().includes(query) ||
        connection.provider.toLowerCase().includes(query)
    )
  }, [search])

  return (
    <>
      <DialogHeader>
        <DialogTitle>Choose a connection</DialogTitle>
        <DialogDescription>Which integration should trigger the router?</DialogDescription>
      </DialogHeader>

      <div className="relative mt-2">
        <HugeiconsIcon icon={Search01Icon} size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
        <Input placeholder="Search connections..." value={search} onChange={(event) => onSearchChange(event.target.value)} className="pl-9 h-9" />
      </div>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {filtered.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-12">No connections found.</p>
        ) : (
          filtered.map((connection) => (
            <button
              key={connection.id}
              type="button"
              onClick={() => onPick(connection)}
              className="group flex items-center gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer border border-transparent"
            >
              <IntegrationLogo provider={connection.provider} size={32} />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-semibold text-foreground">{connection.displayName}</p>
                <p className="text-[13px] text-muted-foreground mt-0.5">{connection.provider}</p>
              </div>
              <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0" />
            </button>
          ))
        )}
      </div>
    </>
  )
}

// --- Step 2: Events ---

interface EventsStepProps {
  connection: MockConnection
  search: string
  onSearchChange: (value: string) => void
  selectedKeys: string[]
  onToggle: (key: string) => void
  onBack: () => void
  onContinue: () => void
}

function EventsStep({ connection, search, onSearchChange, selectedKeys, onToggle, onBack, onContinue }: EventsStepProps) {
  const allTriggers = PROVIDER_TRIGGERS[connection.provider] ?? []
  const selectedSet = new Set(selectedKeys)

  const filtered = useMemo(() => {
    if (!search.trim()) return allTriggers
    const query = search.toLowerCase()
    return allTriggers.filter(
      (trigger) =>
        trigger.displayName.toLowerCase().includes(query) ||
        trigger.key.toLowerCase().includes(query) ||
        trigger.description.toLowerCase().includes(query)
    )
  }, [allTriggers, search])

  const grouped = useMemo(() => {
    const groups: Record<string, typeof filtered> = {}
    for (const trigger of filtered) {
      const resourceType = trigger.resourceType
      if (!groups[resourceType]) groups[resourceType] = []
      groups[resourceType].push(trigger)
    }
    return groups
  }, [filtered])

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <div className="flex items-center gap-2.5">
            <IntegrationLogo provider={connection.provider} size={20} />
            <DialogTitle>Pick events</DialogTitle>
          </div>
        </div>
        <DialogDescription className="mt-2">Choose which webhook events this trigger listens for.</DialogDescription>
      </DialogHeader>

      <div className="relative mt-2">
        <HugeiconsIcon icon={Search01Icon} size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
        <Input placeholder="Search events..." value={search} onChange={(event) => onSearchChange(event.target.value)} className="pl-9 h-9" />
      </div>

      <div className="flex flex-col gap-1 mt-4 flex-1 overflow-y-auto">
        {Object.entries(grouped).map(([resourceType, triggers]) => (
          <div key={resourceType} className="mb-3">
            <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground px-1 mb-1.5">{resourceType}</p>
            <div className="flex flex-col gap-1">
              {triggers.map((trigger) => {
                const isSelected = selectedSet.has(trigger.key)
                return (
                  <button
                    key={trigger.key}
                    type="button"
                    onClick={() => onToggle(trigger.key)}
                    className={`flex items-start gap-3 w-full rounded-xl p-3 text-left transition-colors cursor-pointer ${
                      isSelected ? "bg-primary/5 border border-primary/20" : "bg-muted/50 hover:bg-muted border border-transparent"
                    }`}
                  >
                    <div className="flex items-center justify-center h-6 w-6 rounded-md bg-amber-500/10 shrink-0 mt-0.5">
                      <HugeiconsIcon icon={FlashIcon} size={12} className="text-amber-500" />
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-foreground">{trigger.displayName}</p>
                      <p className="text-[12px] text-muted-foreground mt-0.5 line-clamp-1">{trigger.description}</p>
                    </div>
                    {isSelected && <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0 mt-0.5" />}
                  </button>
                )
              })}
            </div>
          </div>
        ))}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onContinue} disabled={selectedKeys.length === 0} className="w-full">
          {selectedKeys.length > 0
            ? `Continue with ${selectedKeys.length} event${selectedKeys.length > 1 ? "s" : ""}`
            : "Select at least one event"}
        </Button>
      </div>
    </>
  )
}

// --- Step 3: Mode ---

interface ModeStepProps {
  routingMode: "rule" | "triage"
  onModeChange: (mode: "rule" | "triage") => void
  enrichCrossRefs: boolean
  onEnrichChange: (value: boolean) => void
  onBack: () => void
  onCreate: () => void
}

function ModeStep({ routingMode, onModeChange, enrichCrossRefs, onEnrichChange, onBack, onCreate }: ModeStepProps) {
  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Routing mode</DialogTitle>
        </div>
        <DialogDescription className="mt-2">How should the router decide which agent handles each event?</DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3 mt-4 flex-1">
        <button
          type="button"
          onClick={() => onModeChange("rule")}
          className={`flex flex-col gap-2 w-full rounded-xl p-4 text-left transition-colors cursor-pointer ${
            routingMode === "rule" ? "bg-primary/5 border border-primary/20" : "bg-muted/50 hover:bg-muted border border-transparent"
          }`}
        >
          <div className="flex items-center justify-between">
            <p className="text-sm font-semibold text-foreground">Rule-based</p>
            {routingMode === "rule" && <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary" />}
          </div>
          <p className="text-[13px] text-muted-foreground leading-relaxed">
            You define conditions that match payload fields to agents. Deterministic, fast, no LLM cost.
          </p>
          <p className="text-[12px] text-muted-foreground/70 mt-1">
            Best for structured events where you know exactly which agent should handle what.
          </p>
        </button>

        <button
          type="button"
          onClick={() => onModeChange("triage")}
          className={`flex flex-col gap-2 w-full rounded-xl p-4 text-left transition-colors cursor-pointer ${
            routingMode === "triage" ? "bg-primary/5 border border-primary/20" : "bg-muted/50 hover:bg-muted border border-transparent"
          }`}
        >
          <div className="flex items-center justify-between">
            <p className="text-sm font-semibold text-foreground">LLM triage</p>
            {routingMode === "triage" && <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary" />}
          </div>
          <p className="text-[13px] text-muted-foreground leading-relaxed">
            An LLM reads the event and decides which agent(s) to route to. Can also plan enrichment fetches from other connections.
          </p>
          <p className="text-[12px] text-muted-foreground/70 mt-1">
            Best for unstructured input like Slack messages where intent varies.
          </p>
        </button>

        <div className="flex items-center justify-between rounded-xl bg-muted/50 px-4 py-3 mt-2">
          <div className="flex-1 min-w-0">
            <Label className="text-sm font-medium">Enrich with cross-references</Label>
            <p className="text-[12px] text-muted-foreground mt-0.5">Let the LLM fetch context from other connections before routing</p>
          </div>
          <Switch checked={enrichCrossRefs} onCheckedChange={onEnrichChange} />
        </div>
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onCreate} className="w-full">Create trigger</Button>
      </div>
    </>
  )
}

// --- Step 4: Webhook Configuration (only for providers requiring manual setup) ---

interface WebhookStepProps {
  connection: MockConnection
  onDone: () => void
}

function WebhookStep({ connection, onDone }: WebhookStepProps) {
  const [copied, setCopied] = useState(false)
  const webhookConfig = PROVIDER_WEBHOOK_CONFIG[connection.provider]
  const webhookUrl = apiUrl(`/incoming/webhooks/${connection.provider}/${connection.id}`)

  function handleCopy() {
    navigator.clipboard.writeText(webhookUrl)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  // Render markdown-like configuration notes as simple formatted text.
  const formattedNotes = webhookConfig?.configurationNotes
    .split("\n")
    .map((line, index) => {
      // Bold: **text**
      const formatted = line.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
      if (line.trim() === "") return <br key={index} />
      return <p key={index} className="text-[13px] text-muted-foreground leading-relaxed" dangerouslySetInnerHTML={{ __html: formatted }} />
    })

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2.5">
          <IntegrationLogo provider={connection.provider} size={20} />
          <DialogTitle>Configure webhook</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Trigger created. One last step &mdash; configure the webhook URL in {connection.displayName}.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-4 mt-4 flex-1">
        <div className="rounded-xl bg-muted/50 border border-border p-4">
          <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground mb-2">Webhook URL</p>
          <div className="flex items-center gap-2">
            <code className="flex-1 text-[12px] font-mono bg-background rounded-lg px-3 py-2 border border-border break-all select-all">
              {webhookUrl}
            </code>
            <Button variant="outline" size="sm" onClick={handleCopy} className="shrink-0 h-8">
              {copied ? (
                <span className="flex items-center gap-1.5">
                  <HugeiconsIcon icon={Tick02Icon} size={14} className="text-green-500" />
                  Copied
                </span>
              ) : (
                "Copy"
              )}
            </Button>
          </div>
        </div>

        <div className="rounded-xl bg-muted/50 border border-border p-4">
          <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground mb-2">Setup instructions</p>
          <div className="flex flex-col gap-1">
            {formattedNotes}
          </div>
        </div>
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onDone} className="w-full">
          Done
        </Button>
      </div>
    </>
  )
}
