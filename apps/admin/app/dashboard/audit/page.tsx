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

export default function AuditPage() {
  const [orgId, setOrgId] = useState("")
  const [action, setAction] = useState("")

  const { data, isLoading } = $api.useQuery("get", "/admin/v1/audit", {
    params: {
      query: {
        org_id: orgId || undefined,
        action: action || undefined,
      },
    },
  })

  const entries = ((data as { data?: Record<string, unknown>[] })?.data ?? []) as {
    id?: string
    action?: string
    org_id?: string
    ip_address?: string
    created_at?: string
  }[]

  return (
    <div className="space-y-6">
      <PageHeader
        title="Audit Log"
        description="Audit trail entries across all organizations."
      />

      <div className="flex items-center gap-3">
        <Input
          placeholder="Filter by Org ID..."
          value={orgId}
          onChange={(e) => setOrgId(e.target.value)}
          className="max-w-xs"
        />
        <Input
          placeholder="Filter by action..."
          value={action}
          onChange={(e) => setAction(e.target.value)}
          className="max-w-xs"
        />
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : entries.length === 0 ? (
        <div className="flex h-48 items-center justify-center rounded-lg border border-border">
          <p className="text-sm text-muted-foreground">No audit entries found.</p>
        </div>
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Action</TableHead>
                <TableHead>Org ID</TableHead>
                <TableHead>IP Address</TableHead>
                <TableHead>Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {entries.map((entry, idx) => (
                <TableRow key={entry.id ?? idx}>
                  <TableCell className="font-medium">
                    {entry.action ?? "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {entry.org_id ?? "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {entry.ip_address ?? "--"}
                  </TableCell>
                  <TableCell>
                    <TimeAgo date={entry.created_at} />
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
