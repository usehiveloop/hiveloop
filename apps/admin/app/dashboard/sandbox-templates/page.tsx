"use client"

import { useState } from "react"
import { $api } from "@/lib/api/hooks"
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
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
type ScopeFilter = "all" | "public"

interface CreateForm {
  name: string
  externalId: string
  size: string
}

interface EditForm {
  name: string
  externalId: string
  size: string
}

const TEMPLATE_SIZES = [
  { value: "small", label: "Small (1 CPU, 2GB RAM, 10GB Disk)" },
  { value: "medium", label: "Medium (2 CPU, 4GB RAM, 20GB Disk)" },
  { value: "large", label: "Large (4 CPU, 8GB RAM, 40GB Disk)" },
  { value: "xlarge", label: "XLarge (8 CPU, 16GB RAM, 80GB Disk)" },
]

const QUERY_KEY = ["get", "/admin/v1/sandbox-templates"] as const

export default function SandboxTemplatesPage() {
  const queryClient = useQueryClient()
  const [buildStatusFilter, setBuildStatusFilter] =
    useState<BuildStatusFilter>("all")
  const [scopeFilter, setScopeFilter] = useState<ScopeFilter>("all")
  const [orgFilter, setOrgFilter] = useState("")
  const [deleteTarget, setDeleteTarget] = useState<{
    id: string
    name: string
  } | null>(null)

  // Create dialog
  const [createOpen, setCreateOpen] = useState(false)
  const [createForm, setCreateForm] = useState<CreateForm>({
    name: "",
    externalId: "",
    size: "medium",
  })

  // Edit dialog
  const [editingTemplate, setEditingTemplate] = useState<{ id: string } | null>(
    null
  )
  const [editForm, setEditForm] = useState<EditForm>({
    name: "",
    externalId: "",
    size: "medium",
  })

  const queryParams: Record<string, string> = {}
  if (buildStatusFilter !== "all") queryParams.build_status = buildStatusFilter
  if (scopeFilter === "public") queryParams.scope = "public"
  else if (orgFilter.trim()) queryParams.org_id = orgFilter.trim()

  const { data, isLoading } = $api.useQuery(
    "get",
    "/admin/v1/sandbox-templates",
    { params: { query: queryParams } }
  )

  const templates = (data as { data?: Record<string, string>[] })?.data ?? []

  function invalidateList() {
    queryClient.invalidateQueries({ queryKey: QUERY_KEY })
  }

  const createMutation = $api.useMutation(
    "post",
    "/admin/v1/sandbox-templates",
    {
      onSuccess: () => {
        invalidateList()
        setCreateOpen(false)
        setCreateForm({ name: "", externalId: "", size: "medium" })
      },
    }
  )

  const updateMutation = $api.useMutation(
    "put",
    "/admin/v1/sandbox-templates/{id}",
    {
      onSuccess: () => {
        invalidateList()
        setEditingTemplate(null)
      },
    }
  )

  const deleteMutation = $api.useMutation(
    "delete",
    "/admin/v1/sandbox-templates/{id}",
    {
      onSuccess: () => {
        invalidateList()
        setDeleteTarget(null)
      },
    }
  )

  function handleCreate() {
    createMutation.mutate({
      body: {
        name: createForm.name,
        external_id: createForm.externalId,
        size: createForm.size,
      },
    })
  }

  function openEditDialog(tpl: Record<string, string>) {
    setEditForm({
      name: tpl.name || "",
      externalId: tpl.external_id || "",
      size: tpl.size || "medium",
    })
    updateMutation.reset()
    setEditingTemplate({ id: tpl.id! })
  }

  function handleEdit() {
    if (!editingTemplate) return

    updateMutation.mutate({
      params: { path: { id: editingTemplate.id } },
      body: {
        name: editForm.name,
        external_id: editForm.externalId,
        size: editForm.size,
      },
    })
  }

  function handleDelete(id: string) {
    deleteMutation.mutate({ params: { path: { id } } })
  }

  const createError = createMutation.error
    ? ((createMutation.error as { error?: string }).error ??
      "Failed to create template.")
    : null

  const editError = updateMutation.error
    ? ((updateMutation.error as { error?: string }).error ??
      "Failed to update template.")
    : null

  return (
    <div className="space-y-6">
      <PageHeader
        title="Sandbox Templates"
        description="Register pre-built Daytona snapshots as public templates."
        actions={
          <Button onClick={() => setCreateOpen(true)}>
            Register Public Template
          </Button>
        }
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
        <Tabs
          value={scopeFilter}
          onValueChange={(v) => setScopeFilter(v as ScopeFilter)}
        >
          <TabsList>
            <TabsTrigger value="all">All Scopes</TabsTrigger>
            <TabsTrigger value="public">Public Only</TabsTrigger>
          </TabsList>
        </Tabs>
        {scopeFilter !== "public" && (
          <Input
            placeholder="Filter by org ID..."
            value={orgFilter}
            onChange={(event) => setOrgFilter(event.target.value)}
            className="max-w-xs"
          />
        )}
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, index) => (
            <Skeleton key={index} className="h-12 w-full" />
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
                <TableHead>Size</TableHead>
                <TableHead>Scope</TableHead>
                <TableHead>External ID</TableHead>
                <TableHead>Build Status</TableHead>
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
                  <TableCell className="text-sm text-muted-foreground">
                    {tpl.size || "--"}
                  </TableCell>
                  <TableCell className="text-sm">
                    {tpl.org_id ? (
                      <span
                        className="font-mono text-xs text-muted-foreground"
                        title={tpl.org_id}
                      >
                        {tpl.org_id.slice(0, 8)}
                      </span>
                    ) : (
                      <span className="rounded bg-blue-500/10 px-1.5 py-0.5 text-xs font-medium text-blue-600">
                        Public
                      </span>
                    )}
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

      {/* Register Public Template Dialog */}
      <Dialog
        open={createOpen}
        onOpenChange={(open) => {
          setCreateOpen(open)
          if (!open) createMutation.reset()
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Register public template</DialogTitle>
            <DialogDescription>
              Register a pre-built Daytona snapshot as a public template.
              Build snapshots first with{" "}
              <code className="rounded bg-muted px-1 text-xs">
                make build-templates
              </code>.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="create-name">Name</Label>
              <Input
                id="create-name"
                value={createForm.name}
                onChange={(event) =>
                  setCreateForm((form) => ({
                    ...form,
                    name: event.target.value,
                  }))
                }
                placeholder="e.g. Dev Box Medium"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="create-external-id">
                External ID (Daytona snapshot name)
              </Label>
              <Input
                id="create-external-id"
                value={createForm.externalId}
                onChange={(event) =>
                  setCreateForm((form) => ({
                    ...form,
                    externalId: event.target.value,
                  }))
                }
                placeholder="e.g. zira-dev-box-medium-v0.10.0"
                className="font-mono"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="create-size">Size</Label>
              <Select
                value={createForm.size}
                onValueChange={(value) =>
                  setCreateForm((form) => ({ ...form, size: value }))
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TEMPLATE_SIZES.map((size) => (
                    <SelectItem key={size.value} value={size.value}>
                      {size.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {createError && (
              <p className="text-sm text-destructive">{createError}</p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleCreate}
              disabled={createMutation.isPending}
            >
              {createMutation.isPending ? "Registering..." : "Register Template"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Template Dialog */}
      <Dialog
        open={!!editingTemplate}
        onOpenChange={(open) => {
          if (!open) {
            setEditingTemplate(null)
            updateMutation.reset()
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit sandbox template</DialogTitle>
            <DialogDescription>
              Update the template details.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="edit-template-name">Name</Label>
              <Input
                id="edit-template-name"
                value={editForm.name}
                onChange={(event) =>
                  setEditForm((form) => ({
                    ...form,
                    name: event.target.value,
                  }))
                }
                placeholder="Template name"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-external-id">
                External ID (Daytona snapshot name)
              </Label>
              <Input
                id="edit-external-id"
                value={editForm.externalId}
                onChange={(event) =>
                  setEditForm((form) => ({
                    ...form,
                    externalId: event.target.value,
                  }))
                }
                className="font-mono"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-size">Size</Label>
              <Select
                value={editForm.size}
                onValueChange={(value) =>
                  setEditForm((form) => ({ ...form, size: value }))
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TEMPLATE_SIZES.map((size) => (
                    <SelectItem key={size.value} value={size.value}>
                      {size.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
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
            <Button
              onClick={handleEdit}
              disabled={updateMutation.isPending}
            >
              {updateMutation.isPending ? "Saving..." : "Save changes"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation */}
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
              disabled={deleteMutation.isPending}
              onClick={() => deleteTarget && handleDelete(deleteTarget.id)}
            >
              {deleteMutation.isPending ? "Deleting..." : "Delete"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
