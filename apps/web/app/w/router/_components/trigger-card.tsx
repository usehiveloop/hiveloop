"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import {
  MoreHorizontalIcon,
  FlashIcon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { IntegrationLogo } from "@/components/integration-logo"
import type { MockTrigger } from "./mock-data"

interface TriggerCardProps {
  trigger: MockTrigger
  onEdit: (trigger: MockTrigger) => void
  onToggleEnabled: (triggerId: string) => void
  onDelete: (triggerId: string) => void
}

function summarizeCondition(conditions: MockTrigger["rules"][number]["conditions"]): string {
  if (!conditions || conditions.conditions.length === 0) return "catch-all"
  const first = conditions.conditions[0]
  if (first.operator === "exists" || first.operator === "not_exists") {
    return `${first.path} ${first.operator}`
  }
  const valueStr = Array.isArray(first.value) ? first.value.join(", ") : (first.value ?? "")
  const suffix = conditions.conditions.length > 1
    ? ` +${conditions.conditions.length - 1}`
    : ""
  return `${first.path} ${first.operator} ${valueStr}${suffix}`
}

export function TriggerCard({ trigger, onEdit, onToggleEnabled, onDelete }: TriggerCardProps) {
  const uniqueAgents = new Set(trigger.rules.map((rule) => rule.agentName))

  return (
    <div
      className="rounded-xl border border-border p-4 transition-colors hover:border-primary/40 cursor-pointer"
      onClick={() => onEdit(trigger)}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-3 min-w-0">
          <IntegrationLogo provider={trigger.provider} size={28} />
          <div className="min-w-0">
            <p className="text-sm font-semibold text-foreground">{trigger.connectionName}</p>
            <div className="flex items-center gap-1.5 mt-1 flex-wrap">
              {trigger.triggerKeys.map((key) => (
                <Badge key={key} variant="secondary" className="text-[10px] font-mono">
                  {key}
                </Badge>
              ))}
            </div>
          </div>
        </div>

        <div className="flex items-center gap-2 shrink-0" onClick={(event) => event.stopPropagation()}>
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<Button variant="ghost" size="icon-xs" />}
            >
              <HugeiconsIcon icon={MoreHorizontalIcon} size={14} />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" sideOffset={4}>
              <DropdownMenuItem onClick={() => onEdit(trigger)}>
                Edit trigger
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => onToggleEnabled(trigger.id)}>
                {trigger.enabled ? "Disable" : "Enable"}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem className="text-destructive" onClick={() => onDelete(trigger.id)}>
                Delete trigger
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <div className="flex items-center gap-3 mt-3">
        <Badge variant={trigger.routingMode === "triage" ? "default" : "outline"} className="text-[10px]">
          {trigger.routingMode === "triage" ? "LLM triage" : "Rule-based"}
        </Badge>
        {trigger.routingMode === "rule" && trigger.rules.length > 0 && (
          <span className="text-[11px] text-muted-foreground">
            {trigger.rules.length} rule{trigger.rules.length !== 1 ? "s" : ""}
            {uniqueAgents.size > 0 && <> &middot; {uniqueAgents.size} agent{uniqueAgents.size !== 1 ? "s" : ""}</>}
          </span>
        )}
        {trigger.routingMode === "triage" && trigger.enrichCrossReferences && (
          <span className="text-[11px] text-muted-foreground flex items-center gap-1">
            <HugeiconsIcon icon={Tick02Icon} size={10} className="text-primary" />
            cross-ref enrichment
          </span>
        )}
        {!trigger.enabled && (
          <Badge variant="destructive" className="text-[10px]">
            Disabled
          </Badge>
        )}
      </div>

      {trigger.routingMode === "rule" && trigger.rules.length > 0 && (
        <div className="mt-3 border-t border-border pt-3">
          <div className="flex flex-col gap-1.5">
            {trigger.rules.map((rule, index) => (
              <div key={rule.id} className="flex items-center gap-2 text-[12px]">
                <span className="text-muted-foreground/50 font-mono w-4 shrink-0 text-right">
                  {index === trigger.rules.length - 1 ? "\u2514" : "\u251C"}
                </span>
                <span className="inline-flex items-center justify-center h-4 min-w-4 px-1 rounded bg-muted text-[10px] font-mono text-muted-foreground shrink-0">
                  {rule.priority}
                </span>
                <span className="font-medium text-foreground truncate">{rule.agentName}</span>
                <span className="text-muted-foreground truncate">{summarizeCondition(rule.conditions)}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
