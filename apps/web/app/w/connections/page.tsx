"use client"

import { useState, useMemo } from "react"
import { $api } from "@/lib/api/hooks"
import { ConnectionsHeader } from "./_components/connections-header"
import { ConnectionsSearch } from "./_components/connections-search"
import { ConnectionsTable } from "./_components/connections-table"
import { AddConnectionDialog } from "./_components/add-connection-dialog"

export default function ConnectionsPage() {
  const [search, setSearch] = useState("")
  const [addOpen, setAddOpen] = useState(false)

  const { data: inConnections } = $api.useQuery("get", "/v1/in/connections")

  const connections = inConnections?.data ?? []

  const filtered = useMemo(() => {
    if (!search.trim()) return connections
    const query = search.toLowerCase()
    return connections.filter((connection) =>
      (connection.display_name ?? "").toLowerCase().includes(query) ||
      (connection.provider ?? "").toLowerCase().includes(query),
    )
  }, [connections, search])

  return (
    <div className="max-w-464 mx-auto w-full px-4 py-8">
      <ConnectionsHeader count={connections.length} onAddClick={() => setAddOpen(true)} />
      <ConnectionsSearch value={search} onChange={setSearch} />
      <ConnectionsTable connections={filtered} />
      <AddConnectionDialog open={addOpen} onOpenChange={setAddOpen} />
    </div>
  )
}
