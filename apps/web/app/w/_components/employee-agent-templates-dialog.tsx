"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import { Alert02Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { formatCategoryLabel } from "@/lib/format-label"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

type Employee = components["schemas"]["employeeListItem"]
type AgentTemplate = components["schemas"]["employeeAgentTemplateResponse"]

export function EmployeeAgentTemplatesDialog({
  employee,
  open,
  onOpenChange,
}: {
  employee: Employee
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const employeeID = employee.id ?? ""
  const employeeName = employee.name ?? "this employee"
  const [installingSlug, setInstallingSlug] = useState<string | null>(null)

  const templatesQuery = $api.useQuery(
    "get",
    "/v1/employees/{id}/agent-templates",
    {
      params: {
        path: { id: employeeID },
      },
    },
    {
      enabled: open && Boolean(employeeID),
    }
  )
  const installTemplate = $api.useMutation(
    "post",
    "/v1/employees/{id}/agent-templates/{slug}/install"
  )

  const groups = useMemo(
    () => groupTemplates(templatesQuery.data ?? []),
    [templatesQuery.data]
  )
  const error = templatesQuery.isError
    ? extractErrorMessage(templatesQuery.error, "Failed to load templates")
    : null

  function handleInstall(template: AgentTemplate) {
    if (!employeeID || !template.slug) return
    setInstallingSlug(template.slug)
    installTemplate.mutate(
      {
        params: {
          path: {
            id: employeeID,
            slug: template.slug,
          },
        },
      },
      {
        onSuccess: () => {
          toast.success(`${template.name ?? "Template"} installed`)
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/employees"],
          })
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/employees/{id}/agent-templates"],
          })
        },
        onError: (err) => {
          toast.error(extractErrorMessage(err, "Failed to install template"))
        },
        onSettled: () => {
          setInstallingSlug(null)
        },
      }
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Agent templates</DialogTitle>
          <DialogDescription>
            Add category specialists to {employeeName} and sync the employee
            runtime config.
          </DialogDescription>
        </DialogHeader>

        {templatesQuery.isLoading ? (
          <TemplatesSkeleton />
        ) : error ? (
          <div className="flex gap-3 rounded-2xl bg-destructive/10 p-4 text-sm text-destructive">
            <HugeiconsIcon
              icon={Alert02Icon}
              className="mt-0.5 size-4 shrink-0"
              strokeWidth={2}
            />
            <span>{error}</span>
          </div>
        ) : groups.length === 0 ? (
          <div className="rounded-2xl border border-border px-5 py-10 text-center text-sm text-muted-foreground">
            No templates are available for this employee category.
          </div>
        ) : (
          <div className="max-h-[min(520px,65vh)] overflow-y-auto pr-1">
            <div className="flex flex-col gap-6">
              {groups.map((group) => (
                <section key={group.category} className="flex flex-col gap-3">
                  <div className="flex items-center justify-between gap-3">
                    <h3 className="text-sm font-semibold">
                      {formatCategoryLabel(group.category)}
                    </h3>
                    <Badge variant="outline">
                      {group.templates.length}{" "}
                      {group.templates.length === 1 ? "template" : "templates"}
                    </Badge>
                  </div>
                  <div className="divide-y divide-border overflow-hidden rounded-2xl border border-border">
                    {group.templates.map((template) => (
                      <TemplateRow
                        key={template.slug}
                        template={template}
                        installing={installingSlug === template.slug}
                        disabled={installTemplate.isPending}
                        onInstall={() => handleInstall(template)}
                      />
                    ))}
                  </div>
                </section>
              ))}
            </div>
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function TemplateRow({
  template,
  installing,
  disabled,
  onInstall,
}: {
  template: AgentTemplate
  installing: boolean
  disabled: boolean
  onInstall: () => void
}) {
  const installed = Boolean(template.installed)

  return (
    <div className="grid gap-4 p-4 sm:grid-cols-[1fr_auto] sm:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="font-medium">{template.name}</p>
          <Badge
            variant="ghost"
            className={cn(
              installed
                ? "bg-success/15 text-success"
                : "bg-muted/70 text-muted-foreground"
            )}
          >
            {installed ? (
              <HugeiconsIcon
                icon={Tick02Icon}
                className="size-3"
                strokeWidth={2.75}
              />
            ) : null}
            {installed ? "Installed" : "Available"}
          </Badge>
        </div>
        <p className="mt-1 text-sm leading-relaxed text-muted-foreground">
          {template.description}
        </p>
      </div>
      <Button
        type="button"
        variant={installed ? "secondary" : "default"}
        disabled={installed || disabled}
        loading={installing}
        onClick={onInstall}
      >
        {installed ? "Installed" : "Install"}
      </Button>
    </div>
  )
}

function TemplatesSkeleton() {
  return (
    <div className="flex flex-col gap-3">
      {Array.from({ length: 2 }).map((_, index) => (
        <div
          key={index}
          className="grid gap-4 rounded-2xl border border-border p-4 sm:grid-cols-[1fr_auto] sm:items-center"
        >
          <div className="flex flex-col gap-2">
            <Skeleton className="h-4 w-56 rounded-md" />
            <Skeleton className="h-3 w-full rounded-md" />
            <Skeleton className="h-3 w-2/3 rounded-md" />
          </div>
          <Skeleton className="h-9 w-24 rounded-full" />
        </div>
      ))}
    </div>
  )
}

function groupTemplates(templates: AgentTemplate[]) {
  const groups = new Map<string, AgentTemplate[]>()
  for (const template of templates) {
    const category = template.category ?? "other"
    groups.set(category, [...(groups.get(category) ?? []), template])
  }
  return Array.from(groups.entries()).map(([category, groupedTemplates]) => ({
    category,
    templates: groupedTemplates,
  }))
}
