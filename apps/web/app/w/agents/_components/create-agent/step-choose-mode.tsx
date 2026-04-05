import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { PencilEdit02Icon, SparklesIcon, Store01Icon } from "@hugeicons/core-free-icons"
import { ChoiceCard } from "./choice-card"
import type { CreationMode } from "./types"

interface StepChooseModeProps {
  onSelect: (mode: CreationMode) => void
}

export function StepChooseMode({ onSelect }: StepChooseModeProps) {
  return (
    <div>
      <DialogHeader>
        <DialogTitle>Create a new agent</DialogTitle>
        <DialogDescription className="mt-2">
          Build from scratch, let AI generate one for you, or install a ready-made agent from the marketplace.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3 pt-4">
        <ChoiceCard
          icon={PencilEdit02Icon}
          title="Create from scratch"
          description="Write your own system prompt and configure every detail manually."
          onClick={() => onSelect("scratch")}
        />
        <ChoiceCard
          icon={SparklesIcon}
          title="Forge with AI"
          description="Describe what you want and let AI generate an optimized agent for you."
          onClick={() => onSelect("forge")}
        />
        <ChoiceCard
          icon={Store01Icon}
          title="Install from marketplace"
          description="Browse community-built agents and install one in seconds."
          onClick={() => onSelect("marketplace")}
        />
      </div>
    </div>
  )
}
