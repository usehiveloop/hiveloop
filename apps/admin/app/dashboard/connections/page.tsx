"use client"

import { useState } from "react"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { useQueryClient } from "@tanstack/react-query"
import { PageHeader } from "@/components/admin/page-header"
import { StatusBadge } from "@/components/admin/status-badge"
import { TimeAgo } from "@/components/admin/time-ago"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

export default function ConnectionsPage() {
  const [orgId, setOrgId] = useState("")
  const [tab, setTab] = useState("all")
  const queryClient = useQueryClient()

  const revokedFilter = tab === "all" ? undefined : tab === "revoked" ? "true" : "false"

  const { data, isLoading } = $api.useQuery("get", "/admin/v1/connections", {
    params: {
      query: {
        org_id: orgId || undefined,
        revoked: revokedFilter,
      },
    },
  })

  const connections = data?.data ?? []

  async function handleRevoke(id: string) {
    await api.POST("/admin/v1/connections/{id}/revoke", {
      params: { path: { id } },
    })
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/connections"] })
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Connections"
        description="OAuth connections across all organizations."
      />

      <div className="flex items-center gap-3">
        <Input
          placeholder="Filter by Org ID..."
          value={orgId}
          onChange={(e) => setOrgId(e.target.value)}
          className="max-w-xs"
        />
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="all">All</TabsTrigger>
          <TabsTrigger value="active">Active</TabsTrigger>
          <TabsTrigger value="revoked">Revoked</TabsTrigger>
        </TabsList>

        <TabsContent value={tab}>
          {isLoading ? (
            <div className="space-y-3">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : connections.length === 0 ? (
            <div className="flex h-48 items-center justify-center rounded-lg border border-border">
              <p className="text-sm text-muted-foreground">No connections found.</p>
            </div>
          ) : (
            <div className="rounded-lg border border-border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>ID</TableHead>
                    <TableHead>Org ID</TableHead>
                    <TableHead>Integration ID</TableHead>
                    <TableHead>Identity ID</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {connections.map((conn) => {
                    const isRevoked = !!conn.revoked_at
                    return (
                      <TableRow key={conn.id}>
                        <TableCell className="font-mono text-xs">
                          {conn.id ?? "--"}
                        </TableCell>
                        <TableCell className="font-mono text-xs">
                          {conn.org_id ?? "--"}
                        </TableCell>
                        <TableCell className="font-mono text-xs">
                          {conn.integration_id ?? "--"}
                        </TableCell>
                        <TableCell className="font-mono text-xs">
                          {conn.identity_id ?? "--"}
                        </TableCell>
                        <TableCell>
                          <StatusBadge status={isRevoked ? "revoked" : "active"} />
                        </TableCell>
                        <TableCell>
                          <TimeAgo date={conn.created_at} />
                        </TableCell>
                        <TableCell className="text-right">
                          {!isRevoked && (
                            <AlertDialog>
                              <AlertDialogTrigger asChild>
                                <Button variant="destructive" size="xs">
                                  Revoke
                                </Button>
                              </AlertDialogTrigger>
                              <AlertDialogContent>
                                <AlertDialogHeader>
                                  <AlertDialogTitle>Revoke connection?</AlertDialogTitle>
                                  <AlertDialogDescription>
                                    This will permanently revoke this OAuth connection. The user will need to re-authorize.
                                  </AlertDialogDescription>
                                </AlertDialogHeader>
                                <AlertDialogFooter>
                                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                                  <AlertDialogAction
                                    variant="destructive"
                                    onClick={() => handleRevoke(conn.id!)}
                                  >
                                    Revoke
                                  </AlertDialogAction>
                                </AlertDialogFooter>
                              </AlertDialogContent>
                            </AlertDialog>
                          )}
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          )}
        </TabsContent>
      </Tabs>
    </div>
  )
}
