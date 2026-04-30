"use client"

import { useState } from "react"
import { cn } from "@/lib/utils"
import { SkillDetailDialog } from "./skill-detail-dialog"
import { CreateSkillDialog } from "./create-skill-dialog"
import { EditSkillMetadataDialog } from "./edit-skill-metadata-dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { Skeleton } from "@/components/ui/skeleton"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import type { components } from "@/lib/api/schema"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  BookOpen01Icon,
  Add01Icon,
  MoreHorizontalIcon,
  Delete02Icon,
  ArrowRight01Icon,
  GitBranchIcon,
  File01Icon,
  Globe02Icon,
  Edit02Icon,
  CodeIcon,
} from "@hugeicons/core-free-icons"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

type SkillRow = components["schemas"]["skillResponse"]

function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

function SkillHydrationBadge({ skill }: { skill: SkillRow }) {
  const status = skill.hydration_status ?? "pending"

  if (status === "error") {
    return (
      <Tooltip>
        <TooltipTrigger className="cursor-default">
          <Badge variant="secondary" className="text-[10px] bg-red-500/10 text-red-600 dark:text-red-400 border-red-500/20">
            error
          </Badge>
        </TooltipTrigger>
        <TooltipContent className="max-w-xs">
          <p className="text-xs font-mono whitespace-pre-wrap">{skill.hydration_error ?? "Unknown error"}</p>
        </TooltipContent>
      </Tooltip>
    )
  }

  if (status === "pending") {
    return (
      <Badge variant="secondary" className="text-[10px] bg-yellow-500/10 text-yellow-600 dark:text-yellow-400 border-yellow-500/20">
        pending
      </Badge>
    )
  }

  return (
    <Badge variant="default" className="text-[10px] bg-green-500/10 text-green-600 dark:text-green-400 border-green-500/20">
      ready
    </Badge>
  )
}

function PublishedIndicator() {
  return (
    <Tooltip>
      <TooltipTrigger className="cursor-default">
        <HugeiconsIcon icon={Globe02Icon} size={14} className="text-blue-500 shrink-0" />
      </TooltipTrigger>
      <TooltipContent>
        Published to marketplace
      </TooltipContent>
    </Tooltip>
  )
}

interface SkillActionsProps {
  skill: SkillRow
  onEditMetadata: () => void
  onEditContent: () => void
  onDelete: () => void
  onPublish: () => void
  onUnpublish: () => void
}

function SkillActions({
  skill,
  onEditMetadata,
  onEditContent,
  onDelete,
  onPublish,
  onUnpublish,
}: SkillActionsProps) {
  const isPublished = !!skill.public_skill_id

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center justify-center h-8 w-8 rounded-lg transition-colors hover:bg-muted outline-none">
        <HugeiconsIcon icon={MoreHorizontalIcon} size={16} className="text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" sideOffset={4} className="min-w-56">
        <DropdownMenuGroup>
          <DropdownMenuItem onClick={onEditMetadata}>
            <HugeiconsIcon icon={Edit02Icon} size={16} className="text-muted-foreground" />
            Edit metadata
          </DropdownMenuItem>
          <DropdownMenuItem onClick={onEditContent}>
            <HugeiconsIcon icon={CodeIcon} size={16} className="text-muted-foreground" />
            Edit content
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          {isPublished ? (
            <DropdownMenuItem onClick={onUnpublish}>
              <HugeiconsIcon icon={Globe02Icon} size={16} className="text-muted-foreground" />
              Unpublish from marketplace
            </DropdownMenuItem>
          ) : (
            <DropdownMenuItem onClick={onPublish}>
              <HugeiconsIcon icon={Globe02Icon} size={16} className="text-muted-foreground" />
              Publish to marketplace
            </DropdownMenuItem>
          )}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem variant="destructive" onClick={onDelete}>
            <HugeiconsIcon icon={Delete02Icon} size={16} />
            Archive
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}


