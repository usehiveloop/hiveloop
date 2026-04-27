"use client"

import { useEffect, useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { IntegrationLogo } from "@/components/integration-logo"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Plug01Icon,
  Search01Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"

interface AddConnectionDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

interface ResourceItem {
  id: string
  name: string
}

interface ConfigurableResource {
  key: string
  display_name: string
  description: string
}

type Step = "connections" | "resources"

function getConfigurableResources(connection: unknown): ConfigurableResource[] {
  const raw = (connection as Record<string, unknown> | null)?.configurable_resources
  if (!Array.isArray(raw)) return []
  return raw as ConfigurableResource[]
}

function buildSourceConfig(provider: string, picks: ResourceItem[]): Record<string, unknown> {
  const lowered = provider.toLowerCase()
  if (lowered === "github" || lowered === "github-app") {
    // Resource ids are GitHub full_names ("owner/repo"); the connector
    // takes a single repo_owner + bare repo names.
    const owner = picks[0]?.id.split("/")[0] ?? ""
    return {
      repo_owner: owner,
      repositories: picks.map((p) => p.id.split("/").slice(1).join("/")),
      include_prs: true,
      include_issues: true,
    }
  }
  return {}
}

export function AddConnectionDialog({
  open,
  onOpenChange,
}: AddConnectionDialogProps) {
  const [step, setStep] = useState<Step>("connections")
  const [currentIdx, setCurrentIdx] = useState(0)
  const [search, setSearch] = useState("")
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [picks, setPicks] = useState<Map<string, ResourceItem[]>>(new Map())
  const queryClient = useQueryClient()
  const createSource = $api.useMutation("post", "/v1/rag/sources")

  useEffect(() => {
    if (open) {
      setStep("connections")
      setCurrentIdx(0)
      setSearch("")
      setSelected(new Set())
      setPicks(new Map())
    }
  }, [open])

  const { data: connectionsData, isLoading: connectionsLoading } = $api.useQuery(
    "get",
    "/v1/in/connections",
  )
  const { data: supportedData, isLoading: supportedLoading } = $api.useQuery(
    "get",
    "/v1/rag/integrations",
  )
  const isLoading = connectionsLoading || supportedLoading

  const supportedIntegrationIds = useMemo(
    () =>
      new Set(
        (supportedData?.data ?? []).map((i) => i.id).filter((id): id is string => !!id),
      ),
    [supportedData],
  )

  const connections = (connectionsData?.data ?? []).filter((c) => !c.revoked_at)
  const connectionsById = useMemo(
    () => new Map(connections.map((c) => [c.id ?? "", c])),
    [connections],
  )
  const orderedSelected = useMemo(
    () => connections.filter((c) => c.id && selected.has(c.id)),
    [connections, selected],
  )
  const currentConnection = orderedSelected[currentIdx]
  const currentResourceType = currentConnection
    ? getConfigurableResources(currentConnection)[0]?.key ?? ""
    : ""

  const filtered = useMemo(() => {
    if (!search.trim()) return connections
    const query = search.toLowerCase()
    return connections.filter(
      (c) =>
        (c.display_name ?? "").toLowerCase().includes(query) ||
        (c.provider ?? "").toLowerCase().includes(query),
    )
  }, [connections, search])

  function toggle(id: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function togglePick(connId: string, item: ResourceItem) {
    setPicks((prev) => {
      const next = new Map(prev)
      const current = next.get(connId) ?? []
      const idx = current.findIndex((p) => p.id === item.id)
      if (idx >= 0) {
        next.set(connId, current.filter((_, i) => i !== idx))
      } else {
        next.set(connId, [...current, item])
      }
      return next
    })
  }

  function startConfiguring() {
    if (selected.size === 0) return
    setStep("resources")
    setCurrentIdx(0)
  }

  function backFromResources() {
    if (currentIdx > 0) {
      setCurrentIdx(currentIdx - 1)
    } else {
      setStep("connections")
    }
  }

  async function submit() {
    if (createSource.isPending) return
    const results = await Promise.allSettled(
      orderedSelected.map((conn) => {
        const id = conn.id ?? ""
        const provider = conn.provider ?? ""
        const items = picks.get(id) ?? []
        return createSource.mutateAsync({
          body: {
            kind: "INTEGRATION",
            name: conn.display_name ?? provider ?? "Connection",
            in_connection_id: id,
            access_type: "private",
            config: buildSourceConfig(provider, items),
          },
        })
      }),
    )

    const succeeded = results.filter((r) => r.status === "fulfilled").length
    const failed = results.length - succeeded

    if (succeeded > 0) {
      await queryClient.invalidateQueries({
        queryKey: ["get", "/v1/rag/sources"],
      })
      toast.success(
        succeeded === 1
          ? "Connection added to Knowledge Base"
          : `${succeeded} connections added to Knowledge Base`,
      )
    }
    if (failed > 0) {
      const firstError = results.find((r) => r.status === "rejected") as
        | PromiseRejectedResult
        | undefined
      toast.error(
        extractErrorMessage(
          firstError?.reason,
          failed === 1
            ? "Failed to add connection"
            : `Failed to add ${failed} connection${failed > 1 ? "s" : ""}`,
        ),
      )
    }
    if (failed === 0) {
      onOpenChange(false)
    }
  }

  const selectedCount = selected.size
  const currentPicks = currentConnection
    ? picks.get(currentConnection.id ?? "") ?? []
    : []
  const isLastConnection = currentIdx === orderedSelected.length - 1

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex flex-col h-[min(780px,85vh)] p-6">
        {step === "connections" ? (
          <>
            <DialogHeader>
              <DialogTitle>Add connections</DialogTitle>
              <DialogDescription className="mt-2">
                Pick which connections to ingest into your Knowledge Base.
              </DialogDescription>
            </DialogHeader>

            <div className="relative mt-4">
              <HugeiconsIcon
                icon={Search01Icon}
                size={14}
                className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
              />
              <Input
                placeholder="Search connections..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-9 h-9"
              />
            </div>

            <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
              {isLoading ? (
                Array.from({ length: 4 }).map((_, i) => (
                  <Skeleton key={i} className="h-[64px] w-full rounded-xl" />
                ))
              ) : filtered.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-12 gap-3">
                  {search ? (
                    <p className="text-sm text-muted-foreground">
                      No connections found.
                    </p>
                  ) : (
                    <>
                      <div className="flex items-center justify-center size-12 rounded-full bg-muted">
                        <HugeiconsIcon
                          icon={Plug01Icon}
                          size={20}
                          className="text-muted-foreground"
                        />
                      </div>
                      <div className="text-center">
                        <p className="text-sm font-medium text-foreground">
                          No connections yet
                        </p>
                        <p className="text-xs text-muted-foreground mt-1 max-w-[240px]">
                          Head to the Connections page to connect your first
                          integration, then come back here.
                        </p>
                      </div>
                    </>
                  )}
                </div>
              ) : (
                filtered.map((connection) => {
                  const id = connection.id ?? ""
                  const integrationId = connection.in_integration_id ?? ""
                  const isSupported = supportedIntegrationIds.has(integrationId)
                  const isSelected = selected.has(id)
                  const isDisabled = !isSupported
                  return (
                    <button
                      key={id}
                      type="button"
                      onClick={() => !isDisabled && toggle(id)}
                      disabled={isDisabled}
                      title={isDisabled ? "This integration doesn't support knowledge ingestion yet." : undefined}
                      className={`group flex items-start gap-4 w-full rounded-xl p-4 text-left transition-colors ${
                        isDisabled
                          ? "bg-muted/30 border border-transparent opacity-50 cursor-not-allowed"
                          : isSelected
                          ? "bg-primary/5 border border-primary/20 cursor-pointer"
                          : "bg-muted/50 hover:bg-muted border border-transparent cursor-pointer"
                      }`}
                    >
                      <IntegrationLogo
                        provider={connection.provider ?? ""}
                        size={32}
                        className="shrink-0 mt-0.5"
                      />
                      <div className="flex-1 min-w-0">
                        <p className="text-sm font-semibold text-foreground">
                          {connection.display_name}
                        </p>
                        <p className="text-[13px] text-muted-foreground mt-0.5">
                          {isDisabled ? "Not supported yet" : connection.provider}
                        </p>
                      </div>
                      {isSelected ? (
                        <HugeiconsIcon
                          icon={Tick02Icon}
                          size={16}
                          className="text-primary shrink-0 mt-0.5"
                        />
                      ) : (
                        <HugeiconsIcon
                          icon={ArrowRight01Icon}
                          size={16}
                          className="text-muted-foreground/30 shrink-0 mt-0.5"
                        />
                      )}
                    </button>
                  )
                })
              )}
            </div>

            <div className="pt-4 shrink-0">
              <Button
                onClick={startConfiguring}
                disabled={selectedCount === 0}
                className="w-full"
              >
                {selectedCount > 0
                  ? `Configure ${selectedCount} connection${selectedCount > 1 ? "s" : ""}`
                  : "Select connections to add"}
              </Button>
            </div>
          </>
        ) : currentConnection && currentResourceType ? (
          <ResourceStep
            key={currentConnection.id}
            connection={currentConnection}
            resourceType={currentResourceType}
            picks={currentPicks}
            onTogglePick={(item) => togglePick(currentConnection.id ?? "", item)}
            onBack={backFromResources}
            onNext={() => {
              if (isLastConnection) {
                void submit()
              } else {
                setCurrentIdx(currentIdx + 1)
              }
            }}
            isLast={isLastConnection}
            stepLabel={`${currentIdx + 1} of ${orderedSelected.length}`}
            submitting={createSource.isPending}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

interface ResourceStepProps {
  connection: { id?: string; provider?: string; display_name?: string; configurable_resources?: unknown }
  resourceType: string
  picks: ResourceItem[]
  onTogglePick: (item: ResourceItem) => void
  onBack: () => void
  onNext: () => void
  isLast: boolean
  stepLabel: string
  submitting: boolean
}

function ResourceStep({
  connection,
  resourceType,
  picks,
  onTogglePick,
  onBack,
  onNext,
  isLast,
  stepLabel,
  submitting,
}: ResourceStepProps) {
  const { data, isLoading } = $api.useQuery(
    "get",
    "/v1/in/connections/{id}/resources/{type}",
    {
      params: { path: { id: connection.id ?? "", type: resourceType } },
    },
  )
  const items: ResourceItem[] =
    ((data as Record<string, unknown> | undefined)?.resources as ResourceItem[] | undefined) ?? []
  const pickedIds = useMemo(() => new Set(picks.map((p) => p.id)), [picks])
  const resourceLabel = (() => {
    const all = getConfigurableResources(connection)
    return all.find((r) => r.key === resourceType)?.display_name ?? resourceType
  })()

  return (
    <>
      <DialogHeader>
        <button
          type="button"
          onClick={onBack}
          className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors w-fit mb-2"
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} size={14} />
          Back
        </button>
        <DialogTitle className="flex items-center gap-3">
          <IntegrationLogo provider={connection.provider ?? ""} size={28} />
          <span>{connection.display_name ?? connection.provider}</span>
          <span className="ml-auto text-xs font-normal text-muted-foreground">
            {stepLabel}
          </span>
        </DialogTitle>
        <DialogDescription className="mt-2">
          Pick which {resourceLabel.toLowerCase()} to ingest from this connection.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {isLoading ? (
          Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-[64px] w-full rounded-xl" />
          ))
        ) : items.length === 0 ? (
          <p className="text-sm text-muted-foreground py-8 text-center">
            No {resourceLabel.toLowerCase()} available on this connection.
          </p>
        ) : (
          items.map((item) => {
            const isSelected = pickedIds.has(item.id)
            return (
              <button
                key={item.id}
                type="button"
                onClick={() => onTogglePick(item)}
                className={`group flex items-start gap-4 w-full rounded-xl p-4 text-left transition-colors ${
                  isSelected
                    ? "bg-primary/5 border border-primary/20 cursor-pointer"
                    : "bg-muted/50 hover:bg-muted border border-transparent cursor-pointer"
                }`}
              >
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-semibold text-foreground truncate">
                    {item.name}
                  </p>
                  {item.id !== item.name ? (
                    <p className="text-[12px] text-muted-foreground mt-0.5 font-mono truncate">
                      {item.id}
                    </p>
                  ) : null}
                </div>
                {isSelected ? (
                  <HugeiconsIcon
                    icon={Tick02Icon}
                    size={16}
                    className="text-primary shrink-0 mt-0.5"
                  />
                ) : (
                  <span className="size-4 rounded-full border border-border shrink-0 mt-0.5" />
                )}
              </button>
            )
          })
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button
          onClick={onNext}
          disabled={picks.length === 0}
          loading={isLast && submitting}
          className="w-full"
        >
          {picks.length === 0
            ? `Select at least one ${resourceLabel.toLowerCase().replace(/s$/, "")}`
            : isLast
            ? `Add ${picks.length} ${resourceLabel.toLowerCase()} to Knowledge Base`
            : `Next connection`}
        </Button>
      </div>
    </>
  )
}
