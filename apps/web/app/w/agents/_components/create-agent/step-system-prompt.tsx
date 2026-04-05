import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"

interface StepSystemPromptProps {
  onBack: () => void
  onNext: () => void
}

export function StepSystemPrompt({ onBack, onNext }: StepSystemPromptProps) {
  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>System prompt</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Define your agent&apos;s core behavior, personality, and constraints. This is the main instruction that shapes how your agent responds.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1">
        <Textarea
          placeholder={"You are a helpful assistant that triages GitHub issues.\n\nYour responsibilities:\n- Read and classify incoming issues\n- Assign appropriate labels and priority\n- Route to the correct team\n- Notify stakeholders of urgent issues"}
          className="flex-1 min-h-48 font-mono text-sm"
        />
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onNext} className="w-full">Continue</Button>
      </div>
    </div>
  )
}
