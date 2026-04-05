import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, ArrowRight01Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { ChoiceCard } from "./choice-card"
import { ModelCombobox } from "./model-combobox"
import { llmKeys } from "./data"
import { useCreateAgent } from "./context"

export function StepForgeJudge() {
  const { form, goTo } = useCreateAgent()
  const credentialId = form.watch("credentialId")
  const judgeKeyId = form.watch("judgeKeyId")
  const judgeModel = form.watch("judgeModel")

  const selectedKey = llmKeys.find((key) => key.id === judgeKeyId)
  const agentKey = llmKeys.find((key) => key.id === credentialId)
  const isSameProvider = agentKey && selectedKey && agentKey.provider === selectedKey.provider

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={() => goTo("basics")} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Forge judge</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Pick an LLM to evaluate and score your agent during the forge process. A different provider from your agent&apos;s LLM is recommended.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-4 mt-4 flex-1 overflow-y-auto">
        <div className="flex flex-col gap-2">
          <Label className="text-sm">Provider</Label>
          <div className="flex flex-col gap-2">
            {llmKeys.map((key) => (
              <ChoiceCard
                key={key.id}
                logoUrl={key.logo}
                title={key.name}
                description={key.provider}
                onClick={() => {
                  form.setValue("judgeKeyId", key.id)
                  form.setValue("judgeModel", "")
                }}
                trailing={
                  judgeKeyId === key.id ? (
                    <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0 mt-0.5" />
                  ) : (
                    <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
                  )
                }
              />
            ))}
          </div>
        </div>

        {selectedKey && (
          <div className="flex flex-col gap-2">
            <Label className="text-sm">Model</Label>
            <ModelCombobox
              models={selectedKey.models}
              value={judgeModel || null}
              onSelect={(model) => form.setValue("judgeModel", model)}
            />
          </div>
        )}

        {isSameProvider && (
          <div className="rounded-xl border border-amber-500/20 bg-amber-500/5 px-4 py-3 flex gap-3 items-start">
            <span className="text-amber-500 text-sm leading-none mt-0.5">!</span>
            <p className="text-sm text-amber-500/90 leading-snug">
              Using a different AI model for the forge judge reduces bias and can lead to a more efficient agent.
            </p>
          </div>
        )}
      </div>

      <div className="flex flex-col gap-2 pt-4 shrink-0">
        <Button onClick={() => goTo("summary")} disabled={!judgeKeyId || !judgeModel} className="w-full">Continue</Button>
        <Button variant="ghost" onClick={() => goTo("summary")} className="w-full text-muted-foreground">
          Skip — use default judge
        </Button>
      </div>
    </div>
  )
}
