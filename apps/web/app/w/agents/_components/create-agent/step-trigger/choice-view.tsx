"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Cancel01Icon,
  FlashIcon,
} from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { IntegrationLogo } from "@/components/integration-logo"
import type { TriggerSelection } from "./types"

interface ChoiceViewProps {
  triggerSelection: TriggerSelection | null
  onAddTrigger: () => void
  onRemoveTrigger: () => void
  onContinue: () => void
  onBack: () => void
}

export function ChoiceView({ triggerSelection, onAddTrigger, onRemoveTrigger, onContinue, onBack }: ChoiceViewProps) {
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
