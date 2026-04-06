"use client"

import { useState } from "react"
import Link from "next/link"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { ProviderLogo } from "@/components/provider-logo"
import { IntegrationLogos, type IntegrationSummary } from "@/components/integration-logos"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { AgentStatusIndicator } from "./agent-status"
import { AgentActions } from "./agent-actions"
import type { AgentStatus } from "../_data/agents"
import type { components } from "@/lib/api/schema"

type Agent = components["schemas"]["agentResponse"]

interface AgentsTableProps {
  agents: Agent[]
  onEditAgent?: (agent: Agent) => void
}

function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

function getIntegrationSummaries(
  integrations: unknown,
  connectionsById: Map<string, { provider?: string; display_name?: string }>,
): IntegrationSummary[] {
  if (!integrations || typeof integrations !== "object") return []
  const result: IntegrationSummary[] = []
  for (const [connectionId, config] of Object.entries(integrations as Record<string, { actions?: string[] }>)) {
    const connection = connectionsById.get(connectionId)
    if (!connection?.provider) continue
    result.push({
      provider: connection.provider,
      name: connection.display_name ?? connection.provider,
      actions: Array.isArray(config?.actions) ? config.actions : [],
    })
  }
  return result
}

export function AgentsTable({ agents, onEditAgent }: AgentsTableProps) {
  const queryClient = useQueryClient()
  const [deleting, setDeleting] = useState<Agent | null>(null)
  const deleteAgent = $api.useMutation("delete", "/v1/agents/{id}")

  const { data: connectionsData } = $api.useQuery("get", "/v1/in/connections")
  const connections = connectionsData?.data ?? []
  const connectionsById = new Map(
    connections.filter((c) => c.id).map((c) => [c.id as string, c]),
  )

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
          <span className="w-20 shrink-0">Integrations</span>
          <span className="w-28 shrink-0 text-right">Model</span>
          <span className="w-20 shrink-0 text-right">Type</span>
          <span className="w-24 shrink-0 text-right">Created</span>
          <span className="w-6 shrink-0" />
          <span className="w-8 shrink-0" />
        </div>

        {agents.map((agent) => (
          <div key={agent.id}>
            <Link
              href={`/w/agents/${agent.id}`}
              className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary"
            >
              <div className="flex items-center gap-3 flex-1 min-w-0">
                <ProviderLogo provider={agent.provider_id ?? ""} size={24} />
                <span className="text-sm font-medium text-foreground truncate">{agent.name}</span>
              </div>
              <div className="w-20 shrink-0">
                <IntegrationLogos integrations={getIntegrationSummaries(agent.integrations, connectionsById)} size={20} />
              </div>
              <span className="w-28 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums truncate">
                {agent.model}
              </span>
              <span className="w-20 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                {agent.sandbox_type}
              </span>
              <span className="w-24 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                {agent.created_at ? formatDate(agent.created_at) : "\u2014"}
              </span>
              <div className="w-6 shrink-0 flex justify-center">
                <AgentStatusIndicator status={(agent.status ?? "active") as AgentStatus} />
              </div>
              <div className="w-8 shrink-0 flex justify-center" onClick={(e) => e.preventDefault()}>
                <AgentActions onEdit={() => onEditAgent?.(agent)} onDelete={() => setDeleting(agent)} />
              </div>
            </Link>

            <Link
              href={`/w/agents/${agent.id}`}
              className="flex md:hidden flex-col gap-3 rounded-xl border border-border p-4 transition-colors hover:border-primary"
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3 min-w-0 flex-1">
                  <ProviderLogo provider={agent.provider_id ?? ""} size={24} />
                  <span className="text-sm font-medium text-foreground truncate">{agent.name}</span>
                </div>
                <div className="flex items-center gap-2">
                  <IntegrationLogos integrations={getIntegrationSummaries(agent.integrations, connectionsById)} size={18} />
                  <AgentStatusIndicator status={(agent.status ?? "active") as AgentStatus} />
                </div>
              </div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-4 text-xs text-muted-foreground font-mono tabular-nums">
                  <span>{agent.model}</span>
                  <span>{agent.sandbox_type}</span>
                  <span>{agent.created_at ? formatDate(agent.created_at) : "\u2014"}</span>
                </div>
                <div onClick={(e) => e.preventDefault()}>
                  <AgentActions onEdit={() => onEditAgent?.(agent)} onDelete={() => setDeleting(agent)} />
                </div>
              </div>
            </Link>
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
