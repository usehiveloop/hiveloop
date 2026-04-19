"use client"

import { useState, useMemo } from "react"
import { $api } from "@/lib/api/hooks"
import { ConnectionsHeader } from "./_components/connections-header"
import { ConnectionsSearch } from "./_components/connections-search"
import { ConnectionsTable } from "./_components/connections-table"
import { ConnectionsEmpty } from "./_components/connections-empty"
import { AddConnectionDialog } from "./_components/add-connection-dialog"
import { PageLoader } from "@/components/page-loader"
import { useConnectIntegration } from "./_hooks/use-connect-integration"

interface ConnectOptions {
  credentials?: Record<string, string>
  params?: Record<string, string>
  installation?: "outbound"
}

export default function ConnectionsPage() {
  const [search, setSearch] = useState("")
  const [addOpen, setAddOpen] = useState(false)
  const [dialogSearch, setDialogSearch] = useState("")
  const [preSelectedId, setPreSelectedId] = useState<string | null>(null)

  const { data: inConnections, isLoading } = $api.useQuery("get", "/v1/in/connections")
  const { connect, connectingId } = useConnectIntegration()

  const connections = inConnections?.data ?? []

  const filtered = useMemo(() => {
    if (!search.trim()) return connections
    const query = search.toLowerCase()
    return connections.filter((connection) =>
      (connection.display_name ?? "").toLowerCase().includes(query) ||
      (connection.provider ?? "").toLowerCase().includes(query),
    )
  }, [connections, search])

  function handleConnect(integrationId: string, options?: ConnectOptions) {
    connect(integrationId, {
      ...options,
      onSuccess: () => {
        setAddOpen(false)
        setDialogSearch("")
        setPreSelectedId(null)
      },
    })
  }

  function handleShowFormFor(integrationId: string) {
    setPreSelectedId(integrationId)
    setAddOpen(true)
  }

  if (isLoading) {
    return <PageLoader description="Loading your connections" />
  }

  if (connections.length === 0) {
    return (
      <>
        <ConnectionsEmpty
          connectingId={connectingId}
          onConnect={(id) => handleConnect(id)}
          onShowAll={() => setAddOpen(true)}
          onShowFormFor={handleShowFormFor}
        />
        <AddConnectionDialog
          open={addOpen}
          onOpenChange={setAddOpen}
          search={dialogSearch}
          onSearchChange={setDialogSearch}
          connectingId={connectingId}
          onConnect={handleConnect}
          preSelectedIntegrationId={preSelectedId}
          onPreSelectedClear={() => setPreSelectedId(null)}
        />
      </>
    )
  }

  return (
    <div className="max-w-464 mx-auto w-full px-4 py-8">
      <ConnectionsHeader count={connections.length} onAddClick={() => setAddOpen(true)} />
      <ConnectionsSearch value={search} onChange={setSearch} />
      <ConnectionsTable connections={filtered} />
      <AddConnectionDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        search={dialogSearch}
        onSearchChange={setDialogSearch}
        connectingId={connectingId}
        onConnect={handleConnect}
        preSelectedIntegrationId={preSelectedId}
        onPreSelectedClear={() => setPreSelectedId(null)}
      />
    </div>
  )
}
