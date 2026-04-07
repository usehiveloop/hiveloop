"use client"

import { useState, useEffect, useRef } from "react"
import { toast } from "sonner"
import { useAuth } from "@/lib/auth/auth-context"
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
  const [results, setResults] = useState<AdminUser[]>([])
  const [searching, setSearching] = useState(false)
  const [impersonating, setImpersonating] = useState<string | null>(null)
  const debounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    if (!open) {
      setSearch("")
      setResults([])
      setSearching(false)
      setImpersonating(null)
    }
  }, [open])

  useEffect(() => {
    if (debounceTimer.current) {
      clearTimeout(debounceTimer.current)
    }

    if (!search.trim()) {
      setResults([])
      setSearching(false)
      return
    }

    setSearching(true)
    debounceTimer.current = setTimeout(async () => {
      try {
        const response = await fetch(`/api/proxy/admin/v1/users?search=${encodeURIComponent(search.trim())}&limit=10`)
        if (response.ok) {
          const data = await response.json()
          setResults(data.data ?? [])
        }
      } catch {
        // Silently fail — user will see empty results
      } finally {
        setSearching(false)
      }
    }, 300)

    return () => {
      if (debounceTimer.current) {
        clearTimeout(debounceTimer.current)
      }
    }
  }, [search])

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
    <Dialog open={open} onOpenChange={onOpenChange}>
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
