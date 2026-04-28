"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  Clock01Icon,
  GlobeIcon,
  WebhookIcon,
} from "@hugeicons/core-free-icons"
import {
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { ChoiceCard } from "../create-agent/choice-card"

interface TriggerTypePickerViewProps {
  onPick: (triggerType: "webhook" | "http" | "cron") => void
  onBack: () => void
}

export function TriggerTypePickerView({ onPick, onBack }: TriggerTypePickerViewProps) {
  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onBack}
            className="-ml-1 flex h-7 w-7 items-center justify-center rounded-lg transition-colors hover:bg-muted"
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Add a trigger</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Pick how this trigger fires.
        </DialogDescription>
      </DialogHeader>

      <div className="mt-4 flex flex-1 flex-col gap-2 overflow-y-auto">
        <ChoiceCard
          icon={WebhookIcon}
          iconClassName="text-foreground"
          title="Webhook"
          description="Fire on a webhook event from a connected integration like GitHub or Slack."
          onClick={() => onPick("webhook")}
        />
        <ChoiceCard
          icon={GlobeIcon}
          iconClassName="text-foreground"
          title="HTTP"
          description="Fire on an inbound HTTP POST to a unique URL, with optional HMAC verification."
          onClick={() => onPick("http")}
        />
        <ChoiceCard
          icon={Clock01Icon}
          iconClassName="text-foreground"
          title="Cron"
          description="Fire on a recurring schedule using a cron expression."
          onClick={() => onPick("cron")}
        />
      </div>
    </>
  )
}
