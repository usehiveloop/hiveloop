"use client"

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
  PlayIcon,
  Delete02Icon,
} from "@hugeicons/core-free-icons"

interface SourceActionsProps {
  onTriggerRun: () => void
  onDelete: () => void
}

export function SourceActions({ onTriggerRun, onDelete }: SourceActionsProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex h-8 w-8 items-center justify-center rounded-lg outline-none transition-colors hover:bg-muted">
        <HugeiconsIcon
          icon={MoreHorizontalIcon}
          size={16}
          className="text-muted-foreground"
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" sideOffset={4} className="w-48">
        <DropdownMenuGroup>
          <DropdownMenuItem onClick={onTriggerRun}>
            <HugeiconsIcon
              icon={PlayIcon}
              size={16}
              className="text-muted-foreground"
            />
            Trigger run
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem variant="destructive" onClick={onDelete}>
            <HugeiconsIcon icon={Delete02Icon} size={16} />
            Remove source
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
