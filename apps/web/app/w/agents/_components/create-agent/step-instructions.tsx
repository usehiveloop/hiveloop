import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { useCreateAgent } from "./context"

export function StepInstructions() {
  const { form, goTo } = useCreateAgent()
  const instructions = form.watch("instructions")

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={() => goTo("system-prompt")} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Instructions</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Add specific rules and guidelines your agent should follow. These are additional constraints on top of the system prompt.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1">
        <Textarea
          {...form.register("instructions")}
          placeholder={"- Always check for duplicate issues before creating new ones\n- Never close issues without team lead approval\n- Escalate security-related issues to P1 immediately\n- Use professional, concise language in all communications"}
          className="flex-1 min-h-48 font-mono text-sm"
        />
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={() => goTo("skills")} className="w-full">
          {instructions.trim() ? "Continue" : "Skip for now"}
        </Button>
      </div>
    </div>
  )
}
