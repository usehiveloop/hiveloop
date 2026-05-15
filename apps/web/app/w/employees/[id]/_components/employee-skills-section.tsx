import { HugeiconsIcon } from "@hugeicons/react"
import { Tick02Icon } from "@hugeicons/core-free-icons"
import { FormEmptyWell, FormSection } from "@/app/w/_components/form-section"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import { ListRowsSkeleton } from "./list-rows-skeleton"
import type { components } from "@/lib/api/schema"

type Skill = components["schemas"]["skillResponse"]

export function EmployeeSkillsSection({
  skills,
  loading,
  selectedIDs,
  lockedIDs,
  onToggle,
}: {
  skills: Skill[]
  loading: boolean
  selectedIDs: Set<string>
  lockedIDs: Set<string>
  onToggle: (id: string) => void
}) {
  return (
    <FormSection
      title="Skills"
      description="Required employee skills stay attached. Optional skills can be toggled."
    >
      {loading ? (
        <ListRowsSkeleton />
      ) : skills.length === 0 ? (
        <FormEmptyWell message="No skills are available." />
      ) : (
        <div className="grid gap-2">
          {skills.map((skill) => {
            if (!skill.id) return null
            const selected = selectedIDs.has(skill.id)
            const locked = lockedIDs.has(skill.id)
            return (
              <button
                key={skill.id}
                type="button"
                disabled={locked}
                onClick={() => onToggle(skill.id!)}
                className={cn(
                  "flex items-center justify-between gap-3 rounded-xl border px-4 py-3 text-left transition-colors",
                  selected
                    ? "border-primary bg-primary/5"
                    : "border-border bg-muted/50 hover:bg-muted",
                  locked && "cursor-not-allowed opacity-75"
                )}
              >
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="text-sm font-medium text-foreground">
                      {skill.name}
                    </p>
                    {locked ? (
                      <Badge
                        variant="ghost"
                        className="bg-muted text-muted-foreground"
                      >
                        Required
                      </Badge>
                    ) : null}
                  </div>
                  {skill.description ? (
                    <p className="mt-0.5 line-clamp-1 text-xs text-muted-foreground">
                      {skill.description}
                    </p>
                  ) : null}
                </div>
                {selected ? (
                  <HugeiconsIcon
                    icon={Tick02Icon}
                    className="size-4 shrink-0 text-primary"
                    strokeWidth={2.5}
                  />
                ) : null}
              </button>
            )
          })}
        </div>
      )}
    </FormSection>
  )
}
