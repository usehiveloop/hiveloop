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
} from "@hugeicons/core-free-icons"

interface AgentActionsProps {
  onEdit?: () => void
  onDelete?: () => void
}

export function AgentActions({ onEdit, onDelete }: AgentActionsProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center justify-center h-8 w-8 rounded-lg transition-colors hover:bg-muted outline-none">
        <HugeiconsIcon icon={MoreHorizontalIcon} size={16} className="text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" sideOffset={4}>
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
