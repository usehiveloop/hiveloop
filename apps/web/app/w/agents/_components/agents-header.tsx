import { CreateAgentDialog } from "./create-agent-dialog"

export function AgentsHeader({ count }: { count: number }) {
  return (
    <div className="flex items-center justify-between mb-6">
      <div>
        <h1 className="font-heading text-xl font-semibold text-foreground">Agents</h1>
        <p className="text-sm text-muted-foreground mt-1">{count} agents in this workspace</p>
      </div>
      <CreateAgentDialog />
    </div>
  )
}
