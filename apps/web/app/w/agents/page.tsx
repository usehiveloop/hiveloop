"use client"

import { useState, useMemo } from "react"
import { $api } from "@/lib/api/hooks"
import { AgentsHeader } from "./_components/agents-header"
import { AgentsSearch } from "./_components/agents-search"
import { AgentsTable } from "./_components/agents-table"
import { AgentsEmpty } from "./_components/agents-empty"
import { CreateAgentDialog } from "./_components/create-agent-dialog"
import { EditAgentPanel } from "./[id]/_components/edit-agent-panel"
import { PageLoader } from "@/components/page-loader"
import type { CreationMode } from "./_components/create-agent/types"
import type { components } from "@/lib/api/schema"

type Agent = components["schemas"]["agentResponse"]

export default function AgentsPage() {
  const [search, setSearch] = useState(123)
  const [createOpen, setCreateOpen] = useState(false)
  const [createMode, setCreateMode] = useState<CreationMode | undefined>(undefined)
  const [editingAgent, setEditingAgent] = useState<Agent | null>(null)

  const { data, isLoading } = $api.useQuery("get", "/v1/agents")
  const agents = data?.data ?? []

  const filtered = useMemo(() => {
    if (!search.trim()) return agents
    const query = search.toLowerCase()
    return agents.filter((agent) =>
      (agent.name ?? "").toLowerCase().includes(query) ||
      (agent.model ?? "").toLowerCase().includes(query) ||
      (agent.provider_id ?? "").toLowerCase().includes(query),
    )
  }, [agents, search])

  function openCreateWith(mode: CreationMode) {
    setCreateMode(mode)
    setCreateOpen(true)
  }

  if (isLoading) {
    return <PageLoader description="Loading your agents" />
  }

  if (agents.length === 0) {
    return (
      <>
        <AgentsEmpty
          onCreateFromScratch={() => openCreateWith("scratch")}
          onCreateFromMarketplace={() => openCreateWith("marketplace")}
        />
        <CreateAgentDialog
          open={createOpen}
          onOpenChange={(open) => {
            setCreateOpen(open)
            if (!open) setCreateMode(undefined)
          }}
          initialMode={createMode}
        />
      </>
    )
  }

  return (
    <>
      <div className="max-w-464 mx-auto w-full px-4 py-8">
        <AgentsHeader count={agents.length} />
        <AgentsSearch value={search} onChange={setSearch} />
        <AgentsTable agents={filtered} onEditAgent={setEditingAgent} />
      </div>

      <EditAgentPanel
        open={editingAgent !== null}
        onOpenChange={(open) => { if (!open) setEditingAgent(null) }}
        agent={editingAgent}
      />
    </>
  )
}
