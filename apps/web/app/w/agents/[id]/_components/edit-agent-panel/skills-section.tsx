"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import { Tick02Icon } from "@hugeicons/core-free-icons"
import { $api } from "@/lib/api/hooks"
import { Skeleton } from "@/components/ui/skeleton"

interface SkillsSectionProps {
  skillIds: Set<string>
  onToggle: (skillId: string) => void
}

export function SkillsSection({ skillIds, onToggle }: SkillsSectionProps) {
  const { data, isLoading } = $api.useQuery("get", "/v1/skills")
  const skills = data?.data ?? []

  return (
    <section className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <h3 className="text-sm font-medium text-foreground">Skills</h3>
        <p className="text-xs text-muted-foreground">
          Attach skills that give your agent specialized capabilities.
        </p>
      </div>

      <div className="flex flex-col gap-2">
        {isLoading ? (
          Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="h-13 w-full rounded-xl" />
          ))
        ) : skills.length === 0 ? (
          <p className="text-sm text-muted-foreground">No skills available.</p>
        ) : (
          skills.map((skill) => {
            const selected = skillIds.has(skill.id ?? "")
            return (
              <button
                key={skill.id}
                type="button"
                onClick={() => onToggle(skill.id ?? "")}
                className={`flex items-center justify-between rounded-xl border px-4 py-3 text-left transition-colors ${
                  selected
                    ? "border-primary bg-primary/5"
                    : "border-border bg-muted/50 hover:bg-muted"
                }`}
              >
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-foreground">{skill.name}</p>
                  {skill.description && (
                    <p className="text-xs text-muted-foreground mt-0.5 line-clamp-1">{skill.description}</p>
                  )}
                </div>
                {selected && (
                  <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0 ml-2" />
                )}
              </button>
            )
          })
        )}
      </div>
    </section>
  )
}
