import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { type AgentStatus, statusConfig } from "../_data/agents"
import type { components } from "@/lib/api/schema"

type Agent = components["schemas"]["agentResponse"]

function hasIntegrations(agent: Agent): boolean {
  if (!agent.integrations || typeof agent.integrations !== "object") return false
  return Object.keys(agent.integrations).length > 0
}

function hasResources(agent: Agent): boolean {
  if (!agent.resources || typeof agent.resources !== "object") return false
  return Object.keys(agent.resources).length > 0
}

interface AgentStatusIndicatorProps {
  status: AgentStatus
  agent?: Agent
}

export function AgentStatusIndicator({ status, agent }: AgentStatusIndicatorProps) {
  const config = statusConfig[status]

  const needsResources = agent && status === "active" && hasIntegrations(agent) && !hasResources(agent)

  const dotColor = needsResources ? "bg-amber-500" : config.color
  const tooltipText = needsResources
    ? "No resources configured — grant this agent access to specific resources"
    : config.label

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span className={`h-2 w-2 rounded-full shrink-0 ${dotColor}`} />
        }
      />
      <TooltipContent>{tooltipText}</TooltipContent>
    </Tooltip>
  )
}
