"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { ProviderLogo } from "@/components/provider-logo"
import { AddLlmKeyDialog } from "@/app/w/agents/_components/create-agent/add-llm-key-dialog"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import type { components } from "@/lib/api/schema"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArtificialIntelligence01Icon,
  Add01Icon,
  MoreHorizontalIcon,
  Delete02Icon,
  PauseIcon,
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

type Credential = components["schemas"]["credentialResponse"]

function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

function CredentialActions({ onDelete }: { onDelete: () => void }) {
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
          <DropdownMenuItem variant="destructive" onClick={onDelete}>
            <HugeiconsIcon icon={Delete02Icon} size={16} />
            Delete
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function LlmKeysSettings() {
  const queryClient = useQueryClient()
  const [addKeyOpen, setAddKeyOpen] = useState(false)
  const [deleting, setDeleting] = useState<Credential | null>(null)
  const { data, isLoading } = $api.useQuery("get", "/v1/credentials")
  const credentials = data?.data ?? []
  const deleteCredential = $api.useMutation("delete", "/v1/credentials/{id}")

  function handleDelete() {
    if (!deleting?.id) return

    deleteCredential.mutate(
      { params: { path: { id: deleting.id } } },
      {
        onSuccess: () => {
          toast.success(`"${deleting.label}" deleted`)
          queryClient.setQueryData(
            ["get", "/v1/credentials"],
            (old: typeof data) => old ? { ...old, data: old.data?.filter((credential) => credential.id !== deleting.id) } : old,
          )
          setDeleting(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to delete credential"))
          setDeleting(null)
        },
      },
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium text-foreground">Model provider keys</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            API keys for LLM providers that power your agents.
          </p>
        </div>
        <Button size="sm" onClick={() => setAddKeyOpen(true)} variant='secondary'>
          <HugeiconsIcon icon={Add01Icon} size={14} data-icon="inline-start" />
          Add key
        </Button>
      </div>

      <div className="flex flex-col gap-2">
        {isLoading ? (
          Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="h-[52px] w-full rounded-xl" />
          ))
        ) : credentials.length === 0 ? (
          <div className="flex flex-col items-center py-14">
            <div className="text-center mb-6">
              <h2 className="font-heading text-lg font-semibold text-foreground">No LLM keys yet</h2>
              <p className="text-sm text-muted-foreground mt-1.5 max-w-xs">
                Add a provider key to start running agents.
              </p>
            </div>
            <div className="w-full max-w-sm">
              <button
                type="button"
                onClick={() => setAddKeyOpen(true)}
                className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
              >
                <HugeiconsIcon icon={ArtificialIntelligence01Icon} size={20} className="shrink-0 mt-0.5 text-muted-foreground" />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-semibold text-foreground">Add LLM key</p>
                  <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">
                    Connect a provider like OpenAI or Anthropic to power your agents.
                  </p>
                </div>
                <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
              </button>
            </div>
          </div>
        ) : (
          <>
            <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
              <span className="flex-1 min-w-0">Label</span>
              <span className="w-24 shrink-0 text-right">Requests</span>
              <span className="w-28 shrink-0 text-right">Last used</span>
              <span className="w-28 shrink-0 text-right">Created</span>
              <span className="w-8 shrink-0" />
            </div>

            {credentials.map((credential) => (
              <div key={credential.id}>
                {/* Desktop row */}
                <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary">
                  <div className="flex items-center gap-3 flex-1 min-w-0">
                    <ProviderLogo provider={credential.provider_id ?? ""} size={24} />
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-foreground truncate">{credential.label}</p>
                      <p className="text-xs text-muted-foreground">{credential.provider_id}</p>
                    </div>
                  </div>
                  <span className="w-24 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                    {credential.request_count ?? 0}
                  </span>
                  <span className="w-28 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                    {credential.last_used_at ? formatDate(credential.last_used_at) : "\u2014"}
                  </span>
                  <span className="w-28 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                    {credential.created_at ? formatDate(credential.created_at) : "\u2014"}
                  </span>
                  <div className="w-8 shrink-0 flex justify-center">
                    <CredentialActions onDelete={() => setDeleting(credential)} />
                  </div>
                </div>

                {/* Mobile row */}
                <div className="flex md:hidden flex-col gap-3 rounded-xl border border-border p-4 transition-colors hover:border-primary">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3 min-w-0 flex-1">
                      <ProviderLogo provider={credential.provider_id ?? ""} size={24} />
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-foreground truncate">{credential.label}</p>
                        <p className="text-xs text-muted-foreground">{credential.provider_id}</p>
                      </div>
                    </div>
                    <CredentialActions onDelete={() => setDeleting(credential)} />
                  </div>
                  <div className="flex items-center gap-4 text-xs text-muted-foreground font-mono tabular-nums">
                    <span>{credential.request_count ?? 0} requests</span>
                    <span>{credential.created_at ? formatDate(credential.created_at) : "\u2014"}</span>
                  </div>
                </div>
              </div>
            ))}
          </>
        )}
      </div>

      <AddLlmKeyDialog open={addKeyOpen} onOpenChange={setAddKeyOpen} />

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => { if (!open) setDeleting(null) }}
        title="Delete LLM key"
        description={`This will permanently delete "${deleting?.label ?? ""}" and any agents using it will no longer be able to make LLM calls.`}
        confirmText="delete"
        confirmLabel="Delete key"
        destructive
        loading={deleteCredential.isPending}
        onConfirm={handleDelete}
      />
    </div>
  )
}
