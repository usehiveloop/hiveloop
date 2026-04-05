"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  MoreHorizontalIcon,
  RefreshIcon,
  Delete02Icon,
  Settings01Icon,
} from "@hugeicons/core-free-icons"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { IntegrationLogo } from "@/components/integration-logo"

export interface InConnection {
  id?: string
  provider?: string
  display_name?: string
  created_at?: string
}

interface ConnectionsTableProps {
  connections: InConnection[]
}

function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

function StatusDot() {
  return (
    <span className="relative flex h-2 w-2">
      <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-500 opacity-40" />
      <span className="relative inline-flex h-2 w-2 rounded-full bg-green-500" />
    </span>
  )
}

export function ConnectionsTable({ connections }: ConnectionsTableProps) {
  const queryClient = useQueryClient()
  const [disconnecting, setDisconnecting] = useState<InConnection | null>(null)
  const deleteConnection = $api.useMutation("delete", "/v1/in/connections/{id}")

  function handleDisconnect() {
    if (!disconnecting?.id) return

    deleteConnection.mutate(
      { params: { path: { id: disconnecting.id } } },
      {
        onSuccess: () => {
          toast.success(`${disconnecting.display_name ?? "Connection"} disconnected`)
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/in/connections"] })
          setDisconnecting(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to disconnect"))
          setDisconnecting(null)
        },
      },
    )
  }

  if (connections.length === 0) {
    return (
      <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
        No connections found
      </div>
    )
  }

  return (
    <>
      <div className="flex flex-col gap-2">
        <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
          <span className="flex-1 min-w-0">Provider</span>
          <span className="w-20 shrink-0 text-right">Agents</span>
          <span className="w-24 shrink-0 text-right">Connected</span>
          <span className="w-6 shrink-0" />
          <span className="w-8 shrink-0" />
        </div>

        {connections.map((connection) => (
          <div key={connection.id}>
            <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary">
              <div className="flex items-center gap-3 flex-1 min-w-0">
                <IntegrationLogo provider={connection.provider ?? ""} size={24} />
                <span className="text-sm font-medium text-foreground truncate">
                  {connection.display_name}
                </span>
              </div>
              <span className="w-20 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                0
              </span>
              <span className="w-24 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                {connection.created_at ? formatDate(connection.created_at) : "—"}
              </span>
              <div className="w-6 shrink-0 flex justify-center">
                <StatusDot />
              </div>
              <div className="w-8 shrink-0 flex justify-center">
                <ConnectionActions onDisconnect={() => setDisconnecting(connection)} />
              </div>
            </div>

            <div className="flex md:hidden flex-col gap-3 rounded-xl border border-border p-4 transition-colors hover:border-primary">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3 min-w-0 flex-1">
                  <IntegrationLogo provider={connection.provider ?? ""} size={28} />
                  <span className="text-sm font-medium text-foreground truncate">
                    {connection.display_name}
                  </span>
                </div>
                <StatusDot />
              </div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-4 text-xs text-muted-foreground font-mono tabular-nums">
                  <span>0 agents</span>
                  <span>{connection.created_at ? formatDate(connection.created_at) : "—"}</span>
                </div>
                <ConnectionActions onDisconnect={() => setDisconnecting(connection)} />
              </div>
            </div>
          </div>
        ))}
      </div>

      <ConfirmDialog
        open={disconnecting !== null}
        onOpenChange={(open) => { if (!open) setDisconnecting(null) }}
        title="Disconnect integration"
        description={`Are you sure you want to disconnect ${disconnecting?.display_name ?? "this integration"}? Agents using this connection will lose access immediately.`}
        confirmLabel="Disconnect"
        destructive
        loading={deleteConnection.isPending}
        onConfirm={handleDisconnect}
      />
    </>
  )
}

interface ConnectionActionsProps {
  onDisconnect: () => void
}

function ConnectionActions({ onDisconnect }: ConnectionActionsProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center justify-center h-8 w-8 rounded-lg transition-colors hover:bg-muted outline-none">
        <HugeiconsIcon icon={MoreHorizontalIcon} size={16} className="text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" sideOffset={4}>
        <DropdownMenuGroup>
          <DropdownMenuItem>
            <HugeiconsIcon icon={Settings01Icon} size={16} className="text-muted-foreground" />
            Settings
          </DropdownMenuItem>
          <DropdownMenuItem>
            <HugeiconsIcon icon={RefreshIcon} size={16} className="text-muted-foreground" />
            Reconnect
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem variant="destructive" onClick={onDisconnect}>
          <HugeiconsIcon icon={Delete02Icon} size={16} />
          Disconnect
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
