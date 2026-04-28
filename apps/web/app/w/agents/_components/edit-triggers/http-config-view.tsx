"use client"

import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, GlobeIcon } from "@hugeicons/core-free-icons"
import {
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"

interface HttpConfigViewProps {
  onSave: (input: { instructions: string; secretKey: string }) => void
  onBack: () => void
}

export function HttpConfigView({ onSave, onBack }: HttpConfigViewProps) {
  const [instructions, setInstructions] = useState("")
  const [secretKey, setSecretKey] = useState("")

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
            <div className="flex size-6 items-center justify-center rounded-md bg-blue-500/10 text-blue-600 dark:text-blue-400">
              <HugeiconsIcon icon={GlobeIcon} strokeWidth={2} size={12} />
            </div>
            <DialogTitle>HTTP trigger</DialogTitle>
          </div>
        </div>
        <DialogDescription className="mt-2">
          A unique URL is generated for this trigger after the agent is created. Anyone POSTing to it fires the agent.
        </DialogDescription>
      </DialogHeader>

      <div className="mt-4 flex flex-1 flex-col gap-5 overflow-y-auto">
        <div className="flex flex-col gap-2">
          <Label htmlFor="http-instructions">
            Instructions{" "}
            <span className="font-normal text-muted-foreground">(optional)</span>
          </Label>
          <Textarea
            id="http-instructions"
            value={instructions}
            onChange={(event) => setInstructions(event.target.value)}
            placeholder="When this fires, summarize $refs.body and post it to Slack."
            className="min-h-28 font-mono text-[13px]"
          />
          <p className="text-[11px] text-muted-foreground">
            Sent to the agent when the trigger fires. Reference incoming payload fields with{" "}
            <code className="font-mono text-[11px]">$refs.x</code>.
          </p>
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="http-secret">
            Secret{" "}
            <span className="font-normal text-muted-foreground">(optional)</span>
          </Label>
          <Input
            id="http-secret"
            type="password"
            autoComplete="off"
            value={secretKey}
            onChange={(event) => setSecretKey(event.target.value)}
            placeholder="Leave blank to rely on the unguessable URL"
          />
          <p className="text-[11px] text-muted-foreground">
            If set, every request must include this value in any one of:{" "}
            <code className="font-mono text-[11px]">Authorization: Bearer …</code>,{" "}
            <code className="font-mono text-[11px]">X-Api-Key</code>,{" "}
            <code className="font-mono text-[11px]">X-Webhook-Secret</code>, or{" "}
            <code className="font-mono text-[11px]">?secret=…</code>. Stored hashed on our side.
          </p>
        </div>
      </div>

      <div className="shrink-0 pt-4">
        <Button onClick={() => onSave({ instructions, secretKey })} className="w-full">
          Save HTTP trigger
        </Button>
      </div>
    </>
  )
}
