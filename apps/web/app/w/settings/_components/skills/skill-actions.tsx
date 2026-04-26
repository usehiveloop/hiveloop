"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import {
  MoreHorizontalIcon,
  Delete02Icon,
  Globe02Icon,
} from "@hugeicons/core-free-icons"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { components } from "@/lib/api/schema"

type SkillRow = components["schemas"]["skillResponse"]

interface SkillActionsProps {
  skill: SkillRow
  onDelete: () => void
  onPublish: () => void
  onUnpublish: () => void
}

export function SkillActions({ skill, onDelete, onPublish, onUnpublish }: SkillActionsProps) {
  const isPublished = !!skill.public_skill_id

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center justify-center h-8 w-8 rounded-lg transition-colors hover:bg-muted outline-none">
        <HugeiconsIcon icon={MoreHorizontalIcon} size={16} className="text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" sideOffset={4} className="min-w-56">
        <DropdownMenuGroup>
          {isPublished ? (
            <DropdownMenuItem onClick={onUnpublish}>
              <HugeiconsIcon icon={Globe02Icon} size={16} className="text-muted-foreground" />
              Unpublish from marketplace
            </DropdownMenuItem>
          ) : (
            <DropdownMenuItem onClick={onPublish}>
              <HugeiconsIcon icon={Globe02Icon} size={16} className="text-muted-foreground" />
              Publish to marketplace
            </DropdownMenuItem>
          )}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem variant="destructive" onClick={onDelete}>
            <HugeiconsIcon icon={Delete02Icon} size={16} />
            Archive
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
