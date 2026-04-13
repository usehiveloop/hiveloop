import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  MoreHorizontalIcon,
  Edit02Icon,
  Copy01Icon,
  Delete02Icon,
  Archive02Icon,
  PlayIcon,
  CommandLineIcon,
  Key01Icon,
} from "@hugeicons/core-free-icons"
import type { components } from "@/lib/api/schema"

type Agent = components["schemas"]["agentResponse"]

interface AgentActionsProps {
  agent: Agent
  onEdit?: () => void
  onDelete?: () => void
  onEnvVars?: () => void
  onSetupCommands?: () => void
}

export function AgentActions({
  agent,
  onEdit,
  onDelete,
  onEnvVars,
  onSetupCommands,
}: AgentActionsProps) {
  const isDedicated = agent.sandbox_type === "dedicated"

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center justify-center h-8 w-8 rounded-lg transition-colors hover:bg-muted outline-none">
        <HugeiconsIcon icon={MoreHorizontalIcon} size={16} className="text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" sideOffset={4} className="w-64">
        <DropdownMenuGroup>
          <DropdownMenuItem onClick={onEdit}>
            <HugeiconsIcon icon={Edit02Icon} size={16} className="text-muted-foreground" />
            Edit agent
          </DropdownMenuItem>
          <DropdownMenuItem>
            <HugeiconsIcon icon={PlayIcon} size={16} className="text-muted-foreground" />
            Start run
          </DropdownMenuItem>
          <DropdownMenuItem>
            <HugeiconsIcon icon={Copy01Icon} size={16} className="text-muted-foreground" />
            Duplicate
          </DropdownMenuItem>
        </DropdownMenuGroup>
        {isDedicated && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem onClick={onEnvVars}>
                <HugeiconsIcon icon={Key01Icon} size={16} className="text-muted-foreground" />
                Add environment variables
              </DropdownMenuItem>
              <DropdownMenuItem onClick={onSetupCommands}>
                <HugeiconsIcon icon={CommandLineIcon} size={16} className="text-muted-foreground" />
                Add setup commands
              </DropdownMenuItem>
            </DropdownMenuGroup>
          </>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem>
            <HugeiconsIcon icon={Archive02Icon} size={16} className="text-muted-foreground" />
            Archive
          </DropdownMenuItem>
          <DropdownMenuItem variant="destructive" onClick={onDelete}>
            <HugeiconsIcon icon={Delete02Icon} size={16} />
            Delete
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
