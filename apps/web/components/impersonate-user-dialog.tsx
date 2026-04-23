"use client"

import { useState, useEffect } from "react"
import { toast } from "sonner"
import { useAuth } from "@/lib/auth/auth-context"
import { $api } from "@/lib/api/hooks"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"

interface AdminUser {
  id: string
  email: string
  name: string
}

interface ImpersonateUserDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ImpersonateUserDialog({ open, onOpenChange }: ImpersonateUserDialogProps) {
  const { impersonate } = useAuth()
  const [search, setSearch] = useState("")
  const [debouncedSearch, setDebouncedSearch] = useState("")
  const [impersonating, setImpersonating] = useState<string | null>(null)

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setSearch("")
      setDebouncedSearch("")
      setImpersonating(null)
    }
    onOpenChange(nextOpen)
  }

  // Debounce search input so we only fire one query per pause in typing.
  useEffect(() => {
    const trimmed = search.trim()
    const timer = setTimeout(() => setDebouncedSearch(trimmed), trimmed ? 300 : 0)
    return () => clearTimeout(timer)
  }, [search])

  const usersQuery = $api.useQuery(
    "get",
    "/admin/v1/users",
    { params: { query: { search: debouncedSearch, limit: 10 } } },
    { enabled: open && debouncedSearch.length > 0 },
  )

  const results = ((usersQuery.data?.data ?? []) as AdminUser[])
  const searching =
    search.trim().length > 0 &&
    (search.trim() !== debouncedSearch || usersQuery.isFetching)

  async function handleImpersonate(user: AdminUser) {
    setImpersonating(user.id)
    try {
      await impersonate(user.id)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Impersonation failed")
      setImpersonating(null)
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Impersonate User</DialogTitle>
          <DialogDescription>
            Search for a user to view the application as them.
          </DialogDescription>
        </DialogHeader>

        <Input
          placeholder="Search by name or email..."
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          autoFocus
        />

        <div className="max-h-64 overflow-y-auto">
          {searching && (
            <p className="py-4 text-center text-sm text-muted-foreground">
              Searching...
            </p>
          )}

          {!searching && search.trim() && results.length === 0 && (
            <p className="py-4 text-center text-sm text-muted-foreground">
              No users found
            </p>
          )}

          {results.map((user) => (
            <div
              key={user.id}
              className="flex items-center justify-between rounded-lg px-3 py-2 hover:bg-muted"
            >
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{user.name}</p>
                <p className="truncate text-xs text-muted-foreground">{user.email}</p>
              </div>
              <Button
                variant="outline"
                size="sm"
                disabled={impersonating !== null}
                onClick={() => handleImpersonate(user)}
              >
                {impersonating === user.id ? "Switching..." : "Impersonate"}
              </Button>
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}
