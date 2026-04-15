import { HugeiconsIcon } from "@hugeicons/react"
import { SparklesIcon, PencilEdit02Icon, ArrowRight01Icon, Loading03Icon } from "@hugeicons/core-free-icons"

interface RunsEmptyProps {
  onStartRun: () => void
  startingRun?: boolean
  onEditAgent: () => void
}

export function RunsEmpty({ onStartRun, startingRun, onEditAgent }: RunsEmptyProps) {
  return (
    <div className="flex flex-col items-center py-12">
      <div className="text-center mb-8">
        <h2 className="font-heading text-2xl font-semibold text-foreground">
          No runs yet
        </h2>
        <p className="text-sm text-muted-foreground mt-2 max-w-sm">
          Start a run to see your agent in action, or refine its configuration first.
        </p>
      </div>

      <div className="w-full max-w-sm flex flex-col gap-2">
        <button
          type="button"
          onClick={onStartRun}
          disabled={startingRun}
          className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed"
        >
          <HugeiconsIcon icon={SparklesIcon} size={20} className="shrink-0 mt-0.5 text-muted-foreground" />
          <div className="flex-1 min-w-0">
            <p className="text-sm font-semibold text-foreground">
              {startingRun ? "Starting run..." : "Start a run"}
            </p>
            <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">
              Create a new conversation and watch your agent work in real time.
            </p>
          </div>
          {startingRun ? (
            <HugeiconsIcon icon={Loading03Icon} size={16} className="text-muted-foreground animate-spin shrink-0 mt-0.5" />
          ) : (
            <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
          )}
        </button>

        <button
          type="button"
          onClick={onEditAgent}
          className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
        >
          <HugeiconsIcon icon={PencilEdit02Icon} size={20} className="shrink-0 mt-0.5 text-muted-foreground" />
          <div className="flex-1 min-w-0">
            <p className="text-sm font-semibold text-foreground">Edit agent</p>
            <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">
              Tweak the system prompt, model, or integrations before running.
            </p>
          </div>
          <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
        </button>
      </div>
    </div>
  )
}
