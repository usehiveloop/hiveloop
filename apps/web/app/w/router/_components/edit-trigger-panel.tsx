"use client"

import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Cancel01Icon,
  Add01Icon,
  Delete02Icon,
  FlashIcon,
} from "@hugeicons/core-free-icons"
import {
  FloatingPanel,
  FloatingPanelContent,
  FloatingPanelHeader,
  FloatingPanelBody,
  FloatingPanelFooter,
  FloatingPanelTitle,
  FloatingPanelDescription,
} from "@/components/ui/floating-panel"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Switch } from "@/components/ui/switch"
import { Label } from "@/components/ui/label"
import { Input } from "@/components/ui/input"
import { IntegrationLogo } from "@/components/integration-logo"
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select"
import {
  MOCK_AGENTS,
  CONDITION_OPERATORS,
  PAYLOAD_PATHS,
  type MockTrigger,
  type MockRule,
  type MockCondition,
} from "./mock-data"

interface EditTriggerPanelProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  trigger: MockTrigger | null
  onUpdate: (trigger: MockTrigger) => void
  onDelete: (triggerId: string) => void
}

export function EditTriggerPanel({ open, onOpenChange, trigger, onUpdate, onDelete }: EditTriggerPanelProps) {
  const [addingRule, setAddingRule] = useState(false)

  if (!trigger) return null

  function handleRemoveEvent(key: string) {
    if (!trigger) return
    const updated = { ...trigger, triggerKeys: trigger.triggerKeys.filter((existingKey) => existingKey !== key) }
    onUpdate(updated)
  }

  function handleToggleEnabled() {
    if (!trigger) return
    onUpdate({ ...trigger, enabled: !trigger.enabled })
  }

  function handleModeChange(mode: string | null) {
    if (!trigger || !mode) return
    onUpdate({ ...trigger, routingMode: mode as "rule" | "triage" })
  }

  function handleDeleteRule(ruleId: string) {
    if (!trigger) return
    onUpdate({ ...trigger, rules: trigger.rules.filter((rule) => rule.id !== ruleId) })
  }

  function handleAddRule(rule: MockRule) {
    if (!trigger) return
    onUpdate({ ...trigger, rules: [...trigger.rules, rule] })
    setAddingRule(false)
  }

  return (
    <FloatingPanel open={open} onOpenChange={onOpenChange}>
      <FloatingPanelContent width="lg:w-[540px]">
        <FloatingPanelHeader>
          <div className="flex items-center gap-3">
            <IntegrationLogo provider={trigger.provider} size={28} />
            <div>
              <FloatingPanelTitle>{trigger.connectionName} trigger</FloatingPanelTitle>
              <FloatingPanelDescription>{trigger.triggerKeys.length} events &middot; {trigger.routingMode === "triage" ? "LLM triage" : "rule-based"}</FloatingPanelDescription>
            </div>
          </div>
        </FloatingPanelHeader>

        <FloatingPanelBody>
          {/* Events */}
          <section className="mb-6">
            <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground mb-2">Events</p>
            <div className="flex flex-wrap gap-1.5">
              {trigger.triggerKeys.map((key) => (
                <Badge key={key} variant="secondary" className="text-[11px] font-mono gap-1 pr-1">
                  {key}
                  <button
                    type="button"
                    onClick={() => handleRemoveEvent(key)}
                    className="hover:text-destructive transition-colors ml-0.5"
                  >
                    <HugeiconsIcon icon={Cancel01Icon} size={10} />
                  </button>
                </Badge>
              ))}
            </div>
          </section>

          {/* Routing Rules (rule mode only) */}
          {trigger.routingMode === "rule" && (
            <section className="mb-6">
              <div className="flex items-center justify-between mb-2">
                <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">Routing Rules</p>
                <Button variant="ghost" size="xs" onClick={() => setAddingRule(true)}>
                  <HugeiconsIcon icon={Add01Icon} size={12} data-icon="inline-start" />
                  Add rule
                </Button>
              </div>
              <p className="text-[12px] text-muted-foreground mb-3">
                Rules are evaluated top-to-bottom. Multiple rules can match &mdash; all matching agents are dispatched.
              </p>

              <div className="flex flex-col gap-2">
                {trigger.rules.map((rule) => (
                  <RuleRow key={rule.id} rule={rule} provider={trigger.provider} onDelete={() => handleDeleteRule(rule.id)} />
                ))}
              </div>

              {addingRule && (
                <RuleBuilder
                  provider={trigger.provider}
                  onSave={handleAddRule}
                  onCancel={() => setAddingRule(false)}
                />
              )}
            </section>
          )}

          {/* Triage mode info */}
          {trigger.routingMode === "triage" && (
            <section className="mb-6">
              <div className="rounded-xl bg-muted/50 p-4">
                <p className="text-sm font-medium text-foreground mb-1">LLM triage mode</p>
                <p className="text-[13px] text-muted-foreground leading-relaxed">
                  When an event arrives, the routing LLM reads the payload, selects the best agent(s), and optionally plans enrichment fetches from other connections. No rules needed.
                </p>
              </div>
            </section>
          )}

          {/* Settings */}
          <section>
            <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground mb-3">Settings</p>

            <div className="flex flex-col gap-4">
              <div className="flex items-center justify-between">
                <div>
                  <Label className="text-sm">Enabled</Label>
                  <p className="text-[12px] text-muted-foreground mt-0.5">Disabled triggers are ignored by the dispatcher</p>
                </div>
                <Switch checked={trigger.enabled} onCheckedChange={handleToggleEnabled} />
              </div>

              <div className="flex flex-col gap-2">
                <Label className="text-sm">Routing mode</Label>
                <Select value={trigger.routingMode} onValueChange={handleModeChange}>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="rule">Rule-based</SelectItem>
                    <SelectItem value="triage">LLM triage</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="flex items-center justify-between">
                <div>
                  <Label className="text-sm">Cross-reference enrichment</Label>
                  <p className="text-[12px] text-muted-foreground mt-0.5">Let the LLM fetch context from other connections</p>
                </div>
                <Switch
                  checked={trigger.enrichCrossReferences}
                  onCheckedChange={() => onUpdate({ ...trigger, enrichCrossReferences: !trigger.enrichCrossReferences })}
                />
              </div>
            </div>
          </section>
        </FloatingPanelBody>

        <FloatingPanelFooter className="justify-between">
          <Button variant="destructive" size="sm" onClick={() => { onDelete(trigger.id); onOpenChange(false) }}>
            Delete trigger
          </Button>
          <Button size="sm" onClick={() => onOpenChange(false)}>
            Done
          </Button>
        </FloatingPanelFooter>
      </FloatingPanelContent>
    </FloatingPanel>
  )
}

