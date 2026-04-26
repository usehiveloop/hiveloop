"use client"

import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { Skeleton } from "@/components/ui/skeleton"
import { Button } from "@/components/ui/button"
import type { components } from "@/lib/api/schema"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  BookOpen01Icon,
  Add01Icon,
  ArrowRight01Icon,
  GitBranchIcon,
  File01Icon,
  Globe02Icon,
} from "@hugeicons/core-free-icons"
import { SkillActions } from "./skill-actions"

type SkillRow = components["schemas"]["skillResponse"]

function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

function SkillHydrationBadge({ skill }: { skill: SkillRow }) {
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

function PublishedIndicator() {
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

interface SkillsListProps {
  skills: SkillRow[]
  isLoading: boolean
  onCreate: () => void
  onView: (skill: SkillRow) => void
  onDelete: (skill: SkillRow) => void
  onPublish: (skill: SkillRow) => void
  onUnpublish: (skill: SkillRow) => void
}

export function SkillsList({
  skills,
  isLoading,
  onCreate,
  onView,
  onDelete,
  onPublish,
  onUnpublish,
}: SkillsListProps) {
  return (
    <>
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium text-foreground">Skills</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Reusable instruction bundles your agents can invoke on demand.
          </p>
        </div>
        <Button size="sm" variant="secondary" onClick={onCreate}>
          <HugeiconsIcon icon={Add01Icon} size={14} data-icon="inline-start" />
          Add skill
        </Button>
      </div>

      <div className="flex flex-col gap-2">
        {isLoading ? (
          Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="h-[52px] w-full rounded-xl" />
          ))
        ) : skills.length === 0 ? (
          <div className="flex flex-col items-center py-14">
            <div className="text-center mb-6">
              <h2 className="font-heading text-lg font-semibold text-foreground">No skills yet</h2>
              <p className="text-sm text-muted-foreground mt-1.5 max-w-xs">
                Create a skill to give your agents reusable capabilities.
              </p>
            </div>
            <div className="w-full max-w-sm">
              <button
                type="button"
                onClick={onCreate}
                className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
              >
                <HugeiconsIcon icon={BookOpen01Icon} size={20} className="shrink-0 mt-0.5 text-muted-foreground" />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-semibold text-foreground">Create a skill</p>
                  <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">
                    Write inline instructions or sync from a Git repository.
                  </p>
                </div>
                <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
              </button>
            </div>
          </div>
        ) : (
          <>
            <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
              <span className="flex-1 min-w-0">Name</span>
              <span className="w-20 shrink-0 text-right">Source</span>
              <span className="w-20 shrink-0 text-right">Status</span>
              <span className="w-28 shrink-0 text-right">Created</span>
              <span className="w-8 shrink-0" />
            </div>

            {skills.map((skill) => (
              <div key={skill.id}>
                {/* Desktop row */}
                <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary cursor-pointer" onClick={() => onView(skill)}>
                  <div className="flex items-center gap-3 flex-1 min-w-0">
                    <HugeiconsIcon
                      icon={skill.source_type === "git" ? GitBranchIcon : File01Icon}
                      size={16}
                      className="shrink-0 text-muted-foreground"
                    />
                    <div className="min-w-0">
                      <div className="flex items-center gap-1.5">
                        <p className="text-sm font-medium text-foreground truncate">{skill.name}</p>
                        {skill.public_skill_id && <PublishedIndicator />}
                      </div>
                      {skill.description && (
                        <p className="text-xs text-muted-foreground truncate max-w-[280px]">{skill.description}</p>
                      )}
                    </div>
                  </div>
                  <span className="w-20 shrink-0 text-right">
                    <Badge variant="secondary" className="text-[10px]">
                      {skill.source_type === "git" ? "git" : "inline"}
                    </Badge>
                  </span>
                  <span className="w-20 shrink-0 text-right">
                    <SkillHydrationBadge skill={skill} />
                  </span>
                  <span className="w-28 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                    {skill.created_at ? formatDate(skill.created_at) : "\u2014"}
                  </span>
                  <div className="w-8 shrink-0 flex justify-center">
                    <SkillActions
                      skill={skill}
                      onDelete={() => onDelete(skill)}
                      onPublish={() => onPublish(skill)}
                      onUnpublish={() => onUnpublish(skill)}
                    />
                  </div>
                </div>

                {/* Mobile row */}
                <div className="flex md:hidden flex-col gap-3 rounded-xl border border-border p-4 transition-colors hover:border-primary cursor-pointer" onClick={() => onView(skill)}>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3 min-w-0 flex-1">
                      <HugeiconsIcon
                        icon={skill.source_type === "git" ? GitBranchIcon : File01Icon}
                        size={16}
                        className="shrink-0 text-muted-foreground"
                      />
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5">
                          <p className="text-sm font-medium text-foreground truncate">{skill.name}</p>
                          {skill.public_skill_id && <PublishedIndicator />}
                        </div>
                        {skill.description && (
                          <p className="text-xs text-muted-foreground truncate max-w-[280px]">{skill.description}</p>
                        )}
                      </div>
                    </div>
                    <SkillActions
                      skill={skill}
                      onDelete={() => onDelete(skill)}
                      onPublish={() => onPublish(skill)}
                      onUnpublish={() => onUnpublish(skill)}
                    />
                  </div>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground font-mono tabular-nums">
                    <Badge variant="secondary" className="text-[10px]">
                      {skill.source_type === "git" ? "git" : "inline"}
                    </Badge>
                    <SkillHydrationBadge skill={skill} />
                    <span>{skill.created_at ? formatDate(skill.created_at) : "\u2014"}</span>
                  </div>
                </div>
              </div>
            ))}
          </>
        )}
      </div>
    </>
  )
}
