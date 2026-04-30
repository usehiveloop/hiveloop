"use client"

import { useState } from "react"
import Link from "next/link"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { ProviderLogo } from "@/components/provider-logo"
import { Avatar, AvatarImage, AvatarFallback } from "@/components/ui/avatar"
import { IntegrationLogos, type IntegrationSummary } from "@/components/integration-logos"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { AgentStatusIndicator } from "./agent-status"
import { AgentActions } from "./agent-actions"
import { EnvVarsDialog } from "./env-vars-dialog"
import { SetupCommandsDialog } from "./setup-commands-dialog"
import { ConfigureResourcesDialog } from "./configure-resources-dialog"
import type { AgentStatus } from "../_data/agents"
import type { components } from "@/lib/api/schema"

type Agent = components["schemas"]["agentResponse"]

interface AgentsTableProps {
  agents: Agent[]
  onEditAgent?: (agent: Agent) => void
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
  const [envVarsAgent, setEnvVarsAgent] = useState<Agent | null>(null)
  const [setupCommandsAgent, setSetupCommandsAgent] = useState<Agent | null>(null)
  const [resourcesAgent, setResourcesAgent] = useState<Agent | null>(null)
  const deleteAgent = $api.useMutation("delete", "/v1/agents/{id}")

  const { data: connectionsData } = $api.useQuery("get", "/v1/in/connections")
  const connections = connectionsData?.data ?? []
  const connectionsById = new Map(
    connections
      .filter((c): c is typeof c & { id: string } => typeof c.id === "string")
      .map((c) => [c.id, c]),
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
        <div className="flex items-center gap-3 px-4 py-1 font-mono text-[10px] uppercase tracking-[1px] text-muted-foreground/50">
          <span className="min-w-0 flex-1">Name</span>
          <span className="w-24 shrink-0">Integrations</span>
          <span className="w-6 shrink-0" />
          <span className="w-8 shrink-0" />
        </div>

        {agents.map((agent) => (
          <Link
            key={agent.id}
            href={`/w/agents/${agent.id}`}
            className="flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary"
          >
            <div className="flex min-w-0 flex-1 items-center gap-3">
              {agent.avatar_url ? (
                <Avatar size="sm" className="rounded-md after:rounded-md">
                  <AvatarImage src={agent.avatar_url} alt={agent.name ?? ""} className="rounded-md" />
                  <AvatarFallback className="rounded-md">{(agent.name ?? "?").slice(0, 1).toUpperCase()}</AvatarFallback>
                </Avatar>
              ) : (
                <ProviderLogo provider={agent.provider_id ?? ""} size={24} />
              )}
              <span className="truncate text-sm font-medium text-foreground">{agent.name}</span>
            </div>
            <div className="w-24 shrink-0">
              <IntegrationLogos integrations={getIntegrationSummaries(agent.integrations, connectionsById)} size={20} />
            </div>
            <div className="flex w-6 shrink-0 justify-center">
              <AgentStatusIndicator status={(agent.status ?? "active") as AgentStatus} agent={agent} />
            </div>
            <div className="flex w-8 shrink-0 justify-center" onClick={(event) => event.preventDefault()}>
              <AgentActions
                agent={agent}
                onEdit={() => onEditAgent?.(agent)}
                onDelete={() => setDeleting(agent)}
                onEnvVars={() => setEnvVarsAgent(agent)}
                onSetupCommands={() => setSetupCommandsAgent(agent)}
                onConfigureResources={() => setResourcesAgent(agent)}
              />
            </div>
          </Link>
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

      <EnvVarsDialog
        open={envVarsAgent !== null}
        onOpenChange={(open) => { if (!open) setEnvVarsAgent(null) }}
        agentName={envVarsAgent?.name ?? ""}
      />

      <SetupCommandsDialog
        open={setupCommandsAgent !== null}
        onOpenChange={(open) => { if (!open) setSetupCommandsAgent(null) }}
        agentName={setupCommandsAgent?.name ?? ""}
      />

      <ConfigureResourcesDialog
        open={resourcesAgent !== null}
        onOpenChange={(open) => { if (!open) setResourcesAgent(null) }}
        agent={resourcesAgent}
      />
    </>
  )
}
