"use client"

import { useState, useCallback } from "react"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { useQueryClient } from "@tanstack/react-query"
import { PageHeader } from "@/components/admin/page-header"
import { StatusBadge } from "@/components/admin/status-badge"
import { TimeAgo } from "@/components/admin/time-ago"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Card, CardContent } from "@/components/ui/card"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Separator } from "@/components/ui/separator"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Switch } from "@/components/ui/switch"
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
  DropdownMenuSeparator,
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"

type FilterTab = "all" | "active" | "inactive"

function getQueryParams(tab: FilterTab, search: string) {
  const params: {
    search?: string
    active?: string
    limit?: number
  } = { limit: 50 }

  if (search.trim()) params.search = search.trim()

  switch (tab) {
    case "active":
      params.active = "true"
      break
    case "inactive":
      params.active = "false"
      break
  }

  return params
}

function formatNumber(value: number | undefined): string {
  if (value == null) return "0"
  return new Intl.NumberFormat("en-US").format(value)
}

function TableSkeleton() {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Rate Limit</TableHead>
          <TableHead>Created</TableHead>
          <TableHead className="w-12" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {Array.from({ length: 8 }).map((_, i) => (
          <TableRow key={i}>
            <TableCell><Skeleton className="h-4 w-36" /></TableCell>
            <TableCell><Skeleton className="h-5 w-16" /></TableCell>
            <TableCell><Skeleton className="h-4 w-20" /></TableCell>
            <TableCell><Skeleton className="h-4 w-20" /></TableCell>
            <TableCell><Skeleton className="h-8 w-8" /></TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function OrgDetailSkeleton() {
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="space-y-1">
            <Skeleton className="h-3 w-20" />
            <Skeleton className="h-5 w-28" />
          </div>
        ))}
      </div>
    </div>
  )
}

function OrgDetailDialog({
  orgId,
  open,
  onClose,
}: {
  orgId: string | null
  open: boolean
  onClose: () => void
}) {
  const { data, isLoading, error } = $api.useQuery(
    "get",
    "/admin/v1/orgs/{id}",
    { params: { path: { id: orgId! } } },
    { enabled: !!orgId }
  )

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{data?.name ?? "Organization Details"}</DialogTitle>
          <DialogDescription>
            Detailed information about this organization.
          </DialogDescription>
        </DialogHeader>

        {isLoading ? (
          <OrgDetailSkeleton />
        ) : error ? (
          <p className="py-4 text-center text-sm text-destructive">
            Failed to load organization details.
          </p>
        ) : data ? (
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-x-6 gap-y-3">
              <DetailItem label="Status">
                <StatusBadge status={data.active ? "active" : "stopped"} />
              </DetailItem>
              <DetailItem label="Rate Limit">
                {data.rate_limit ? `${formatNumber(data.rate_limit)} req/min` : "Unlimited"}
              </DetailItem>
              <DetailItem label="Members">
                {formatNumber(data.member_count)}
              </DetailItem>
              <DetailItem label="Credentials">
                {formatNumber(data.credential_count)}
              </DetailItem>
              <DetailItem label="Agents">
                {formatNumber(data.agent_count)}
              </DetailItem>
              <DetailItem label="Sandboxes">
                {formatNumber(data.sandbox_count)}
              </DetailItem>
            </div>

            {data.allowed_origins && data.allowed_origins.length > 0 && (
              <>
                <Separator />
                <div className="space-y-1.5">
                  <p className="text-xs font-medium text-muted-foreground">
                    Allowed Origins
                  </p>
                  <div className="flex flex-wrap gap-1.5">
                    {data.allowed_origins.map((origin) => (
                      <span
                        key={origin}
                        className="rounded-md bg-muted px-2 py-0.5 text-xs font-mono"
                      >
                        {origin}
                      </span>
                    ))}
                  </div>
                </div>
              </>
            )}

            <Separator />
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>Created: <TimeAgo date={data.created_at} /></span>
              <span>Updated: <TimeAgo date={data.updated_at} /></span>
            </div>
          </div>
        ) : null}

        <DialogFooter showCloseButton />
      </DialogContent>
    </Dialog>
  )
}

function DetailItem({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="space-y-0.5">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <div className="text-sm font-medium">{children}</div>
    </div>
  )
}

