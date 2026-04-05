"use client"

import { useState } from "react"
import { $api } from "@/lib/api/hooks"
import { ConnectionsHeader } from "./_components/connections-header"
import { ConnectionsSearch } from "./_components/connections-search"
import { ConnectionsTable } from "./_components/connections-table"
import { connections } from "./_data/connections"

export default function ConnectionsPage() {
  const [search, setSearch] = useState("")

  const { data: availableIntegrations } = $api.useQuery("get", "/v1/in/integrations/available")

  console.log("availableIntegrations", availableIntegrations)

  const filtered = connections.filter((c) =>
    c.displayName.toLowerCase().includes(search.toLowerCase())
  )

  return (
    <div className="max-w-464 mx-auto w-full px-4 py-8">
      <ConnectionsHeader count={connections.length} />
      <ConnectionsSearch value={search} onChange={setSearch} />
      <ConnectionsTable connections={filtered} />
    </div>
  )
}
