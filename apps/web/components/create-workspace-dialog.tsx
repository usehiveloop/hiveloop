"use client"

import { useState } from "react"
import { toast } from "sonner"
import { api } from "@/lib/api/client"
import { useAuth } from "@/lib/auth/auth-context"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"

interface CreateWorkspaceDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CreateWorkspaceDialog({ open, onOpenChange }: CreateWorkspaceDialogProps) {
  const { addOrg } = useAuth()
  const [name, setName] = useState("")
  const [loading, setLoading] = useState(false)

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    if (!name.trim()) return

    setLoading(true)

    const response = await api.POST("/v1/orgs", {
      body: { name: name.trim() },
    })

    if (response.error) {
      toast.error("Failed to create workspace")
      setLoading(false)
      return
    }

    const created = response.data as { id: string; name: string }
    addOrg({ id: created.id, name: created.name, role: "admin" })
    toast.success(`Workspace "${created.name}" created`)
    setName("")
    setLoading(false)
    onOpenChange(false)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        if (!value) setName("")
        onOpenChange(value)
      }}
    >
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Create workspace</DialogTitle>
          <DialogDescription>
            A workspace is a shared space for your team to manage agents and connections.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="space-y-2">
            <Label htmlFor="workspace-name">Name</Label>
            <Input
              id="workspace-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="My workspace"
              autoFocus
              required
            />
          </div>

          <DialogFooter>
            <Button type="submit" loading={loading}>
              Create workspace
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