// --- Rule Row ---

interface RuleRowProps {
  rule: MockRule
  provider: string
  onDelete: () => void
}

function RuleRow({ rule, provider, onDelete }: RuleRowProps) {
  const conditionSummary = rule.conditions === null || rule.conditions.conditions.length === 0
    ? "catch-all (always matches)"
    : rule.conditions.conditions.map((condition) => {
        if (condition.operator === "exists" || condition.operator === "not_exists") {
          return `${condition.path} ${condition.operator}`
        }
        const valueStr = Array.isArray(condition.value) ? condition.value.join(", ") : (condition.value ?? "")
        return `${condition.path} ${condition.operator} "${valueStr}"`
      }).join(rule.conditions.mode === "all" ? " AND " : " OR ")

  return (
    <div className="flex items-start gap-3 rounded-xl border border-border p-3 group">
      <div className="flex items-center justify-center h-6 w-6 rounded-md bg-amber-500/10 shrink-0 mt-0.5">
        <HugeiconsIcon icon={FlashIcon} size={12} className="text-amber-500" />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="inline-flex items-center justify-center h-4 min-w-4 px-1 rounded bg-muted text-[10px] font-mono text-muted-foreground">
            P{rule.priority}
          </span>
          <span className="text-sm font-medium text-foreground">{rule.agentName}</span>
        </div>
        <p className="text-[12px] text-muted-foreground mt-1 font-mono">{conditionSummary}</p>
      </div>
      <button
        type="button"
        onClick={onDelete}
        className="opacity-0 group-hover:opacity-100 transition-opacity shrink-0 mt-0.5 hover:text-destructive text-muted-foreground"
      >
        <HugeiconsIcon icon={Delete02Icon} size={14} />
      </button>
    </div>
  )
}

// --- Rule Builder ---

interface RuleBuilderProps {
  provider: string
  onSave: (rule: MockRule) => void
  onCancel: () => void
}

