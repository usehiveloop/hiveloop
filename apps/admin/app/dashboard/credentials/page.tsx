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

export default function CredentialsPage() {
  const queryClient = useQueryClient()
  const [tab, setTab] = useState("all")
  const [orgFilter, setOrgFilter] = useState("")
  const [providerFilter, setProviderFilter] = useState("")
  const [revokingId, setRevokingId] = useState<string | null>(null)
  const [editingCredential, setEditingCredential] = useState<{
    id: string
    label: string
    identity_id: string
  } | null>(null)
  const [editForm, setEditForm] = useState({ label: "", identity_id: "" })
  const [editError, setEditError] = useState<string | null>(null)
  const [editSaving, setEditSaving] = useState(false)

  const revokedParam = tab === "active" ? "false" : tab === "revoked" ? "true" : undefined

  const { data, isLoading, error } = $api.useQuery("get", "/admin/v1/credentials", {
    params: {
      query: {
        ...(orgFilter ? { org_id: orgFilter } : {}),
        ...(providerFilter ? { provider_id: providerFilter } : {}),
        ...(revokedParam ? { revoked: revokedParam } : {}),
        limit: 50,
      },
    },
  })

  async function handleRevoke(id: string) {
    setRevokingId(id)
    try {
      await api.POST("/admin/v1/credentials/{id}/revoke", {
        params: { path: { id } },
      })
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/credentials"] })
    } finally {
      setRevokingId(null)
    }
  }

  function openEditDialog(cred: { id?: string; label?: string; identity_id?: string }) {
    setEditForm({
      label: cred.label || "",
      identity_id: cred.identity_id || "",
    })
    setEditError(null)
    setEditingCredential({
      id: cred.id!,
      label: cred.label || "",
      identity_id: cred.identity_id || "",
    })
  }

  async function handleEdit() {
    if (!editingCredential) return
    setEditSaving(true)
    setEditError(null)
    try {
      const res = await api.PUT("/admin/v1/credentials/{id}", {
        params: { path: { id: editingCredential.id } },
        body: {
          label: editForm.label,
          identity_id: editForm.identity_id || undefined,
        },
      })
      if (res.error) {
        const msg = (res.error as { message?: string }).message || "Failed to update credential."
        setEditError(msg)
        return
      }
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/credentials"] })
      setEditingCredential(null)
    } catch {
      setEditError("An unexpected error occurred.")
    } finally {
      setEditSaving(false)
    }
  }

  const credentials = data?.data ?? []

  return (
    <div className="space-y-6">
      <PageHeader
        title="Credentials"
        description="Manage encrypted API credentials across all organizations."
      />

      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="active">Active</TabsTrigger>
            <TabsTrigger value="revoked">Revoked</TabsTrigger>
          </TabsList>
        </Tabs>

        <div className="flex items-center gap-2">
          <Input
            placeholder="Filter by Org ID..."
            value={orgFilter}
            onChange={(e) => setOrgFilter(e.target.value)}
            className="w-48"
          />
          <Input
            placeholder="Filter by Provider..."
            value={providerFilter}
            onChange={(e) => setProviderFilter(e.target.value)}
            className="w-48"
          />
        </div>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : error ? (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          Failed to load credentials. Please try again.
        </div>
      ) : credentials.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>No credentials found</EmptyTitle>
            <EmptyDescription>
              {tab !== "all"
                ? "No credentials match the current filter. Try changing the tab or clearing filters."
                : "There are no credentials in the system yet."}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Label</TableHead>
                <TableHead>Provider</TableHead>
                <TableHead>Org ID</TableHead>
                <TableHead>Identity ID</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {credentials.map((cred) => (
                <TableRow key={cred.id}>
                  <TableCell className="font-medium">
                    {cred.label || <span className="text-muted-foreground">--</span>}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {cred.provider_id || "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {cred.org_id ? cred.org_id.slice(0, 8) + "..." : "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {cred.identity_id ? cred.identity_id.slice(0, 8) + "..." : "--"}
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={cred.revoked_at ? "revoked" : "active"} />
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <TimeAgo date={cred.created_at} />
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => openEditDialog(cred)}
                      >
                        Edit
                      </Button>
                      {!cred.revoked_at && (
                        <AlertDialog>
                          <AlertDialogTrigger asChild>
                            <Button
                              variant="destructive"
                              size="sm"
                              disabled={revokingId === cred.id}
                            >
                              {revokingId === cred.id ? "Revoking..." : "Revoke"}
                            </Button>
                          </AlertDialogTrigger>
                          <AlertDialogContent>
                            <AlertDialogHeader>
                              <AlertDialogTitle>Revoke credential</AlertDialogTitle>
                              <AlertDialogDescription>
                                This will permanently revoke this credential. Any tokens or
                                proxied requests using it will stop working immediately. This
                                action cannot be undone.
                              </AlertDialogDescription>
                            </AlertDialogHeader>
                            <AlertDialogFooter>
                              <AlertDialogCancel>Cancel</AlertDialogCancel>
                              <AlertDialogAction
                                variant="destructive"
                                onClick={() => handleRevoke(cred.id!)}
                              >
                                Revoke
                              </AlertDialogAction>
                            </AlertDialogFooter>
                          </AlertDialogContent>
                        </AlertDialog>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <Dialog
        open={!!editingCredential}
        onOpenChange={(open) => !open && setEditingCredential(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit credential</DialogTitle>
            <DialogDescription>
              Update the label or identity assignment for this credential.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="edit-label">Label</Label>
              <Input
                id="edit-label"
                value={editForm.label}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, label: e.target.value }))
                }
                placeholder="Credential label"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-identity-id">Identity ID</Label>
              <Input
                id="edit-identity-id"
                value={editForm.identity_id}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, identity_id: e.target.value }))
                }
                placeholder="Leave empty to unassign"
              />
            </div>
            {editError && (
              <p className="text-sm text-destructive">{editError}</p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setEditingCredential(null)}
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
