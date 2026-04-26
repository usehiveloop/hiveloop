"use client"

import { useState, useMemo } from "react"
import { $api } from "@/lib/api/hooks"
import { Button } from "@/components/ui/button"
import { PageHeader } from "@/components/page-header"
import { ConnectionsSearch } from "./_components/connections-search"
import { ConnectionsTable } from "./_components/connections-table"
import { ConnectionsEmpty } from "./_components/connections-empty"
import { AddConnectionDialog } from "./_components/add-connection-dialog"
import { PageLoader } from "@/components/page-loader"
import { useConnectIntegration } from "./_hooks/use-connect-integration"
import { HugeiconsIcon } from "@hugeicons/react"
import { Add01Icon } from "@hugeicons/core-free-icons"

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

  return (
    <>
      <PageHeader
        title="Connections"
        actions={
          <Button onClick={() => setAddOpen(true)}>
            <HugeiconsIcon icon={Add01Icon} size={16} data-icon="inline-start" />
            Add connection
          </Button>
        }
      />

      {isLoading ? (
        <PageLoader description="Loading your connections" />
      ) : connections.length === 0 ? (
        <ConnectionsEmpty
          connectingId={connectingId}
          onConnect={(id) => handleConnect(id)}
          onShowAll={() => setAddOpen(true)}
          onShowFormFor={handleShowFormFor}
        />
      ) : (
        <div className="mx-auto w-full max-w-4xl px-6 py-10">
          <ConnectionsSearch value={search} onChange={setSearch} />
          <ConnectionsTable connections={filtered} />
        </div>
      )}

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