function RuleBuilder({ provider, onSave, onCancel }: RuleBuilderProps) {
  const [agentId, setAgentId] = useState("")
  const [priority, setPriority] = useState("1")
  const [isCatchAll, setIsCatchAll] = useState(false)
  const [matchMode, setMatchMode] = useState<"all" | "any">("all")
  const [conditions, setConditions] = useState<MockCondition[]>([{ path: "", operator: "equals", value: "" }])

  const payloadPaths = PAYLOAD_PATHS[provider] ?? []

  function addCondition() {
    setConditions((previous) => [...previous, { path: "", operator: "equals", value: "" }])
  }

  function removeCondition(index: number) {
    setConditions((previous) => previous.filter((_, conditionIndex) => conditionIndex !== index))
  }

  function updateCondition(index: number, field: keyof MockCondition, value: string) {
    setConditions((previous) =>
      previous.map((condition, conditionIndex) =>
        conditionIndex === index ? { ...condition, [field]: value } : condition
      )
    )
  }

  function handleSave() {
    const selectedAgent = MOCK_AGENTS.find((agent) => agent.id === agentId)
    if (!selectedAgent) return

    const rule: MockRule = {
      id: `rule-${Date.now()}`,
      agentId,
      agentName: selectedAgent.name,
      priority: parseInt(priority, 10) || 1,
      conditions: isCatchAll ? null : {
        mode: matchMode,
        conditions: conditions.filter((condition) => condition.path.trim() !== ""),
      },
    }
    onSave(rule)
  }

  const isOperatorWithoutValue = (operator: string) => operator === "exists" || operator === "not_exists"

  return (
    <div className="mt-3 rounded-xl border border-primary/20 bg-primary/5 p-4">
      <p className="text-sm font-semibold text-foreground mb-4">Add routing rule</p>

      {/* Agent */}
      <div className="flex flex-col gap-2 mb-4">
        <Label className="text-sm">Agent</Label>
        <Select value={agentId} onValueChange={(value) => setAgentId(value ?? "")}>
          <SelectTrigger className="w-full">
            <SelectValue placeholder="Select an agent..." />
          </SelectTrigger>
          <SelectContent>
            {MOCK_AGENTS.map((agent) => (
              <SelectItem key={agent.id} value={agent.id}>
                <div className="flex flex-col">
                  <span>{agent.name}</span>
                </div>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Priority */}
      <div className="flex flex-col gap-2 mb-4">
        <Label className="text-sm">Priority</Label>
        <div className="flex items-center gap-2">
          <Input
            type="number"
            min={1}
            max={99}
            value={priority}
            onChange={(event) => setPriority(event.target.value)}
            className="w-20"
          />
          <span className="text-[12px] text-muted-foreground">1 = runs first, 99 = fallback</span>
        </div>
      </div>

      {/* Catch-all toggle */}
      <div className="flex items-center justify-between rounded-xl bg-background px-3 py-2.5 mb-4">
        <div>
          <Label className="text-sm">Catch-all (always matches)</Label>
          <p className="text-[11px] text-muted-foreground mt-0.5">No conditions &mdash; this rule matches every event</p>
        </div>
        <Switch checked={isCatchAll} onCheckedChange={setIsCatchAll} />
      </div>

      {/* Conditions */}
      {!isCatchAll && (
        <div className="mb-4">
          <div className="flex items-center gap-2 mb-3">
            <Label className="text-sm">Match</Label>
            <Select value={matchMode} onValueChange={(value) => setMatchMode((value ?? "all") as "all" | "any")}>
              <SelectTrigger className="w-fit" size="sm">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">all (AND)</SelectItem>
                <SelectItem value="any">any (OR)</SelectItem>
              </SelectContent>
            </Select>
            <span className="text-[12px] text-muted-foreground">of the following:</span>
          </div>

          <div className="flex flex-col gap-2">
            {conditions.map((condition, index) => (
              <div key={index} className="flex items-start gap-2">
                <Input
                  placeholder="path"
                  value={condition.path}
                  onChange={(event) => updateCondition(index, "path", event.target.value)}
                  className="flex-1 font-mono text-[12px] h-8"
                />
                <Select value={condition.operator} onValueChange={(value) => updateCondition(index, "operator", value ?? "equals")}>
                  <SelectTrigger className="w-[120px] shrink-0" size="sm">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {CONDITION_OPERATORS.map((operator) => (
                      <SelectItem key={operator.value} value={operator.value}>{operator.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {!isOperatorWithoutValue(condition.operator) && (
                  <Input
                    placeholder="value"
                    value={(condition.value as string) ?? ""}
                    onChange={(event) => updateCondition(index, "value", event.target.value)}
                    className="flex-1 font-mono text-[12px] h-8"
                  />
                )}
                {conditions.length > 1 && (
                  <button
                    type="button"
                    onClick={() => removeCondition(index)}
                    className="shrink-0 h-8 w-8 flex items-center justify-center text-muted-foreground hover:text-destructive transition-colors"
                  >
                    <HugeiconsIcon icon={Cancel01Icon} size={12} />
                  </button>
                )}
              </div>
            ))}
          </div>

          <Button variant="ghost" size="xs" onClick={addCondition} className="mt-2">
            <HugeiconsIcon icon={Add01Icon} size={12} data-icon="inline-start" />
            Add condition
          </Button>
        </div>
      )}

      {/* Available paths */}
      {!isCatchAll && payloadPaths.length > 0 && (
        <div className="rounded-lg bg-background border border-border p-3 mb-4">
          <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground mb-2">
            Available payload paths for {provider}
          </p>
          <div className="flex flex-col gap-0.5">
            {payloadPaths.map((item) => (
              <div key={item.path} className="flex items-center justify-between text-[11px]">
                <span className="font-mono text-foreground">{item.path}</span>
                <span className="text-muted-foreground font-mono">{item.example}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Actions */}
      <div className="flex items-center gap-2 justify-end">
        <Button variant="ghost" size="sm" onClick={onCancel}>Cancel</Button>
        <Button size="sm" disabled={!agentId} onClick={handleSave}>Save rule</Button>
      </div>
    </div>
  )
}
