"use client"

import { useMemo } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  Search01Icon,
  Notification03Icon,
  FlashIcon,
  Settings02Icon,
  Cancel01Icon,
} from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"

interface SelectedEvent {
  key: string
  displayName: string
  refs: Record<string, string>
  conditions: { mode: "all" | "any"; conditions: Array<{ path: string; operator: string; value: unknown }> } | null
}

interface TriggerPickerViewProps {
  provider: string
  connectionName: string
  search: string
  onSearchChange: (value: string) => void
  selectedEvents: Map<string, SelectedEvent>
  onToggleEvent: (key: string, displayName: string, refs: Record<string, string>) => void
  onRemoveEvent: (key: string) => void
  onConfigureEvent: (key: string) => void
  onConfirm: () => void
  onBack: () => void
}

const CONVERSATIONAL_KEYS = new Set(["app_mention", "message.im", "message.channels", "message.groups"])

export function TriggerPickerView({ provider, connectionName, search, onSearchChange, selectedEvents, onToggleEvent, onRemoveEvent, onConfigureEvent, onConfirm, onBack }: TriggerPickerViewProps) {
  const { data: triggersData, isLoading } = $api.useQuery(
    "get",
    "/v1/catalog/integrations/{id}/triggers",
    { params: { path: { id: provider } } },
    { enabled: !!provider },
  )

  const triggers = useMemo(() => {
    if (!triggersData) return []
    const allTriggers = triggersData.triggers ?? []
    return allTriggers.filter((trigger) => !CONVERSATIONAL_KEYS.has(trigger.key ?? ""))
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
    // Sort selected triggers to top within each group.
    for (const group of Object.values(groups)) {
      group.sort((first, second) => {
        const firstSelected = selectedEvents.has(first.key ?? "") ? 0 : 1
        const secondSelected = selectedEvents.has(second.key ?? "") ? 0 : 1
        return firstSelected - secondSelected
      })
    }
    return groups
  }, [filtered, selectedEvents])

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <div className="flex items-center gap-2.5">
            <IntegrationLogo provider={provider} size={20} />
            <DialogTitle>{connectionName} events</DialogTitle>
          </div>
        </div>
        <DialogDescription className="mt-2">
          Select events that trigger this agent. Use the gear icon to add filters.
        </DialogDescription>
      </DialogHeader>

      <div className="relative mt-4">
        <HugeiconsIcon icon={Search01Icon} size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
        <Input placeholder="Search events..." value={search} onChange={(event) => onSearchChange(event.target.value)} className="pl-9 h-9" />
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
                  const triggerKey = trigger.key ?? ""
                  const isSelected = selectedEvents.has(triggerKey)
                  const selectedEvent = selectedEvents.get(triggerKey)
                  const hasFilters = selectedEvent?.conditions && selectedEvent.conditions.conditions.length > 0

                  return (
                    <div
                      key={triggerKey}
                      className={`flex items-start gap-3 w-full rounded-xl p-3 transition-colors ${
                        isSelected ? "bg-primary/5 border border-primary/20" : "bg-muted/50 hover:bg-muted border border-transparent cursor-pointer"
                      }`}
                      onClick={isSelected ? undefined : () => onToggleEvent(triggerKey, trigger.display_name ?? "", (trigger as Record<string, unknown>).refs as Record<string, string> ?? {})}
                    >
                      <div className="flex items-center justify-center h-6 w-6 rounded-md bg-amber-500/10 shrink-0 mt-0.5">
                        <HugeiconsIcon icon={FlashIcon} size={12} className="text-amber-500" />
                      </div>
                      <div className="flex-1 min-w-0">
                        <p className="text-sm font-medium text-foreground">{trigger.display_name}</p>
                        <p className="text-[12px] text-muted-foreground mt-0.5 line-clamp-1">{trigger.description}</p>
                        {hasFilters && (
                          <Badge variant="secondary" className="text-[10px] mt-1.5">
                            {selectedEvent.conditions!.conditions.length} filter{selectedEvent.conditions!.conditions.length !== 1 ? "s" : ""}
                          </Badge>
                        )}
                      </div>
                      {isSelected && (
                        <div className="flex items-center gap-1 shrink-0 mt-0.5" onClick={(event) => event.stopPropagation()}>
                          <button
                            type="button"
                            onClick={() => onConfigureEvent(triggerKey)}
                            className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors cursor-pointer"
                            title="Configure filters"
                          >
                            <HugeiconsIcon icon={Settings02Icon} size={14} className="text-muted-foreground" />
                          </button>
                          <button
                            type="button"
                            onClick={() => onRemoveEvent(triggerKey)}
                            className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-destructive/10 transition-colors cursor-pointer"
                            title="Remove"
                          >
                            <HugeiconsIcon icon={Cancel01Icon} size={12} className="text-destructive" />
                          </button>
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
            </div>
          ))
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onConfirm} disabled={selectedEvents.size === 0} className="w-full">
          {selectedEvents.size > 0
            ? `Continue with ${selectedEvents.size} event${selectedEvents.size > 1 ? "s" : ""}`
            : "Select at least one event"}
        </Button>
      </div>
    </>
  )
}
