"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Cancel01Icon,
  FlashIcon,
  Add01Icon,
} from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { IntegrationLogo } from "@/components/integration-logo"
import { useCreateAgent } from "../context"

interface ChoiceViewProps {
  onAddTrigger: () => void
  onContinue: () => void
  onBack: () => void
}

export function ChoiceView({ onAddTrigger, onContinue, onBack }: ChoiceViewProps) {
  const { triggers, removeTrigger } = useCreateAgent()

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Webhook triggers</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Optionally configure webhook events that automatically start this agent.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3 mt-6 flex-1 overflow-y-auto">
        {triggers.length > 0 ? (
          <>
            {triggers.map((trigger, index) => (
              <div key={`${trigger.connectionId}-${index}`} className="rounded-xl border border-primary/20 bg-primary/5 p-4">
                <div className="flex items-start gap-3">
                  <IntegrationLogo provider={trigger.provider} size={28} className="shrink-0 mt-0.5" />
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-semibold text-foreground">{trigger.connectionName}</p>
                    <div className="flex flex-wrap gap-1 mt-1.5">
                      {trigger.triggerDisplayNames.map((displayName) => (
                        <Badge key={displayName} variant="secondary" className="text-2xs">{displayName}</Badge>
                      ))}
                    </div>
                    {trigger.conditions && trigger.conditions.conditions.length > 0 && (
                      <p className="text-mini text-muted-foreground mt-1.5">
                        {trigger.conditions.conditions.length} filter{trigger.conditions.conditions.length !== 1 ? "s" : ""} ({trigger.conditions.mode})
                      </p>
                    )}
                  </div>
                  <button type="button" onClick={() => removeTrigger(index)} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-destructive/10 transition-colors shrink-0">
                    <HugeiconsIcon icon={Cancel01Icon} size={14} className="text-destructive" />
                  </button>
                </div>
              </div>
            ))}

            <button type="button" onClick={onAddTrigger} className="group flex items-center gap-3 w-full rounded-xl bg-muted/50 p-3 text-left transition-colors hover:bg-muted cursor-pointer border border-transparent">
              <HugeiconsIcon icon={Add01Icon} size={16} className="text-muted-foreground shrink-0" />
              <span className="text-sm text-muted-foreground">Add another trigger</span>
            </button>
          </>
        ) : (
          <>
            <button type="button" onClick={onAddTrigger} className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer border border-transparent">
              <div className="flex items-center justify-center h-10 w-10 rounded-lg bg-primary/10 shrink-0">
                <HugeiconsIcon icon={FlashIcon} size={20} className="text-primary" />
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-sm font-semibold text-foreground">Add a trigger</p>
                <p className="text-sm-alt text-muted-foreground mt-0.5 leading-relaxed">
                  Start this agent automatically when a webhook event fires — like a new issue, PR, or deployment.
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
              <p className="text-sm text-muted-foreground text-center">Skip this step to invoke this agent only through Zira.</p>
            </div>
          </>
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onContinue} className="w-full">
          {triggers.length > 0 ? `Continue with ${triggers.length} trigger${triggers.length > 1 ? "s" : ""}` : "Skip for now"}
        </Button>
      </div>
    </>
  )
}
