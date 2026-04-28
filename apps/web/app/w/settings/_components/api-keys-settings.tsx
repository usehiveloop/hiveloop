"use client"

import * as React from "react"
import { useState } from "react"
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { Skeleton } from "@/components/ui/skeleton"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import type { components } from "@/lib/api/schema"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Key01Icon,
  Add01Icon,
  MoreHorizontalIcon,
  Delete02Icon,
  PauseIcon,
  Copy01Icon,
  ArrowRight01Icon,
} from "@hugeicons/core-free-icons"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

type ApiKey = components["schemas"]["apiKeyResponse"]

function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

function ScopeBadge({ scopes }: { scopes?: string[] }) {
  if (!scopes || scopes.length === 0) return <span className="text-[11px] text-muted-foreground">{"\u2014"}</span>

  if (scopes.length === 1 && scopes[0] === "all") {
    return <Badge variant="secondary" className="text-[10px]">all</Badge>
  }

  return (
    <Tooltip>
      <TooltipTrigger className="cursor-default">
        <Badge variant="secondary" className="text-[10px]">{scopes.length} scopes</Badge>
      </TooltipTrigger>
      <TooltipContent>
        {scopes.join(", ")}
      </TooltipContent>
    </Tooltip>
  )
}

function ApiKeyActions({ onRevoke }: { onRevoke: () => void }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center justify-center h-8 w-8 rounded-lg transition-colors hover:bg-muted outline-none">
        <HugeiconsIcon icon={MoreHorizontalIcon} size={16} className="text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" sideOffset={4}>
        <DropdownMenuGroup>
          <DropdownMenuItem>
            <HugeiconsIcon icon={PauseIcon} size={16} className="text-muted-foreground" />
            Deactivate
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem variant="destructive" onClick={onRevoke}>
            <HugeiconsIcon icon={Delete02Icon} size={16} />
            Revoke
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function CreateApiKeyDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState("")
  const [createdKey, setCreatedKey] = useState<string | null>(null)
  const createKey = $api.useMutation("post", "/v1/api-keys")

  function reset() {
    setName("")
    setCreatedKey(null)
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) reset()
    onOpenChange(nextOpen)
  }

  function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    if (!name.trim()) return

    createKey.mutate(
      { body: { name: name.trim(), scopes: ["all"] } },
      {
        onSuccess: (response) => {
          const key = (response as { key?: string })?.key
          if (key) {
            setCreatedKey(key)
          } else {
            toast.success("API key created")
            handleOpenChange(false)
          }
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/api-keys"] })
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to create API key"))
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent showCloseButton className="sm:max-w-md max-h-[90dvh] overflow-y-auto">
        <DialogTitle>{createdKey ? "API key created" : "Create API key"}</DialogTitle>

        {createdKey ? (
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted-foreground">
              Copy your API key now. You won&apos;t be able to see it again.
            </p>
            <div className="flex items-center gap-2">
              <Input value={createdKey} readOnly className="font-mono text-xs" />
              <Button
                variant="outline"
                size="icon-sm"
                onClick={() => {
                  navigator.clipboard.writeText(createdKey)
                  toast.success("Copied to clipboard")
                }}
              >
                <HugeiconsIcon icon={Copy01Icon} size={14} />
              </Button>
            </div>
            <Button onClick={() => handleOpenChange(false)} className="w-full">Done</Button>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="flex flex-col gap-5">
            <div className="flex flex-col gap-2">
              <Label htmlFor="api-key-name">Name</Label>
              <Input
                id="api-key-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="e.g. CI/CD pipeline"
                required
                autoFocus
              />
            </div>

            <Button type="submit" className="w-full" loading={createKey.isPending} disabled={!name.trim()}>
              Create key
            </Button>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

export function ApiKeysSettings() {
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [revoking, setRevoking] = useState<ApiKey | null>(null)
  const { data, isLoading } = $api.useQuery("get", "/v1/api-keys")
  const apiKeys = data?.data ?? []
  const revokeKey = $api.useMutation("delete", "/v1/api-keys/{id}")

  function handleRevoke() {
    if (!revoking?.id) return

    revokeKey.mutate(
      { params: { path: { id: revoking.id } } },
      {
        onSuccess: () => {
          toast.success(`"${revoking.name}" revoked`)
          queryClient.setQueryData(
            ["get", "/v1/api-keys"],
            (old: typeof data) => old ? { ...old, data: old.data?.filter((key) => key.id !== revoking.id) } : old,
          )
          setRevoking(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to revoke API key"))
          setRevoking(null)
        },
      },
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium text-foreground">API keys</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Keys for programmatic access to your workspace.
          </p>
        </div>
        <Button size="sm" variant="secondary" onClick={() => setCreateOpen(true)}>
          <HugeiconsIcon icon={Add01Icon} size={14} data-icon="inline-start" />
          Create key
        </Button>
      </div>

      <div className="flex flex-col gap-2">
        {isLoading ? (
          Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="h-13 w-full rounded-xl" />
          ))
        ) : apiKeys.length === 0 ? (
          <div className="flex flex-col items-center py-14">
            <div className="text-center mb-6">
              <h2 className="font-heading text-lg font-semibold text-foreground">No API keys yet</h2>
              <p className="text-sm text-muted-foreground mt-1.5 max-w-xs">
                Create a key to access the API programmatically.
              </p>
            </div>
            <div className="w-full max-w-sm">
              <button
                type="button"
                onClick={() => setCreateOpen(true)}
                className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
              >
                <HugeiconsIcon icon={Key01Icon} size={20} className="shrink-0 mt-0.5 text-muted-foreground" />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-semibold text-foreground">Create API key</p>
                  <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">
                    Generate a key to authenticate requests to the Hiveloop API.
                  </p>
                </div>
                <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
              </button>
            </div>
          </div>
        ) : (
          <>
            <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
              <span className="flex-1 min-w-0">Name</span>
              <span className="w-32 shrink-0">Key</span>
              <span className="w-20 shrink-0">Scopes</span>
              <span className="w-28 shrink-0 text-right">Created</span>
              <span className="w-8 shrink-0" />
            </div>

            {apiKeys.map((apiKey) => (
              <div key={apiKey.id}>
                {/* Desktop row */}
                <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary">
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-foreground truncate">{apiKey.name}</p>
                  </div>
                  <span className="w-32 shrink-0 text-[11px] text-muted-foreground font-mono tabular-nums truncate">
                    {apiKey.key_prefix ? `${apiKey.key_prefix}...` : "\u2014"}
                  </span>
                  <div className="w-20 shrink-0">
                    <ScopeBadge scopes={apiKey.scopes} />
                  </div>
                  <span className="w-28 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                    {apiKey.created_at ? formatDate(apiKey.created_at) : "\u2014"}
                  </span>
                  <div className="w-8 shrink-0 flex justify-center">
                    <ApiKeyActions onRevoke={() => setRevoking(apiKey)} />
                  </div>
                </div>

                {/* Mobile row */}
                <div className="flex md:hidden flex-col gap-3 rounded-xl border border-border p-4 transition-colors hover:border-primary">
                  <div className="flex items-center justify-between">
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-foreground truncate">{apiKey.name}</p>
                      <p className="text-xs text-muted-foreground font-mono">{apiKey.key_prefix ? `${apiKey.key_prefix}...` : "\u2014"}</p>
                    </div>
                    <ApiKeyActions onRevoke={() => setRevoking(apiKey)} />
                  </div>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground font-mono tabular-nums">
                    <span>{apiKey.created_at ? formatDate(apiKey.created_at) : "\u2014"}</span>
                    <ScopeBadge scopes={apiKey.scopes} />
                  </div>
                </div>
              </div>
            ))}
          </>
        )}
      </div>

      <CreateApiKeyDialog open={createOpen} onOpenChange={setCreateOpen} />

      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(open) => { if (!open) setRevoking(null) }}
        title="Revoke API key"
        description={`This will permanently revoke "${revoking?.name ?? ""}". Any requests using this key will be rejected immediately.`}
        confirmText="delete"
        confirmLabel="Revoke key"
        destructive
        loading={revokeKey.isPending}
        onConfirm={handleRevoke}
      />
    </div>
  )
}
