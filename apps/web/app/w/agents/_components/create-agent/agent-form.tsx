"use client"

import * as React from "react"
import Link from "next/link"
import { Controller } from "react-hook-form"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Skeleton } from "@/components/ui/skeleton"
import { Badge } from "@/components/ui/badge"
import { LlmKeyCard } from "@/components/llm-key-card"
import { AllModelsCombobox } from "@/components/all-models-combobox"
import { IntegrationLogo } from "@/components/integration-logo"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  FlashIcon,
  Key01Icon,
  MagicWandIcon,
  Plug01Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import { useCreateAgent } from "./context"
import { useEnhancePrompt } from "./use-enhance-prompt"
import { SystemPromptEditor } from "@/app/w/agents/new/_components/system-prompt-editor"
import { AddLlmKeyDialog } from "./add-llm-key-dialog"
import {
  ManageIntegrationsDialog,
  type AgentIntegrations,
} from "@/app/w/agents/_components/manage-integrations-dialog"
import {
  EditTriggersDialog,
  HttpEndpointPill,
  TriggerTypeAvatar,
  triggerDisplayName,
} from "@/app/w/agents/_components/edit-triggers-dialog"
import { ToolPermissionsSection } from "./tool-permissions"
import { ImagePicker } from "@/components/image-picker"
import { CategoryCombobox } from "@/app/w/agents/new/_components/category-combobox"
import { CreateSkillDialog } from "@/app/w/settings/_components/create-skill-dialog"
import type { components } from "@/lib/api/schema"
import type { SkillPreview } from "./types"

type SkillResponse = components["schemas"]["skillResponse"]

function toSkillPreview(skill: SkillResponse): SkillPreview | null {
  if (!skill.id || !skill.name || !skill.slug) return null
  return {
    id: skill.id,
    slug: skill.slug,
    name: skill.name,
    description: skill.description ?? "",
    sourceType: skill.source_type === "git" ? "git" : "inline",
    scope: skill.org_id ? "org" : "public",
    tags: skill.tags ?? [],
    installCount: skill.install_count ?? 0,
    featured: skill.featured ?? false,
  }
}

