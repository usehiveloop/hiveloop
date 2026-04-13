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
import { Textarea } from "@/components/ui/textarea"
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
  buildCommands: string
  size: string
}

interface EditForm {
  name: string
  buildCommands: string
  size: string
}

const TEMPLATE_SIZES = [
  { value: "small", label: "Small (1 CPU, 2GB RAM, 10GB Disk)" },
  { value: "medium", label: "Medium (2 CPU, 4GB RAM, 20GB Disk)" },
  { value: "large", label: "Large (4 CPU, 8GB RAM, 40GB Disk)" },
  { value: "xlarge", label: "XLarge (8 CPU, 16GB RAM, 80GB Disk)" },
]

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
    buildCommands: "",
    size: "medium",
  })
  const [createError, setCreateError] = useState<string | null>(null)
  const [createSaving, setCreateSaving] = useState(false)

  // Edit dialog
  const [editingTemplate, setEditingTemplate] = useState<{ id: string } | null>(
    null
  )
  const [editForm, setEditForm] = useState<EditForm>({
    name: "",
    buildCommands: "",
    size: "medium",
  })
  const [editError, setEditError] = useState<string | null>(null)
  const [editSaving, setEditSaving] = useState(false)

  // Build logs dialog
  const [logsTemplate, setLogsTemplate] = useState<{
    id: string
    name: string
    logs: string
  } | null>(null)

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
    queryClient.invalidateQueries({
      queryKey: ["get", "/admin/v1/sandbox-templates"],
    })
  }

  async function handleCreate() {
    setCreateSaving(true)
    setCreateError(null)
    try {
      const buildCommands = createForm.buildCommands
        .split("\n")
        .filter((line: string) => line.trim() !== "")

      const res = await api.POST("/admin/v1/sandbox-templates", {
        body: {
          name: createForm.name,
          build_commands: buildCommands,
          size: createForm.size,
        },
      })
      if (res.error) {
        const msg =
          (res.error as { error?: string }).error || "Failed to create template."
        setCreateError(msg)
        return
      }
      invalidateList()
      setCreateOpen(false)
      setCreateForm({ name: "", buildCommands: "", size: "medium" })
    } catch {
      setCreateError("An unexpected error occurred.")
    } finally {
      setCreateSaving(false)
    }
  }

  function openEditDialog(tpl: Record<string, string>) {
    setEditForm({
      name: tpl.name || "",
      buildCommands: tpl.build_commands || "",
      size: tpl.size || "medium",
    })
    setEditError(null)
    setEditingTemplate({ id: tpl.id! })
  }

  async function handleEdit() {
    if (!editingTemplate) return
    setEditSaving(true)
    setEditError(null)
    try {
      const buildCommands = editForm.buildCommands
        .split("\n")
        .filter((line: string) => line.trim() !== "")

      const res = await api.PUT("/admin/v1/sandbox-templates/{id}", {
        params: { path: { id: editingTemplate.id } },
        body: {
          name: editForm.name,
          size: editForm.size,
          build_commands: buildCommands,
        },
      })
      if (res.error) {
        const msg =
          (res.error as { error?: string }).error ||
          "Failed to update template."
        setEditError(msg)
        return
      }
      invalidateList()
      setEditingTemplate(null)
    } catch {
      setEditError("An unexpected error occurred.")
    } finally {
      setEditSaving(false)
    }
  }

  async function handleDelete(id: string) {
    await api.DELETE("/admin/v1/sandbox-templates/{id}", {
      params: { path: { id } },
    })
    invalidateList()
    setDeleteTarget(null)
  }

  async function handleBuild(id: string) {
    await api.POST("/admin/v1/sandbox-templates/{id}/build", {
      params: { path: { id } },
    })
    invalidateList()
  }

  async function handleRetry(id: string) {
    await api.POST("/admin/v1/sandbox-templates/{id}/retry", {
      params: { path: { id } },
    })
    invalidateList()
  }

  async function handleViewLogs(tpl: Record<string, string>) {
    const res = await api.GET("/admin/v1/sandbox-templates/{id}", {
      params: { path: { id: tpl.id! } },
    })
    const detail = res.data as Record<string, string> | undefined
    setLogsTemplate({
      id: tpl.id!,
      name: tpl.name || tpl.id!,
      logs: detail?.build_logs || "(no logs)",
    })
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Sandbox Templates"
        description="Manage sandbox templates across all organizations."
        actions={
          <Button onClick={() => setCreateOpen(true)}>
            Create Public Template
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
            onChange={(e) => setOrgFilter(e.target.value)}
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
                        {(tpl.build_status === "pending" ||
                          tpl.build_status === "ready") && (
                          <DropdownMenuItem
                            onClick={() => handleBuild(tpl.id!)}
                          >
                            Build
                          </DropdownMenuItem>
                        )}
                        {tpl.build_status === "failed" && (
                          <DropdownMenuItem
                            onClick={() => handleRetry(tpl.id!)}
                          >
                            Retry Build
                          </DropdownMenuItem>
                        )}
                        <DropdownMenuItem
                          onClick={() => handleViewLogs(tpl)}
                        >
                          View Logs
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

      {/* Create Public Template Dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create public template</DialogTitle>
            <DialogDescription>
              Create a platform-wide sandbox template available to all
              organizations.
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
                placeholder="e.g. Python ML Environment"
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
            <div className="space-y-2">
              <Label htmlFor="create-commands">
                Build Commands (one per line)
              </Label>
              <Textarea
                id="create-commands"
                value={createForm.buildCommands}
                onChange={(event) =>
                  setCreateForm((form) => ({
                    ...form,
                    buildCommands: event.target.value,
                  }))
                }
                placeholder={"apt-get install -y python3\npip install numpy"}
                rows={6}
              />
            </div>
            {createError && (
              <p className="text-sm text-destructive">{createError}</p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button onClick={handleCreate} disabled={createSaving}>
              {createSaving ? "Creating..." : "Create Template"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Template Dialog */}
      <Dialog
        open={!!editingTemplate}
        onOpenChange={(open) => !open && setEditingTemplate(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit sandbox template</DialogTitle>
            <DialogDescription>
              Update the template details. Changing build commands will reset the
              build status.
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
            <div className="space-y-2">
              <Label htmlFor="edit-commands">
                Build Commands (one per line)
              </Label>
              <Textarea
                id="edit-commands"
                value={editForm.buildCommands}
                onChange={(event) =>
                  setEditForm((form) => ({
                    ...form,
                    buildCommands: event.target.value,
                  }))
                }
                rows={6}
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
              onClick={() => deleteTarget && handleDelete(deleteTarget.id)}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Build Logs Dialog */}
      <Dialog
        open={!!logsTemplate}
        onOpenChange={(open) => !open && setLogsTemplate(null)}
      >
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              Build Logs: {logsTemplate?.name}
            </DialogTitle>
          </DialogHeader>
          <div className="max-h-96 overflow-auto rounded bg-muted p-4">
            <pre className="whitespace-pre-wrap font-mono text-xs">
              {logsTemplate?.logs}
            </pre>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setLogsTemplate(null)}
            >
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
