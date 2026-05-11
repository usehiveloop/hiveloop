"use client"

import { useMemo } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Plug01Icon,
  Search01Icon,
} from "@hugeicons/core-free-icons"
import {
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"

interface ConnectionPickerViewProps {
  search: string
  onSearchChange: (value: string) => void
  onPickConnection: (
    connectionId: string,
    connectionName: string,
    provider: string
  ) => void
  onBack: () => void
  connectionIds?: Set<string>
}

export function ConnectionPickerView({
  search,
  onSearchChange,
  onPickConnection,
  onBack,
  connectionIds,
}: ConnectionPickerViewProps) {
  const { data: connectionsData, isLoading } = $api.useQuery(
    "get",
    "/v1/in/connections"
  )
  const allConnections = useMemo(
    () => connectionsData?.data ?? [],
    [connectionsData]
  )
  const connections = useMemo(
    () =>
      connectionIds
        ? allConnections.filter((connection) =>
            connectionIds.has(connection.id!)
          )
        : allConnections,
    [allConnections, connectionIds]
  )

  const filtered = useMemo(() => {
    if (!search.trim()) return connections
    const query = search.toLowerCase()
    return connections.filter(
      (connection) =>
        (connection.display_name ?? "").toLowerCase().includes(query) ||
        (connection.provider ?? "").toLowerCase().includes(query)
    )
  }, [connections, search])

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onBack}
            className="-ml-1 flex h-7 w-7 items-center justify-center rounded-lg transition-colors hover:bg-muted"
          >
            <HugeiconsIcon
              icon={ArrowLeft01Icon}
              size={16}
              className="text-muted-foreground"
            />
          </button>
          <DialogTitle>Choose connection</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Pick which integration connection this trigger listens on.
        </DialogDescription>
      </DialogHeader>

      <div className="relative mt-4">
        <HugeiconsIcon
          icon={Search01Icon}
          size={14}
          className="absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          placeholder="Search connections..."
          value={search}
          onChange={(event) => onSearchChange(event.target.value)}
          className="h-9 pl-9"
        />
      </div>

      <div className="mt-4 flex flex-1 flex-col gap-2 overflow-y-auto">
        {isLoading ? (
          Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-[64px] w-full rounded-xl" />
          ))
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-3 py-12 text-center">
            <div className="flex size-12 items-center justify-center rounded-full bg-muted">
              <HugeiconsIcon
                icon={connections.length === 0 ? Plug01Icon : Search01Icon}
                size={20}
                className="text-muted-foreground"
              />
            </div>
            <p className="max-w-xs text-sm text-muted-foreground">
              {connections.length === 0
                ? "No connections in this workspace yet. Add one from the integrations section to enable webhook triggers."
                : `No connections matching “${search.trim()}”.`}
            </p>
          </div>
        ) : (
          filtered.map((connection) => (
            <button
              key={connection.id}
              type="button"
              onClick={() =>
                onPickConnection(
                  connection.id!,
                  connection.display_name ?? connection.provider ?? "",
                  connection.provider ?? ""
                )
              }
              className="group flex w-full cursor-pointer items-start gap-4 rounded-xl border border-transparent bg-muted/50 p-4 text-left transition-colors hover:bg-muted"
            >
              <IntegrationLogo
                provider={connection.provider ?? ""}
                size={32}
                className="mt-0.5 shrink-0"
              />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-semibold text-foreground">
                  {connection.display_name}
                </p>
                <p className="mt-0.5 text-[13px] text-muted-foreground">
                  {connection.provider}
                </p>
              </div>
              <HugeiconsIcon
                icon={ArrowRight01Icon}
                size={16}
                className="mt-0.5 shrink-0 text-muted-foreground/30"
              />
            </button>
          ))
        )}
      </div>
    </>
  )
}
