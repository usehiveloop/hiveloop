"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import { CheckmarkCircle02Icon, ArtificialIntelligence01Icon } from "@hugeicons/core-free-icons"
import type { SubagentPreview } from "./types"

interface SubagentCardProps {
  subagent: SubagentPreview
  selected: boolean
  onToggle: () => void
}

export function SubagentCard({ subagent, selected, onToggle }: SubagentCardProps) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className={`group flex items-start gap-3 w-full rounded-xl p-4 text-left transition-colors cursor-pointer ${
        selected
          ? "bg-primary/5 border border-primary/20"
          : "bg-muted/50 hover:bg-muted border border-transparent"
      }`}
    >
      <div className="flex items-center justify-center size-8 rounded-lg shrink-0 bg-violet-500/10">
        <HugeiconsIcon
          icon={ArtificialIntelligence01Icon}
          size={16}
          className="text-violet-500"
        />
      </div>

      <div className="flex-1 min-w-0">
        <p className="text-sm font-semibold text-foreground truncate">{subagent.name}</p>
        <p className="text-sm-alt text-muted-foreground mt-0.5 line-clamp-2">{subagent.description}</p>

        <div className="flex items-center gap-1.5 mt-2 flex-wrap">
          <ScopeBadge scope={subagent.scope} />
          {subagent.model && (
            <span className="text-2xs font-medium text-muted-foreground bg-background/60 border border-border/60 rounded-full px-1.5 py-0.5">
              {subagent.model}
            </span>
          )}
        </div>
      </div>

      <div className="shrink-0 mt-0.5">
        {selected ? (
          <HugeiconsIcon icon={CheckmarkCircle02Icon} size={18} className="text-primary" />
        ) : (
          <div className="size-4.5 rounded-full border border-muted-foreground/30 group-hover:border-muted-foreground/50 transition-colors" />
        )}
      </div>
    </button>
  )
}

function ScopeBadge({ scope }: { scope: SubagentPreview["scope"] }) {
  if (scope === "public") {
    return (
      <span className="text-2xs font-medium uppercase tracking-wide text-emerald-600 bg-emerald-500/10 rounded-full px-1.5 py-0.5">
        Public
      </span>
    )
  }
  return (
    <span className="text-2xs font-medium uppercase tracking-wide text-blue-600 bg-blue-500/10 rounded-full px-1.5 py-0.5">
      Your org
    </span>
  )
}
