"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import { CheckmarkCircle02Icon, GithubIcon, FileEditIcon, StarIcon } from "@hugeicons/core-free-icons"
import type { SkillPreview } from "./types"

interface SkillCardProps {
  skill: SkillPreview
  selected: boolean
  onToggle: (id: string) => void
}

export function SkillCard({ skill, selected, onToggle }: SkillCardProps) {
  return (
    <button
      type="button"
      onClick={() => onToggle(skill.id)}
      className={`group flex items-start gap-3 w-full rounded-xl p-4 text-left transition-colors cursor-pointer ${
        selected
          ? "bg-primary/5 border border-primary/20"
          : "bg-muted/50 hover:bg-muted border border-transparent"
      }`}
    >
      <div className={`flex items-center justify-center size-8 rounded-lg shrink-0 ${
        skill.sourceType === "git" ? "bg-foreground/5" : "bg-blue-500/10"
      }`}>
        <HugeiconsIcon
          icon={skill.sourceType === "git" ? GithubIcon : FileEditIcon}
          size={16}
          className={skill.sourceType === "git" ? "text-foreground" : "text-blue-500"}
        />
      </div>

      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 min-w-0">
          <p className="text-sm font-semibold text-foreground truncate">{skill.name}</p>
          {skill.featured && (
            <HugeiconsIcon icon={StarIcon} size={12} className="text-amber-500 shrink-0" />
          )}
        </div>
        <p className="text-[13px] text-muted-foreground mt-0.5 line-clamp-2">{skill.description}</p>

        <div className="flex items-center gap-1.5 mt-2 flex-wrap">
          <ScopeBadge scope={skill.scope} />
          {skill.tags.slice(0, 3).map((tag) => (
            <span
              key={tag}
              className="text-[10px] font-medium text-muted-foreground bg-background/60 border border-border/60 rounded-full px-1.5 py-0.5"
            >
              {tag}
            </span>
          ))}
        </div>
      </div>

      <div className="shrink-0 mt-0.5">
        {selected ? (
          <HugeiconsIcon icon={CheckmarkCircle02Icon} size={18} className="text-primary" />
        ) : (
          <div className="size-[18px] rounded-full border border-muted-foreground/30 group-hover:border-muted-foreground/50 transition-colors" />
        )}
      </div>
    </button>
  )
}

interface ScopeBadgeProps {
  scope: SkillPreview["scope"]
}

function ScopeBadge({ scope }: ScopeBadgeProps) {
  if (scope === "public") {
    return (
      <span className="text-[10px] font-medium uppercase tracking-wide text-emerald-600 bg-emerald-500/10 rounded-full px-1.5 py-0.5">
        Public
      </span>
    )
  }
  return (
    <span className="text-[10px] font-medium uppercase tracking-wide text-blue-600 bg-blue-500/10 rounded-full px-1.5 py-0.5">
      Your org
    </span>
  )
}
