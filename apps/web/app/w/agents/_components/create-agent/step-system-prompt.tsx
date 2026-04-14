import { useCallback } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { useCreateAgent } from "./context"
import { ProviderPromptEditor } from "./provider-prompt-editor"

export function StepSystemPrompt() {
  const { form, goTo } = useCreateAgent()
  const providerPrompts = form.watch("providerPrompts")

  const hasAtLeastOnePrompt = Object.values(providerPrompts).some((prompt) => prompt.trim())

  const handleChange = useCallback(
    (nextValue: Record<string, string>) => {
      form.setValue("providerPrompts", nextValue)
    },
    [form],
  )

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={() => goTo("basics")} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>System prompt</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Define your agent&apos;s core behavior per provider. You need at least one provider prompt to continue.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1 min-h-0">
        <ProviderPromptEditor value={providerPrompts} onChange={handleChange} />
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={() => goTo("instructions")} className="w-full" disabled={!hasAtLeastOnePrompt}>
          Continue
        </Button>
      </div>
    </div>
  )
}
