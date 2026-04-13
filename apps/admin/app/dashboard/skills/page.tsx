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
import { Badge } from "@/components/ui/badge"
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

type StatusFilter = "all" | "draft" | "published" | "archived"
type ScopeFilter = "all" | "global"

export default function SkillsPage() {
  const queryClient = useQueryClient()
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all")
  const [scopeFilter, setScopeFilter] = useState<ScopeFilter>("all")
  const [search, setSearch] = useState("")
  const [confirmDelete, setConfirmDelete] = useState<{ id: string; name: string } | null>(null)

  // Create dialog
  const [createOpen, setCreateOpen] = useState(false)
  const [createForm, setCreateForm] = useState({
    name: "",
    description: "",
    source_type: "git" as "inline" | "git",
    repo_url: "",
    repo_subpath: "",
    repo_ref: "main",
    tags: "",
    featured: false,
  })
  const [createError, setCreateError] = useState<string | null>(null)
  const [createSaving, setCreateSaving] = useState(false)

  // Edit dialog
  const [editingSkill, setEditingSkill] = useState<{ id: string } | null>(null)
  const [editForm, setEditForm] = useState({
    name: "",
    description: "",
    status: "published",
    featured: false,
    tags: "",
    repo_ref: "main",
  })
  const [editError, setEditError] = useState<string | null>(null)
  const [editSaving, setEditSaving] = useState(false)

  const queryParams: Record<string, string> = {}
  if (statusFilter !== "all") queryParams.status = statusFilter
  if (scopeFilter === "global") queryParams.scope = "global"
  if (search.trim()) queryParams.q = search.trim()

  const { data, isLoading } = $api.useQuery("get", "/admin/v1/skills", {
    params: { query: queryParams },
  })

  const skills = (data)?.data ?? []

  async function handleDelete(id: string) {
    await api.DELETE("/admin/v1/skills/{id}", {
      params: { path: { id } },
    })
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/skills"] })
    setConfirmDelete(null)
  }

  async function handleCreate() {
    setCreateSaving(true)
    setCreateError(null)
    try {
      const tags = createForm.tags.split(",").map((tag) => tag.trim()).filter(Boolean)
      const body: Record<string, unknown> = {
        name: createForm.name,
        description: createForm.description || undefined,
        source_type: createForm.source_type,
        tags,
        featured: createForm.featured,
      }
      if (createForm.source_type === "git") {
        body.repo_url = createForm.repo_url
        body.repo_subpath = createForm.repo_subpath || undefined
        body.repo_ref = createForm.repo_ref || "main"
      }
      const res = await api.POST("/admin/v1/skills", { body: body as never })
      if (res.error) {
        const errorData = res.error as { error?: string }
        setCreateError(errorData.error || "Failed to create skill.")
        return
      }
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/skills"] })
      setCreateOpen(false)
      setCreateForm({
        name: "",
        description: "",
        source_type: "git",
        repo_url: "",
        repo_subpath: "",
        repo_ref: "main",
        tags: "",
        featured: false,
      })
    } catch {
      setCreateError("An unexpected error occurred.")
    } finally {
      setCreateSaving(false)
    }
  }

  function openEditDialog(skill: Record<string, unknown>) {
    const tags = Array.isArray(skill.tags) ? (skill.tags as string[]).join(", ") : ""
    setEditForm({
      name: (skill.name as string) || "",
      description: (skill.description as string) || "",
      status: (skill.status as string) || "published",
      featured: (skill.featured as boolean) || false,
      tags,
      repo_ref: (skill.repo_ref as string) || "main",
    })
    setEditError(null)
    setEditingSkill({ id: skill.id as string })
  }

  async function handleEdit() {
    if (!editingSkill) return
    setEditSaving(true)
    setEditError(null)
    try {
      const tags = editForm.tags.split(",").map((tag) => tag.trim()).filter(Boolean)
      const res = await api.PUT("/admin/v1/skills/{id}", {
        params: { path: { id: editingSkill.id } },
        body: {
          name: editForm.name,
          description: editForm.description,
          status: editForm.status,
          featured: editForm.featured,
          tags,
          repo_ref: editForm.repo_ref,
        } as never,
      })
      if (res.error) {
        const errorData = res.error as { error?: string }
        setEditError(errorData.error || "Failed to update skill.")
        return
      }
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/skills"] })
      setEditingSkill(null)
    } catch {
      setEditError("An unexpected error occurred.")
    } finally {
      setEditSaving(false)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <PageHeader
          title="Skills"
          description="Manage global and org-scoped skills."
        />
        <Button onClick={() => setCreateOpen(true)}>Create Global Skill</Button>
      </div>

      <div className="flex items-center gap-4">
        <Tabs
          value={statusFilter}
          onValueChange={(value) => setStatusFilter(value as StatusFilter)}
        >
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="draft">Draft</TabsTrigger>
            <TabsTrigger value="published">Published</TabsTrigger>
            <TabsTrigger value="archived">Archived</TabsTrigger>
          </TabsList>
        </Tabs>
        <Tabs
          value={scopeFilter}
          onValueChange={(value) => setScopeFilter(value as ScopeFilter)}
        >
          <TabsList>
            <TabsTrigger value="all">All Scopes</TabsTrigger>
            <TabsTrigger value="global">Global Only</TabsTrigger>
          </TabsList>
        </Tabs>
        <Input
          placeholder="Search skills..."
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          className="max-w-xs"
        />
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, index) => (
            <Skeleton key={index} className="h-12 w-full" />
          ))}
        </div>
      ) : skills.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12">
          <p className="text-sm text-muted-foreground">No skills found.</p>
        </div>
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Source</TableHead>
                <TableHead>Scope</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Tags</TableHead>
                <TableHead>Installs</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {skills.map((skill) => (
                <TableRow key={skill.id as string}>
                  <TableCell>
                    <div>
                      <p className="font-medium">{skill.name as string}</p>
                      <p className="text-xs text-muted-foreground truncate max-w-xs">
                        {(skill.description as string) || "No description"}
                      </p>
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant={skill.source_type === "git" ? "default" : "secondary"}>
                      {skill.source_type as string}
                    </Badge>
                    {skill.source_type === "git" && skill.repo_url && (
                      <p className="text-xs text-muted-foreground mt-1 truncate max-w-xs">
                        {skill.repo_url as string}
                      </p>
                    )}
                  </TableCell>
                  <TableCell>
                    {skill.org_id ? (
                      <span className="text-xs text-muted-foreground font-mono">{(skill.org_id as string).slice(0, 8)}...</span>
                    ) : (
                      <Badge variant="outline">Global</Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={skill.status as string} />
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {Array.isArray(skill.tags) && (skill.tags as string[]).map((tag) => (
                        <Badge key={tag} variant="secondary" className="text-xs">
                          {tag}
                        </Badge>
                      ))}
                      {skill.featured && (
                        <Badge variant="default" className="text-xs">Featured</Badge>
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {skill.install_count as number}
                  </TableCell>
                  <TableCell>
                    <TimeAgo date={skill.created_at as string} />
                  </TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="sm">...</Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => openEditDialog(skill)}>Edit</DropdownMenuItem>
                        <DropdownMenuItem
                          className="text-destructive"
                          onClick={() => setConfirmDelete({ id: skill.id as string, name: skill.name as string })}
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

      {/* Create Dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Create Global Skill</DialogTitle>
            <DialogDescription>
              Global skills are visible to all organizations.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Name</Label>
              <Input
                value={createForm.name}
                onChange={(event) => setCreateForm({ ...createForm, name: event.target.value })}
                placeholder="use-railway"
              />
            </div>
            <div className="space-y-2">
              <Label>Description</Label>
              <Textarea
                value={createForm.description}
                onChange={(event) => setCreateForm({ ...createForm, description: event.target.value })}
                placeholder="What this skill does..."
                className="min-h-20"
              />
            </div>
            <div className="space-y-2">
              <Label>Source Type</Label>
              <Select
                value={createForm.source_type}
                onValueChange={(value) => setCreateForm({ ...createForm, source_type: value as "inline" | "git" })}
              >
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="git">Git Repository</SelectItem>
                  <SelectItem value="inline">Inline</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {createForm.source_type === "git" && (
              <>
                <div className="space-y-2">
                  <Label>Repository URL</Label>
                  <Input
                    value={createForm.repo_url}
                    onChange={(event) => setCreateForm({ ...createForm, repo_url: event.target.value })}
                    placeholder="https://github.com/railwayapp/railway-skills"
                  />
                </div>
                <div className="space-y-2">
                  <Label>Subpath (optional)</Label>
                  <Input
                    value={createForm.repo_subpath}
                    onChange={(event) => setCreateForm({ ...createForm, repo_subpath: event.target.value })}
                    placeholder="plugins/railway/skills/use-railway"
                  />
                </div>
                <div className="space-y-2">
                  <Label>Branch / Ref</Label>
                  <Input
                    value={createForm.repo_ref}
                    onChange={(event) => setCreateForm({ ...createForm, repo_ref: event.target.value })}
                    placeholder="main"
                  />
                </div>
              </>
            )}
            <div className="space-y-2">
              <Label>Tags (comma-separated)</Label>
              <Input
                value={createForm.tags}
                onChange={(event) => setCreateForm({ ...createForm, tags: event.target.value })}
                placeholder="devops, railway, deployments"
              />
            </div>
            {createError && (
              <p className="text-sm text-destructive">{createError}</p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>Cancel</Button>
            <Button onClick={handleCreate} disabled={createSaving || !createForm.name}>
              {createSaving ? "Creating..." : "Create Skill"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Dialog */}
      <Dialog open={editingSkill !== null} onOpenChange={(open) => { if (!open) setEditingSkill(null) }}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Edit Skill</DialogTitle>
            <DialogDescription>Update skill properties.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Name</Label>
              <Input
                value={editForm.name}
                onChange={(event) => setEditForm({ ...editForm, name: event.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label>Description</Label>
              <Textarea
                value={editForm.description}
                onChange={(event) => setEditForm({ ...editForm, description: event.target.value })}
                className="min-h-20"
              />
            </div>
            <div className="space-y-2">
              <Label>Status</Label>
              <Select
                value={editForm.status}
                onValueChange={(value) => setEditForm({ ...editForm, status: value })}
              >
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="draft">Draft</SelectItem>
                  <SelectItem value="published">Published</SelectItem>
                  <SelectItem value="archived">Archived</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Tags (comma-separated)</Label>
              <Input
                value={editForm.tags}
                onChange={(event) => setEditForm({ ...editForm, tags: event.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label>Branch / Ref</Label>
              <Input
                value={editForm.repo_ref}
                onChange={(event) => setEditForm({ ...editForm, repo_ref: event.target.value })}
              />
            </div>
            {editError && (
              <p className="text-sm text-destructive">{editError}</p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditingSkill(null)}>Cancel</Button>
            <Button onClick={handleEdit} disabled={editSaving}>
              {editSaving ? "Saving..." : "Save Changes"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation */}
      <AlertDialog open={confirmDelete !== null} onOpenChange={(open) => { if (!open) setConfirmDelete(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete skill</AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently delete &quot;{confirmDelete?.name}&quot; and all its versions. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => confirmDelete && handleDelete(confirmDelete.id)}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
