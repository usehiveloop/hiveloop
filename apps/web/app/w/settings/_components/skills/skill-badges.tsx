import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { HugeiconsIcon } from "@hugeicons/react"
import { Globe02Icon } from "@hugeicons/core-free-icons"
import type { SkillRow } from "./types"

export function SkillHydrationBadge({ skill }: { skill: SkillRow }) {
  const status = skill.hydration_status ?? "pending"

  if (status === "error") {
    return (
      <Tooltip>
        <TooltipTrigger className="cursor-default">
          <Badge variant="secondary" className="text-[10px] bg-red-500/10 text-red-600 dark:text-red-400 border-red-500/20">
            error
          </Badge>
        </TooltipTrigger>
        <TooltipContent className="max-w-xs">
          <p className="text-xs font-mono whitespace-pre-wrap">{skill.hydration_error ?? "Unknown error"}</p>
        </TooltipContent>
      </Tooltip>
    )
  }

  if (status === "pending") {
    return (
      <Badge variant="secondary" className="text-[10px] bg-yellow-500/10 text-yellow-600 dark:text-yellow-400 border-yellow-500/20">
        pending
      </Badge>
    )
  }

  return (
    <Badge variant="default" className="text-[10px] bg-green-500/10 text-green-600 dark:text-green-400 border-green-500/20">
      ready
    </Badge>
  )
}

export function PublishedIndicator() {
  return (
    <Tooltip>
      <TooltipTrigger className="cursor-default">
        <HugeiconsIcon icon={Globe02Icon} size={14} className="text-blue-500 shrink-0" />
      </TooltipTrigger>
      <TooltipContent>
        Published to marketplace
      </TooltipContent>
    </Tooltip>
  )
}
