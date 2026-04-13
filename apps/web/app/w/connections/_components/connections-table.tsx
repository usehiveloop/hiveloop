"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { apiUrl } from "@/lib/api/client"
import { extractErrorMessage } from "@/lib/api/error"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  MoreHorizontalIcon,
  RefreshIcon,
  Delete02Icon,
  Settings01Icon,
  Alert02Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { IntegrationLogo } from "@/components/integration-logo"

export interface InConnection {
  id?: string
  provider?: string
  display_name?: string
  webhook_configured?: boolean
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
  const [webhookConfiguring, setWebhookConfiguring] = useState<InConnection | null>(null)
  const deleteConnection = $api.useMutation("delete", "/v1/in/connections/{id}")
  const markWebhookConfigured = $api.useMutation("patch", "/v1/in/connections/{id}/webhook-configured")

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
                {connection.webhook_configured === false ? (
                  <WebhookWarning onClick={() => setWebhookConfiguring(connection)} />
                ) : (
                  <StatusDot />
                )}
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
                {connection.webhook_configured === false ? (
                  <WebhookWarning onClick={() => setWebhookConfiguring(connection)} />
                ) : (
                  <StatusDot />
                )}
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

      <WebhookConfigDialog
        connection={webhookConfiguring}
        open={webhookConfiguring !== null}
        onOpenChange={(open) => { if (!open) setWebhookConfiguring(null) }}
        loading={markWebhookConfigured.isPending}
        onConfirm={() => {
          if (!webhookConfiguring?.id) return
          markWebhookConfigured.mutate(
            { params: { path: { id: webhookConfiguring.id } } },
            {
              onSuccess: () => {
                toast.success("Webhook marked as configured")
                queryClient.invalidateQueries({ queryKey: ["get", "/v1/in/connections"] })
                setWebhookConfiguring(null)
              },
              onError: (error) => {
                toast.error(extractErrorMessage(error, "Failed to update connection"))
              },
            },
          )
        }}
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

// --- Webhook configuration warning & dialog ---

function WebhookWarning({ onClick }: { onClick: () => void }) {
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger
          onClick={onClick}
          className="flex items-center justify-center h-5 w-5 rounded-full bg-amber-500/10 cursor-pointer transition-colors hover:bg-amber-500/20"
        >
          <HugeiconsIcon icon={Alert02Icon} size={12} className="text-amber-500" />
        </TooltipTrigger>
        <TooltipContent>
          <p>Manual webhook configuration required</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

interface WebhookConfigDialogProps {
  connection: InConnection | null
  open: boolean
  onOpenChange: (open: boolean) => void
  loading: boolean
  onConfirm: () => void
}

function WebhookConfigDialog({ connection, open, onOpenChange, loading, onConfirm }: WebhookConfigDialogProps) {
  const [copied, setCopied] = useState(false)

  if (!connection) return null

  const webhookUrl = apiUrl(`/incoming/webhooks/${connection.provider}/${connection.id}`)

  function handleCopy() {
    navigator.clipboard.writeText(webhookUrl)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <div className="flex items-center gap-2.5">
            <IntegrationLogo provider={connection.provider ?? ""} size={20} />
            <DialogTitle>Configure webhook</DialogTitle>
          </div>
          <DialogDescription>
            {connection.display_name} requires a webhook URL to be configured manually in the provider dashboard.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4 mt-4">
          <div className="rounded-xl bg-muted/50 border border-border p-4">
            <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground mb-2">Webhook URL</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 text-[12px] font-mono bg-background rounded-lg px-3 py-2 border border-border break-all select-all">
                {webhookUrl}
              </code>
              <Button variant="outline" size="sm" onClick={handleCopy} className="shrink-0 h-8">
                {copied ? (
                  <span className="flex items-center gap-1.5">
                    <HugeiconsIcon icon={Tick02Icon} size={14} className="text-green-500" />
                    Copied
                  </span>
                ) : (
                  "Copy"
                )}
              </Button>
            </div>
          </div>

          <div className="rounded-xl bg-muted/50 border border-border p-4">
            <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground mb-2">Setup instructions</p>
            <ol className="list-decimal list-inside text-[13px] text-muted-foreground leading-relaxed space-y-1">
              <li>Open your {connection.display_name} project dashboard</li>
              <li>Go to <strong>Settings</strong> &rarr; <strong>Webhooks</strong></li>
              <li>Paste the webhook URL shown above</li>
              <li>Click <strong>Save</strong></li>
            </ol>
          </div>
        </div>

        <div className="pt-4">
          <Button onClick={onConfirm} disabled={loading} className="w-full">
            {loading ? "Saving..." : "I have configured the webhook"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