export default function OrgsPage() {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState("")
  const [debouncedSearch, setDebouncedSearch] = useState("")
  const [tab, setTab] = useState<FilterTab>("all")
  const [mutating, setMutating] = useState<string | null>(null)

  // Detail dialog
  const [detailOrgId, setDetailOrgId] = useState<string | null>(null)

  // Delete confirm
  const [deleteOrg, setDeleteOrg] = useState<{ id: string; name: string } | null>(null)

  // Edit dialog
  const [editingOrg, setEditingOrg] = useState<{
    id: string
    name: string
    rate_limit: number
    active: boolean
    allowed_origins: string[]
  } | null>(null)
  const [editForm, setEditForm] = useState({
    name: "",
    rate_limit: 0,
    active: true,
    allowed_origins: "",
  })
  const [editError, setEditError] = useState<string | null>(null)
  const [editSaving, setEditSaving] = useState(false)

  // Debounce search
  const [timer, setTimer] = useState<ReturnType<typeof setTimeout> | null>(null)
  const handleSearchChange = useCallback(
    (value: string) => {
      setSearch(value)
      if (timer) clearTimeout(timer)
      const t = setTimeout(() => setDebouncedSearch(value), 300)
      setTimer(t)
    },
    [timer]
  )

  const queryParams = getQueryParams(tab, debouncedSearch)
  const { data, isLoading, error } = $api.useQuery("get", "/admin/v1/orgs", {
    params: { query: queryParams },
  })

  const orgs = data?.data ?? []

  async function handleActivate(orgId: string) {
    setMutating(orgId)
    try {
      await api.POST("/admin/v1/orgs/{id}/activate", {
        params: { path: { id: orgId } },
      })
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/orgs"] })
    } finally {
      setMutating(null)
    }
  }

  async function handleDeactivate(orgId: string) {
    setMutating(orgId)
    try {
      await api.POST("/admin/v1/orgs/{id}/deactivate", {
        params: { path: { id: orgId } },
      })
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/orgs"] })
    } finally {
      setMutating(null)
    }
  }

  async function handleDelete() {
    if (!deleteOrg) return
    setMutating(deleteOrg.id)
    try {
      await api.DELETE("/admin/v1/orgs/{id}", {
        params: { path: { id: deleteOrg.id } },
      })
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/orgs"] })
    } finally {
      setMutating(null)
      setDeleteOrg(null)
    }
  }

  function openEditDialog(org: {
    id?: string
    name?: string
    rate_limit?: number
    active?: boolean
    allowed_origins?: string[]
  }) {
    setEditForm({
      name: org.name || "",
      rate_limit: org.rate_limit || 0,
      active: org.active !== false,
      allowed_origins: (org.allowed_origins || []).join("\n"),
    })
    setEditError(null)
    setEditingOrg({
      id: org.id!,
      name: org.name || "",
      rate_limit: org.rate_limit || 0,
      active: org.active !== false,
      allowed_origins: org.allowed_origins || [],
    })
  }

  async function handleEditOrg() {
    if (!editingOrg) return
    setEditSaving(true)
    setEditError(null)
    try {
      const origins = editForm.allowed_origins
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean)
      const res = await api.PUT("/admin/v1/orgs/{id}", {
        params: { path: { id: editingOrg.id } },
        body: {
          name: editForm.name,
          rate_limit: editForm.rate_limit,
          active: editForm.active,
          allowed_origins: origins,
        },
      })
      if (res.error) {
        const msg = (res.error as { error?: string }).error || "Failed to update organization."
        setEditError(msg)
        return
      }
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/orgs"] })
      setEditingOrg(null)
    } catch {
      setEditError("An unexpected error occurred.")
    } finally {
      setEditSaving(false)
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Organizations"
        description="Manage all platform organizations."
      />

      {/* Filters */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <Tabs
          value={tab}
          onValueChange={(v) => setTab(v as FilterTab)}
        >
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="active">Active</TabsTrigger>
            <TabsTrigger value="inactive">Inactive</TabsTrigger>
          </TabsList>
        </Tabs>
        <Input
          placeholder="Search by name..."
          value={search}
          onChange={(e) => handleSearchChange(e.target.value)}
          className="sm:max-w-xs"
        />
      </div>

      {/* Table */}
      {isLoading ? (
        <TableSkeleton />
      ) : error ? (
        <Card>
          <CardContent className="py-8 text-center text-sm text-destructive">
            Failed to load organizations.
          </CardContent>
        </Card>
      ) : orgs.length === 0 ? (
        <Card>
          <CardContent className="py-12 text-center text-sm text-muted-foreground">
            {debouncedSearch || tab !== "all"
              ? "No organizations match the current filters."
              : "No organizations found."}
          </CardContent>
        </Card>
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Rate Limit</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {orgs.map((org) => {
                const isActive = org.active !== false
                return (
                  <TableRow key={org.id}>
                    <TableCell>
                      <button
                        className="font-medium text-foreground underline-offset-4 hover:underline"
                        onClick={() => setDetailOrgId(org.id!)}
                      >
                        {org.name}
                      </button>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={isActive ? "active" : "stopped"} />
                    </TableCell>
                    <TableCell>
                      {org.rate_limit
                        ? `${formatNumber(org.rate_limit)} req/min`
                        : <span className="text-muted-foreground">Unlimited</span>}
                    </TableCell>
                    <TableCell>
                      <TimeAgo date={org.created_at} />
                    </TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={mutating === org.id}
                          >
                            {mutating === org.id ? "..." : "Actions"}
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            onClick={() => setDetailOrgId(org.id!)}
                          >
                            View Details
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() => openEditDialog(org)}
                          >
                            Edit
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          {isActive ? (
                            <DropdownMenuItem
                              onClick={() => handleDeactivate(org.id!)}
                            >
                              Deactivate
                            </DropdownMenuItem>
                          ) : (
                            <DropdownMenuItem
                              onClick={() => handleActivate(org.id!)}
                            >
                              Activate
                            </DropdownMenuItem>
                          )}
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() =>
                              setDeleteOrg({
                                id: org.id!,
                                name: org.name!,
                              })
                            }
                          >
                            Delete Organization
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}

      {data?.has_more && (
        <p className="text-center text-xs text-muted-foreground">
          Showing first {orgs.length} results. Refine your search for more.
        </p>
      )}

      {/* Org Detail Dialog */}
      <OrgDetailDialog
        orgId={detailOrgId}
        open={!!detailOrgId}
        onClose={() => setDetailOrgId(null)}
      />

      {/* Edit Org Dialog */}
      <Dialog
        open={!!editingOrg}
        onOpenChange={(open) => !open && setEditingOrg(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit Organization</DialogTitle>
            <DialogDescription>
              Update the organization settings.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="edit-org-name">Name</Label>
              <Input
                id="edit-org-name"
                value={editForm.name}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, name: e.target.value }))
                }
                placeholder="Organization name"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-org-rate-limit">Rate Limit (req/min)</Label>
              <Input
                id="edit-org-rate-limit"
                type="number"
                min={0}
                value={editForm.rate_limit}
                onChange={(e) =>
                  setEditForm((f) => ({
                    ...f,
                    rate_limit: parseInt(e.target.value, 10) || 0,
                  }))
                }
                placeholder="0 for unlimited"
              />
            </div>
            <div className="flex items-center justify-between">
              <Label htmlFor="edit-org-active">Active</Label>
              <Switch
                id="edit-org-active"
                checked={editForm.active}
                onCheckedChange={(checked) =>
                  setEditForm((f) => ({ ...f, active: checked }))
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-org-origins">Allowed Origins (one per line)</Label>
              <Textarea
                id="edit-org-origins"
                value={editForm.allowed_origins}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, allowed_origins: e.target.value }))
                }
                placeholder={"https://example.com\nhttps://app.example.com"}
                rows={3}
              />
            </div>
            {editError && (
              <p className="text-sm text-destructive">{editError}</p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setEditingOrg(null)}
            >
              Cancel
            </Button>
            <Button onClick={handleEditOrg} disabled={editSaving}>
              {editSaving ? "Saving..." : "Save changes"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirm */}
      <AlertDialog
        open={!!deleteOrg}
        onOpenChange={(open) => {
          if (!open) setDeleteOrg(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Organization</AlertDialogTitle>
            <AlertDialogDescription>
              Permanently delete{" "}
              <span className="font-medium text-foreground">{deleteOrg?.name}</span> and all
              associated data including credentials, agents, and sandboxes. This action cannot
              be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={handleDelete}
              disabled={mutating === deleteOrg?.id}
            >
              {mutating === deleteOrg?.id ? "Deleting..." : "Delete"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
