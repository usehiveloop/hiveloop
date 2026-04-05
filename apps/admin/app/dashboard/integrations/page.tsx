"use client"

import { useState } from "react"
import { $api } from "@/lib/api/hooks"
import { PageHeader } from "@/components/admin/page-header"
import { TimeAgo } from "@/components/admin/time-ago"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

export default function IntegrationsPage() {
  const [orgId, setOrgId] = useState("")

  const { data, isLoading } = $api.useQuery("get", "/admin/v1/integrations", {
    params: { query: { org_id: orgId || undefined } },
  })

  const integrations = data?.data ?? []

  return (
    <div className="space-y-6">
      <PageHeader
        title="Integrations"
        description="OAuth integrations across all organizations."
      />

      <div className="flex items-center gap-3">
        <Input
          placeholder="Filter by Org ID..."
          value={orgId}
          onChange={(e) => setOrgId(e.target.value)}
          className="max-w-xs"
        />
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : integrations.length === 0 ? (
        <div className="flex h-48 items-center justify-center rounded-lg border border-border">
          <p className="text-sm text-muted-foreground">No integrations found.</p>
        </div>
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Display Name</TableHead>
                <TableHead>Provider</TableHead>
                <TableHead>Unique Key</TableHead>
                <TableHead>Org ID</TableHead>
                <TableHead>Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {integrations.map((integration) => (
                <TableRow key={integration.id}>
                  <TableCell className="font-medium">
                    {integration.display_name ?? "--"}
                  </TableCell>
                  <TableCell>{integration.provider ?? "--"}</TableCell>
                  <TableCell className="font-mono text-xs">
                    {integration.unique_key ?? "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {integration.org_id ?? "--"}
                  </TableCell>
                  <TableCell>
                    <TimeAgo date={integration.created_at} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
