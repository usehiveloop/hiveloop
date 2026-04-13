import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  Tick02Icon,
  BrowserIcon,
  SourceCodeIcon,
  BrainIcon,
} from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { ChoiceCard } from "./choice-card"
import { useCreateAgent } from "./context"

const toolIcons: Record<string, typeof BrowserIcon> = {
  "chrome": BrowserIcon,
  "codedb": SourceCodeIcon,
  "codebase-memory": BrainIcon,
}

export function StepSandboxTools() {
  const { selectedSandboxTools, toggleSandboxTool, goTo } = useCreateAgent()

  const { data: sandboxTools, isLoading } = $api.useQuery("get", "/v1/agents/sandbox-tools")

  const tools = sandboxTools ?? []

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => goTo("sandbox")}
            className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1"
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Sandbox tools</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Choose the tools and services available inside your agent&apos;s sandbox. You can change these later.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3 mt-4 flex-1 overflow-y-auto">
        {isLoading ? (
          Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="h-[80px] w-full rounded-xl" />
          ))
        ) : (
          tools.map((tool) => {
            const toolId = tool.id ?? ""
            const isSelected = selectedSandboxTools.has(toolId)

            return (
              <ChoiceCard
                key={toolId}
                icon={toolIcons[toolId]}
                iconClassName={isSelected ? "text-primary" : undefined}
                title={tool.name ?? ""}
                description={tool.description ?? ""}
                onClick={() => toggleSandboxTool(toolId)}
                selected={isSelected}
                trailing={
                  isSelected ? (
                    <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0 mt-0.5" />
                  ) : null
                }
              />
            )
          })
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={() => goTo("integrations")} className="w-full">
          {selectedSandboxTools.size > 0
            ? `Continue with ${selectedSandboxTools.size} tool${selectedSandboxTools.size > 1 ? "s" : ""}`
            : "Skip for now"}
        </Button>
      </div>
    </div>
  )
}
