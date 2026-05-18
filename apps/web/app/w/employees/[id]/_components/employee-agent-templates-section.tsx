"use client"

import { useEffect, useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  Add01Icon,
  BotIcon,
  CubeIcon,
  Settings02Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import { ChoiceCard } from "@/app/w/agents/_components/create-agent/choice-card"
import { FormEmptyWell, FormSection } from "@/app/w/_components/form-section"
import { AllModelsCombobox } from "@/components/all-models-combobox"
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
import { useSandboxTemplates } from "@/hooks/use-sandbox-template"
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
  const [editingTemplate, setEditingTemplate] = useState<AgentTemplate | null>(
    null
  )
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

  const templates = useMemo(
    () => templatesQuery.data ?? [],
    [templatesQuery.data]
  )
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
                <InstalledTemplateRow
                  key={template.slug}
                  template={template}
                  onEdit={() => setEditingTemplate(template)}
                />
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

      <EditSpecialistAgentDialog
        employeeID={employeeID}
        template={editingTemplate}
        open={Boolean(editingTemplate)}
        onOpenChange={(open) => {
          if (!open) setEditingTemplate(null)
        }}
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

function InstalledTemplateRow({
  template,
  onEdit,
}: {
  template: AgentTemplate
  onEdit: () => void
}) {
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
      <div className="flex shrink-0 items-center gap-2">
        <button
          type="button"
          onClick={onEdit}
          disabled={!template.subagent_id}
          className="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40"
          aria-label={`Edit ${template.name ?? "specialist"}`}
          title="Edit specialist"
        >
          <HugeiconsIcon icon={Settings02Icon} size={15} strokeWidth={2.25} />
        </button>
        <Badge
          variant="ghost"
          className="gap-1.5 bg-success/15 text-success"
        >
          <HugeiconsIcon
            icon={Tick02Icon}
            className="size-3"
            strokeWidth={2.75}
          />
          Installed
        </Badge>
      </div>
    </div>
  )
}