export function AgentForm() {
  const {
    mode,
    form,
    selectedIntegrations,
    selectedActions,
    setSelectedIntegrations,
    setSelectedActions,
    selectedSkills,
    selectedSubagents,
    toggleSkill,
    triggers,
    addTrigger,
    removeTrigger,
    updateTrigger,
    handleSubmit,
    isSubmitting,
  } = useCreateAgent()

  const { activeOrg } = useAuth()
  const showLlmKey = activeOrg?.byok !== false

  const credentialId = form.watch("credentialId")
  const model = form.watch("model")
  const sharedMemory = form.watch("sharedMemory")
  const permissions = form.watch("permissions")

  const { enhance, isEnhancing, output } = useEnhancePrompt()

  React.useEffect(() => {
    if (isEnhancing) {
      form.setValue("systemPrompt", output)
    }
  }, [output, isEnhancing, form])

  const handleEnhance = React.useCallback(
    (brief: string) => {
      void enhance({
        brief,
        values: form.getValues(),
        selectedIntegrations,
        selectedActions,
        selectedSkills,
        selectedSubagents,
        triggers,
      })
    },
    [enhance, form, selectedIntegrations, selectedActions, selectedSkills, selectedSubagents, triggers],
  )
  const name = form.watch("name")

  React.useEffect(() => {
    if (!showLlmKey) {
      form.setValue("credentialId", "")
    }
  }, [showLlmKey, form])

  const { data: credentialsData, isLoading: credentialsLoading } = $api.useQuery(
    "get",
    "/v1/credentials",
    {},
    { enabled: showLlmKey }
  )
  const credentials = credentialsData?.data ?? []

  const { data: connectionsData } = $api.useQuery("get", "/v1/in/connections")
  const connections = connectionsData?.data ?? []
  const connectionsById = new Map(connections.map((c) => [c.id, c]))

  const { data: skillsData, isLoading: skillsLoading } = $api.useQuery(
    "get",
    "/v1/skills"
  )
  const skills = skillsData?.data ?? []

  const { data: categoriesData, isLoading: categoriesLoading } = $api.useQuery(
    "get",
    "/v1/agents/categories"
  )
  const categories = React.useMemo(
    () =>
      (categoriesData ?? []).map((c) => ({
        id: c.id ?? "",
        name: c.name ?? c.id ?? "",
        description: c.description ?? undefined,
      })),
    [categoriesData]
  )

  const [addKeyOpen, setAddKeyOpen] = React.useState(false)
  const [integrationsOpen, setIntegrationsOpen] = React.useState(false)
  const [editTriggersOpen, setEditTriggersOpen] = React.useState(false)
  const [createSkillOpen, setCreateSkillOpen] = React.useState(false)

  const agentIntegrations: AgentIntegrations = React.useMemo(() => {
    const result: AgentIntegrations = {}
    for (const id of selectedIntegrations) {
      const actions = selectedActions[id]
      result[id] = { actions: actions ? Array.from(actions) : [] }
    }
    return result
  }, [selectedIntegrations, selectedActions])

  function saveIntegrations(next: AgentIntegrations) {
    setSelectedIntegrations(new Set(Object.keys(next)))
    const nextActions: Record<string, Set<string>> = {}
    for (const [id, cfg] of Object.entries(next)) {
      nextActions[id] = new Set(cfg.actions)
    }
    setSelectedActions(nextActions)
  }

  function removeIntegration(connectionId: string) {
    setSelectedIntegrations((prev) => {
      const next = new Set(prev)
      next.delete(connectionId)
      return next
    })
    setSelectedActions((prev) => {
      const next = { ...prev }
      delete next[connectionId]
      return next
    })
  }

  const canSubmit = Boolean(
    name.trim() &&
      (!showLlmKey || (credentialId && model)) &&
      !isSubmitting
  )

  const isEdit = mode === "edit"
  const pageTitle = isEdit ? "Edit agent" : "New agent"
  const submitLabel = isEdit ? "Save changes" : "Create agent"

  return (
    <>
      <PageHeader
        title={pageTitle}
        actions={
          <>
            <Button variant="ghost" render={<Link href="/w/agents" />}>
              Cancel
            </Button>
            <Button
              onClick={handleSubmit}
              disabled={!canSubmit}
              loading={isSubmitting}
            >
              {submitLabel}
            </Button>
          </>
        }
      />

      <div className="mx-auto w-full max-w-2xl px-6 pt-10 pb-20">
        <div className="divide-y divide-border/60 [&>section]:py-7 [&>section:first-child]:pt-0 [&>section:last-child]:pb-0">
          <Section
            title="Persona"
            description="Give your agent a specific name and description. Golden rule: One agent that does one thing, and does it really well"
            aside={
              <Controller
                name="avatarUrl"
                control={form.control}
                render={({ field }) => (
                  <ImagePicker
                    assetType="avatar"
                    value={field.value || undefined}
                    onChange={(url) => field.onChange(url ?? "")}
                    fallback="A"
                    ariaLabel={field.value ? "Replace avatar" : "Upload avatar"}
                  />
                )}
              />
            }
          >
            <div className="flex flex-col gap-2">
              <Label htmlFor="name" className="text-[13px] font-medium">
                Name
              </Label>
              <Input
                id="name"
                {...form.register("name")}
                placeholder="e.g. Issue Triage Agent"
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="desc" className="text-[13px] font-medium">
                Description{" "}
                <span className="font-normal text-muted-foreground">(optional)</span>
              </Label>
              <Textarea
                id="desc"
                {...form.register("description")}
                className="min-h-20"
                placeholder="What this agent does."
              />
            </div>
          </Section>

          <Section
            title="Category"
            description="Group this agent by the function it serves in your workspace."
          >
            <Controller
              name="category"
              control={form.control}
              render={({ field }) => (
                <CategoryCombobox
                  categories={categories}
                  loading={categoriesLoading}
                  value={field.value}
                  onSelect={field.onChange}
                />
              )}
            />
          </Section>

          <Section
            title="Model"
            description="The model your agent runs on. Pick one that matches the agent's task."
          >
            <AllModelsCombobox
              value={model || null}
              onSelect={(value) => form.setValue("model", value)}
            />
          </Section>

          {showLlmKey ? (
          <Section
            title="LLM key"
            description="Pick a credential to bill this agent's usage to. The platform picks one automatically when no credential is selected."
          >
            <div className="flex flex-col gap-2">
              {credentialsLoading ? (
                Array.from({ length: 2 }).map((_, i) => (
                  <Skeleton key={i} className="h-[60px] w-full rounded-xl" />
                ))
              ) : credentials.length === 0 ? (
                <EmptyWell
                  icon={Key01Icon}
                  message="No LLM keys connected yet."
                  action={
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setAddKeyOpen(true)}
                    >
                      <HugeiconsIcon
                        icon={Key01Icon}
                        size={14}
                        data-icon="inline-start"
                      />
                      Add LLM key
                    </Button>
                  }
                />
              ) : (
                <>
                {credentials.map((credential) => {
                  const id = credential.id ?? ""
                  return (
                    <LlmKeyCard
                      key={id}
                      label={credential.label}
                      providerId={credential.provider_id ?? ""}
                      selected={credentialId === id}
                      onClick={() => {
                        form.setValue("credentialId", id)
                      }}
                    />
                  )
                })}
                <Button
                  variant="outline"
                  size="sm"
                  className="mt-1 w-fit"
                  onClick={() => setAddKeyOpen(true)}
                >
                  <HugeiconsIcon
                    icon={Key01Icon}
                    size={14}
                    data-icon="inline-start"
                  />
                  Add LLM key
                </Button>
                </>
              )}
            </div>

          </Section>
          ) : null}

          <Section
            title="Integrations"
            description="External services your agent can access."
          >
            <div className="flex flex-col gap-2">
              {selectedIntegrations.size === 0 ? (
                <EmptyWell
                  icon={Plug01Icon}
                  message="No integrations configured."
                  action={
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setIntegrationsOpen(true)}
                    >
                      Manage integrations
                    </Button>
                  }
                />
              ) : (
                <>
                {Array.from(selectedIntegrations).map((connectionId) => {
                  const connection = connectionsById.get(connectionId)
                  const actions = selectedActions[connectionId] ?? new Set()
                  return (
                    <div
                      key={connectionId}
                      className="flex items-center justify-between gap-3 rounded-xl border border-border bg-muted/50 p-3"
                    >
                      <div className="flex min-w-0 items-center gap-3">
                        <IntegrationLogo
                          provider={connection?.provider ?? ""}
                          size={32}
                        />
                        <div className="min-w-0">
                          <p className="truncate text-sm font-medium text-foreground">
                            {connection?.display_name ?? connectionId}
                          </p>
                          <p className="text-xs text-muted-foreground">
                            {actions.size} action{actions.size !== 1 ? "s" : ""}{" "}
                            enabled
                          </p>
                        </div>
                      </div>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="shrink-0 text-destructive hover:text-destructive"
                        onClick={() => removeIntegration(connectionId)}
                      >
                        Remove
                      </Button>
                    </div>
                  )
                })}
                <Button
                  variant="outline"
                  size="sm"
                  className="mt-1 w-fit"
                  onClick={() => setIntegrationsOpen(true)}
                >
                  Manage integrations
                </Button>
                </>
              )}
            </div>
          </Section>

          <Section
            title="Triggers"
            description="Webhook events that automatically invoke this agent."
          >
            <div className="flex flex-col gap-2">
              {triggers.length === 0 ? (
                <EmptyWell
                  icon={FlashIcon}
                  message="No triggers configured. This agent is invoked manually."
                  action={
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setEditTriggersOpen(true)}
                    >
                      <HugeiconsIcon
                        icon={FlashIcon}
                        size={14}
                        data-icon="inline-start"
                      />
                      Edit triggers
                    </Button>
                  }
                />
              ) : (
                <>
                {triggers.map((trigger, index) => (
                  <div
                    key={`${trigger.triggerType}-${trigger.connectionId}-${index}`}
                    className="flex items-start gap-3 rounded-xl border border-border bg-muted/50 p-3"
                  >
                    <TriggerTypeAvatar trigger={trigger} size={28} />
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-foreground">
                        {triggerDisplayName(trigger)}
                      </p>
                      {trigger.triggerType === "webhook" ? (
                        <div className="mt-1 flex flex-wrap gap-1">
                          {trigger.triggerKeys.map((key) => (
                            <Badge
                              key={key}
                              variant="secondary"
                              className="font-mono text-[10px]"
                            >
                              {key}
                            </Badge>
                          ))}
                        </div>
                      ) : null}
                      {trigger.triggerType === "cron" && trigger.cronSchedule ? (
                        <p className="mt-1 font-mono text-[11px] text-muted-foreground">
                          {trigger.cronSchedule}
                        </p>
                      ) : null}
                      {trigger.triggerType === "http" ? (
                        <div className="mt-2">
                          <HttpEndpointPill />
                          {trigger.secretKey ? (
                            <p className="mt-1.5 text-[11px] text-muted-foreground">
                              HMAC verification enabled
                            </p>
                          ) : null}
                        </div>
                      ) : null}
                    </div>
                  </div>
                ))}
                <Button
                  variant="outline"
                  size="sm"
                  className="mt-1 w-fit"
                  onClick={() => setEditTriggersOpen(true)}
                >
                  <HugeiconsIcon
                    icon={FlashIcon}
                    size={14}
                    data-icon="inline-start"
                  />
                  Edit triggers
                </Button>
                </>
              )}
            </div>
          </Section>

          <Section
            title="Skills"
            description="Attach skills that give your agent specialized capabilities."
          >
            <div className="flex flex-col gap-2">
              {skillsLoading ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <Skeleton key={i} className="h-[52px] w-full rounded-xl" />
                ))
              ) : skills.length === 0 ? (
                <EmptyWell
                  icon={MagicWandIcon}
                  message="No skills available yet."
                  action={
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setCreateSkillOpen(true)}
                    >
                      <HugeiconsIcon
                        icon={Add01Icon}
                        size={14}
                        data-icon="inline-start"
                      />
                      Create skill
                    </Button>
                  }
                />
              ) : (
                <>
                {skills.map((skill) => {
                  const preview = toSkillPreview(skill)
                  if (!preview) return null
                  const selected = selectedSkills.has(preview.id)
                  return (
                    <button
                      key={preview.id}
                      type="button"
                      onClick={() => toggleSkill(preview)}
                      className={
                        "flex items-center justify-between rounded-xl border px-4 py-3 text-left transition-colors " +
                        (selected
                          ? "border-primary bg-primary/5"
                          : "border-border bg-muted/50 hover:bg-muted")
                      }
                    >
                      <div className="min-w-0 flex-1">
                        <p className="text-sm font-medium text-foreground">
                          {preview.name}
                        </p>
                        {preview.description ? (
                          <p className="mt-0.5 line-clamp-1 text-xs text-muted-foreground">
                            {preview.description}
                          </p>
                        ) : null}
                      </div>
                      {selected ? (
                        <HugeiconsIcon
                          icon={Tick02Icon}
                          size={16}
                          className="ml-2 shrink-0 text-primary"
                        />
                      ) : null}
                    </button>
                  )
                })}
                <Button
                  variant="outline"
                  size="sm"
                  className="mt-1 w-fit"
                  onClick={() => setCreateSkillOpen(true)}
                >
                  <HugeiconsIcon
                    icon={Add01Icon}
                    size={14}
                    data-icon="inline-start"
                  />
                  Create skill
                </Button>
                </>
              )}
            </div>
          </Section>

          <Section
            title="Tool permissions"
            description="Click a tool to cycle between allow, require approval, and deny."
          >
            <ToolPermissionsSection
              mode={mode}
              permissions={permissions}
              onChange={(next) => form.setValue("permissions", next)}
            />
          </Section>

          <Section
            title="Instructions"
            description="The base instructions that shape your agent's behavior. Markdown supported."
          >
            <Controller
              name="systemPrompt"
              control={form.control}
              render={({ field }) => (
                <SystemPromptEditor
                  value={field.value}
                  onChange={field.onChange}
                  onEnhance={handleEnhance}
                  isEnhancing={isEnhancing}
                />
              )}
            />
          </Section>

          <Section title="Advanced">
            <div className="flex items-start justify-between gap-6">
              <div className="flex-1">
                <Label className="text-[13px] font-medium">Workspace memory</Label>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  All agents already have long-term memory. This setting allows this agent to share memories with other agents in your workspace.
                </p>
              </div>
              <Switch
                checked={sharedMemory}
                onCheckedChange={(checked) =>
                  form.setValue("sharedMemory", checked)
                }
                className="mt-0.5"
              />
            </div>
          </Section>
        </div>
      </div>

      <AddLlmKeyDialog
        open={addKeyOpen}
        onOpenChange={setAddKeyOpen}
        onCreated={(id) => {
          form.setValue("credentialId", id)
          setAddKeyOpen(false)
        }}
      />

      <ManageIntegrationsDialog
        open={integrationsOpen}
        onOpenChange={setIntegrationsOpen}
        agentIntegrations={agentIntegrations}
        onSave={(next) => {
          saveIntegrations(next)
          setIntegrationsOpen(false)
        }}
      />

      <EditTriggersDialog
        open={editTriggersOpen}
        onOpenChange={setEditTriggersOpen}
        triggers={triggers}
        connectionIds={selectedIntegrations}
        onAdd={addTrigger}
        onRemove={removeTrigger}
        onUpdate={updateTrigger}
      />

      <CreateSkillDialog
        open={createSkillOpen}
        onOpenChange={setCreateSkillOpen}
        onCreated={(skill) => {
          const preview = toSkillPreview(skill)
          if (preview && !selectedSkills.has(preview.id)) {
            toggleSkill(preview)
          }
        }}
      />
    </>
  )
}

function Section({
  title,
  description,
  aside,
  children,
}: {
  title: string
  description?: string
  aside?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <section className="flex flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0 flex-1">
          <h2 className="text-[15px] font-medium text-foreground">{title}</h2>
          {description ? (
            <p className="mt-0.5 text-[12px] text-muted-foreground">
              {description}
            </p>
          ) : null}
        </div>
        {aside ? <div className="shrink-0">{aside}</div> : null}
      </div>
      {children}
    </section>
  )
}

function EmptyWell({
  icon,
  message,
  action,
}: {
  icon: typeof Plug01Icon
  message: string
  action?: React.ReactNode
}) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-xl bg-muted/30 px-6 py-9 text-center">
      <div className="flex size-10 items-center justify-center rounded-lg bg-muted text-muted-foreground">
        <HugeiconsIcon icon={icon} strokeWidth={2} className="size-4" />
      </div>
      <p className="text-[13px] text-muted-foreground">{message}</p>
      {action ? <div className="mt-1">{action}</div> : null}
    </div>
  )
}
