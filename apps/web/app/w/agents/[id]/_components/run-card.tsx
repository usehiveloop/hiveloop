import type { Run } from "../_data/agent-detail"

type RunCardProps = {
  run: Run
  onClick: () => void
}

const statusConfig: Record<string, { color: string; label: string; pulse?: boolean }> = {
  running: { color: "bg-green-500", label: "Running", pulse: true },
  waiting_approval: { color: "bg-yellow-500", label: "Approval needed", pulse: true },
  completed: { color: "bg-green-500", label: "Completed" },
  error: { color: "bg-destructive", label: "Error" },
}

export function RunCard({ run, onClick }: RunCardProps) {
  const isWaiting = run.events.some((e) => e.type === "approval" && e.approvalStatus === "pending")
  const displayStatus = isWaiting ? statusConfig.waiting_approval : (statusConfig[run.status] ?? statusConfig.completed)

  return (
    <button
      onClick={onClick}
      className="flex items-center gap-3 rounded-xl border border-border px-4 py-3 text-left transition-colors hover:border-primary cursor-pointer"
    >
      <span className={`h-2 w-2 rounded-full shrink-0 ${displayStatus.color} ${displayStatus.pulse ? "animate-pulse" : ""}`} />
      <span className="text-sm font-medium text-foreground truncate flex-1 min-w-0">{run.subject}</span>
      <span className={`font-mono text-mini uppercase tracking-micro shrink-0 ${isWaiting ? "text-yellow-500" : "text-muted-foreground"}`}>
        {displayStatus.label}
      </span>
      <span className="font-mono text-mini text-muted-foreground shrink-0">{run.duration}</span>
      <span className="font-mono text-mini text-muted-foreground shrink-0 tabular-nums">${run.cost.toFixed(2)}</span>
    </button>
  )
}
