"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import { FlashIcon, ArrowRight01Icon } from "@hugeicons/core-free-icons"

interface RouterEmptyProps {
  onCreateTrigger: () => void
}

export function RouterEmpty({ onCreateTrigger }: RouterEmptyProps) {
  return (
    <div className="flex flex-col items-center pt-[20vh] pb-24 px-4">
      <div className="text-center mb-8">
        <h2 className="font-heading text-2xl font-semibold text-foreground">
          No triggers configured
        </h2>
        <p className="text-sm text-muted-foreground mt-2 max-w-sm">
          Triggers listen for webhook events from your connections and route them to the right agent.
        </p>
      </div>

      <div className="w-full max-w-sm flex flex-col gap-2">
        <button
          type="button"
          onClick={onCreateTrigger}
          className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
        >
          <div className="flex items-center justify-center h-8 w-8 rounded-lg bg-amber-500/10 shrink-0">
            <HugeiconsIcon icon={FlashIcon} size={16} className="text-amber-500" />
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-sm font-semibold text-foreground">Create your first trigger</p>
            <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">
              Pick a connection, select which events to listen for, and choose how events are routed to your agents.
            </p>
          </div>
          <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
        </button>
      </div>
    </div>
  )
}