function EditSpecialistAgentDialog({
  employeeID,
  template,
  open,
  onOpenChange,
}: {
  employeeID: string
  template: AgentTemplate | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const subagentID = template?.subagent_id ?? ""
  const [model, setModel] = useState("")
  const [sandboxTemplateId, setSandboxTemplateId] = useState("")
  const agentQuery = $api.useQuery(
    "get",
    "/v1/agents/{id}",
    {
      params: { path: { id: subagentID } },
    },
    {
      enabled: open && Boolean(subagentID),
    }
  )
  const { data: sandboxTemplatesData, isLoading: sandboxTemplatesLoading } =
    useSandboxTemplates()
  const sandboxTemplates = sandboxTemplatesData?.data ?? []
  const updateAgent = $api.useMutation("put", "/v1/agents/{id}")

  useEffect(() => {
    if (!open) return
    setModel(agentQuery.data?.model ?? "")
    setSandboxTemplateId(agentQuery.data?.sandbox_template_id ?? "")
  }, [agentQuery.data?.model, agentQuery.data?.sandbox_template_id, open])

  function handleSave() {
    if (!subagentID) return
    const nextModel = model.trim()
    const currentModel = (agentQuery.data?.model ?? "").trim()
    updateAgent.mutate(
      {
        params: { path: { id: subagentID } },
        body: {
          model: nextModel !== currentModel ? nextModel : undefined,
          sandbox_template_id: sandboxTemplateId || "",
        },
      },
      {
        onSuccess: () => {
          toast.success(`${template?.name ?? "Specialist"} updated`)
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents"] })
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/agents/{id}"],
          })
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/employees/{id}"],
          })
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/employees/{id}/agent-templates"],
          })
          onOpenChange(false)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to update specialist"))
        },
      }
    )
  }

  const loadingAgent = agentQuery.isLoading || (open && !agentQuery.data)
  const canSave = Boolean(subagentID) && !loadingAgent && !updateAgent.isPending

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col overflow-hidden p-6 sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Edit {template?.name ?? "specialist"}</DialogTitle>
          <DialogDescription>
            Update the model and sandbox template used by this specialist.
          </DialogDescription>
        </DialogHeader>

        <div className="mt-4 flex flex-1 flex-col gap-6 overflow-y-auto pr-1">
          {agentQuery.isError ? (
            <div className="flex gap-3 rounded-xl bg-destructive/10 p-4 text-sm text-destructive">
              <HugeiconsIcon
                icon={Alert02Icon}
                className="mt-0.5 size-4 shrink-0"
                strokeWidth={2}
              />
              <span>
                {extractErrorMessage(
                  agentQuery.error,
                  "Failed to load specialist"
                )}
              </span>
            </div>
          ) : loadingAgent ? (
            <div className="flex flex-col gap-3">
              <Skeleton className="h-[62px] rounded-xl" />
              <Skeleton className="h-[52px] rounded-xl" />
              <Skeleton className="h-[52px] rounded-xl" />
            </div>
          ) : (
            <>
              <FormSection
                title="Model"
                description="The model this specialist runs on. Pick one that matches the specialist's task."
              >
                <AllModelsCombobox
                  value={model || null}
                  onSelect={(value) => setModel(value)}
                />
              </FormSection>

              <FormSection
                title="Sandbox template"
                description="Optional. Pick a template to launch this agent's sandbox from a pre-built image. Leave empty to use the platform default."
              >
                <div className="flex flex-col gap-2">
                  {sandboxTemplatesLoading ? (
                    Array.from({ length: 2 }).map((_, i) => (
                      <Skeleton key={i} className="h-[52px] w-full rounded-xl" />
                    ))
                  ) : sandboxTemplates.length === 0 ? (
                    <FormEmptyWell
                      icon={CubeIcon}
                      message="No sandbox templates yet. Create one in Settings → Sandboxes."
                    />
                  ) : (
                    <>
                      <button
                        type="button"
                        onClick={() => setSandboxTemplateId("")}
                        className={
                          "flex items-center justify-between rounded-xl border px-4 py-3 text-left transition-colors " +
                          (!sandboxTemplateId
                            ? "border-primary bg-primary/5"
                            : "border-border bg-muted/50 hover:bg-muted")
                        }
                      >
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-medium text-foreground">
                            Default base image
                          </p>
                          <p className="mt-0.5 line-clamp-1 text-xs text-muted-foreground">
                            Launch from the platform&apos;s standard sandbox
                            image.
                          </p>
                        </div>
                        {!sandboxTemplateId ? (
                          <HugeiconsIcon
                            icon={Tick02Icon}
                            size={16}
                            className="ml-2 shrink-0 text-primary"
                          />
                        ) : null}
                      </button>
                      {sandboxTemplates.map((tmpl) => {
                        if (!tmpl.id) return null
                        const selected = sandboxTemplateId === tmpl.id
                        const ready = tmpl.build_status === "ready"
                        return (
                          <button
                            key={tmpl.id}
                            type="button"
                            disabled={!ready}
                            onClick={() => setSandboxTemplateId(tmpl.id!)}
                            className={
                              "flex items-center justify-between rounded-xl border px-4 py-3 text-left transition-colors " +
                              (selected
                                ? "border-primary bg-primary/5"
                                : "border-border bg-muted/50 hover:bg-muted") +
                              (!ready ? " cursor-not-allowed opacity-60" : "")
                            }
                          >
                            <div className="min-w-0 flex-1">
                              <p className="text-sm font-medium text-foreground">
                                {tmpl.name}
                              </p>
                              <p className="mt-0.5 line-clamp-1 text-xs text-muted-foreground">
                                {tmpl.size ?? "default"} ·{" "}
                                {tmpl.build_status ?? "pending"}
                              </p>
                            </div>
                            {selected ? (
                              <HugeiconsIcon
                                icon={Tick02Icon}
                                size={16}
                                className="ml-2 shrink-0 text-primary"
                              />
                            ) : !ready ? (
                              <Badge
                                variant="secondary"
                                className="ml-2 shrink-0 text-[10px]"
                              >
                                {tmpl.build_status}
                              </Badge>
                            ) : null}
                          </button>
                        )
                      })}
                    </>
                  )}
                </div>
              </FormSection>
            </>
          )}
        </div>

        <DialogFooter className="mt-5 shrink-0">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button
            type="button"
            onClick={handleSave}
            disabled={!canSave}
            loading={updateAgent.isPending}
          >
            Save changes
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
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
