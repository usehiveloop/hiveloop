"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { ProviderLogo } from "@/components/provider-logo"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { AgentStatusIndicator } from "./agent-status"
import { AgentActions } from "./agent-actions"
import type { AgentStatus } from "../_data/agents"
import type { components } from "@/lib/api/schema"

type Agent = components["schemas"]["agentResponse"]

interface AgentsTableProps {
  agents: Agent[]
}

function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

export function AgentsTable({ agents }: AgentsTableProps) {
  const queryClient = useQueryClient()
  const [deleting, setDeleting] = useState<Agent | null>(null)
  const deleteAgent = $api.useMutation("delete", "/v1/agents/{id}")

  function handleDelete() {
    if (!deleting?.id) return

    deleteAgent.mutate(
      { params: { path: { id: deleting.id } } },
      {
        onSuccess: () => {
          toast.success(`"${deleting.name}" deleted`)
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents"] })
          setDeleting(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to delete agent"))
          setDeleting(null)
        },
      },
    )
  }

  if (agents.length === 0) {
    return (
      <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
        No agents found
      </div>
    )
  }

  return (
    <>
      <div className="flex flex-col gap-2">
        <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
          <span className="flex-1 min-w-0">Name</span>
          <span className="w-28 shrink-0 text-right">Model</span>
          <span className="w-20 shrink-0 text-right">Type</span>
          <span className="w-24 shrink-0 text-right">Created</span>
          <span className="w-6 shrink-0" />
          <span className="w-8 shrink-0" />
        </div>

        {agents.map((agent) => (
          <div key={agent.id}>
            <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary">
              <div className="flex items-center gap-3 flex-1 min-w-0">
                <ProviderLogo provider={agent.provider_id ?? ""} size={24} />
                <span className="text-sm font-medium text-foreground truncate">{agent.name}</span>
              </div>
              <span className="w-28 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums truncate">
                {agent.model}
              </span>
              <span className="w-20 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                {agent.sandbox_type}
              </span>
              <span className="w-24 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                {agent.created_at ? formatDate(agent.created_at) : "—"}
              </span>
              <div className="w-6 shrink-0 flex justify-center">
                <AgentStatusIndicator status={(agent.status ?? "active") as AgentStatus} />
              </div>
              <div className="w-8 shrink-0 flex justify-center">
                <AgentActions onDelete={() => setDeleting(agent)} />
              </div>
            </div>

            <div className="flex md:hidden flex-col gap-3 rounded-xl border border-border p-4 transition-colors hover:border-primary">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3 min-w-0 flex-1">
                  <ProviderLogo provider={agent.provider_id ?? ""} size={24} />
                  <span className="text-sm font-medium text-foreground truncate">{agent.name}</span>
                </div>
                <AgentStatusIndicator status={(agent.status ?? "active") as AgentStatus} />
              </div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-4 text-xs text-muted-foreground font-mono tabular-nums">
                  <span>{agent.model}</span>
                  <span>{agent.sandbox_type}</span>
                  <span>{agent.created_at ? formatDate(agent.created_at) : "—"}</span>
                </div>
                <AgentActions onDelete={() => setDeleting(agent)} />
              </div>
            </div>
          </div>
        ))}
      </div>

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => { if (!open) setDeleting(null) }}
        title="Delete agent"
        description={`This will permanently delete the agent and all its data. This action cannot be undone.`}
        confirmText={deleting?.name ?? ""}
        confirmLabel="Delete agent"
        destructive
        loading={deleteAgent.isPending}
        onConfirm={handleDelete}
      />
    </>
  )
}
