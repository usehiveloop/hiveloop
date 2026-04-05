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
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
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

type BuildStatusFilter = "all" | "ready" | "pending" | "building" | "failed"

export default function SandboxTemplatesPage() {
  const queryClient = useQueryClient()
  const [buildStatusFilter, setBuildStatusFilter] =
    useState<BuildStatusFilter>("all")
  const [orgFilter, setOrgFilter] = useState("")
  const [deleteTarget, setDeleteTarget] = useState<{
    id: string
    name: string
  } | null>(null)
  const [editingTemplate, setEditingTemplate] = useState<{ id: string } | null>(null)
  const [editForm, setEditForm] = useState({ name: "" })
  const [editError, setEditError] = useState<string | null>(null)
  const [editSaving, setEditSaving] = useState(false)

  const queryParams: Record<string, string> = {}
  if (buildStatusFilter !== "all") queryParams.build_status = buildStatusFilter
  if (orgFilter.trim()) queryParams.org_id = orgFilter.trim()

  const { data, isLoading } = $api.useQuery(
    "get",
    "/admin/v1/sandbox-templates",
    { params: { query: queryParams } }
  )

  const templates = (data as { data?: Record<string, string>[] })?.data ?? []

  async function handleDelete(id: string) {
    await api.DELETE("/admin/v1/sandbox-templates/{id}", {
      params: { path: { id } },
    })
    queryClient.invalidateQueries({
      queryKey: ["get", "/admin/v1/sandbox-templates"],
    })
    setDeleteTarget(null)
  }

  function openEditDialog(tpl: Record<string, string>) {
    setEditForm({ name: tpl.name || "" })
    setEditError(null)
    setEditingTemplate({ id: tpl.id! })
  }

  async function handleEdit() {
    if (!editingTemplate) return
    setEditSaving(true)
    setEditError(null)
    try {
      const res = await api.PUT("/admin/v1/sandbox-templates/{id}", {
        params: { path: { id: editingTemplate.id } },
        body: { name: editForm.name },
      })
      if (res.error) {
        const msg = (res.error as { message?: string }).message || "Failed to update template."
        setEditError(msg)
        return
      }
      queryClient.invalidateQueries({
        queryKey: ["get", "/admin/v1/sandbox-templates"],
      })
      setEditingTemplate(null)
    } catch {
      setEditError("An unexpected error occurred.")
    } finally {
      setEditSaving(false)
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Sandbox Templates"
        description="Manage sandbox templates across all organizations."
      />

      <div className="flex items-center gap-4">
        <Tabs
          value={buildStatusFilter}
          onValueChange={(v) => setBuildStatusFilter(v as BuildStatusFilter)}
        >
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="ready">Ready</TabsTrigger>
            <TabsTrigger value="pending">Pending</TabsTrigger>
            <TabsTrigger value="building">Building</TabsTrigger>
            <TabsTrigger value="failed">Failed</TabsTrigger>
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
      ) : templates.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12">
          <p className="text-sm text-muted-foreground">
            No sandbox templates found.
          </p>
        </div>
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Org ID</TableHead>
                <TableHead>External ID</TableHead>
                <TableHead>Build Status</TableHead>
                <TableHead>Build Error</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {templates.map((tpl) => (
                <TableRow key={tpl.id}>
                  <TableCell className="font-medium">
                    {tpl.name || "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {tpl.org_id?.slice(0, 8) ?? "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {tpl.external_id || "--"}
                  </TableCell>
                  <TableCell>
                    {tpl.build_status ? (
                      <StatusBadge status={tpl.build_status} />
                    ) : (
                      "--"
                    )}
                  </TableCell>
                  <TableCell
                    className="max-w-xs truncate text-muted-foreground"
                    title={tpl.build_error || undefined}
                  >
                    {tpl.build_error || "--"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <TimeAgo date={tpl.created_at} />
                  </TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon-sm">
                          ...
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => openEditDialog(tpl)}>
                          Edit
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          className="text-destructive"
                          onClick={() =>
                            setDeleteTarget({
                              id: tpl.id!,
                              name: tpl.name || tpl.id!,
                            })
                          }
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
        open={!!deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete sandbox template</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to permanently delete &quot;
              {deleteTarget?.name}&quot;? This action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => deleteTarget && handleDelete(deleteTarget.id)}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog
        open={!!editingTemplate}
        onOpenChange={(open) => !open && setEditingTemplate(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit sandbox template</DialogTitle>
            <DialogDescription>
              Update the name of this sandbox template.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="edit-template-name">Name</Label>
              <Input
                id="edit-template-name"
                value={editForm.name}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, name: e.target.value }))
                }
                placeholder="Template name"
              />
            </div>
            {editError && (
              <p className="text-sm text-destructive">{editError}</p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setEditingTemplate(null)}
            >
              Cancel
            </Button>
            <Button onClick={handleEdit} disabled={editSaving}>
              {editSaving ? "Saving..." : "Save changes"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
