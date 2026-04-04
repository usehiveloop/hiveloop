import type { Run } from "../_data/agent-detail"
import { RunCard } from "./run-card"

type RecentRunsProps = {
  runs: Run[]
  onSelectRun: (run: Run) => void
}

export function RecentRuns({ runs, onSelectRun }: RecentRunsProps) {
  return (
    <div className="flex flex-col gap-3">
      <h2 className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground">
        Recent runs
      </h2>
      <div className="flex flex-col gap-2">
        {runs.map((run) => (
          <RunCard key={run.id} run={run} onClick={() => onSelectRun(run)} />
        ))}
      </div>
    </div>
  )
}
