"use client"

import { useState } from "react"
import { toast } from "sonner"
import { useQueryClient } from "@tanstack/react-query"
import { CreateSkillDialog } from "./create-skill-dialog"
import { SkillDetailDialog } from "./skill-detail-dialog"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"

type SkillRow = components["schemas"]["skillResponse"]

export function SkillsSettings() {
  const queryClient = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [selected, setSelected] = useState<SkillRow | null>(null)
  const [deleting, setDeleting] = useState<SkillRow | null>(null)
  const { data, isLoading } = $api.useQuery("get", "/v1/skills", {
    params: { query: { scope: "all", limit: 100 } },
  })
  const deleteSkill = $api.useMutation("delete", "/v1/skills/{id}")
  const skills = data?.data ?? []

  function handleDelete() {
    if (!deleting?.id) return
    deleteSkill.mutate(
      { params: { path: { id: deleting.id } } },
      {
        onSuccess: () => {
          toast.success("Skill archived")
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills"] })
          setDeleting(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to archive skill"))
          setDeleting(null)
        },
      }
    )
  }

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h2 className="text-sm font-medium text-foreground">Skill library</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Global and workspace skills can be installed on Hivy.
          </p>
        </div>
        <Button onClick={() => setCreating(true)}>New skill</Button>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
        </div>
      ) : skills.length === 0 ? (
        <div className="border border-border p-6 text-sm text-muted-foreground">
          No skills yet.
        </div>
      ) : (
        <div className="divide-y divide-border border border-border">
          {skills.map((skill) => (
            <div
              key={skill.id}
              className="flex items-center justify-between gap-4 p-4"
            >
              <button
                type="button"
                className="min-w-0 text-left"
                onClick={() => setSelected(skill)}
              >
                <div className="truncate text-sm font-medium">
                  {skill.name}
                </div>
                <div className="mt-1 line-clamp-2 text-sm text-muted-foreground">
                  {skill.description ?? "No description"}
                </div>
              </button>
              {skill.source_type === "org" ? (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setDeleting(skill)}
                >
                  Archive
                </Button>
              ) : null}
            </div>
          ))}
        </div>
      )}

      <CreateSkillDialog open={creating} onOpenChange={setCreating} />
      {selected ? (
        <SkillDetailDialog
          skill={selected}
          open
          onOpenChange={(open) => {
            if (!open) setSelected(null)
          }}
        />
      ) : null}
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null)
        }}
        title="Archive skill"
        description={`This will archive "${deleting?.name ?? ""}". Hivy will no longer be able to invoke it.`}
        confirmLabel="Archive"
        confirmText={deleting?.name ?? ""}
        destructive
        loading={deleteSkill.isPending}
        onConfirm={handleDelete}
      />
    </div>
  )
}
