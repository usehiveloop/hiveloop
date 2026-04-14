"use client"

import { useEffect, useMemo, useRef, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, Search01Icon, Cancel01Icon, Database02Icon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import { useCreateAgent } from "./context"
import { SkillCard } from "./skill-card"
import type { SkillPreview } from "./types"

type ScopeTab = "all" | "public" | "own"

const SCOPE_TABS: { value: ScopeTab; label: string }[] = [
  { value: "all", label: "All" },
  { value: "public", label: "Public" },
  { value: "own", label: "Your org" },
]

const PAGE_SIZE = 24

type SkillResponse = components["schemas"]["skillResponse"]

function toSkillPreview(skill: SkillResponse): SkillPreview | null {
  if (!skill.id || !skill.name || !skill.slug) return null
  return {
    id: skill.id,
    slug: skill.slug,
    name: skill.name,
    description: skill.description ?? "",
    sourceType: skill.source_type === "git" ? "git" : "inline",
    scope: skill.org_id ? "org" : "public",
    tags: skill.tags ?? [],
    installCount: skill.install_count ?? 0,
    featured: skill.featured ?? false,
  }
}

export function StepSkills() {
  const { mode, selectedSkills, toggleSkill, clearSkills, goTo } = useCreateAgent()
  const [searchInput, setSearchInput] = useState("")
  const [debouncedSearch, setDebouncedSearch] = useState("")
  const [scope, setScope] = useState<ScopeTab>("all")
  const loadMoreRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handle = setTimeout(() => setDebouncedSearch(searchInput.trim()), 300)
    return () => clearTimeout(handle)
  }, [searchInput])

  const queryParams = useMemo(
    () => ({
      params: {
        query: {
          scope,
          q: debouncedSearch || undefined,
          limit: PAGE_SIZE,
        },
      },
    }),
    [scope, debouncedSearch],
  )

  const {
    data,
    isLoading,
    isFetchingNextPage,
    hasNextPage,
    fetchNextPage,
  } = $api.useInfiniteQuery(
    "get",
    "/v1/skills",
    queryParams,
    {
      pageParamName: "cursor",
      initialPageParam: undefined,
      getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
    },
  )

  const skills = useMemo(() => {
    const pages = data?.pages ?? []
    const seen = new Set<string>()
    const rows: SkillPreview[] = []
    for (const page of pages) {
      for (const raw of page.data ?? []) {
        const preview = toSkillPreview(raw)
        if (!preview || seen.has(preview.id)) continue
        seen.add(preview.id)
        rows.push(preview)
      }
    }
    return rows.sort((first, second) => {
      const firstSelected = selectedSkills.has(first.id) ? 0 : 1
      const secondSelected = selectedSkills.has(second.id) ? 0 : 1
      if (firstSelected !== secondSelected) return firstSelected - secondSelected
      const firstFeatured = first.featured ? 0 : 1
      const secondFeatured = second.featured ? 0 : 1
      if (firstFeatured !== secondFeatured) return firstFeatured - secondFeatured
      return second.installCount - first.installCount
    })
  }, [data, selectedSkills])

  useEffect(() => {
    const el = loadMoreRef.current
    if (!el || !hasNextPage) return
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && !isFetchingNextPage) {
          fetchNextPage()
        }
      },
      { rootMargin: "120px" },
    )
    observer.observe(el)
    return () => observer.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])

  const selectedCount = selectedSkills.size
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
          value={searchInput}
          onChange={(event) => setSearchInput(event.target.value)}
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
        {isLoading ? (
          Array.from({ length: 5 }).map((_, index) => (
            <Skeleton key={index} className="h-[88px] w-full rounded-xl" />
          ))
        ) : skills.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-3">
            <div className="flex items-center justify-center size-12 rounded-full bg-muted">
              <HugeiconsIcon icon={Database02Icon} size={20} className="text-muted-foreground" />
            </div>
            <div className="text-center">
              <p className="text-sm font-medium text-foreground">No skills found</p>
              <p className="text-xs text-muted-foreground mt-1 max-w-[260px]">
                {debouncedSearch ? "Try a different search term or switch scopes." : "No skills available in this scope yet."}
              </p>
            </div>
          </div>
        ) : (
          <>
            {skills.map((skill) => (
              <SkillCard
                key={skill.id}
                skill={skill}
                selected={selectedSkills.has(skill.id)}
                onToggle={() => toggleSkill(skill)}
              />
            ))}
            {hasNextPage && (
              <div ref={loadMoreRef} className="py-4 flex items-center justify-center">
                {isFetchingNextPage ? (
                  <Skeleton className="h-[88px] w-full rounded-xl" />
                ) : (
                  <span className="text-xs text-muted-foreground">Load more</span>
                )}
              </div>
            )}
          </>
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={() => goTo("subagents")} className="w-full">
          {selectedCount > 0 ? `Continue with ${selectedCount} skill${selectedCount > 1 ? "s" : ""}` : "Skip for now"}
        </Button>
      </div>
    </div>
  )
}
