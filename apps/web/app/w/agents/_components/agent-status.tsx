import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { type AgentStatus, statusConfig } from "../_data/agents"

export function AgentStatusIndicator({ status }: { status: AgentStatus }) {
  const config = statusConfig[status]

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span className={`h-2 w-2 rounded-full shrink-0 ${config.color}`} />
        }
      />
      <TooltipContent>{config.label}</TooltipContent>
    </Tooltip>
  )
}
