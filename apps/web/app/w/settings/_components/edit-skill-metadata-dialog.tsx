"use client"

import { useEffect, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import type { components } from "@/lib/api/schema"

type SkillRow = components["schemas"]["skillResponse"]

interface EditSkillMetadataDialogProps {
  skill: SkillRow | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function EditSkillMetadataDialog({
  skill,
  open,
  onOpenChange,
}: EditSkillMetadataDialogProps) {
  const queryClient = useQueryClient()
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const updateSkill = $api.useMutation("patch", "/v1/skills/{id}")

  useEffect(() => {
    if (open && skill) {
      setName(skill.name ?? "")
      setDescription(skill.description ?? "")
    }
  }, [open, skill])

  function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    if (!skill?.id || !name.trim()) return

    updateSkill.mutate(
      {
        params: { path: { id: skill.id } },
        body: {
          name: name.trim(),
          description: description.trim(),
        },
      },
      {
        onSuccess: () => {
          toast.success("Skill updated")
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills"] })
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills/{id}"] })
          onOpenChange(false)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to update skill"))
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton className="sm:max-w-lg">
        <DialogTitle>Edit skill</DialogTitle>

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <Label htmlFor="edit-skill-name">Name</Label>
              <Input
                id="edit-skill-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="e.g. Browser automation"
                required
                autoFocus
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="edit-skill-description">Description</Label>
              <Textarea
                id="edit-skill-description"
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                placeholder="What this skill does..."
                rows={4}
              />
            </div>
          </div>

          <Button
            type="submit"
            className="w-full"
            loading={updateSkill.isPending}
            disabled={!name.trim()}
          >
            Save changes
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  )
}