export function SkillsSettings() {
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [viewing, setViewing] = useState<SkillRow | null>(null)
  const [viewingMode, setViewingMode] = useState<"view" | "edit">("view")
  const [editingMetadata, setEditingMetadata] = useState<SkillRow | null>(null)
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
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium text-foreground">Skills</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Reusable instruction bundles your agents can invoke on demand.
          </p>
        </div>
        <Button size="sm" variant="secondary" onClick={() => setCreateOpen(true)}>
          <HugeiconsIcon icon={Add01Icon} size={14} data-icon="inline-start" />
          Add skill
        </Button>
      </div>

      <div className="flex flex-col gap-2">
        {isLoading ? (
          Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="h-[52px] w-full rounded-xl" />
          ))
        ) : skills.length === 0 ? (
          <div className="flex flex-col items-center py-14">
            <div className="text-center mb-6">
              <h2 className="font-heading text-lg font-semibold text-foreground">No skills yet</h2>
              <p className="text-sm text-muted-foreground mt-1.5 max-w-xs">
                Create a skill to give your agents reusable capabilities.
              </p>
            </div>
            <div className="w-full max-w-sm">
              <button
                type="button"
                onClick={() => setCreateOpen(true)}
                className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
              >
                <HugeiconsIcon icon={BookOpen01Icon} size={20} className="shrink-0 mt-0.5 text-muted-foreground" />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-semibold text-foreground">Create a skill</p>
                  <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">
                    Write inline instructions or sync from a Git repository.
                  </p>
                </div>
                <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
              </button>
            </div>
          </div>
        ) : (
          <>
            <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
              <span className="flex-1 min-w-0">Name</span>
              <span className="w-20 shrink-0 text-right">Source</span>
              <span className="w-20 shrink-0 text-right">Status</span>
              <span className="w-28 shrink-0 text-right">Created</span>
              <span className="w-8 shrink-0" />
            </div>

            {skills.map((skill) => (
              <div key={skill.id}>
                {/* Desktop row */}
                <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary cursor-pointer" onClick={() => { setViewingMode("view"); setViewing(skill) }}>
                  <div className="flex items-center gap-3 flex-1 min-w-0">
                    <HugeiconsIcon
                      icon={skill.source_type === "git" ? GitBranchIcon : File01Icon}
                      size={16}
                      className="shrink-0 text-muted-foreground"
                    />
                    <div className="min-w-0">
                      <div className="flex items-center gap-1.5">
                        <p className="text-sm font-medium text-foreground truncate">{skill.name}</p>
                        {skill.public_skill_id && <PublishedIndicator />}
                      </div>
                      {skill.description && (
                        <p className="text-xs text-muted-foreground truncate max-w-[280px]">{skill.description}</p>
                      )}
                    </div>
                  </div>
                  <span className="w-20 shrink-0 text-right">
                    <Badge variant="secondary" className="text-[10px]">
                      {skill.source_type === "git" ? "git" : "inline"}
                    </Badge>
                  </span>
                  <span className="w-20 shrink-0 text-right">
                    <SkillHydrationBadge skill={skill} />
                  </span>
                  <span className="w-28 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                    {skill.created_at ? formatDate(skill.created_at) : "\u2014"}
                  </span>
                  <div className="w-8 shrink-0 flex justify-center" onClick={(event) => event.stopPropagation()}>
                    <SkillActions
                      skill={skill}
                      onEditMetadata={() => setEditingMetadata(skill)}
                      onEditContent={() => { setViewingMode("edit"); setViewing(skill) }}
                      onDelete={() => setDeleting(skill)}
                      onPublish={() => setPublishing(skill)}
                      onUnpublish={() => setUnpublishing(skill)}
                    />
                  </div>
                </div>

                {/* Mobile row */}
                <div className="flex md:hidden flex-col gap-3 rounded-xl border border-border p-4 transition-colors hover:border-primary cursor-pointer" onClick={() => { setViewingMode("view"); setViewing(skill) }}>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3 min-w-0 flex-1">
                      <HugeiconsIcon
                        icon={skill.source_type === "git" ? GitBranchIcon : File01Icon}
                        size={16}
                        className="shrink-0 text-muted-foreground"
                      />
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5">
                          <p className="text-sm font-medium text-foreground truncate">{skill.name}</p>
                          {skill.public_skill_id && <PublishedIndicator />}
                        </div>
                        {skill.description && (
                          <p className="text-xs text-muted-foreground truncate max-w-[280px]">{skill.description}</p>
                        )}
                      </div>
                    </div>
                    <div onClick={(event) => event.stopPropagation()}>
                      <SkillActions
                        skill={skill}
                        onEditMetadata={() => setEditingMetadata(skill)}
                        onEditContent={() => { setViewingMode("edit"); setViewing(skill) }}
                        onDelete={() => setDeleting(skill)}
                        onPublish={() => setPublishing(skill)}
                        onUnpublish={() => setUnpublishing(skill)}
                      />
                    </div>
                  </div>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground font-mono tabular-nums">
                    <Badge variant="secondary" className="text-[10px]">
                      {skill.source_type === "git" ? "git" : "inline"}
                    </Badge>
                    <SkillHydrationBadge skill={skill} />
                    <span>{skill.created_at ? formatDate(skill.created_at) : "\u2014"}</span>
                  </div>
                </div>
              </div>
            ))}
          </>
        )}
      </div>

      <CreateSkillDialog open={createOpen} onOpenChange={setCreateOpen} />

      <SkillDetailDialog
        skill={viewing}
        open={viewing !== null}
        initialEditing={viewingMode === "edit"}
        onOpenChange={(open) => { if (!open) setViewing(null) }}
      />

      <EditSkillMetadataDialog
        skill={editingMetadata}
        open={editingMetadata !== null}
        onOpenChange={(open) => { if (!open) setEditingMetadata(null) }}
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
