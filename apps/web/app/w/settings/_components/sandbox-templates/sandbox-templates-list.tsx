"use client"

import * as React from "react"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { ConfirmDialog } from "@/components/confirm-dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { extractErrorMessage } from "@/lib/api/error"
import { toast } from "sonner"
import { CreateSandboxTemplateModal } from "./create-modal"
import {
  useSandboxTemplates,
  useTriggerBuild,
  useDeleteSandboxTemplate,
  type SandboxTemplate,
} from "@/hooks/use-sandbox-template"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  MoreHorizontalIcon,
  Delete02Icon,
  PlayCircleIcon,
  ContainerIcon,
  ArrowRight01Icon,
} from "@hugeicons/core-free-icons"

function isPublicTemplate(template: SandboxTemplate): boolean {
  return !!(template as Record<string, unknown>).is_public
}

export function SandboxTemplatesList() {
  const [createModalOpen, setCreateModalOpen] = useState(false)
  const [deletingTemplate, setDeletingTemplate] = useState<SandboxTemplate | null>(null)

  const { data: templates = [], isLoading, refetch } = useSandboxTemplates()

  const deleteTemplate = useDeleteSandboxTemplate()
  const buildMutation = useTriggerBuild()

  function getStatusBadge(status?: string) {
    switch (status) {
      case "ready":
        return <Badge variant="default" className="bg-green-500/10 text-green-600 border-green-500/20">Ready</Badge>
      case "building":
        return <Badge variant="default" className="bg-blue-500/10 text-blue-600 border-blue-500/20">Building</Badge>
      case "failed":
        return <Badge variant="default" className="bg-red-500/10 text-red-600 border-red-500/20">Failed</Badge>
      default:
        return <Badge variant="secondary">Pending</Badge>
    }
  }

  function handleBuild(template: SandboxTemplate) {
    if (!template.id) return

    buildMutation.mutate(
      { params: { path: { id: template.id } } },
      {
        onError: (err: unknown) => {
          toast.error(extractErrorMessage(err, "Failed to trigger build"))
        },
      }
    )
  }

  function handleDelete() {
    if (!deletingTemplate?.id) return

    deleteTemplate.mutate(
      { params: { path: { id: deletingTemplate.id } } },
      {
        onSuccess: () => {
          toast.success(`Deleted "${deletingTemplate.name}"`)
          setDeletingTemplate(null)
        },
        onError: (err: unknown) => {
          toast.error(extractErrorMessage(err, "Failed to delete template"))
          setDeletingTemplate(null)
        },
      }
    )
  }

  function handleCreateSuccess(template: SandboxTemplate) {
    refetch()
  }

  if (isLoading) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <Skeleton className="h-6 w-48" />
            <Skeleton className="h-4 w-64 mt-2" />
          </div>
          <Skeleton className="h-10 w-32" />
        </div>
        <div className="space-y-3">
          {[1, 2, 3].map((index) => (
            <Skeleton key={index} className="h-20 w-full" />
          ))}
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-medium">Sandbox Templates</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Custom sandbox environments for your agents.
          </p>
        </div>
        <Button onClick={() => setCreateModalOpen(true)}>
          <HugeiconsIcon icon={Add01Icon} size={16} className="mr-2" />
          New Template
        </Button>
      </div>

      {templates.length === 0 ? (
        <div className="flex flex-col items-center py-14">
          <div className="text-center mb-6">
            <h2 className="font-heading text-lg font-semibold text-foreground">No sandbox templates yet</h2>
            <p className="text-sm text-muted-foreground mt-1.5 max-w-xs">
              Create a template to define custom sandbox environments for your agents.
            </p>
          </div>
          <div className="w-full max-w-sm">
            <button
              type="button"
              onClick={() => setCreateModalOpen(true)}
              className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
            >
              <HugeiconsIcon icon={ContainerIcon} size={20} className="shrink-0 mt-0.5 text-muted-foreground" />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-semibold text-foreground">Create template</p>
                <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">
                  Define a custom environment with build commands and dependencies.
                </p>
              </div>
              <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
            </button>
          </div>
        </div>
      ) : (
        <div className="space-y-1.5">
          {templates.map((template) => {
            const isPublic = isPublicTemplate(template)
            const tags = Array.isArray((template as Record<string, unknown>).tags)
              ? ((template as Record<string, unknown>).tags as string[])
              : []

            return (
              <div
                key={template.id}
                className="flex items-center justify-between rounded-lg border border-border px-4 py-2.5"
              >
                <div className="flex items-center gap-3 min-w-0">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium truncate">{template.name}</span>
                      {getStatusBadge(template.build_status)}
                      {isPublic && (
                        <Badge variant="secondary" className="text-[10px] px-1.5 py-0">Public</Badge>
                      )}
                    </div>
                    {tags.length > 0 && (
                      <div className="flex items-center gap-1 mt-0.5">
                        {tags.map((tag) => (
                          <span key={tag} className="text-[11px] text-muted-foreground">
                            {tag}
                          </span>
                        ))}
                      </div>
                    )}
                  </div>
                </div>

                <div className="flex items-center gap-2">
                  {!isPublic && template.build_status !== "ready" && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleBuild(template)}
                      loading={buildMutation.isPending}
                      disabled={template.build_status === "building"}
                    >
                      <HugeiconsIcon icon={PlayCircleIcon} size={14} className="mr-1" />
                      Build
                    </Button>
                  )}
                  {!isPublic && (
                    <DropdownMenu>
                      <DropdownMenuTrigger className="flex items-center justify-center h-8 w-8 rounded-lg transition-colors hover:bg-muted outline-none">
                        <HugeiconsIcon icon={MoreHorizontalIcon} size={16} className="text-muted-foreground" />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem
                          className="text-destructive focus:text-destructive"
                          onClick={() => setDeletingTemplate(template)}
                        >
                          <HugeiconsIcon icon={Delete02Icon} size={14} className="mr-2" />
                          Delete
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      )}

      <CreateSandboxTemplateModal
        open={createModalOpen}
        onOpenChange={setCreateModalOpen}
        onSuccess={handleCreateSuccess}
      />

      <ConfirmDialog
        open={deletingTemplate !== null}
        onOpenChange={(open) => { if (!open) setDeletingTemplate(null) }}
        title="Delete sandbox template"
        description={`This will permanently delete "${deletingTemplate?.name}" and all its data. This action cannot be undone.`}
        confirmText={deletingTemplate?.name ?? ""}
        confirmLabel="Delete"
        destructive
        loading={deleteTemplate.isPending}
        onConfirm={handleDelete}
      />
    </div>
  )
}
