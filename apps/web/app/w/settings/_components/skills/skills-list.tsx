import { HugeiconsIcon } from "@hugeicons/react"
import {
  BookOpen01Icon,
  ArrowRight01Icon,
  GitBranchIcon,
  File01Icon,
} from "@hugeicons/core-free-icons"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { SkillActions } from "./skill-actions"
import { PublishedIndicator, SkillHydrationBadge } from "./skill-badges"
import { formatDate, type SkillRow } from "./types"

interface SkillsListProps {
  skills: SkillRow[]
  isLoading: boolean
  onOpenCreate: () => void
  onView: (skill: SkillRow) => void
  onDelete: (skill: SkillRow) => void
  onPublish: (skill: SkillRow) => void
  onUnpublish: (skill: SkillRow) => void
}

export function SkillsList({
  skills,
  isLoading,
  onOpenCreate,
  onView,
  onDelete,
  onPublish,
  onUnpublish,
}: SkillsListProps) {
  if (isLoading) {
    return (
      <>
        {Array.from({ length: 3 }).map((_, index) => (
          <Skeleton key={index} className="h-[52px] w-full rounded-xl" />
        ))}
      </>
    )
  }

  if (skills.length === 0) {
    return (
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
            onClick={onOpenCreate}
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
    )
  }

  return (
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
              {skill.created_at ? formatDate(skill.created_at) : "—"}
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
              <span>{skill.created_at ? formatDate(skill.created_at) : "—"}</span>
            </div>
          </div>
        </div>
      ))}
    </>
  )
}
