"use client"

import { useState, useMemo } from "react"
import { AnimatePresence, motion } from "motion/react"
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
import { Switch } from "@/components/ui/switch"
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

export function ConditionBuilderView({ triggerDisplayNames, refs, initialConditions, onConfirm, onBack }: ConditionBuilderViewProps) {
  const [matchAll, setMatchAll] = useState(initialConditions?.mode !== "any")
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
      onConfirm({ mode: matchAll ? "all" : "any", conditions: validConditions })
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
          Optionally add conditions to filter when this trigger fires. No filters means every matching event triggers it.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3 mt-4 flex-1 overflow-y-auto">
        {/* Event context */}
        <div className="rounded-xl bg-muted/50 p-3">
          <p className="text-xs text-muted-foreground">
            {triggerDisplayNames.join(", ")}
          </p>
        </div>

        {/* Match mode toggle */}
        {conditions.length > 1 && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.15 }}
            className="flex items-center justify-between rounded-xl bg-muted/50 p-3"
          >
            <div>
              <Label className="text-sm">Match all conditions</Label>
              <p className="text-mini text-muted-foreground mt-0.5">
                {matchAll ? "All conditions must pass (AND)" : "Any condition can pass (OR)"}
              </p>
            </div>
            <Switch checked={matchAll} onCheckedChange={setMatchAll} size="sm" />
          </motion.div>
        )}

        {/* Condition cards */}
        <AnimatePresence initial={false}>
          {conditions.map((condition, index) => (
            <motion.div
              key={index}
              initial={{ opacity: 0, y: -8, height: 0 }}
              animate={{ opacity: 1, y: 0, height: "auto" }}
              exit={{ opacity: 0, y: -8, height: 0 }}
              transition={{ duration: 0.15 }}
            >
              <div className="rounded-xl bg-muted/50 border border-transparent p-4">
                <div className="flex items-start justify-between mb-3">
                  <span className="text-mini font-medium uppercase tracking-wider text-muted-foreground">
                    Condition {index + 1}
                  </span>
                  <button
                    type="button"
                    onClick={() => removeCondition(index)}
                    className="flex items-center justify-center h-6 w-6 rounded-lg hover:bg-destructive/10 transition-colors"
                  >
                    <HugeiconsIcon icon={Cancel01Icon} size={12} className="text-destructive" />
                  </button>
                </div>

                <div className="flex flex-col gap-2.5">
                  {/* Field */}
                  <div className="flex flex-col gap-1.5">
                    <Label className="text-mini text-muted-foreground">Field</Label>
                    {customPathIndex === index ? (
                      <Input
                        placeholder="payload.path"
                        value={condition.path}
                        onChange={(event) => updateCondition(index, "path", event.target.value)}
                        className="font-mono text-xs"
                        autoFocus
                      />
                    ) : (
                      <Select value={condition.path || undefined} onValueChange={(value) => handlePathSelect(index, value)}>
                        <SelectTrigger className="w-full">
                          <SelectValue placeholder="Select field..." />
                        </SelectTrigger>
                        <SelectContent>
                          {pathOptions.map((option) => (
                            <SelectItem key={option.path} value={option.path}>
                              <span className="font-mono text-mini">{option.label}</span>
                            </SelectItem>
                          ))}
                          <SelectItem value="__custom__">
                            <span className="text-muted-foreground">Custom path...</span>
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    )}
                  </div>

                  {/* Operator */}
                  <div className="flex flex-col gap-1.5">
                    <Label className="text-mini text-muted-foreground">Operator</Label>
                    <Select value={condition.operator} onValueChange={(value) => updateCondition(index, "operator", value ?? "equals")}>
                      <SelectTrigger className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {OPERATORS.map((operator) => (
                          <SelectItem key={operator.value} value={operator.value}>{operator.label}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>

                  {/* Value */}
                  {!OPERATORS_WITHOUT_VALUE.has(condition.operator) && (
                    <motion.div
                      initial={{ opacity: 0, height: 0 }}
                      animate={{ opacity: 1, height: "auto" }}
                      exit={{ opacity: 0, height: 0 }}
                      transition={{ duration: 0.12 }}
                      className="flex flex-col gap-1.5"
                    >
                      <Label className="text-mini text-muted-foreground">Value</Label>
                      <Input
                        placeholder="value"
                        value={typeof condition.value === "string" ? condition.value : String(condition.value ?? "")}
                        onChange={(event) => updateCondition(index, "value", event.target.value)}
                        className="font-mono text-xs"
                      />
                    </motion.div>
                  )}
                </div>
              </div>
            </motion.div>
          ))}
        </AnimatePresence>

        {/* Add filter button */}
        <Button variant="outline" size="sm" onClick={addCondition} className="w-fit">
          <HugeiconsIcon icon={Add01Icon} size={12} data-icon="inline-start" />
          Add filter
        </Button>
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={handleConfirm} className="w-full">
          {conditions.length > 0 ? `Add trigger with ${conditions.length} filter${conditions.length > 1 ? "s" : ""}` : "Add trigger (no filters)"}
        </Button>
      </div>
    </>
  )
}
