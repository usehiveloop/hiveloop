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
import { Textarea } from "@/components/ui/textarea"
import { Label } from "@/components/ui/label"
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
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

type FilterTab = "all" | "confirmed" | "unconfirmed" | "banned"

function getQueryParams(tab: FilterTab, search: string) {
  const params: {
    search?: string
    banned?: string
    confirmed?: string
    limit?: number
  } = { limit: 50 }

  if (search.trim()) params.search = search.trim()

  switch (tab) {
    case "confirmed":
      params.confirmed = "true"
      break
    case "unconfirmed":
      params.confirmed = "false"
      break
    case "banned":
      params.banned = "true"
      break
  }

  return params
}

function TableSkeleton() {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Email</TableHead>
          <TableHead>Name</TableHead>
          <TableHead>Confirmed</TableHead>
          <TableHead>Banned</TableHead>
          <TableHead>Created</TableHead>
          <TableHead className="w-12" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {Array.from({ length: 8 }).map((_, i) => (
          <TableRow key={i}>
            <TableCell><Skeleton className="h-4 w-40" /></TableCell>
            <TableCell><Skeleton className="h-4 w-28" /></TableCell>
            <TableCell><Skeleton className="h-5 w-16" /></TableCell>
            <TableCell><Skeleton className="h-5 w-16" /></TableCell>
            <TableCell><Skeleton className="h-4 w-20" /></TableCell>
            <TableCell><Skeleton className="h-8 w-8" /></TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

export default function UsersPage() {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState("")
  const [debouncedSearch, setDebouncedSearch] = useState("")
  const [tab, setTab] = useState<FilterTab>("all")
  const [mutating, setMutating] = useState<string | null>(null)

  // Ban dialog
  const [banDialogUser, setBanDialogUser] = useState<{ id: string; email: string } | null>(null)
  const [banReason, setBanReason] = useState("")

  // Delete confirm
  const [deleteUser, setDeleteUser] = useState<{ id: string; email: string } | null>(null)

  // Edit dialog
  const [editingUser, setEditingUser] = useState<{ id: string; name: string; email: string } | null>(null)
  const [editForm, setEditForm] = useState({ name: "", email: "" })
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
  const { data, isLoading, error } = $api.useQuery("get", "/admin/v1/users", {
    params: { query: queryParams },
  })

  const users = data?.data ?? []

  async function handleBan() {
    if (!banDialogUser) return
    setMutating(banDialogUser.id)
    try {
      await api.POST("/admin/v1/users/{id}/ban", {
        params: { path: { id: banDialogUser.id } },
      })
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/users"] })
    } finally {
      setMutating(null)
      setBanDialogUser(null)
      setBanReason("")
    }
  }

  async function handleUnban(userId: string) {
    setMutating(userId)
    try {
      await api.POST("/admin/v1/users/{id}/unban", {
        params: { path: { id: userId } },
      })
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/users"] })
    } finally {
      setMutating(null)
    }
  }

  async function handleConfirmEmail(userId: string) {
    setMutating(userId)
    try {
      await api.POST("/admin/v1/users/{id}/confirm-email", {
        params: { path: { id: userId } },
      })
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/users"] })
    } finally {
      setMutating(null)
    }
  }

  async function handleDelete() {
    if (!deleteUser) return
    setMutating(deleteUser.id)
    try {
      await api.DELETE("/admin/v1/users/{id}", {
        params: { path: { id: deleteUser.id } },
      })
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/users"] })
    } finally {
      setMutating(null)
      setDeleteUser(null)
    }
  }

  function openEditDialog(user: { id?: string; name?: string; email?: string }) {
    setEditForm({
      name: user.name || "",
      email: user.email || "",
    })
    setEditError(null)
    setEditingUser({
      id: user.id!,
      name: user.name || "",
      email: user.email || "",
    })
  }

  async function handleEditUser() {
    if (!editingUser) return
    setEditSaving(true)
    setEditError(null)
    try {
      const res = await api.PUT("/admin/v1/users/{id}", {
        params: { path: { id: editingUser.id } },
        body: {
          name: editForm.name,
          email: editForm.email,
        },
      })
      if (res.error) {
        const msg = (res.error as { error?: string }).error || "Failed to update user."
        setEditError(msg)
        return
      }
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/users"] })
      setEditingUser(null)
    } catch {
      setEditError("An unexpected error occurred.")
    } finally {
      setEditSaving(false)
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Users"
        description="Manage all platform users."
      />

      {/* Filters */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <Tabs
          value={tab}
          onValueChange={(v) => setTab(v as FilterTab)}
        >
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="confirmed">Confirmed</TabsTrigger>
            <TabsTrigger value="unconfirmed">Unconfirmed</TabsTrigger>
            <TabsTrigger value="banned">Banned</TabsTrigger>
          </TabsList>
        </Tabs>
        <Input
          placeholder="Search by email or name..."
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
            Failed to load users.
          </CardContent>
        </Card>
      ) : users.length === 0 ? (
        <Card>
          <CardContent className="py-12 text-center text-sm text-muted-foreground">
            {debouncedSearch || tab !== "all"
              ? "No users match the current filters."
              : "No users found."}
          </CardContent>
        </Card>
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Email</TableHead>
                <TableHead>Name</TableHead>
                <TableHead>Confirmed</TableHead>
                <TableHead>Banned</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.map((user) => {
                const isBanned = !!user.banned_at
                const isConfirmed = !!user.email_confirmed_at
                return (
                  <TableRow key={user.id}>
                    <TableCell className="font-medium">{user.email}</TableCell>
                    <TableCell>{user.name || <span className="text-muted-foreground">--</span>}</TableCell>
                    <TableCell>
                      <StatusBadge status={isConfirmed ? "verified" : "pending"} />
                    </TableCell>
                    <TableCell>
                      {isBanned ? (
                        <StatusBadge status="banned" />
                      ) : (
                        <span className="text-sm text-muted-foreground">--</span>
                      )}
                    </TableCell>
                    <TableCell>
                      <TimeAgo date={user.created_at} />
                    </TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={mutating === user.id}
                          >
                            {mutating === user.id ? "..." : "Actions"}
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            onClick={() => openEditDialog(user)}
                          >
                            Edit
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          {!isConfirmed && (
                            <DropdownMenuItem
                              onClick={() => handleConfirmEmail(user.id!)}
                            >
                              Confirm Email
                            </DropdownMenuItem>
                          )}
                          {isBanned ? (
                            <DropdownMenuItem
                              onClick={() => handleUnban(user.id!)}
                            >
                              Unban
                            </DropdownMenuItem>
                          ) : (
                            <DropdownMenuItem
                              onClick={() =>
                                setBanDialogUser({
                                  id: user.id!,
                                  email: user.email!,
                                })
                              }
                            >
                              Ban User
                            </DropdownMenuItem>
                          )}
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() =>
                              setDeleteUser({
                                id: user.id!,
                                email: user.email!,
                              })
                            }
                          >
                            Delete User
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
          Showing first {users.length} results. Refine your search for more.
        </p>
      )}

      {/* Edit User Dialog */}
      <Dialog
        open={!!editingUser}
        onOpenChange={(open) => !open && setEditingUser(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit User</DialogTitle>
            <DialogDescription>
              Update the name or email for this user.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="edit-user-name">Name</Label>
              <Input
                id="edit-user-name"
                value={editForm.name}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, name: e.target.value }))
                }
                placeholder="User name"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-user-email">Email</Label>
              <Input
                id="edit-user-email"
                type="email"
                value={editForm.email}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, email: e.target.value }))
                }
                placeholder="user@example.com"
              />
            </div>
            {editError && (
              <p className="text-sm text-destructive">{editError}</p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setEditingUser(null)}
            >
              Cancel
            </Button>
            <Button onClick={handleEditUser} disabled={editSaving}>
              {editSaving ? "Saving..." : "Save changes"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Ban Dialog */}
      <Dialog
        open={!!banDialogUser}
        onOpenChange={(open) => {
          if (!open) {
            setBanDialogUser(null)
            setBanReason("")
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Ban User</DialogTitle>
            <DialogDescription>
              Ban <span className="font-medium text-foreground">{banDialogUser?.email}</span>. This
              will revoke all their refresh tokens and prevent login.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="ban-reason">Reason (optional)</Label>
            <Textarea
              id="ban-reason"
              placeholder="Reason for banning this user..."
              value={banReason}
              onChange={(e) => setBanReason(e.target.value)}
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setBanDialogUser(null)
                setBanReason("")
              }}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleBan}
              disabled={mutating === banDialogUser?.id}
            >
              {mutating === banDialogUser?.id ? "Banning..." : "Ban User"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirm */}
      <AlertDialog
        open={!!deleteUser}
        onOpenChange={(open) => {
          if (!open) setDeleteUser(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete User</AlertDialogTitle>
            <AlertDialogDescription>
              Permanently delete{" "}
              <span className="font-medium text-foreground">{deleteUser?.email}</span> and all
              associated data. This action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={handleDelete}
              disabled={mutating === deleteUser?.id}
            >
              {mutating === deleteUser?.id ? "Deleting..." : "Delete"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
