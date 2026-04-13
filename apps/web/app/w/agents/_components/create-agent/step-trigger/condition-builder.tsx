"use client"

import { useState, useMemo } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  Cancel01Icon,
  Add01Icon,
} from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select"
import type { TriggerConditionsConfig, TriggerConditionConfig } from "../types"

interface ConditionBuilderViewProps {
  provider: string
  triggerDisplayNames: string[]
  refs: Record<string, string>
  initialConditions?: TriggerConditionsConfig | null
  onConfirm: (conditions: TriggerConditionsConfig | null) => void
  onBack: () => void
}

const OPERATORS = [
  { value: "equals", label: "equals" },
  { value: "not_equals", label: "not equals" },
  { value: "contains", label: "contains" },
  { value: "not_contains", label: "not contains" },
  { value: "one_of", label: "one of" },
  { value: "not_one_of", label: "not one of" },
  { value: "matches", label: "matches (regex)" },
  { value: "exists", label: "exists" },
  { value: "not_exists", label: "not exists" },
]

const OPERATORS_WITHOUT_VALUE = new Set(["exists", "not_exists"])

export function ConditionBuilderView({ provider, triggerDisplayNames, refs, initialConditions, onConfirm, onBack }: ConditionBuilderViewProps) {
  const [matchMode, setMatchMode] = useState<"all" | "any">(initialConditions?.mode ?? "all")
  const [conditions, setConditions] = useState<TriggerConditionConfig[]>(initialConditions?.conditions ?? [])
  const [customPathIndex, setCustomPathIndex] = useState<number | null>(null)

  const pathOptions = useMemo(() => {
    const options: { label: string; path: string }[] = []
    for (const [refName, dotPath] of Object.entries(refs)) {
      const label = refName.replace(/_/g, " ")
      options.push({ label, path: dotPath })
    }
    return options
  }, [refs])

  function addCondition() {
    setConditions((previous) => [...previous, { path: "", operator: "equals", value: "" }])
  }

  function removeCondition(index: number) {
    setConditions((previous) => previous.filter((_, conditionIndex) => conditionIndex !== index))
    if (customPathIndex === index) setCustomPathIndex(null)
  }

  function updateCondition(index: number, field: keyof TriggerConditionConfig, fieldValue: unknown) {
    setConditions((previous) =>
      previous.map((condition, conditionIndex) =>
        conditionIndex === index ? { ...condition, [field]: fieldValue } : condition
      )
    )
  }

  function handlePathSelect(index: number, selectedValue: string | null) {
    if (!selectedValue) return
    if (selectedValue === "__custom__") {
      setCustomPathIndex(index)
      updateCondition(index, "path", "")
    } else {
      setCustomPathIndex(null)
      updateCondition(index, "path", selectedValue)
    }
  }

  function handleConfirm() {
    const validConditions = conditions.filter((condition) => condition.path.trim() !== "")
    if (validConditions.length === 0) {
      onConfirm(null)
    } else {
      onConfirm({ mode: matchMode, conditions: validConditions })
    }
  }

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Filters</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Optionally add conditions to filter which events trigger this agent. No filters means every matching event triggers it.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-4 mt-4 flex-1 overflow-y-auto">
        <div className="rounded-xl bg-muted/50 p-3">
          <p className="text-[12px] text-muted-foreground">
            Triggering on: {triggerDisplayNames.join(", ")}
          </p>
        </div>

        {conditions.length > 0 && (
          <div className="flex items-center gap-2">
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
        )}

        <div className="flex flex-col gap-2">
          {conditions.map((condition, index) => (
            <div key={index} className="flex items-start gap-2">
              {customPathIndex === index ? (
                <Input
                  placeholder="payload.path"
                  value={condition.path}
                  onChange={(event) => updateCondition(index, "path", event.target.value)}
                  className="flex-1 font-mono text-[12px] h-8"
                  autoFocus
                />
              ) : (
                <Select value={condition.path || undefined} onValueChange={(value) => handlePathSelect(index, value)}>
                  <SelectTrigger className="flex-1" size="sm">
                    <SelectValue placeholder="Select field..." />
                  </SelectTrigger>
                  <SelectContent>
                    {pathOptions.map((option) => (
                      <SelectItem key={option.path} value={option.path}>
                        <span className="font-mono text-[11px]">{option.label}</span>
                      </SelectItem>
                    ))}
                    <SelectItem value="__custom__">
                      <span className="text-muted-foreground">Custom path...</span>
                    </SelectItem>
                  </SelectContent>
                </Select>
              )}

              <Select value={condition.operator} onValueChange={(value) => updateCondition(index, "operator", value ?? "equals")}>
                <SelectTrigger className="w-[120px] shrink-0" size="sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {OPERATORS.map((operator) => (
                    <SelectItem key={operator.value} value={operator.value}>{operator.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>

              {!OPERATORS_WITHOUT_VALUE.has(condition.operator) && (
                <Input
                  placeholder="value"
                  value={typeof condition.value === "string" ? condition.value : String(condition.value ?? "")}
                  onChange={(event) => updateCondition(index, "value", event.target.value)}
                  className="flex-1 font-mono text-[12px] h-8"
                />
              )}

              <button
                type="button"
                onClick={() => removeCondition(index)}
                className="shrink-0 h-8 w-8 flex items-center justify-center text-muted-foreground hover:text-destructive transition-colors"
              >
                <HugeiconsIcon icon={Cancel01Icon} size={12} />
              </button>
            </div>
          ))}
        </div>

        <Button variant="ghost" size="xs" onClick={addCondition} className="w-fit">
          <HugeiconsIcon icon={Add01Icon} size={12} data-icon="inline-start" />
          Add filter
        </Button>

        {pathOptions.length > 0 && conditions.length > 0 && (
          <div className="rounded-lg bg-muted/50 border border-border p-3">
            <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground mb-2">
              Available fields for {provider}
            </p>
            <div className="flex flex-col gap-0.5">
              {pathOptions.map((option) => (
                <div key={option.path} className="flex items-center justify-between text-[11px]">
                  <span className="text-foreground">{option.label}</span>
                  <span className="text-muted-foreground font-mono">{option.path}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={handleConfirm} className="w-full">
          {conditions.length > 0 ? `Add trigger with ${conditions.length} filter${conditions.length > 1 ? "s" : ""}` : "Add trigger (no filters)"}
        </Button>
      </div>
    </>
  )
}
