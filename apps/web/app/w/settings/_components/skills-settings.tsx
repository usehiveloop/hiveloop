"use client"

import { useState } from "react"
import { SkillDetailDialog } from "./skill-detail-dialog"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import type { components } from "@/lib/api/schema"
import { CreateSkillDialog } from "./skills/create-skill-dialog"
import { SkillsList } from "./skills/skills-list"

type SkillRow = components["schemas"]["skillResponse"]

export function SkillsSettings() {
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [viewing, setViewing] = useState<SkillRow | null>(null)
  const [deleting, setDeleting] = useState<SkillRow | null>(null)
  const [publishing, setPublishing] = useState<SkillRow | null>(null)
  const [unpublishing, setUnpublishing] = useState<SkillRow | null>(null)
  const { data, isLoading } = $api.useQuery("get", "/v1/skills", {
    params: { query: { scope: "own" } },
  })
  const skills = data?.data ?? []
  const deleteSkill = $api.useMutation("delete", "/v1/skills/{id}")
  const publishSkill = $api.useMutation("post", "/v1/skills/{id}/publish")
  const unpublishSkill = $api.useMutation("delete", "/v1/skills/{id}/publish")

  function handleDelete() {
    if (!deleting?.id) return
    deleteSkill.mutate(
      { params: { path: { id: deleting.id } } },
      {
        onSuccess: () => {
          toast.success(`"${deleting.name}" archived`)
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills"] })
          setDeleting(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to archive skill"))
          setDeleting(null)
        },
      },
    )
  }

  function handlePublish() {
    if (!publishing?.id) return
    publishSkill.mutate(
      { params: { path: { id: publishing.id } } },
      {
        onSuccess: () => {
          toast.success(`"${publishing.name}" published to marketplace`)
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills"] })
          setPublishing(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to publish skill"))
          setPublishing(null)
        },
      },
    )
  }

  function handleUnpublish() {
    if (!unpublishing?.id) return
    unpublishSkill.mutate(
      { params: { path: { id: unpublishing.id } } },
      {
        onSuccess: () => {
          toast.success(`"${unpublishing.name}" removed from marketplace`)
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills"] })
          setUnpublishing(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to unpublish skill"))
          setUnpublishing(null)
        },
      },
    )
  }

  return (
    <div className="space-y-4">
      <SkillsList
        skills={skills}
        isLoading={isLoading}
        onCreate={() => setCreateOpen(true)}
        onView={setViewing}
        onDelete={setDeleting}
        onPublish={setPublishing}
        onUnpublish={setUnpublishing}
      />

      <CreateSkillDialog open={createOpen} onOpenChange={setCreateOpen} />

      <SkillDetailDialog
        skill={viewing}
        open={viewing !== null}
        onOpenChange={(open) => { if (!open) setViewing(null) }}
      />

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => { if (!open) setDeleting(null) }}
        title="Archive skill"
        description={`This will archive "${deleting?.name ?? ""}". Agents using this skill will no longer be able to invoke it.`}
        confirmText="archive"
        confirmLabel="Archive skill"
        destructive
        loading={deleteSkill.isPending}
        onConfirm={handleDelete}
      />

      <ConfirmDialog
        open={publishing !== null}
        onOpenChange={(open) => { if (!open) setPublishing(null) }}
        title="Publish to marketplace"
        description={`This will make "${publishing?.name ?? ""}" publicly available in the marketplace. Other users will be able to discover and install it.`}
        confirmLabel="Publish"
        loading={publishSkill.isPending}
        onConfirm={handlePublish}
      />

      <ConfirmDialog
        open={unpublishing !== null}
        onOpenChange={(open) => { if (!open) setUnpublishing(null) }}
        title="Unpublish from marketplace"
        description={`This will remove "${unpublishing?.name ?? ""}" from the public marketplace. Agents that already have it installed will no longer receive updates.`}
        confirmLabel="Unpublish"
        destructive
        loading={unpublishSkill.isPending}
        onConfirm={handleUnpublish}
      />
    </div>
  )
}
