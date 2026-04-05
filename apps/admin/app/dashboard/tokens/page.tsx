"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { PageHeader } from "@/components/admin/page-header"
import { StatusBadge } from "@/components/admin/status-badge"
import { TimeAgo } from "@/components/admin/time-ago"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty"

export default function TokensPage() {
  const queryClient = useQueryClient()
  const [tab, setTab] = useState("all")
  const [orgFilter, setOrgFilter] = useState("")
  const [revokingId, setRevokingId] = useState<string | null>(null)

  const revokedParam = tab === "active" ? "false" : tab === "revoked" ? "true" : undefined

  const { data, isLoading, error } = $api.useQuery("get", "/admin/v1/tokens", {
    params: {
      query: {
        ...(orgFilter ? { org_id: orgFilter } : {}),
        ...(revokedParam ? { revoked: revokedParam } : {}),
        limit: 50,
      },
    },
  })

  async function handleRevoke(id: string) {
    setRevokingId(id)
    try {
      await api.POST("/admin/v1/tokens/{id}/revoke", {
        params: { path: { id } },
      })
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/tokens"] })
    } finally {
      setRevokingId(null)
    }
  }

  const tokens = data?.data ?? []

  function formatExpiry(expiresAt: string | undefined) {
    if (!expiresAt) return <span className="text-muted-foreground">--</span>
    const d = new Date(expiresAt)
    const isExpired = d.getTime() < Date.now()
    return (
      <span className={isExpired ? "text-destructive" : "text-muted-foreground"}>
        {d.toLocaleDateString()}
        {isExpired && " (expired)"}
      </span>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Proxy Tokens"
        description="Manage proxy tokens across all organizations."
      />

      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="active">Active</TabsTrigger>
            <TabsTrigger value="revoked">Revoked</TabsTrigger>
          </TabsList>
        </Tabs>

        <Input
          placeholder="Filter by Org ID..."
          value={orgFilter}
          onChange={(e) => setOrgFilter(e.target.value)}
          className="w-48"
        />
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : error ? (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          Failed to load tokens. Please try again.
        </div>
      ) : tokens.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>No tokens found</EmptyTitle>
            <EmptyDescription>
              {tab !== "all"
                ? "No tokens match the current filter. Try changing the tab or clearing filters."
                : "There are no proxy tokens in the system yet."}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>JTI</TableHead>
                <TableHead>Org ID</TableHead>
                <TableHead>Credential ID</TableHead>
                <TableHead>Expires</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.map((token) => (
                <TableRow key={token.id}>
                  <TableCell className="font-mono text-xs">
                    {token.jti ? token.jti.slice(0, 12) + "..." : "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {token.org_id ? token.org_id.slice(0, 8) + "..." : "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {token.credential_id
                      ? token.credential_id.slice(0, 8) + "..."
                      : "--"}
                  </TableCell>
                  <TableCell>{formatExpiry(token.expires_at)}</TableCell>
                  <TableCell>
                    <StatusBadge status={token.revoked_at ? "revoked" : "active"} />
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <TimeAgo date={token.created_at} />
                  </TableCell>
                  <TableCell className="text-right">
                    {!token.revoked_at && (
                      <AlertDialog>
                        <AlertDialogTrigger asChild>
                          <Button
                            variant="destructive"
                            size="sm"
                            disabled={revokingId === token.id}
                          >
                            {revokingId === token.id ? "Revoking..." : "Revoke"}
                          </Button>
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>Revoke token</AlertDialogTitle>
                            <AlertDialogDescription>
                              This will permanently revoke this proxy token. Any active
                              sessions or requests using it will fail immediately. This action
                              cannot be undone.
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>Cancel</AlertDialogCancel>
                            <AlertDialogAction
                              variant="destructive"
                              onClick={() => handleRevoke(token.id!)}
                            >
                              Revoke
                            </AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    )}
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
