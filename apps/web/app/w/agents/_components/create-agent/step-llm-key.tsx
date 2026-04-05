import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, ArrowRight01Icon, Key01Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { ChoiceCard } from "./choice-card"
import { llmKeys } from "./data"

interface StepLlmKeyProps {
  selectedKey: string | null
  onSelect: (keyId: string) => void
  onBack: () => void
}

export function StepLlmKey({ selectedKey, onSelect, onBack }: StepLlmKeyProps) {
  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Select an LLM key</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Choose which AI provider your agent will use. You can add a new key if you haven&apos;t connected one yet.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {llmKeys.map((key) => (
          <ChoiceCard
            key={key.id}
            logoUrl={key.logo}
            title={key.name}
            description={`${key.provider} · ${key.models.length} models available`}
            onClick={() => onSelect(key.id)}
            trailing={
              selectedKey === key.id ? (
                <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0 mt-0.5" />
              ) : (
                <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
              )
            }
          />
        ))}
      </div>

      <div className="pt-4 shrink-0">
        <Button variant="outline" className="w-full">
          <HugeiconsIcon icon={Key01Icon} size={16} data-icon="inline-start" />
          Add LLM key
        </Button>
      </div>
    </div>
  )
}
