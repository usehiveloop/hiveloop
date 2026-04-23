"use client"

import { useState } from "react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { HugeiconsIcon } from "@hugeicons/react"
import { Add01Icon, Delete02Icon } from "@hugeicons/core-free-icons"

interface SetupCommandsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  agentName: string
}

export function SetupCommandsDialog({ open, onOpenChange, agentName }: SetupCommandsDialogProps) {
  const [commands, setCommands] = useState<string[]>([""])

  function addCommand() {
    setCommands([...commands, ""])
  }

  function removeCommand(index: number) {
    setCommands(commands.filter((_, idx) => idx !== index))
  }

  function updateCommand(index: number, value: string) {
    setCommands(commands.map((command, idx) => (idx === index ? value : command)))
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Setup commands</DialogTitle>
          <DialogDescription>
            Shell commands to run after the sandbox for <strong>{agentName}</strong> starts. Commands execute sequentially in order.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-2">
          {commands.map((command, index) => (
            <div key={index} className="flex items-center gap-2">
              <div className="flex items-center gap-2 flex-1">
                <span className="text-2xs font-mono text-muted-foreground/50 w-5 text-right shrink-0">
                  {index + 1}.
                </span>
                <Input
                  placeholder="npm install"
                  value={command}
                  onChange={(event) => updateCommand(index, event.target.value)}
                  className="flex-1 font-mono text-xs"
                />
              </div>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => removeCommand(index)}
                disabled={commands.length === 1}
              >
                <HugeiconsIcon icon={Delete02Icon} size={14} className="text-muted-foreground" />
              </Button>
            </div>
          ))}

          <Button variant="ghost" size="sm" className="self-start ml-7" onClick={addCommand}>
            <HugeiconsIcon icon={Add01Icon} size={14} data-icon="inline-start" />
            Add command
          </Button>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button>Save commands</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
