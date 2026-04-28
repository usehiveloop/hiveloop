"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { IntegrationLogo } from "@/components/integration-logo"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { SearchPanel } from "./_components/search-panel"
import { SourceActions } from "./_components/source-actions"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  BookOpen01Icon,
  Books02Icon,
  CheckmarkBadge01Icon,
  ConnectIcon,
  File01Icon,
  Globe02Icon,
  Loading03Icon,
  TextFontIcon,
} from "@hugeicons/core-free-icons"
import { AddConnectionDialog } from "./_components/add-connection-dialog"
import { AddWebsiteDialog } from "./_components/add-website-dialog"
import type { components } from "@/lib/api/schema"

type RagSource = components["schemas"]["ragSourceResponse"]

function isSourceIndexing(s: RagSource): boolean {
  return (
    s.latest_attempt?.status === "in_progress" ||
    s.status === "INITIAL_INDEXING"
  )
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
  const [addWebsiteOpen, setAddWebsiteOpen] = useState(false)
  const [deleting, setDeleting] = useState<RagSource | null>(null)
  const queryClient = useQueryClient()
  const { data: sourcesData, isLoading: sourcesLoading } = $api.useQuery(
    "get",
    "/v1/rag/sources",
    {},
    {
      refetchInterval: (query) => {
        const data = query.state.data as { data?: RagSource[] } | undefined
        const anyRunning = data?.data?.some(isSourceIndexing) ?? false
        return anyRunning ? 3000 : false
      },
    },
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
    { icon: Globe02Icon, label: "Add URL", onClick: () => setAddWebsiteOpen(true) },
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

        <SearchPanel />

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
              <span className="w-20 shrink-0">Docs</span>
              <span className="w-10 shrink-0 text-center">Status</span>
              <span className="w-20 shrink-0 text-right">Updated</span>
              <span className="w-8 shrink-0" />
            </div>

            {sources.map((s) => {
              const provider = s.in_connection_id
                ? providerByConnId.get(s.in_connection_id) ?? ""
                : ""
              const attempt = s.latest_attempt
              const indexed = attempt?.total_docs_indexed ?? s.total_docs_indexed ?? 0
              const isRunning = isSourceIndexing(s)
              const errorMsg =
                attempt?.status === "failed"
                  ? attempt?.error_msg ?? "Indexing failed"
                  : s.status === "ERROR"
                    ? "Source in error state"
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
                  <div className="w-20 shrink-0 text-sm text-muted-foreground">
                    {indexed} {indexed === 1 ? "doc" : "docs"}
                  </div>
                  <div className="flex w-10 shrink-0 justify-center">
                    {isRunning ? (
                      <Tooltip>
                        <TooltipTrigger render={<span className="inline-flex" />}>
                          <span className="inline-flex">
                            <HugeiconsIcon
                              icon={Loading03Icon}
                              size={18}
                              className="animate-spin text-muted-foreground"
                            />
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>Indexing…</TooltipContent>
                      </Tooltip>
                    ) : errorMsg ? (
                      <Tooltip>
                        <TooltipTrigger render={<span className="inline-flex" />}>
                          <span className="inline-flex">
                            <HugeiconsIcon
                              icon={Alert02Icon}
                              size={18}
                              className="text-destructive"
                            />
                          </span>
                        </TooltipTrigger>
                        <TooltipContent className="max-w-xs whitespace-pre-wrap">
                          {errorMsg}
                        </TooltipContent>
                      </Tooltip>
                    ) : (
                      <Tooltip>
                        <TooltipTrigger render={<span className="inline-flex" />}>
                          <span className="inline-flex">
                            <HugeiconsIcon
                              icon={CheckmarkBadge01Icon}
                              size={18}
                              className="text-emerald-500"
                            />
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>Indexed</TooltipContent>
                      </Tooltip>
                    )}
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

      <AddWebsiteDialog
        open={addWebsiteOpen}
        onOpenChange={setAddWebsiteOpen}
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
