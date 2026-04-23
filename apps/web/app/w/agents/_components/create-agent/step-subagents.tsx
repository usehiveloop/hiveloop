"use client"

import { useEffect, useMemo, useRef, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, Search01Icon, Cancel01Icon, ArtificialIntelligence01Icon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import { useCreateAgent } from "./context"
import { SubagentCard } from "./subagent-card"
import type { SubagentPreview } from "./types"

const PAGE_SIZE = 24

type SubagentResponse = components["schemas"]["subagentResponse"]

function toSubagentPreview(subagent: SubagentResponse): SubagentPreview | null {
  if (!subagent.id || !subagent.name) return null
  return {
    id: subagent.id,
    name: subagent.name,
    description: subagent.description ?? "",
    model: subagent.model ?? "",
    scope: subagent.org_id ? "org" : "public",
  }
}

export function StepSubagents() {
  const { selectedSubagents, toggleSubagent, clearSubagents, goTo } = useCreateAgent()
  const [searchInput, setSearchInput] = useState("")
  const [debouncedSearch, setDebouncedSearch] = useState("")
  const loadMoreRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handle = setTimeout(() => setDebouncedSearch(searchInput.trim()), 300)
    return () => clearTimeout(handle)
  }, [searchInput])

  const queryParams = useMemo(
    () => ({
      params: {
        query: {
          q: debouncedSearch || undefined,
          limit: PAGE_SIZE,
        },
      },
    }),
    [debouncedSearch],
  )

  const {
    data,
    isLoading,
    isFetchingNextPage,
    hasNextPage,
    fetchNextPage,
  } = $api.useInfiniteQuery(
    "get",
    "/v1/subagents",
    queryParams,
    {
      pageParamName: "cursor",
      initialPageParam: undefined,
      getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined,
    },
  )

  const subagents = useMemo(() => {
    const pages = data?.pages ?? []
    const seen = new Set<string>()
    const rows: SubagentPreview[] = []
    for (const page of pages) {
      for (const raw of page.data ?? []) {
        const preview = toSubagentPreview(raw)
        if (!preview || seen.has(preview.id)) continue
        seen.add(preview.id)
        rows.push(preview)
      }
    }
    return rows.sort((first, second) => {
      const firstSelected = selectedSubagents.has(first.id) ? 0 : 1
      const secondSelected = selectedSubagents.has(second.id) ? 0 : 1
      if (firstSelected !== secondSelected) return firstSelected - secondSelected
      return first.name.localeCompare(second.name)
    })
  }, [data, selectedSubagents])

  useEffect(() => {
    const element = loadMoreRef.current
    if (!element || !hasNextPage) return
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && !isFetchingNextPage) {
          fetchNextPage()
        }
      },
      { rootMargin: "120px" },
    )
    observer.observe(element)
    return () => observer.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])

  const selectedCount = selectedSubagents.size

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => goTo("skills")}
            className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1"
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Attach sub-agents</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Sub-agents are specialized agents your agent can delegate tasks to. Pick as many as you need.
        </DialogDescription>
      </DialogHeader>

      <div className="relative mt-4">
        <HugeiconsIcon icon={Search01Icon} size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="Search sub-agents..."
          value={searchInput}
          onChange={(event) => setSearchInput(event.target.value)}
          className="pl-9 h-9"
        />
      </div>

      <div className="flex items-center gap-1 mt-3">
        {selectedCount > 0 && (
          <button
            type="button"
            onClick={clearSubagents}
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
            <Skeleton key={index} className="h-22 w-full rounded-xl" />
          ))
        ) : subagents.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-3">
            <div className="flex items-center justify-center size-12 rounded-full bg-muted">
              <HugeiconsIcon icon={ArtificialIntelligence01Icon} size={20} className="text-muted-foreground" />
            </div>
            <div className="text-center">
              <p className="text-sm font-medium text-foreground">No sub-agents found</p>
              <p className="text-xs text-muted-foreground mt-1 max-w-[260px]">
                {debouncedSearch ? "Try a different search term." : "No sub-agents available yet."}
              </p>
            </div>
          </div>
        ) : (
          <>
            {subagents.map((subagent) => (
              <SubagentCard
                key={subagent.id}
                subagent={subagent}
                selected={selectedSubagents.has(subagent.id)}
                onToggle={() => toggleSubagent(subagent)}
              />
            ))}
            {hasNextPage && (
              <div ref={loadMoreRef} className="py-4 flex items-center justify-center">
                {isFetchingNextPage ? (
                  <Skeleton className="h-22 w-full rounded-xl" />
                ) : (
                  <span className="text-xs text-muted-foreground">Load more</span>
                )}
              </div>
            )}
          </>
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={() => goTo("summary")} className="w-full">
          {selectedCount > 0 ? `Continue with ${selectedCount} sub-agent${selectedCount > 1 ? "s" : ""}` : "Skip for now"}
        </Button>
      </div>
    </div>
  )
}
