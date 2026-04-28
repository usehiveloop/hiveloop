"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  Cancel01Icon,
  Edit02Icon,
  FlashIcon,
} from "@hugeicons/core-free-icons"
import {
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import type { TriggerConfig } from "../create-agent/types"
import {
  HttpEndpointPill,
  TriggerTypeAvatar,
  triggerDisplayName,
} from "./trigger-type-display"

interface TriggerListViewProps {
  triggers: TriggerConfig[]
  onAdd: () => void
  onEdit: (index: number) => void
  onRemove: (index: number) => void
  onDone: () => void
}

export function TriggerListView({
  triggers,
  onAdd,
  onEdit,
  onRemove,
  onDone,
}: TriggerListViewProps) {
  return (
    <>
      <DialogHeader>
        <DialogTitle>Edit triggers</DialogTitle>
        <DialogDescription className="mt-2">
          Add, edit, or remove webhook events that invoke this agent.
        </DialogDescription>
      </DialogHeader>

      <div className="mt-4 flex flex-1 flex-col gap-2 overflow-y-auto">
        {triggers.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-3 py-12 text-center">
            <div className="flex size-12 items-center justify-center rounded-full bg-muted">
              <HugeiconsIcon
                icon={FlashIcon}
                size={20}
                className="text-muted-foreground"
              />
            </div>
            <p className="max-w-xs text-sm text-muted-foreground">
              No triggers configured. Add one to invoke this agent automatically on webhook events.
            </p>
          </div>
        ) : (
          triggers.map((trigger, index) => (
            <div
              key={`${trigger.triggerType}-${trigger.connectionId}-${index}`}
              className="flex items-start gap-3 rounded-xl border border-border bg-muted/50 p-3"
            >
              <TriggerTypeAvatar trigger={trigger} size={28} />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-foreground">
                  {triggerDisplayName(trigger)}
                </p>
                {trigger.triggerType === "webhook" ? (
                  <>
                    <div className="mt-1 flex flex-wrap gap-1">
                      {trigger.triggerDisplayNames.map((displayName, keyIndex) => (
                        <Badge
                          key={`${displayName}-${keyIndex}`}
                          variant="secondary"
                          className="font-mono text-[10px]"
                        >
                          {displayName}
                        </Badge>
                      ))}
                    </div>
                    {trigger.conditions && trigger.conditions.conditions.length > 0 ? (
                      <p className="mt-1 text-[11px] text-muted-foreground">
                        {trigger.conditions.conditions.length} filter
                        {trigger.conditions.conditions.length !== 1 ? "s" : ""} ({trigger.conditions.mode})
                      </p>
                    ) : null}
                  </>
                ) : null}
                {trigger.triggerType === "cron" && trigger.cronSchedule ? (
                  <p className="mt-1 font-mono text-[11px] text-muted-foreground">
                    {trigger.cronSchedule}
                  </p>
                ) : null}
                {trigger.triggerType === "http" ? (
                  <div className="mt-2">
                    <HttpEndpointPill />
                    {trigger.secretKey ? (
                      <p className="mt-1.5 text-[11px] text-muted-foreground">
                        HMAC verification enabled
                      </p>
                    ) : null}
                  </div>
                ) : null}
              </div>
              <div className="flex shrink-0 items-center gap-1">
                {trigger.triggerType === "webhook" ? (
                  <button
                    type="button"
                    onClick={() => onEdit(index)}
                    className="flex h-7 w-7 items-center justify-center rounded-lg transition-colors hover:bg-muted"
                    title="Edit"
                  >
                    <HugeiconsIcon icon={Edit02Icon} size={14} className="text-muted-foreground" />
                  </button>
                ) : null}
                <button
                  type="button"
                  onClick={() => onRemove(index)}
                  className="flex h-7 w-7 items-center justify-center rounded-lg transition-colors hover:bg-destructive/10"
                  title="Remove"
                >
                  <HugeiconsIcon icon={Cancel01Icon} size={14} className="text-destructive" />
                </button>
              </div>
            </div>
          ))
        )}
      </div>

      <div className="flex shrink-0 flex-col gap-2 pt-4">
        <Button variant="secondary" onClick={onAdd} className="w-full">
          <HugeiconsIcon icon={Add01Icon} size={14} data-icon="inline-start" />
          Add trigger
        </Button>
        <Button onClick={onDone} disabled={triggers.length === 0} className="w-full">
          Done
        </Button>
      </div>
    </>
  )
}
