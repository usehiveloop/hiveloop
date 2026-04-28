"use client"

import { useState } from "react"
import cronstrue from "cronstrue"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, Clock01Icon } from "@hugeicons/core-free-icons"
import {
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { CronScheduleInput } from "@/app/w/agents/new/_components/cron-schedule-input"

interface CronConfigViewProps {
  onSave: (input: { cronSchedule: string; instructions: string }) => void
  onBack: () => void
}

export function CronConfigView({ onSave, onBack }: CronConfigViewProps) {
  const [cronSchedule, setCronSchedule] = useState("")
  const [instructions, setInstructions] = useState("")

  const valid = (() => {
    if (!cronSchedule.trim()) return false
    try {
      cronstrue.toString(cronSchedule)
      return true
    } catch {
      return false
    }
  })()

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
          <div className="flex items-center gap-2">
            <div className="flex size-6 items-center justify-center rounded-md bg-amber-500/10 text-amber-600 dark:text-amber-400">
              <HugeiconsIcon icon={Clock01Icon} strokeWidth={2} size={12} />
            </div>
            <DialogTitle>Cron trigger</DialogTitle>
          </div>
        </div>
        <DialogDescription className="mt-2">
          Fires the agent on a recurring schedule. Times are evaluated in UTC.
        </DialogDescription>
      </DialogHeader>

      <div className="mt-4 flex flex-1 flex-col gap-5 overflow-y-auto">
        <div className="flex flex-col gap-2">
          <Label>Schedule</Label>
          <CronScheduleInput value={cronSchedule} onChange={setCronSchedule} />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="cron-instructions">
            Instructions{" "}
            <span className="font-normal text-muted-foreground">(optional)</span>
          </Label>
          <Textarea
            id="cron-instructions"
            value={instructions}
            onChange={(event) => setInstructions(event.target.value)}
            placeholder="Run the morning standup digest."
            className="min-h-28 font-mono text-[13px]"
          />
          <p className="text-[11px] text-muted-foreground">
            Sent to the agent on every fire.
          </p>
        </div>
      </div>

      <div className="shrink-0 pt-4">
        <Button
          onClick={() => onSave({ cronSchedule: cronSchedule.trim(), instructions })}
          disabled={!valid}
          className="w-full"
        >
          Save cron trigger
        </Button>
      </div>
    </>
  )
}
