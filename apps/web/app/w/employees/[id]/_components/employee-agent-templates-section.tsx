"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  Add01Icon,
  BotIcon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import { ChoiceCard } from "@/app/w/agents/_components/create-agent/choice-card"
import { FormEmptyWell, FormSection } from "@/app/w/_components/form-section"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { formatCategoryLabel } from "@/lib/format-label"
import type { components } from "@/lib/api/schema"

type AgentTemplate = components["schemas"]["employeeAgentTemplateResponse"]

export function EmployeeAgentTemplatesSection({
  employeeID,
  employeeName,
}: {
  employeeID: string
  employeeName: string
}) {
  const [dialogOpen, setDialogOpen] = useState(false)
  const templatesQuery = $api.useQuery(
    "get",
    "/v1/employees/{id}/agent-templates",
    {
      params: { path: { id: employeeID } },
    },
    {
      enabled: Boolean(employeeID),
    }
  )

  const templates = useMemo(() => templatesQuery.data ?? [], [templatesQuery.data])
  const installedTemplates = useMemo(
    () => templates.filter((template) => template.installed),
    [templates]
  )

  return (
    <>
      <FormSection
        title="Specialists"
        description="Install category specialists for this employee."
      >
        <div className="flex flex-col gap-2">
          {templatesQuery.isLoading ? (
            <TemplatesSkeleton />
          ) : installedTemplates.length === 0 ? (
            <FormEmptyWell
              icon={BotIcon}
              message="No specialists installed."
              action={
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setDialogOpen(true)}
                >
                  Manage specialists
                </Button>
              }
            />
          ) : (
            <>
              {installedTemplates.map((template) => (
                <InstalledTemplateRow key={template.slug} template={template} />
              ))}
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="mt-1 w-fit"
                onClick={() => setDialogOpen(true)}
              >
                Manage specialists
              </Button>
            </>
          )}
        </div>
      </FormSection>

      <EmployeeAgentTemplatesDialog
        employeeID={employeeID}
        employeeName={employeeName}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        templates={templates}
        loading={templatesQuery.isLoading}
        error={
          templatesQuery.isError
            ? extractErrorMessage(
                templatesQuery.error,
                "Failed to load templates"
              )
            : null
        }
      />
    </>
  )
}

function EmployeeAgentTemplatesDialog({
  employeeID,
  employeeName,
  open,
  onOpenChange,
  templates,
  loading,
  error,
}: {
  employeeID: string
  employeeName: string
  open: boolean
  onOpenChange: (open: boolean) => void
  templates: AgentTemplate[]
  loading: boolean
  error: string | null
}) {
  const queryClient = useQueryClient()
  const [installingSlug, setInstallingSlug] = useState<string | null>(null)
  const installTemplate = $api.useMutation(
    "post",
    "/v1/employees/{id}/agent-templates/{slug}/install"
  )

  function install(template: AgentTemplate) {
    if (!employeeID || !template.slug || template.installed) return
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
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/employees/{id}"],
          })
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/employees/{id}/agent-templates"],
          })
        },
        onError: (err) => {
          toast.error(extractErrorMessage(err, "Failed to install template"))
        },
        onSettled: () => setInstallingSlug(null),
      }
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[min(680px,85vh)] flex-col overflow-hidden p-6 sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Manage specialists</DialogTitle>
          <DialogDescription>
            Add category specialists to {employeeName}.
          </DialogDescription>
        </DialogHeader>

        <div className="mt-4 flex flex-1 flex-col gap-2 overflow-y-auto">
          {loading ? (
            <TemplatesSkeleton />
          ) : error ? (
            <div className="flex gap-3 rounded-xl bg-destructive/10 p-4 text-sm text-destructive">
              <HugeiconsIcon
                icon={Alert02Icon}
                className="mt-0.5 size-4 shrink-0"
                strokeWidth={2}
              />
              <span>{error}</span>
            </div>
          ) : templates.length === 0 ? (
            <div className="flex flex-col items-center justify-center gap-3 py-12">
              <div className="flex size-12 items-center justify-center rounded-full bg-muted">
                <HugeiconsIcon
                  icon={BotIcon}
                  size={20}
                  className="text-muted-foreground"
                />
              </div>
              <div className="text-center">
                <p className="text-sm font-medium text-foreground">
                  No specialists available
                </p>
                <p className="mt-1 max-w-[260px] text-xs text-muted-foreground">
                  This employee category has no installable specialists yet.
                </p>
              </div>
            </div>
          ) : (
            templates.map((template) => (
              <TemplateChoiceCard
                key={template.slug}
                template={template}
                installing={installingSlug === template.slug}
                disabled={installTemplate.isPending}
                onInstall={() => install(template)}
              />
            ))
          )}
        </div>

        <div className="shrink-0 pt-4">
          <Button
            type="button"
            variant="outline"
            className="w-full"
            onClick={() => onOpenChange(false)}
          >
            Done
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function TemplateChoiceCard({
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
    <ChoiceCard
      icon={BotIcon}
      logoSize={32}
      title={template.name ?? "Agent template"}
      description={template.description ?? ""}
      selected={installed}
      onClick={installed || disabled ? () => {} : onInstall}
      trailing={
        installed ? (
          <HugeiconsIcon
            icon={Tick02Icon}
            size={16}
            className="mt-0.5 shrink-0 text-primary"
          />
        ) : (
          <span
            className={[
              "inline-flex h-8 shrink-0 items-center justify-center rounded-full bg-primary px-3 text-xs font-medium text-primary-foreground",
              disabled ? "opacity-60" : "",
            ]
              .filter(Boolean)
              .join(" ")}
          >
            <HugeiconsIcon icon={Add01Icon} data-icon="inline-start" />
            {installing ? "Installing" : "Install"}
          </span>
        )
      }
    />
  )
}

function InstalledTemplateRow({ template }: { template: AgentTemplate }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-xl border border-border bg-muted/50 p-3">
      <div className="flex min-w-0 items-center gap-3">
        <div className="flex size-8 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <HugeiconsIcon icon={BotIcon} className="size-4" strokeWidth={2} />
        </div>
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-foreground">
            {template.name}
          </p>
          <p className="truncate text-xs text-muted-foreground">
            {formatCategoryLabel(template.category)}
          </p>
        </div>
      </div>
      <Badge
        variant="ghost"
        className="shrink-0 gap-1.5 bg-success/15 text-success"
      >
        <HugeiconsIcon icon={Tick02Icon} className="size-3" strokeWidth={2.75} />
        Installed
      </Badge>
    </div>
  )
}

function TemplatesSkeleton() {
  return (
    <div className="flex flex-col gap-2" aria-busy="true">
      {Array.from({ length: 2 }).map((_, index) => (
        <Skeleton key={index} className="h-[66px] rounded-xl" />
      ))}
    </div>
  )
}
