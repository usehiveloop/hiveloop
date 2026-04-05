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

export default function ConnectSessionsPage() {
  const [orgId, setOrgId] = useState("")
  const [tab, setTab] = useState("all")
  const queryClient = useQueryClient()

  const expiredFilter = tab === "all" ? undefined : tab === "expired" ? "true" : "false"

  const { data, isLoading } = $api.useQuery("get", "/admin/v1/connect-sessions", {
    params: {
      query: {
        org_id: orgId || undefined,
        expired: expiredFilter,
      },
    },
  })

  const sessions = data?.data ?? []

  async function handleDelete(id: string) {
    await api.DELETE("/admin/v1/connect-sessions/{id}", {
      params: { path: { id } },
    })
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/connect-sessions"] })
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Connect Sessions"
        description="OAuth connect sessions across all organizations."
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
          <TabsTrigger value="expired">Expired</TabsTrigger>
        </TabsList>

        <TabsContent value={tab}>
          {isLoading ? (
            <div className="space-y-3">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : sessions.length === 0 ? (
            <div className="flex h-48 items-center justify-center rounded-lg border border-border">
              <p className="text-sm text-muted-foreground">No connect sessions found.</p>
            </div>
          ) : (
            <div className="rounded-lg border border-border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>External ID</TableHead>
                    <TableHead>Org ID</TableHead>
                    <TableHead>Identity ID</TableHead>
                    <TableHead>Activated</TableHead>
                    <TableHead>Expires</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sessions.map((session) => {
                    const isExpired = session.expires_at
                      ? new Date(session.expires_at).getTime() < Date.now()
                      : false
                    const isActivated = !!session.activated_at

                    return (
                      <TableRow key={session.id}>
                        <TableCell className="font-mono text-xs">
                          {session.external_id ?? "--"}
                        </TableCell>
                        <TableCell className="font-mono text-xs">
                          {session.org_id ?? "--"}
                        </TableCell>
                        <TableCell className="font-mono text-xs">
                          {session.identity_id ?? "--"}
                        </TableCell>
                        <TableCell>
                          <StatusBadge status={isActivated ? "active" : "pending"} />
                        </TableCell>
                        <TableCell>
                          {isExpired ? (
                            <StatusBadge status="error" />
                          ) : (
                            <TimeAgo date={session.expires_at} />
                          )}
                        </TableCell>
                        <TableCell>
                          <TimeAgo date={session.created_at} />
                        </TableCell>
                        <TableCell className="text-right">
                          <AlertDialog>
                            <AlertDialogTrigger asChild>
                              <Button variant="destructive" size="xs">
                                Delete
                              </Button>
                            </AlertDialogTrigger>
                            <AlertDialogContent>
                              <AlertDialogHeader>
                                <AlertDialogTitle>Delete session?</AlertDialogTitle>
                                <AlertDialogDescription>
                                  This will permanently delete this connect session. This action cannot be undone.
                                </AlertDialogDescription>
                              </AlertDialogHeader>
                              <AlertDialogFooter>
                                <AlertDialogCancel>Cancel</AlertDialogCancel>
                                <AlertDialogAction
                                  variant="destructive"
                                  onClick={() => handleDelete(session.id!)}
                                >
                                  Delete
                                </AlertDialogAction>
                              </AlertDialogFooter>
                            </AlertDialogContent>
                          </AlertDialog>
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
