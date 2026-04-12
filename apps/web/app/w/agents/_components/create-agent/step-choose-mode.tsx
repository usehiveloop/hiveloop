import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { PencilEdit02Icon, Store01Icon } from "@hugeicons/core-free-icons"
import { ChoiceCard } from "./choice-card"
import { useCreateAgent } from "./context"

export function StepChooseMode() {
  const { setMode } = useCreateAgent()

  return (
    <div>
      <DialogHeader>
        <DialogTitle>Create a new agent</DialogTitle>
        <DialogDescription className="mt-2">
          Build from scratch or install a ready-made agent from the marketplace.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3 pt-4">
        <ChoiceCard
          icon={PencilEdit02Icon}
          title="Create from scratch"
          description="Write your own system prompt and configure every detail manually."
          onClick={() => setMode("scratch")}
        />
        <ChoiceCard
          icon={Store01Icon}
          title="Install from marketplace"
          description="Browse community-built agents and install one in seconds."
          onClick={() => setMode("marketplace")}
        />
      </div>
    </div>
  )
}
