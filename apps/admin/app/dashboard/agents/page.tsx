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

type StatusFilter = "all" | "active" | "archived"

export default function AgentsPage() {
  const queryClient = useQueryClient()
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all")
  const [orgFilter, setOrgFilter] = useState("")
  const [confirmAction, setConfirmAction] = useState<{
    type: "archive" | "delete"
    id: string
    name: string
  } | null>(null)
  const [editingAgent, setEditingAgent] = useState<{ id: string } | null>(null)
  const [editForm, setEditForm] = useState({
    name: "",
    description: "",
    model: "",
    system_prompt: "",
    sandbox_type: "shared",
    status: "active",
  })
  const [editError, setEditError] = useState<string | null>(null)
  const [editSaving, setEditSaving] = useState(false)

  const queryParams: Record<string, string> = {}
  if (statusFilter !== "all") queryParams.status = statusFilter
  if (orgFilter.trim()) queryParams.org_id = orgFilter.trim()

  const { data, isLoading } = $api.useQuery("get", "/admin/v1/agents", {
    params: { query: queryParams },
  })

  const agents = (data as { data?: Record<string, string>[] })?.data ?? []

  async function handleArchive(id: string) {
    await api.POST("/admin/v1/agents/{id}/archive", {
      params: { path: { id } },
    })
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/agents"] })
    setConfirmAction(null)
  }

  async function handleDelete(id: string) {
    await api.DELETE("/admin/v1/agents/{id}", {
      params: { path: { id } },
    })
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/agents"] })
    setConfirmAction(null)
  }

  function openEditDialog(agent: Record<string, string>) {
    setEditForm({
      name: agent.name || "",
      description: agent.description || "",
      model: agent.model || "",
      system_prompt: agent.system_prompt || "",
      sandbox_type: agent.sandbox_type || "shared",
      status: agent.status || "active",
    })
    setEditError(null)
    setEditingAgent({ id: agent.id! })
  }

  async function handleEdit() {
    if (!editingAgent) return
    setEditSaving(true)
    setEditError(null)
    try {
      const res = await api.PUT("/admin/v1/agents/{id}", {
        params: { path: { id: editingAgent.id } },
        body: {
          name: editForm.name,
          description: editForm.description,
          model: editForm.model,
          system_prompt: editForm.system_prompt,
          sandbox_type: editForm.sandbox_type,
          status: editForm.status,
        },
      })
      if (res.error) {
        const msg = (res.error as { message?: string }).message || "Failed to update agent."
        setEditError(msg)
        return
      }
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/agents"] })
      setEditingAgent(null)
    } catch {
      setEditError("An unexpected error occurred.")
    } finally {
      setEditSaving(false)
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Agents"
        description="Manage agents across all organizations."
      />

      <div className="flex items-center gap-4">
        <Tabs
          value={statusFilter}
          onValueChange={(v) => setStatusFilter(v as StatusFilter)}
        >
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="active">Active</TabsTrigger>
            <TabsTrigger value="archived">Archived</TabsTrigger>
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
      ) : agents.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12">
          <p className="text-sm text-muted-foreground">No agents found.</p>
        </div>
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Model</TableHead>
                <TableHead>Org ID</TableHead>
                <TableHead>Sandbox Type</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {agents.map((agent) => (
                <TableRow key={agent.id}>
                  <TableCell className="font-medium">
                    {agent.name || "--"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {agent.model || "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {agent.org_id?.slice(0, 8) ?? "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                  </TableCell>
                  <TableCell className="capitalize">
                    {agent.sandbox_type || "--"}
                  </TableCell>
                  <TableCell>
                    {agent.status ? (
                      <StatusBadge status={agent.status} />
                    ) : (
                      "--"
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <TimeAgo date={agent.created_at} />
                  </TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon-sm">
                          ...
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem
                          onClick={() => openEditDialog(agent)}
                        >
                          Edit
                        </DropdownMenuItem>
                        {agent.status !== "archived" && (
                          <DropdownMenuItem
                            onClick={() =>
                              setConfirmAction({
                                type: "archive",
                                id: agent.id!,
                                name: agent.name || agent.id!,
                              })
                            }
                          >
                            Archive
                          </DropdownMenuItem>
                        )}
                        <DropdownMenuItem
                          onClick={() =>
                            setConfirmAction({
                              type: "delete",
                              id: agent.id!,
                              name: agent.name || agent.id!,
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
              {confirmAction?.type === "archive"
                ? "Archive agent"
                : "Delete agent"}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmAction?.type === "archive"
                ? `Are you sure you want to archive "${confirmAction.name}"? The agent will be deactivated.`
                : `Are you sure you want to permanently delete "${confirmAction?.name}"? This action cannot be undone.`}
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
                if (confirmAction.type === "archive") {
                  handleArchive(confirmAction.id)
                } else {
                  handleDelete(confirmAction.id)
                }
              }}
            >
              {confirmAction?.type === "archive" ? "Archive" : "Delete"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog
        open={!!editingAgent}
        onOpenChange={(open) => !open && setEditingAgent(null)}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Edit agent</DialogTitle>
            <DialogDescription>
              Update agent configuration and settings.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="edit-agent-name">Name</Label>
              <Input
                id="edit-agent-name"
                value={editForm.name}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, name: e.target.value }))
                }
                placeholder="Agent name"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-agent-description">Description</Label>
              <Input
                id="edit-agent-description"
                value={editForm.description}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, description: e.target.value }))
                }
                placeholder="Agent description"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-agent-model">Model</Label>
              <Input
                id="edit-agent-model"
                value={editForm.model}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, model: e.target.value }))
                }
                placeholder="e.g. gpt-4o"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-agent-system-prompt">System Prompt</Label>
              <Textarea
                id="edit-agent-system-prompt"
                value={editForm.system_prompt}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, system_prompt: e.target.value }))
                }
                placeholder="System prompt for the agent"
                rows={4}
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>Sandbox Type</Label>
                <Select
                  value={editForm.sandbox_type}
                  onValueChange={(v) =>
                    setEditForm((f) => ({ ...f, sandbox_type: v }))
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="dedicated">Dedicated</SelectItem>
                    <SelectItem value="shared">Shared</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>Status</Label>
                <Select
                  value={editForm.status}
                  onValueChange={(v) =>
                    setEditForm((f) => ({ ...f, status: v }))
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="active">Active</SelectItem>
                    <SelectItem value="archived">Archived</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            {editError && (
              <p className="text-sm text-destructive">{editError}</p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setEditingAgent(null)}
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
