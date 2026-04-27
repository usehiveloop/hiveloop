"use client"

import { useState } from "react"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Checkbox } from "@/components/ui/checkbox"
import { HugeiconsIcon } from "@hugeicons/react"
import { Search01Icon } from "@hugeicons/core-free-icons"
import type { components } from "@/lib/api/schema"

type Hit = components["schemas"]["ragSearchHit"]

export function SearchPanel() {
  const [query, setQuery] = useState("")
  const [rerank, setRerank] = useState(false)
  const [bypassACL, setBypassACL] = useState(true)
  const [hits, setHits] = useState<Hit[]>([])
  const search = $api.useMutation("post", "/v1/rag/search")

  function handleSearch() {
    const q = query.trim()
    if (!q || search.isPending) return
    search.mutate(
      { body: { query: q, rerank, limit: 10, bypass_acl: bypassACL } },
      {
        onSuccess: (data) => {
          setHits(data?.hits ?? [])
          if ((data?.hits ?? []).length === 0) {
            toast.message("No results")
          }
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Search failed"))
        },
      },
    )
  }

  return (
    <div className="flex flex-col gap-3 rounded-xl border border-border p-4">
      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <HugeiconsIcon
            icon={Search01Icon}
            size={16}
            className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleSearch()
            }}
            placeholder="Search the knowledge base..."
            className="pl-9"
          />
        </div>
        <label className="flex items-center gap-2 text-sm text-muted-foreground">
          <Checkbox
            checked={rerank}
            onCheckedChange={(v) => setRerank(Boolean(v))}
          />
          Rerank
        </label>
        <label className="flex items-center gap-2 text-sm text-muted-foreground">
          <Checkbox
            checked={bypassACL}
            onCheckedChange={(v) => setBypassACL(Boolean(v))}
          />
          Bypass ACL
        </label>
        <Button onClick={handleSearch} loading={search.isPending}>
          Search
        </Button>
      </div>

      {hits.length > 0 ? (
        <div className="flex flex-col gap-2">
          {hits.map((hit) => (
            <div
              key={hit.id}
              className="flex flex-col gap-1 rounded-lg border border-border bg-muted/30 p-3"
            >
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <span className="font-mono">{hit.doc_id}</span>
                <span className="ml-auto flex items-center gap-2">
                  <span>score {hit.score?.toFixed(3)}</span>
                  {rerank && hit.rerank_score !== undefined ? (
                    <span>· rerank {hit.rerank_score.toFixed(3)}</span>
                  ) : null}
                </span>
              </div>
              {hit.blurb ? (
                <p className="text-sm text-foreground">{hit.blurb}</p>
              ) : null}
              {hit.content ? (
                <details className="text-xs text-muted-foreground">
                  <summary className="cursor-pointer">Full chunk</summary>
                  <pre className="mt-2 whitespace-pre-wrap font-mono text-[11px]">
                    {hit.content}
                  </pre>
                </details>
              ) : null}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}
