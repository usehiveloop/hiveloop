"use client"

import { useMemo, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, Search01Icon, Cancel01Icon, Database02Icon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { useCreateAgent } from "./context"
import { MOCK_SKILLS } from "./mock-skills"
import { SkillCard } from "./skill-card"

type ScopeTab = "all" | "public" | "org"

const SCOPE_TABS: { value: ScopeTab; label: string }[] = [
  { value: "all", label: "All" },
  { value: "public", label: "Public" },
  { value: "org", label: "Your org" },
]

export function StepSkills() {
  const { mode, selectedSkillIds, toggleSkill, clearSkills, goTo } = useCreateAgent()
  const [search, setSearch] = useState("")
  const [scope, setScope] = useState<ScopeTab>("all")

  const filtered = useMemo(() => {
    let result = MOCK_SKILLS

    if (scope !== "all") {
      result = result.filter((skill) => skill.scope === scope)
    }

    const query = search.trim().toLowerCase()
    if (query) {
      result = result.filter(
        (skill) =>
          skill.name.toLowerCase().includes(query) ||
          skill.description.toLowerCase().includes(query) ||
          skill.tags.some((tag) => tag.toLowerCase().includes(query)),
      )
    }

    return [...result].sort((firstSkill, secondSkill) => {
      const firstSelected = selectedSkillIds.has(firstSkill.id) ? 0 : 1
      const secondSelected = selectedSkillIds.has(secondSkill.id) ? 0 : 1
      if (firstSelected !== secondSelected) return firstSelected - secondSelected

      const firstFeatured = firstSkill.featured ? 0 : 1
      const secondFeatured = secondSkill.featured ? 0 : 1
      if (firstFeatured !== secondFeatured) return firstFeatured - secondFeatured

      return secondSkill.installCount - firstSkill.installCount
    })
  }, [scope, search, selectedSkillIds])

  const selectedCount = selectedSkillIds.size
  const backTarget = mode === "forge" ? "forge-judge" : "instructions"

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => goTo(backTarget)}
            className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1"
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Attach skills</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Skills are reusable instructions your agent can invoke on demand. Pick as many as you like — your agent only loads them when needed.
        </DialogDescription>
      </DialogHeader>

      <div className="relative mt-4">
        <HugeiconsIcon icon={Search01Icon} size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="Search skills..."
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          className="pl-9 h-9"
        />
      </div>

      <div className="flex items-center gap-1 mt-3">
        {SCOPE_TABS.map((tab) => {
          const active = scope === tab.value
          return (
            <button
              key={tab.value}
              type="button"
              onClick={() => setScope(tab.value)}
              className={`text-xs font-medium px-3 py-1.5 rounded-full transition-colors cursor-pointer ${
                active
                  ? "bg-foreground text-background"
                  : "bg-muted/60 text-muted-foreground hover:bg-muted"
              }`}
            >
              {tab.label}
            </button>
          )
        })}
        {selectedCount > 0 && (
          <button
            type="button"
            onClick={clearSkills}
            className="ml-auto flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
          >
            <HugeiconsIcon icon={Cancel01Icon} size={12} />
            Clear {selectedCount}
          </button>
        )}
      </div>

      <div className="flex flex-col gap-2 mt-3 flex-1 overflow-y-auto pr-1">
        {filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-3">
            <div className="flex items-center justify-center size-12 rounded-full bg-muted">
              <HugeiconsIcon icon={Database02Icon} size={20} className="text-muted-foreground" />
            </div>
            <div className="text-center">
              <p className="text-sm font-medium text-foreground">No skills found</p>
              <p className="text-xs text-muted-foreground mt-1 max-w-[260px]">
                {search ? "Try a different search term or switch scopes." : "No skills available in this scope yet."}
              </p>
            </div>
          </div>
        ) : (
          filtered.map((skill) => (
            <SkillCard
              key={skill.id}
              skill={skill}
              selected={selectedSkillIds.has(skill.id)}
              onToggle={toggleSkill}
            />
          ))
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={() => goTo("summary")} className="w-full">
          {selectedCount > 0 ? `Continue with ${selectedCount} skill${selectedCount > 1 ? "s" : ""}` : "Skip for now"}
        </Button>
      </div>
    </div>
  )
}
