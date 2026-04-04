"use client"

import { useState } from "react"
import { AgentsHeader } from "./_components/agents-header"
import { AgentsSearch } from "./_components/agents-search"
import { AgentsTable } from "./_components/agents-table"
import { agents } from "./_data/agents"

export default function AgentsPage() {
  const [search, setSearch] = useState("")

  const filtered = agents.filter((a) =>
    a.name.toLowerCase().includes(search.toLowerCase())
  )

  return (
    <div className="max-w-464 mx-auto w-full px-4 py-8">
      <AgentsHeader count={agents.length} />

      <AgentsSearch value={search} onChange={setSearch} />
      <AgentsTable agents={filtered} />
    </div>
  )
}
