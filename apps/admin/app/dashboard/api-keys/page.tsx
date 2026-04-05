"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { PageHeader } from "@/components/admin/page-header"
import { StatusBadge } from "@/components/admin/status-badge"
import { TimeAgo } from "@/components/admin/time-ago"
import { Badge } from "@/components/ui/badge"
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

export default function ApiKeysPage() {
  const queryClient = useQueryClient()
  const [tab, setTab] = useState("all")
  const [orgFilter, setOrgFilter] = useState("")
  const [revokingId, setRevokingId] = useState<string | null>(null)

  const revokedParam = tab === "active" ? "false" : tab === "revoked" ? "true" : undefined

  const { data, isLoading, error } = $api.useQuery("get", "/admin/v1/api-keys", {
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
      await api.POST("/admin/v1/api-keys/{id}/revoke", {
        params: { path: { id } },
      })
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/api-keys"] })
    } finally {
      setRevokingId(null)
    }
  }

  const apiKeys = data?.data ?? []

  function formatExpiry(expiresAt: string | undefined) {
    if (!expiresAt) return <span className="text-muted-foreground">Never</span>
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
        title="API Keys"
        description="Manage API keys across all organizations."
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
          Failed to load API keys. Please try again.
        </div>
      ) : apiKeys.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>No API keys found</EmptyTitle>
            <EmptyDescription>
              {tab !== "all"
                ? "No API keys match the current filter. Try changing the tab or clearing filters."
                : "There are no API keys in the system yet."}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Key Prefix</TableHead>
                <TableHead>Org ID</TableHead>
                <TableHead>Scopes</TableHead>
                <TableHead>Expires</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {apiKeys.map((key) => (
                <TableRow key={key.id}>
                  <TableCell className="font-medium">
                    {key.name || <span className="text-muted-foreground">--</span>}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {key.key_prefix || "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {key.org_id ? key.org_id.slice(0, 8) + "..." : "--"}
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {key.scopes && key.scopes.length > 0 ? (
                        key.scopes.map((scope) => (
                          <Badge key={scope} variant="secondary">
                            {scope}
                          </Badge>
                        ))
                      ) : (
                        <span className="text-muted-foreground">--</span>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>{formatExpiry(key.expires_at)}</TableCell>
                  <TableCell>
                    <StatusBadge status={key.revoked_at ? "revoked" : "active"} />
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <TimeAgo date={key.created_at} />
                  </TableCell>
                  <TableCell className="text-right">
                    {!key.revoked_at && (
                      <AlertDialog>
                        <AlertDialogTrigger asChild>
                          <Button
                            variant="destructive"
                            size="sm"
                            disabled={revokingId === key.id}
                          >
                            {revokingId === key.id ? "Revoking..." : "Revoke"}
                          </Button>
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>Revoke API key</AlertDialogTitle>
                            <AlertDialogDescription>
                              This will permanently revoke this API key. Any requests
                              authenticated with this key will be rejected immediately. This
                              action cannot be undone.
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>Cancel</AlertDialogCancel>
                            <AlertDialogAction
                              variant="destructive"
                              onClick={() => handleRevoke(key.id!)}
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
