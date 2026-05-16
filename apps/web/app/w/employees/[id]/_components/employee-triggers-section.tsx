"use client"

import * as React from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { FlashIcon } from "@hugeicons/core-free-icons"
import { FormEmptyWell, FormSection } from "@/app/w/_components/form-section"
import {
  EditTriggersDialog,
  HttpEndpointPill,
  TriggerTypeAvatar,
  triggerDisplayName,
} from "@/app/w/agents/_components/edit-triggers-dialog"
import type { TriggerConfig } from "@/app/w/agents/_components/create-agent/types"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

export function EmployeeTriggersSection({
  triggers,
  connectionIDs,
  dialogOpen,
  onDialogOpenChange,
  onAdd,
  onRemove,
  onUpdate,
}: {
  triggers: TriggerConfig[]
  connectionIDs: Set<string>
  dialogOpen: boolean
  onDialogOpenChange: (open: boolean) => void
  onAdd: (trigger: TriggerConfig) => void
  onRemove: (index: number) => void
  onUpdate: (index: number, newTriggers: TriggerConfig[]) => void
}) {
  return (
    <>
      <FormSection
        title="Triggers"
        description="Webhook events and HTTP endpoints that send messages to this employee."
      >
        <div className="flex flex-col gap-2">
          {triggers.length === 0 ? (
            <FormEmptyWell
              icon={FlashIcon}
              message="No triggers configured. This employee is invoked manually."
              action={
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => onDialogOpenChange(true)}
                >
                  <HugeiconsIcon
                    icon={FlashIcon}
                    size={14}
                    data-icon="inline-start"
                  />
                  Edit triggers
                </Button>
              }
            />
          ) : (
            <>
              {triggers.map((trigger, index) => (
                <TriggerRow
                  key={`${trigger.triggerType}-${trigger.connectionId}-${index}`}
                  trigger={trigger}
                />
              ))}
              <Button
                variant="outline"
                size="sm"
                className="mt-1 w-fit"
                onClick={() => onDialogOpenChange(true)}
              >
                <HugeiconsIcon
                  icon={FlashIcon}
                  size={14}
                  data-icon="inline-start"
                />
                Edit triggers
              </Button>
            </>
          )}
        </div>
      </FormSection>

      <EditTriggersDialog
        open={dialogOpen}
        onOpenChange={onDialogOpenChange}
        triggers={triggers}
        connectionIds={connectionIDs}
        onAdd={onAdd}
        onRemove={onRemove}
        onUpdate={onUpdate}
      />
    </>
  )
}

function TriggerRow({ trigger }: { trigger: TriggerConfig }) {
  return (
    <div className="flex items-start gap-3 rounded-xl border border-border bg-muted/50 p-3">
      <TriggerTypeAvatar trigger={trigger} size={28} />
      <div className="min-w-0 flex-1">
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
                {trigger.conditions.conditions.length !== 1 ? "s" : ""} (
                {trigger.conditions.mode})
              </p>
            ) : null}
          </>
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
    </div>
  )
}
