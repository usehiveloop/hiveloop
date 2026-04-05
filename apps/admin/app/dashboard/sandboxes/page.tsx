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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
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

type StatusFilter = "all" | "running" | "stopped" | "error"

function formatMemoryMB(bytes: number | undefined): string {
  if (bytes == null) return "--"
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function formatCPU(usec: number | undefined): string {
  if (usec == null) return "--"
  return `${(usec / 1_000_000).toFixed(2)}s`
}

export default function SandboxesPage() {
  const queryClient = useQueryClient()
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all")
  const [orgFilter, setOrgFilter] = useState("")
  const [confirmAction, setConfirmAction] = useState<{
    type: "stop" | "delete" | "cleanup"
    id: string
  } | null>(null)

  const queryParams: Record<string, string> = {}
  if (statusFilter !== "all") queryParams.status = statusFilter
  if (orgFilter.trim()) queryParams.org_id = orgFilter.trim()

  const { data, isLoading } = $api.useQuery("get", "/admin/v1/sandboxes", {
    params: { query: queryParams },
  })

  const sandboxes = (data as { data?: Record<string, unknown>[] })?.data ?? []

  async function handleStop(id: string) {
    await api.POST("/admin/v1/sandboxes/{id}/stop", {
      params: { path: { id } },
    })
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/sandboxes"] })
    setConfirmAction(null)
  }

  async function handleDelete(id: string) {
    await api.DELETE("/admin/v1/sandboxes/{id}", {
      params: { path: { id } },
    })
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/sandboxes"] })
    setConfirmAction(null)
  }

  async function handleCleanup() {
    await api.POST("/admin/v1/sandboxes/cleanup")
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/sandboxes"] })
    setConfirmAction(null)
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Sandboxes"
        description="Manage sandboxes across all organizations."
        actions={
          <Button
            variant="outline"
            onClick={() =>
              setConfirmAction({ type: "cleanup", id: "cleanup" })
            }
          >
            Cleanup
          </Button>
        }
      />

      <div className="flex items-center gap-4">
        <Tabs
          value={statusFilter}
          onValueChange={(v) => setStatusFilter(v as StatusFilter)}
        >
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="running">Running</TabsTrigger>
            <TabsTrigger value="stopped">Stopped</TabsTrigger>
            <TabsTrigger value="error">Error</TabsTrigger>
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
      ) : sandboxes.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12">
          <p className="text-sm text-muted-foreground">No sandboxes found.</p>
        </div>
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>Org ID</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Agent ID</TableHead>
                <TableHead>Memory Used</TableHead>
                <TableHead>Memory Limit</TableHead>
                <TableHead>CPU</TableHead>
                <TableHead>Last Active</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {sandboxes.map((sandbox) => (
                <TableRow key={sandbox.id as string}>
                  <TableCell className="font-mono text-xs">
                    {(sandbox.id as string)?.slice(0, 8) ?? "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {(sandbox.org_id as string)?.slice(0, 8) ?? "--"}
                  </TableCell>
                  <TableCell className="capitalize">
                    {(sandbox.sandbox_type as string) || "--"}
                  </TableCell>
                  <TableCell>
                    {sandbox.status ? (
                      <StatusBadge status={sandbox.status as string} />
                    ) : (
                      "--"
                    )}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {(sandbox.agent_id as string)?.slice(0, 8) ?? "--"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatMemoryMB(sandbox.memory_used_bytes as number)}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatMemoryMB(sandbox.memory_limit_bytes as number)}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatCPU(sandbox.cpu_usage_usec as number)}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <TimeAgo date={sandbox.last_active_at as string} />
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <TimeAgo date={sandbox.created_at as string} />
                  </TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon-sm">
                          ...
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        {sandbox.status === "running" && (
                          <DropdownMenuItem
                            onClick={() =>
                              setConfirmAction({
                                type: "stop",
                                id: sandbox.id as string,
                              })
                            }
                          >
                            Stop
                          </DropdownMenuItem>
                        )}
                        <DropdownMenuItem
                          onClick={() =>
                            setConfirmAction({
                              type: "delete",
                              id: sandbox.id as string,
                            })
                          }
                          className="text-destructive"
                        >
                          Delete
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <AlertDialog
        open={!!confirmAction}
        onOpenChange={(open) => !open && setConfirmAction(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmAction?.type === "stop"
                ? "Stop sandbox"
                : confirmAction?.type === "cleanup"
                  ? "Cleanup sandboxes"
                  : "Delete sandbox"}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmAction?.type === "stop"
                ? "Are you sure you want to force-stop this sandbox?"
                : confirmAction?.type === "cleanup"
                  ? "This will delete all errored and stale stopped sandboxes (stopped > 24h). Continue?"
                  : "Are you sure you want to permanently delete this sandbox? This action cannot be undone."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant={
                confirmAction?.type === "delete" ? "destructive" : "default"
              }
              onClick={() => {
                if (!confirmAction) return
                if (confirmAction.type === "stop") {
                  handleStop(confirmAction.id)
                } else if (confirmAction.type === "cleanup") {
                  handleCleanup()
                } else {
                  handleDelete(confirmAction.id)
                }
              }}
            >
              {confirmAction?.type === "stop"
                ? "Stop"
                : confirmAction?.type === "cleanup"
                  ? "Cleanup"
                  : "Delete"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
