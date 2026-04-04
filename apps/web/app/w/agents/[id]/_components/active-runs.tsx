import type { Run } from "../_data/agent-detail"
import { RunCard } from "./run-card"

type ActiveRunsProps = {
  runs: Run[]
  onSelectRun: (run: Run) => void
}

export function ActiveRuns({ runs, onSelectRun }: ActiveRunsProps) {
  if (runs.length === 0) {
    return (
      <div className="rounded-xl border border-border p-6">
        <h2 className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground mb-4">Active runs</h2>
        <p className="text-sm text-muted-foreground">No active runs right now.</p>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-3">
      <h2 className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground">
        Active runs ({runs.length})
      </h2>
      <div className="flex flex-col gap-2">
        {runs.map((run) => (
          <RunCard key={run.id} run={run} onClick={() => onSelectRun(run)} />
        ))}
      </div>
    </div>
  )
}
