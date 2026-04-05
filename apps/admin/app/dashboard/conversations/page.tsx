"use client"

import { useState } from "react"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { useQueryClient } from "@tanstack/react-query"
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
} from "@/components/ui/alert-dialog"

type StatusFilter = "all" | "active" | "ended" | "error"

export default function ConversationsPage() {
  const queryClient = useQueryClient()
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all")
  const [orgFilter, setOrgFilter] = useState("")
  const [agentFilter, setAgentFilter] = useState("")
  const [endTarget, setEndTarget] = useState<{ id: string } | null>(null)

  const queryParams: Record<string, string> = {}
  if (statusFilter !== "all") queryParams.status = statusFilter
  if (orgFilter.trim()) queryParams.org_id = orgFilter.trim()
  if (agentFilter.trim()) queryParams.agent_id = agentFilter.trim()

  const { data, isLoading } = $api.useQuery("get", "/admin/v1/conversations", {
    params: { query: queryParams },
  })

  const conversations =
    (data as { data?: Record<string, string>[] })?.data ?? []

  async function handleEnd(id: string) {
    await api.DELETE("/admin/v1/conversations/{id}", {
      params: { path: { id } },
    })
    queryClient.invalidateQueries({
      queryKey: ["get", "/admin/v1/conversations"],
    })
    setEndTarget(null)
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Conversations"
        description="Manage agent conversations across all organizations."
      />

      <div className="flex items-center gap-4">
        <Tabs
          value={statusFilter}
          onValueChange={(v) => setStatusFilter(v as StatusFilter)}
        >
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="active">Active</TabsTrigger>
            <TabsTrigger value="ended">Ended</TabsTrigger>
            <TabsTrigger value="error">Error</TabsTrigger>
          </TabsList>
        </Tabs>
        <Input
          placeholder="Filter by org ID..."
          value={orgFilter}
          onChange={(e) => setOrgFilter(e.target.value)}
          className="max-w-xs"
        />
        <Input
          placeholder="Filter by agent ID..."
          value={agentFilter}
          onChange={(e) => setAgentFilter(e.target.value)}
          className="max-w-xs"
        />
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : conversations.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12">
          <p className="text-sm text-muted-foreground">
            No conversations found.
          </p>
        </div>
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>Agent ID</TableHead>
                <TableHead>Org ID</TableHead>
                <TableHead>Sandbox ID</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Ended</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {conversations.map((conv) => (
                <TableRow key={conv.id}>
                  <TableCell className="font-mono text-xs">
                    {conv.id?.slice(0, 8) ?? "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {conv.agent_id?.slice(0, 8) ?? "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {conv.org_id?.slice(0, 8) ?? "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {conv.sandbox_id?.slice(0, 8) ?? "--"}
                  </TableCell>
                  <TableCell>
                    {conv.status ? (
                      <StatusBadge status={conv.status} />
                    ) : (
                      "--"
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <TimeAgo date={conv.created_at} />
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <TimeAgo date={conv.ended_at} />
                  </TableCell>
                  <TableCell>
                    {conv.status === "active" && (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive"
                        onClick={() => setEndTarget({ id: conv.id! })}
                      >
                        End
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <AlertDialog
        open={!!endTarget}
        onOpenChange={(open) => !open && setEndTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>End conversation</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to force-end this conversation? The
              conversation will be terminated immediately.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => endTarget && handleEnd(endTarget.id)}
            >
              End conversation
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
