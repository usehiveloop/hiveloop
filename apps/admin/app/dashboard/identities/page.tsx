"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { PageHeader } from "@/components/admin/page-header"
import { TimeAgo } from "@/components/admin/time-ago"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
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
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"

function truncateJson(value: unknown, maxLen = 60): string {
  if (value === null || value === undefined) return "--"
  const str = typeof value === "string" ? value : JSON.stringify(value)
  if (str.length <= maxLen) return str
  return str.slice(0, maxLen) + "..."
}

export default function IdentitiesPage() {
  const queryClient = useQueryClient()
  const [orgFilter, setOrgFilter] = useState("")
  const [externalIdFilter, setExternalIdFilter] = useState("")
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const [editingIdentity, setEditingIdentity] = useState<{
    id: string
    external_id: string
    meta: string
  } | null>(null)
  const [editForm, setEditForm] = useState({ external_id: "", meta: "" })
  const [editError, setEditError] = useState<string | null>(null)
  const [editSaving, setEditSaving] = useState(false)

  const { data, isLoading, error } = $api.useQuery("get", "/admin/v1/identities", {
    params: {
      query: {
        ...(orgFilter ? { org_id: orgFilter } : {}),
        ...(externalIdFilter ? { external_id: externalIdFilter } : {}),
        limit: 50,
      },
    },
  })

  async function handleDelete(id: string) {
    setDeletingId(id)
    try {
      await api.DELETE("/admin/v1/identities/{id}", {
        params: { path: { id } },
      })
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/identities"] })
    } finally {
      setDeletingId(null)
    }
  }

  function openEditDialog(identity: { id?: string; external_id?: string; meta?: unknown }) {
    const metaStr =
      identity.meta === null || identity.meta === undefined
        ? ""
        : typeof identity.meta === "string"
          ? identity.meta
          : JSON.stringify(identity.meta, null, 2)
    setEditForm({
      external_id: identity.external_id || "",
      meta: metaStr,
    })
    setEditError(null)
    setEditingIdentity({
      id: identity.id!,
      external_id: identity.external_id || "",
      meta: metaStr,
    })
  }

  async function handleEdit() {
    if (!editingIdentity) return
    setEditSaving(true)
    setEditError(null)

    let parsedMeta: Record<string, unknown> | undefined = undefined
    if (editForm.meta.trim()) {
      try {
        parsedMeta = JSON.parse(editForm.meta) as Record<string, unknown>
      } catch {
        setEditError("Meta must be valid JSON.")
        setEditSaving(false)
        return
      }
    }

    try {
      const res = await api.PUT("/admin/v1/identities/{id}", {
        params: { path: { id: editingIdentity.id } },
        body: {
          external_id: editForm.external_id,
          meta: parsedMeta,
        },
      })
      if (res.error) {
        const msg = (res.error as { message?: string }).message || "Failed to update identity."
        setEditError(msg)
        return
      }
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/identities"] })
      setEditingIdentity(null)
    } catch {
      setEditError("An unexpected error occurred.")
    } finally {
      setEditSaving(false)
    }
  }

  const identities = data?.data ?? []

  return (
    <div className="space-y-6">
      <PageHeader
        title="Identities"
        description="Manage end-user identities across all organizations."
      />

      <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
        <Input
          placeholder="Search by External ID..."
          value={externalIdFilter}
          onChange={(e) => setExternalIdFilter(e.target.value)}
          className="w-64"
        />
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
          Failed to load identities. Please try again.
        </div>
      ) : identities.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>No identities found</EmptyTitle>
            <EmptyDescription>
              {externalIdFilter || orgFilter
                ? "No identities match the current filters. Try clearing your search."
                : "There are no identities in the system yet."}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>External ID</TableHead>
                <TableHead>Org ID</TableHead>
                <TableHead>Meta</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {identities.map((identity) => (
                <TableRow key={identity.id}>
                  <TableCell className="font-mono text-xs">
                    {identity.external_id || "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {identity.org_id
                      ? identity.org_id.slice(0, 8) + "..."
                      : "--"}
                  </TableCell>
                  <TableCell className="max-w-xs">
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className="cursor-default font-mono text-xs text-muted-foreground">
                            {truncateJson(identity.meta)}
                          </span>
                        </TooltipTrigger>
                        {identity.meta && (
                          <TooltipContent side="bottom" className="max-w-sm">
                            <pre className="whitespace-pre-wrap text-xs">
                              {typeof identity.meta === "string"
                                ? identity.meta
                                : JSON.stringify(identity.meta, null, 2)}
                            </pre>
                          </TooltipContent>
                        )}
                      </Tooltip>
                    </TooltipProvider>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <TimeAgo date={identity.created_at} />
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => openEditDialog(identity)}
                      >
                        Edit
                      </Button>
                      <AlertDialog>
                        <AlertDialogTrigger asChild>
                          <Button
                            variant="destructive"
                            size="sm"
                            disabled={deletingId === identity.id}
                          >
                            {deletingId === identity.id ? "Deleting..." : "Delete"}
                          </Button>
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>Delete identity</AlertDialogTitle>
                            <AlertDialogDescription>
                              This will permanently delete this identity and cascade to all
                              related data including credentials, tokens, and sandbox
                              associations. This action cannot be undone.
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>Cancel</AlertDialogCancel>
                            <AlertDialogAction
                              variant="destructive"
                              onClick={() => handleDelete(identity.id!)}
                            >
                              Delete
                            </AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <Dialog
        open={!!editingIdentity}
        onOpenChange={(open) => !open && setEditingIdentity(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit identity</DialogTitle>
            <DialogDescription>
              Update the external ID or metadata for this identity.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="edit-external-id">External ID</Label>
              <Input
                id="edit-external-id"
                value={editForm.external_id}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, external_id: e.target.value }))
                }
                placeholder="External ID"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-meta">Meta (JSON)</Label>
              <Textarea
                id="edit-meta"
                value={editForm.meta}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, meta: e.target.value }))
                }
                placeholder='{"key": "value"}'
                rows={6}
                className="font-mono text-xs"
              />
            </div>
            {editError && (
              <p className="text-sm text-destructive">{editError}</p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setEditingIdentity(null)}
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
