"use client"

import { useState, useMemo } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { $api } from "@/lib/api/hooks"
import { Button } from "@/components/ui/button"
import { PageHeader } from "@/components/page-header"
import { AgentsSearch } from "./_components/agents-search"
import { AgentsTable } from "./_components/agents-table"
import { AgentsEmpty } from "./_components/agents-empty"
import { AgentsSkeleton } from "./_components/agents-skeleton"
import { HugeiconsIcon } from "@hugeicons/react"
import { Add01Icon } from "@hugeicons/core-free-icons"

export default function AgentsPage() {
  const router = useRouter()
  const [search, setSearch] = useState("")

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

  return (
    <>
      <PageHeader
        title="Agents"
        actions={
          <Button render={<Link href="/w/agents/new" />}>
            <HugeiconsIcon icon={Add01Icon} size={16} data-icon="inline-start" />
            New agent
          </Button>
        }
      />

      {isLoading ? (
        <AgentsSkeleton />
      ) : agents.length === 0 ? (
        <AgentsEmpty />
      ) : (
        <div className="mx-auto w-full max-w-4xl px-6 py-10">
          <AgentsSearch value={search} onChange={setSearch} />
          <AgentsTable
            agents={filtered}
            onEditAgent={(agent) => {
              if (agent.id) router.push(`/w/agents/${agent.id}/edit`)
            }}
          />
        </div>
      )}
    </>
  )
}
