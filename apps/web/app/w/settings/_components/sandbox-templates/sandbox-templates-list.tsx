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
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { toast } from "sonner"
import { CreateSandboxTemplateModal } from "./create-modal"
import type { components } from "@/lib/api/schema"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  MoreHorizontalIcon,
  Delete02Icon,
  PlayCircleIcon,
} from "@hugeicons/core-free-icons"

type SandboxTemplate = components["schemas"]["sandboxTemplateResponse"]

export function SandboxTemplatesList() {
  const [createModalOpen, setCreateModalOpen] = useState(false)
  const [buildingTemplateId, setBuildingTemplateId] = useState<string | null>(null)
  const [deletingTemplate, setDeletingTemplate] = useState<SandboxTemplate | null>(null)

  const { data, isLoading, refetch } = $api.useQuery("get", "/v1/sandbox-templates", {}, {
    refetchOnWindowFocus: true,
  })

  const deleteMutation = $api.useMutation("delete", "/v1/sandbox-templates/{id}")
  const buildMutation = $api.useMutation("post", "/v1/sandbox-templates/{id}/build")

  const templates = (data as { data?: SandboxTemplate[] })?.data ?? []

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

  async function handleBuild(template: SandboxTemplate) {
    if (!template.id) {
      return
    }
    try {
      const result = await buildMutation.mutateAsync({
        params: { path: { id: template.id } },
      })
      const response = result as { stream_url?: string }
      if (response.stream_url) {
        setBuildingTemplateId(template.id)
        refetch()
      }
    } catch (err) {
      toast.error(extractErrorMessage(err, "Failed to trigger build"))
    }
  }

  async function handleDelete() {
    if (!deletingTemplate?.id) {
      return
    }
    try {
      await deleteMutation.mutateAsync({
        params: { path: { id: deletingTemplate.id } },
      })
      toast.success(`Deleted "${deletingTemplate.name}"`)
      refetch()
      setDeletingTemplate(null)
    } catch (err) {
      toast.error(extractErrorMessage(err, "Failed to delete template"))
      setDeletingTemplate(null)
    }
  }

  function handleCreateSuccess(template: SandboxTemplate) {
    refetch()
    if (template.id) {
      setBuildingTemplateId(template.id)
    }
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
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-20 w-full" />
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
        <div className="rounded-lg border border-dashed border-border p-8 text-center">
          <p className="text-sm text-muted-foreground">
            No sandbox templates yet. Create one to get started.
          </p>
          <Button
            variant="outline"
            className="mt-4"
            onClick={() => setCreateModalOpen(true)}
          >
            <HugeiconsIcon icon={Add01Icon} size={16} className="mr-2" />
            Create Template
          </Button>
        </div>
      ) : (
        <div className="space-y-3">
          {templates.map((template) => (
            <div
              key={template.id}
              className="flex items-center justify-between rounded-lg border border-border p-4"
            >
              <div className="flex items-center gap-4">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-medium truncate">{template.name}</span>
                    {getStatusBadge(template.build_status)}
                    {buildingTemplateId === template.id && template.build_status === "building" && (
                      <span className="text-xs text-muted-foreground animate-pulse">
                        Building...
                      </span>
                    )}
                  </div>
                  {template.build_commands && (
                    <p className="text-xs text-muted-foreground mt-1 truncate font-mono">
                      {template.build_commands.split("\n")[0]}
                    </p>
                  )}
                </div>
              </div>

              <div className="flex items-center gap-2">
                {template.build_status !== "ready" && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => handleBuild(template)}
                    loading={buildMutation.isPending && buildingTemplateId === template.id}
                    disabled={template.build_status === "building"}
                  >
                    <HugeiconsIcon icon={PlayCircleIcon} size={14} className="mr-1" />
                    Build
                  </Button>
                )}
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
              </div>
            </div>
          ))}
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
        loading={deleteMutation.isPending}
        onConfirm={handleDelete}
      />
    </div>
  )
}
