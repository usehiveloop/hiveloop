import { HugeiconsIcon } from "@hugeicons/react"
import { BrainIcon } from "@hugeicons/core-free-icons"
import { type Agent } from "../_data/agents"
import { AgentStatusIndicator } from "./agent-status"
import { IntegrationStack } from "./integration-stack"
import { AgentActions } from "./agent-actions"

function formatTokens(n: number) {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(0)}k`
  return n.toString()
}

export function AgentsTable({ agents }: { agents: Agent[] }) {
  if (agents.length === 0) {
    return (
      <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
        No agents found
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-2">
      {/* Column headers — desktop only */}
      <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
        <span className="flex-1 min-w-0">Name</span>
        <span className="w-6 shrink-0 text-center">Mem</span>
        <span className="w-20 shrink-0 text-right">Integrations</span>
        <span className="w-16 shrink-0 text-right">Runs</span>
        <span className="w-16 shrink-0 text-right">Spend</span>
        <span className="w-14 shrink-0 text-right">Tokens</span>
        <span className="w-6 shrink-0" />
        <span className="w-8 shrink-0" />
      </div>

      {agents.map((agent) => (
        <div key={agent.id}>
          {/* Desktop card */}
          <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2 transition-colors hover:border-primary cursor-pointer">
            <span className="text-sm font-medium text-foreground truncate flex-1 min-w-0">{agent.name}</span>
            <div className="w-6 shrink-0 flex justify-center">
              {agent.hasMemory && <HugeiconsIcon icon={BrainIcon} size={14} className="text-primary" />}
            </div>
            <div className="w-20 shrink-0 flex justify-end">
              <IntegrationStack integrations={agent.integrations} />
            </div>
            <span className="w-16 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">{agent.totalRuns.toLocaleString()}</span>
            <span className="w-16 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">${agent.totalSpend.toFixed(2)}</span>
            <span className="w-14 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">{formatTokens(agent.totalTokens)}</span>
            <div className="w-6 shrink-0 flex justify-center">
              <AgentStatusIndicator status={agent.status} />
            </div>
            <div className="w-8 shrink-0 flex justify-center">
              <AgentActions />
            </div>
          </div>

          {/* Mobile card */}
          <div className="flex md:hidden flex-col gap-3 rounded-xl border border-border p-4 transition-colors hover:border-primary cursor-pointer">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-1.5 min-w-0 flex-1">
                <span className="text-sm font-medium text-foreground truncate">{agent.name}</span>
                {agent.hasMemory && (
                  <HugeiconsIcon icon={BrainIcon} size={14} className="text-primary shrink-0" />
                )}
              </div>
              <AgentActions />
            </div>
            <div className="flex items-center justify-between">
              <IntegrationStack integrations={agent.integrations} />
              <AgentStatusIndicator status={agent.status} />
            </div>
            <div className="flex items-center gap-4 text-xs text-muted-foreground font-mono tabular-nums">
              <span>{agent.totalRuns.toLocaleString()} runs</span>
              <span>${agent.totalSpend.toFixed(2)}</span>
              <span>{formatTokens(agent.totalTokens)}</span>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}
