"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { IntegrationLogo } from "@/components/integration-logo"
import { SourceActions } from "./_components/source-actions"
import type { components } from "@/lib/api/schema"

type RagSource = components["schemas"]["ragSourceResponse"]
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  BookOpen01Icon,
  Books02Icon,
  ConnectIcon,
  File01Icon,
  Globe02Icon,
  Search01Icon,
  TextFontIcon,
} from "@hugeicons/core-free-icons"
import { AddConnectionDialog } from "./_components/add-connection-dialog"

const FILTERS = ["Type", "Creator"] as const

const STATUS_VARIANT: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  ACTIVE: "secondary",
  INITIAL_INDEXING: "default",
  PAUSED: "outline",
  ERROR: "destructive",
  DELETING: "destructive",
  DISCONNECTED: "destructive",
}

function formatRelative(iso: string): string {
  const then = new Date(iso).getTime()
  const diff = Math.max(0, Date.now() - then)
  const m = Math.floor(diff / 60_000)
  if (m < 1) return "just now"
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  return `${d}d ago`
}

export default function KnowledgePage() {
  const [addConnectionOpen, setAddConnectionOpen] = useState(false)
  const [deleting, setDeleting] = useState<RagSource | null>(null)
  const queryClient = useQueryClient()
  const { data: sourcesData, isLoading: sourcesLoading } = $api.useQuery(
    "get",
    "/v1/rag/sources",
  )
  const { data: connectionsData } = $api.useQuery(
    "get",
    "/v1/in/connections",
  )
  const triggerSync = $api.useMutation("post", "/v1/rag/sources/{id}/sync")
  const deleteSource = $api.useMutation("delete", "/v1/rag/sources/{id}")

  function handleTriggerRun(source: RagSource) {
    if (!source.id) return
    triggerSync.mutate(
      {
        params: { path: { id: source.id } },
        body: { from_beginning: true },
      },
      {
        onSuccess: (data) => {
          if (data?.deduplicated) {
            toast.message("Run already queued — skipped duplicate")
          } else {
            toast.success(`Triggered run for "${source.name}"`)
          }
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/rag/sources"] })
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to trigger run"))
        },
      },
    )
  }

  function handleDelete() {
    if (!deleting?.id) return
    deleteSource.mutate(
      { params: { path: { id: deleting.id } } },
      {
        onSuccess: () => {
          toast.success(`Removed "${deleting.name}"`)
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/rag/sources"] })
          setDeleting(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to remove source"))
          setDeleting(null)
        },
      },
    )
  }

  const providerByConnId = useMemo(() => {
    const m = new Map<string, string>()
    for (const c of connectionsData?.data ?? []) {
      if (c.id) m.set(c.id, c.provider ?? "")
    }
    return m
  }, [connectionsData])

  const sources = sourcesData?.data ?? []

  const actions = [
    {
      icon: ConnectIcon,
      label: "Add Connection",
      onClick: () => setAddConnectionOpen(true),
    },
    { icon: Globe02Icon, label: "Add URL", onClick: () => {} },
    { icon: File01Icon, label: "Add Files", onClick: () => {} },
    { icon: TextFontIcon, label: "Create Text", onClick: () => {} },
  ] as const

  return (
    <>
      <PageHeader title="Knowledge" />

      <div className="mx-auto w-full max-w-4xl space-y-6 px-6 py-10">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight text-foreground">
              Knowledge Base
            </h1>
            <HugeiconsIcon
              icon={BookOpen01Icon}
              size={20}
              className="text-muted-foreground"
            />
          </div>
          <Badge variant="outline" className="h-7 gap-2 px-3 text-sm font-normal">
            <span className="size-2 rounded-full bg-emerald-500" />
            <span className="text-muted-foreground">RAG Storage:</span>
            <span className="font-semibold text-foreground">0 B</span>
            <span className="text-muted-foreground">/ 1.0 MB</span>
          </Badge>
        </div>

        <div className="grid grid-cols-4 gap-3">
          {actions.map(({ icon, label, onClick }) => (
            <button
              key={label}
              type="button"
              onClick={onClick}
              className="flex flex-col items-start gap-3 rounded-xl border border-border bg-background p-4 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/30"
            >
              <HugeiconsIcon icon={icon} size={22} className="text-foreground" />
              <span className="text-sm font-medium text-foreground">
                {label}
              </span>
            </button>
          ))}
        </div>

        <div className="relative">
          <HugeiconsIcon
            icon={Search01Icon}
            size={16}
            className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <Input placeholder="Search Knowledge Base..." className="pl-9" />
        </div>

        <div className="flex items-center gap-2">
          {FILTERS.map((label) => (
            <Badge
              key={label}
              variant="outline"
              className="cursor-pointer gap-1 text-muted-foreground hover:bg-muted/50"
            >
              <HugeiconsIcon icon={Add01Icon} size={12} />
              {label}
            </Badge>
          ))}
        </div>

        {sourcesLoading ? (
          <div className="flex flex-col gap-2">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-14 w-full rounded-xl" />
            ))}
          </div>
        ) : sources.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-3 rounded-2xl border border-border bg-muted/40 px-6 py-16 text-center">
            <div className="flex size-12 items-center justify-center rounded-2xl border border-border bg-background">
              <HugeiconsIcon
                icon={Books02Icon}
                size={20}
                className="text-foreground"
              />
            </div>
            <div className="text-sm font-semibold text-foreground">
              No sources found
            </div>
            <div className="text-sm text-muted-foreground">
              Add a connection, URL, file, or text snippet to get started.
            </div>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            <div className="flex items-center gap-3 px-4 py-1 font-mono text-[10px] uppercase tracking-[1px] text-muted-foreground/50">
              <span className="min-w-0 flex-1">Source</span>
              <span className="w-32 shrink-0">Progress</span>
              <span className="w-24 shrink-0">Status</span>
              <span className="w-20 shrink-0 text-right">Updated</span>
              <span className="w-8 shrink-0" />
            </div>

            {sources.map((s) => {
              const provider = s.in_connection_id
                ? providerByConnId.get(s.in_connection_id) ?? ""
                : ""
              const attempt = s.latest_attempt
              const indexed = attempt?.total_docs_indexed ?? s.total_docs_indexed ?? 0
              const estimated = attempt?.docs_estimated ?? null
              const isRunning =
                attempt?.status === "in_progress" ||
                s.status === "INITIAL_INDEXING"
              const pct =
                estimated && estimated > 0
                  ? Math.min(100, Math.round((indexed / estimated) * 100))
                  : null
              return (
                <div
                  key={s.id}
                  className="flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary"
                >
                  <div className="flex min-w-0 flex-1 items-center gap-3">
                    {provider ? (
                      <IntegrationLogo provider={provider} size={24} />
                    ) : (
                      <div className="size-6 shrink-0 rounded-md bg-muted" />
                    )}
                    <span className="truncate text-sm font-medium text-foreground">
                      {s.name}
                    </span>
                  </div>
                  <div className="w-32 shrink-0">
                    {isRunning && pct !== null ? (
                      <div className="flex items-center gap-2">
                        <div className="h-1.5 w-16 overflow-hidden rounded-full bg-muted">
                          <div
                            className="h-full bg-primary transition-all"
                            style={{ width: `${pct}%` }}
                          />
                        </div>
                        <span className="text-xs text-muted-foreground">
                          {indexed}/{estimated}
                        </span>
                      </div>
                    ) : isRunning ? (
                      <span className="text-xs text-muted-foreground">
                        Indexing… ({indexed})
                      </span>
                    ) : (
                      <span className="text-sm text-muted-foreground">
                        {indexed} {indexed === 1 ? "doc" : "docs"}
                      </span>
                    )}
                  </div>
                  <div className="w-24 shrink-0">
                    <Badge variant={STATUS_VARIANT[s.status ?? ""] ?? "outline"}>
                      {s.status}
                    </Badge>
                  </div>
                  <div className="w-20 shrink-0 text-right text-xs text-muted-foreground">
                    {s.updated_at ? formatRelative(s.updated_at) : "—"}
                  </div>
                  <div className="flex w-8 shrink-0 justify-center">
                    <SourceActions
                      onTriggerRun={() => handleTriggerRun(s)}
                      onDelete={() => setDeleting(s)}
                    />
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      <AddConnectionDialog
        open={addConnectionOpen}
        onOpenChange={setAddConnectionOpen}
      />

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null)
        }}
        title="Remove source"
        description="This will remove the source and stop further indexing. Any documents already indexed will be deleted from the knowledge base. This action cannot be undone."
        confirmText={deleting?.name ?? ""}
        confirmLabel="Remove source"
        destructive
        loading={deleteSource.isPending}
        onConfirm={handleDelete}
      />
    </>
  )
}
