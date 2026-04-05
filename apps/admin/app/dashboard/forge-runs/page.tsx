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

type StatusFilter = "all" | "running" | "completed" | "failed" | "cancelled"

function formatCost(cost: number | undefined): string {
  if (cost == null) return "--"
  return `$${cost.toFixed(4)}`
}

function formatScore(score: number | undefined): string {
  if (score == null) return "--"
  return `${(score * 100).toFixed(1)}%`
}

export default function ForgeRunsPage() {
  const queryClient = useQueryClient()
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all")
  const [orgFilter, setOrgFilter] = useState("")
  const [cancelTarget, setCancelTarget] = useState<{ id: string } | null>(null)

  const queryParams: Record<string, string> = {}
  if (statusFilter !== "all") queryParams.status = statusFilter
  if (orgFilter.trim()) queryParams.org_id = orgFilter.trim()

  const { data, isLoading } = $api.useQuery("get", "/admin/v1/forge-runs", {
    params: { query: queryParams },
  })

  const runs = (data as { data?: Record<string, unknown>[] })?.data ?? []

  async function handleCancel(id: string) {
    await api.POST("/admin/v1/forge-runs/{id}/cancel", {
      params: { path: { id } },
    })
    queryClient.invalidateQueries({
      queryKey: ["get", "/admin/v1/forge-runs"],
    })
    setCancelTarget(null)
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Forge Runs"
        description="Manage forge optimization runs across all organizations."
      />

      <div className="flex items-center gap-4">
        <Tabs
          value={statusFilter}
          onValueChange={(v) => setStatusFilter(v as StatusFilter)}
        >
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="running">Running</TabsTrigger>
            <TabsTrigger value="completed">Completed</TabsTrigger>
            <TabsTrigger value="failed">Failed</TabsTrigger>
            <TabsTrigger value="cancelled">Cancelled</TabsTrigger>
          </TabsList>
        </Tabs>
        <Input
          placeholder="Filter by org ID..."
          value={orgFilter}
          onChange={(e) => setOrgFilter(e.target.value)}
          className="max-w-xs"
        />
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : runs.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12">
          <p className="text-sm text-muted-foreground">No forge runs found.</p>
        </div>
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>Agent ID</TableHead>
                <TableHead>Org ID</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Iteration</TableHead>
                <TableHead>Score</TableHead>
                <TableHead>Cost</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {runs.map((run) => (
                <TableRow key={run.id as string}>
                  <TableCell className="font-mono text-xs">
                    {(run.id as string)?.slice(0, 8) ?? "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {(run.agent_id as string)?.slice(0, 8) ?? "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {(run.org_id as string)?.slice(0, 8) ?? "--"}
                  </TableCell>
                  <TableCell>
                    {run.status ? (
                      <StatusBadge status={run.status as string} />
                    ) : (
                      "--"
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {run.current_iteration != null && run.max_iterations != null
                      ? `${run.current_iteration}/${run.max_iterations}`
                      : "--"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatScore(run.final_score as number)}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatCost(run.total_cost as number)}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <TimeAgo date={run.created_at as string} />
                  </TableCell>
                  <TableCell>
                    {(run.status === "running" ||
                      run.status === "pending") && (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive"
                        onClick={() =>
                          setCancelTarget({ id: run.id as string })
                        }
                      >
                        Cancel
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
        open={!!cancelTarget}
        onOpenChange={(open) => !open && setCancelTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Cancel forge run</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to cancel this forge run? The run will be
              terminated and cannot be resumed.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => cancelTarget && handleCancel(cancelTarget.id)}
            >
              Cancel run
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
