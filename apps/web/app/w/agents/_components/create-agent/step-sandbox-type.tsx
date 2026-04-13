import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, CloudServerIcon, LaptopProgrammingIcon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { ChoiceCard } from "./choice-card"
import { useCreateAgent } from "./context"

export function StepSandboxType() {
  const { form, goTo } = useCreateAgent()

  function handleSelect(type: "shared" | "dedicated") {
    form.setValue("sandboxType", type)
    goTo(type === "dedicated" ? "sandbox-tools" : "integrations")
  }

  return (
    <div>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={() => goTo("mode")} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Choose a workspace</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Workspaces are isolated environments where your agent runs. Choose the type that fits your agent&apos;s needs.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3 pt-4">
        <ChoiceCard
          icon={CloudServerIcon}
          title="Shared workspace"
          description="End-to-end encrypted. Best for agents that interact with APIs, process data, and call tools — without needing file system access."
          onClick={() => handleSelect("shared")}
        />
        <ChoiceCard
          icon={LaptopProgrammingIcon}
          title="Dedicated workspace"
          description="Full system access. For agents that need to read and write files, run shell commands, use code interpreters, or interact with a development environment."
          onClick={() => handleSelect("dedicated")}
        />
      </div>
    </div>
  )
}
