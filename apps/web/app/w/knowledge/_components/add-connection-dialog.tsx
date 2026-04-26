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
  ArrowRight01Icon,
  Plug01Icon,
  Search01Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"

interface AddConnectionDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function AddConnectionDialog({
  open,
  onOpenChange,
}: AddConnectionDialogProps) {
  const [search, setSearch] = useState("")
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const queryClient = useQueryClient()
  const createSource = $api.useMutation("post", "/v1/rag/sources")

  useEffect(() => {
    if (open) {
      setSearch("")
      setSelected(new Set())
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

  async function handleAdd() {
    if (selected.size === 0 || createSource.isPending) return
    const ids = Array.from(selected)
    const byId = new Map(connections.map((c) => [c.id ?? "", c]))

    const results = await Promise.allSettled(
      ids.map((id) => {
        const conn = byId.get(id)
        return createSource.mutateAsync({
          body: {
            kind: "INTEGRATION",
            name: conn?.display_name ?? conn?.provider ?? "Connection",
            in_connection_id: id,
            access_type: "private",
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
  const isSubmitting = createSource.isPending

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex flex-col h-[min(780px,85vh)] p-6">
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
            onClick={handleAdd}
            disabled={selectedCount === 0}
            loading={isSubmitting}
            className="w-full"
          >
            {selectedCount > 0
              ? `Add ${selectedCount} connection${selectedCount > 1 ? "s" : ""} to Knowledge Base`
              : "Select connections to add"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
