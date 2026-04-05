import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { ModelCombobox } from "./model-combobox"
import { llmKeys } from "./data"

interface StepBasicsProps {
  selectedKeyId: string | null
  onBack: () => void
  onSubmit: () => void
}

export function StepBasics({ selectedKeyId, onBack, onSubmit }: StepBasicsProps) {
  const key = llmKeys.find((item) => item.id === selectedKeyId)

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Agent details</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Give your agent a name, pick a model, and optionally describe what it does.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-5 mt-4 flex-1">
        <div className="flex flex-col gap-2">
          <Label htmlFor="agent-name" className="text-sm">Name</Label>
          <Input id="agent-name" placeholder="e.g. Issue Triage Agent" />
        </div>

        <div className="flex flex-col gap-2">
          <Label className="text-sm">Model</Label>
          {key ? (
            <ModelCombobox models={key.models} />
          ) : (
            <Input disabled placeholder="Select an LLM key first" />
          )}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="agent-description" className="text-sm">
            Description <span className="text-muted-foreground font-normal">(optional)</span>
          </Label>
          <Textarea id="agent-description" placeholder="Briefly describe what this agent does..." className="min-h-24" />
        </div>
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onSubmit} className="w-full">Continue</Button>
      </div>
    </div>
  )
}
