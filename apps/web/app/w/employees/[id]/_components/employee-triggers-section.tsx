"use client"

import * as React from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Delete02Icon,
  FlashIcon,
  Settings02Icon,
} from "@hugeicons/core-free-icons"
import { FormEmptyWell, FormSection } from "@/app/w/_components/form-section"
import {
  EditTriggersDialog,
  HttpEndpointField,
  TriggerTypeAvatar,
  triggerDisplayName,
} from "@/app/w/employees/_components/triggers/edit-triggers-dialog"
import type { TriggerConfig } from "@/app/w/employees/_components/triggers/types"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { ConfirmDialog } from "@/components/confirm-dialog"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"

export function EmployeeTriggersSection({
  triggers,
  connectionIDs,
  dialogOpen,
  onDialogOpenChange,
  onAdd,
  onRemove,
  onUpdate,
  onPersistUpdate,
  onPersistRemove,
  persisting,
}: {
  triggers: TriggerConfig[]
  connectionIDs: Set<string>
  dialogOpen: boolean
  onDialogOpenChange: (open: boolean) => void
  onAdd: (trigger: TriggerConfig) => void
  onRemove: (index: number) => void
  onUpdate: (index: number, newTriggers: TriggerConfig[]) => void
  onPersistUpdate: (index: number, newTriggers: TriggerConfig[]) => void
  onPersistRemove: (index: number) => void
  persisting: boolean
}) {
  const [editingHTTPIndex, setEditingHTTPIndex] = React.useState<number | null>(
    null
  )
  const [confirmingDelete, setConfirmingDelete] = React.useState(false)
  const editingHTTPTrigger =
    editingHTTPIndex === null ? null : triggers[editingHTTPIndex]

  function handleUpdateHTTPTrigger(input: {
    instructions: string
    secretKey: string
  }) {
    if (editingHTTPIndex === null || !editingHTTPTrigger) return
    onPersistUpdate(editingHTTPIndex, [
      {
        ...editingHTTPTrigger,
        instructions: input.instructions.trim() || undefined,
        secretKey: input.secretKey.trim() || undefined,
      },
    ])
    setEditingHTTPIndex(null)
  }

  function handleDeleteHTTPTrigger() {
    if (editingHTTPIndex === null) return
    onPersistRemove(editingHTTPIndex)
    setConfirmingDelete(false)
    setEditingHTTPIndex(null)
  }

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
                  onOpenSettings={
                    trigger.triggerType === "http"
                      ? () => setEditingHTTPIndex(index)
                      : undefined
                  }
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

      <HTTPTriggerSettingsDialog
        trigger={editingHTTPTrigger}
        open={Boolean(editingHTTPTrigger)}
        onOpenChange={(open) => {
          if (!open) setEditingHTTPIndex(null)
        }}
        onSave={handleUpdateHTTPTrigger}
        onDelete={() => setConfirmingDelete(true)}
        loading={persisting}
      />

      <ConfirmDialog
        open={confirmingDelete}
        onOpenChange={setConfirmingDelete}
        title="Delete HTTP trigger"
        description="This endpoint will stop accepting trigger requests immediately after the change is saved."
        confirmLabel="Delete trigger"
        destructive
        loading={persisting}
        onConfirm={handleDeleteHTTPTrigger}
      />
    </>
  )
}

function TriggerRow({
  trigger,
  onOpenSettings,
}: {
  trigger: TriggerConfig
  onOpenSettings?: () => void
}) {
  return (
    <div className="relative flex items-start gap-3 rounded-xl border border-border bg-muted/50 p-3">
      <TriggerTypeAvatar trigger={trigger} size={28} />
      <div className="min-w-0 flex-1 pr-6">
        <p className="text-sm font-medium text-foreground">{triggerDisplayName(trigger)}</p>
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
            <HttpEndpointField url={trigger.endpointUrl} />
            {trigger.secretKey || trigger.secretSet ? (
              <p className="mt-1.5 text-[11px] text-muted-foreground">
                HMAC verification enabled
              </p>
            ) : null}
          </div>
        ) : null}
      </div>
      {onOpenSettings ? (
        <button
          type="button"
          onClick={onOpenSettings}
          className="absolute right-3 top-3 inline-flex shrink-0 items-center justify-center text-muted-foreground transition-colors hover:text-foreground"
          aria-label="Edit HTTP trigger"
          title="Edit HTTP trigger"
        >
          <HugeiconsIcon
            icon={Settings02Icon}
            size={15}
            strokeWidth={2.25}
          />
        </button>
      ) : null}
    </div>
  )
}

function HTTPTriggerSettingsDialog({
  trigger,
  open,
  onOpenChange,
  onSave,
  onDelete,
  loading,
}: {
  trigger: TriggerConfig | null | undefined
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (input: { instructions: string; secretKey: string }) => void
  onDelete: () => void
  loading: boolean
}) {
  const [instructions, setInstructions] = React.useState("")
  const [secretKey, setSecretKey] = React.useState("")

  React.useEffect(() => {
    if (!open) return
    setInstructions(trigger?.instructions ?? "")
    setSecretKey("")
  }, [open, trigger?.instructions])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>HTTP trigger</DialogTitle>
          <DialogDescription>
            Edit the instructions sent with this endpoint, or remove the trigger.
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="http-trigger-endpoint">Endpoint</Label>
            <HttpEndpointField url={trigger?.endpointUrl} />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="http-trigger-instructions">Instructions</Label>
            <Textarea
              id="http-trigger-instructions"
              value={instructions}
              onChange={(event) => setInstructions(event.target.value)}
              placeholder="When this endpoint receives a request, summarize the payload and decide what needs to happen next."
              className="min-h-32"
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="http-trigger-secret">New secret</Label>
            <Input
              id="http-trigger-secret"
              type="password"
              autoComplete="off"
              value={secretKey}
              onChange={(event) => setSecretKey(event.target.value)}
              placeholder="Leave blank to keep the current secret"
            />
            <p className="text-[11px] text-muted-foreground">
              {trigger?.secretSet
                ? "A secret is currently set. Enter a new value to rotate it."
                : "Set a secret to require HMAC-style shared-secret verification."}{" "}
              Requests can send it with{" "}
              <code className="font-mono text-[11px]">Authorization: Bearer …</code>,{" "}
              <code className="font-mono text-[11px]">X-Api-Key</code>,{" "}
              <code className="font-mono text-[11px]">X-Webhook-Secret</code>, or{" "}
              <code className="font-mono text-[11px]">?secret=…</code>.
            </p>
          </div>
        </div>

        <DialogFooter className="gap-2 sm:justify-between">
          <Button variant="destructive" onClick={onDelete} disabled={loading}>
            <HugeiconsIcon icon={Delete02Icon} size={14} data-icon="inline-start" />
            Delete trigger
          </Button>
          <Button loading={loading} onClick={() => onSave({ instructions, secretKey })}>
            Save changes
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
