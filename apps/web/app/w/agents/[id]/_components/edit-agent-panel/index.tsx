"use client"

import { useState } from "react"
import {
  FloatingPanel,
  FloatingPanelContent,
  FloatingPanelHeader,
  FloatingPanelBody,
  FloatingPanelFooter,
  FloatingPanelTitle,
  FloatingPanelDescription,
} from "@/components/ui/floating-panel"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { ProviderPromptEditor } from "@/app/w/agents/_components/create-agent/provider-prompt-editor"
import { AddTriggerDialog } from "@/app/w/agents/_components/add-trigger-dialog"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { Skeleton } from "@/components/ui/skeleton"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  CloudServerIcon,
  LaptopProgrammingIcon,
  Tick02Icon,
  Cancel01Icon,
  FlashIcon,
} from "@hugeicons/core-free-icons"
import { Badge } from "@/components/ui/badge"
import { IntegrationLogo } from "@/components/integration-logo"
import { LlmKeyCard } from "@/components/llm-key-card"
import { ProviderModelCombobox } from "@/components/provider-model-combobox"
import { $api } from "@/lib/api/hooks"
import {
  ManageIntegrationsDialog,
} from "@/app/w/agents/_components/manage-integrations-dialog"
import { EditAgentProvider, useEditAgent } from "./context"
import { SkillsSection } from "./skills-section"
import { ToolPermissionsSection } from "./tool-permissions"
import type { components } from "@/lib/api/schema"

type Agent = components["schemas"]["agentResponse"]

interface EditAgentPanelProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  agent?: Agent | null
}

export function EditAgentPanel({ open, onOpenChange, agent }: EditAgentPanelProps) {
  return (
    <EditAgentProvider agent={agent ?? null} open={open} onClose={() => onOpenChange(false)}>
      <FloatingPanel open={open} onOpenChange={onOpenChange}>
        <FloatingPanelContent width="lg:w-[540px]">
          <EditAgentForm />
        </FloatingPanelContent>
      </FloatingPanel>
    </EditAgentProvider>
  )
}

// ---------------------------------------------------------------------------

function SectionHeader({ title, description }: { title: string; description?: string }) {
  return (
    <div className="flex flex-col gap-1">
      <h3 className="text-sm font-medium text-foreground">{title}</h3>
      {description && <p className="text-xs text-muted-foreground">{description}</p>}
    </div>
  )
}

function SandboxOption({
  icon,
  title,
  description,
  selected,
  onClick,
}: {
  icon: typeof CloudServerIcon
  title: string
  description: string
  selected: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex items-start gap-3 rounded-xl border p-3 text-left transition-colors ${
        selected
          ? "border-primary bg-primary/5"
          : "border-border bg-muted/50 hover:bg-muted"
      }`}
    >
      <HugeiconsIcon
        icon={icon}
        size={18}
        className={selected ? "text-primary" : "text-muted-foreground"}
      />
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium text-foreground">{title}</p>
        <p className="text-xs text-muted-foreground mt-0.5">{description}</p>
      </div>
      {selected && (
        <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0 mt-0.5" />
      )}
    </button>
  )
}

// ---------------------------------------------------------------------------

function EditAgentForm() {
  const { form, integrations, triggers, skillIds, isSubmitting, setIntegrations, removeIntegration, addTrigger, removeTrigger, toggleSkill, handleSave } = useEditAgent()
  const [integrationsOpen, setIntegrationsOpen] = useState(false)
  const [addTriggerOpen, setAddTriggerOpen] = useState(false)

  const credentialId = form.watch("credentialId")
  const sandboxType = form.watch("sandboxType")
  const sharedMemory = form.watch("sharedMemory")

  const { data: connectionsData } = $api.useQuery("get", "/v1/in/connections")
  const connections = connectionsData?.data ?? []
  const connectionsById = new Map(connections.map((c) => [c.id, c]))

  const { data: credentialsData, isLoading: credentialsLoading } = $api.useQuery("get", "/v1/credentials")
  const credentials = credentialsData?.data ?? []
  const selectedCredential = credentials.find((c) => c.id === credentialId)

  return (
    <>
      <FloatingPanelHeader>
        <FloatingPanelTitle>Edit agent</FloatingPanelTitle>
        <FloatingPanelDescription>
          Update your agent&apos;s configuration and behavior.
        </FloatingPanelDescription>
      </FloatingPanelHeader>

      <FloatingPanelBody className="flex flex-col gap-8">
        {/* Basics */}
        <section className="flex flex-col gap-4">
          <SectionHeader title="Basics" />

          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-name" className="text-sm">Name</Label>
            <Input id="edit-name" {...form.register("name")} />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-description" className="text-sm">
              Description{" "}
              <span className="text-muted-foreground font-normal">(optional)</span>
            </Label>
            <Textarea
              id="edit-description"
              {...form.register("description")}
              className="min-h-20"
            />
          </div>
        </section>

        <Separator />

        {/* Credential & Model */}
        <section className="flex flex-col gap-4">
          <SectionHeader
            title="LLM key"
            description="The AI provider credential and model your agent uses."
          />

          <div className="flex flex-col gap-2">
            {credentialsLoading ? (
              Array.from({ length: 2 }).map((_, i) => (
                <Skeleton key={i} className="h-[60px] w-full rounded-xl" />
              ))
            ) : credentials.length === 0 ? (
              <p className="text-sm text-muted-foreground">No credentials yet.</p>
            ) : (
              credentials.map((credential) => {
                const id = credential.id ?? ""
                return (
                  <LlmKeyCard
                    key={id}
                    label={credential.label}
                    providerId={credential.provider_id ?? ""}
                    selected={credentialId === id}
                    onClick={() => {
                      if (credentialId !== id) form.setValue("model", "")
                      form.setValue("credentialId", id)
                    }}
                  />
                )
              })
            )}
          </div>

          {credentialId && (
            <div className="flex flex-col gap-2">
              <Label className="text-sm">Model</Label>
              <ProviderModelCombobox
                providerId={selectedCredential?.provider_id ?? ""}
                value={form.watch("model") || null}
                onSelect={(value) => form.setValue("model", value)}
              />
            </div>
          )}
        </section>

        <Separator />

        {/* Sandbox */}
        <section className="flex flex-col gap-4">
          <SectionHeader
            title="Workspace"
            description="The isolated environment where your agent runs."
          />

          <div className="flex flex-col gap-2">
            <SandboxOption
              icon={CloudServerIcon}
              title="Shared workspace"
              description="For agents that interact with APIs and tools."
              selected={sandboxType === "shared"}
              onClick={() => form.setValue("sandboxType", "shared")}
            />
            <SandboxOption
              icon={LaptopProgrammingIcon}
              title="Dedicated workspace"
              description="Full system access for file I/O and shell commands."
              selected={sandboxType === "dedicated"}
              onClick={() => form.setValue("sandboxType", "dedicated")}
            />
          </div>
        </section>

        <Separator />

        {/* System Prompt */}
        <section className="flex flex-col gap-4">
          <SectionHeader
            title="System prompt"
            description="Define per-provider prompts that shape your agent's behavior."
          />
          <ProviderPromptEditor
            value={form.watch("providerPrompts")}
            onChange={(nextValue) => form.setValue("providerPrompts", nextValue)}
            className="min-h-80"
          />
        </section>

        <Separator />

        {/* Instructions */}
        <section className="flex flex-col gap-4">
          <SectionHeader
            title="Instructions"
            description="Additional rules and guidelines for the agent."
          />
          <Textarea
            {...form.register("instructions")}
            className="min-h-28 font-mono text-sm"
          />
        </section>

        <Separator />

        {/* Integrations */}
        <section className="flex flex-col gap-4">
          <SectionHeader
            title="Integrations"
            description="External services your agent can access."
          />

          <div className="flex flex-col gap-2">
            {Object.keys(integrations).length === 0 ? (
              <p className="text-sm text-muted-foreground">No integrations configured.</p>
            ) : (
              Object.entries(integrations).map(([connectionId, config]) => {
                const connection = connectionsById.get(connectionId)
                return (
                  <div
                    key={connectionId}
                    className="flex items-center justify-between rounded-xl border border-border bg-muted/50 p-3"
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <IntegrationLogo provider={connection?.provider ?? ""} size={32} />
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-foreground truncate">
                          {connection?.display_name ?? connectionId}
                        </p>
                        <p className="text-xs text-muted-foreground">
                          {config.actions.length} action{config.actions.length !== 1 ? "s" : ""} enabled
                        </p>
                      </div>
                    </div>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-destructive hover:text-destructive shrink-0"
                      onClick={() => removeIntegration(connectionId)}
                    >
                      Remove
                    </Button>
                  </div>
                )
              })
            )}

            <Button
              variant="outline"
              size="sm"
              className="w-fit mt-1"
              onClick={() => setIntegrationsOpen(true)}
            >
              Manage integrations
            </Button>
          </div>
        </section>

        <Separator />

        {/* Triggers */}
        <section className="flex flex-col gap-4">
          <SectionHeader
            title="Triggers"
            description="Webhook events that automatically invoke this agent."
          />

          <div className="flex flex-col gap-2">
            {triggers.length === 0 ? (
              <p className="text-sm text-muted-foreground">No triggers configured. This agent is invoked through Zira or manually.</p>
            ) : (
              triggers.map((trigger, index) => (
                <div
                  key={`${trigger.connectionId}-${index}`}
                  className="flex items-start gap-3 rounded-xl border border-border bg-muted/50 p-3"
                >
                  <div className="flex items-center justify-center h-6 w-6 rounded-md bg-amber-500/10 shrink-0 mt-0.5">
                    <HugeiconsIcon icon={FlashIcon} size={12} className="text-amber-500" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-foreground">{trigger.connectionName}</p>
                    <div className="flex flex-wrap gap-1 mt-1">
                      {trigger.triggerKeys.map((key) => (
                        <Badge key={key} variant="secondary" className="text-[10px] font-mono">{key}</Badge>
                      ))}
                    </div>
                    {trigger.conditions && trigger.conditions.conditions.length > 0 && (
                      <p className="text-[11px] text-muted-foreground mt-1">
                        {trigger.conditions.conditions.length} filter{trigger.conditions.conditions.length !== 1 ? "s" : ""}
                      </p>
                    )}
                  </div>
                  <button
                    type="button"
                    onClick={() => removeTrigger(index)}
                    className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-destructive/10 transition-colors shrink-0"
                  >
                    <HugeiconsIcon icon={Cancel01Icon} size={14} className="text-destructive" />
                  </button>
                </div>
              ))
            )}

            <Button
              variant="outline"
              size="sm"
              className="w-fit mt-1"
              onClick={() => setAddTriggerOpen(true)}
            >
              <HugeiconsIcon icon={FlashIcon} size={14} data-icon="inline-start" />
              Add trigger
            </Button>
          </div>

          <AddTriggerDialog
            open={addTriggerOpen}
            onOpenChange={setAddTriggerOpen}
            onAdd={addTrigger}
            connectionIds={new Set(Object.keys(integrations))}
          />
        </section>

        <Separator />

        {/* Skills */}
        <SkillsSection skillIds={skillIds} onToggle={toggleSkill} />

        <Separator />

        {/* Tool Permissions */}
        <section className="flex flex-col gap-4">
          <SectionHeader
            title="Tool permissions"
            description="Control which built-in tools this agent can use. Click a tool to cycle between allow, require approval, and deny."
          />
          <ToolPermissionsSection
            permissions={form.watch("permissions")}
            onChange={(nextPermissions) => form.setValue("permissions", nextPermissions)}
          />
        </section>

        <Separator />

        {/* Advanced */}
        <section className="flex flex-col gap-4">
          <SectionHeader title="Advanced" />

          <div className="flex items-center justify-between">
            <div className="flex flex-col gap-0.5">
              <Label className="text-sm">Shared memory</Label>
              <p className="text-xs text-muted-foreground">
                Allow this agent to retain context across runs.
              </p>
            </div>
            <Switch
              checked={sharedMemory}
              onCheckedChange={(checked) => form.setValue("sharedMemory", checked)}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="edit-team" className="text-sm">
              Team{" "}
              <span className="text-muted-foreground font-normal">(optional)</span>
            </Label>
            <Input id="edit-team" {...form.register("team")} />
          </div>
        </section>
      </FloatingPanelBody>

      <FloatingPanelFooter>
        <Button className="w-full" onClick={handleSave} loading={isSubmitting}>
          Save changes
        </Button>
      </FloatingPanelFooter>

      <ManageIntegrationsDialog
        open={integrationsOpen}
        onOpenChange={setIntegrationsOpen}
        agentIntegrations={integrations}
        onSave={setIntegrations}
      />
    </>
  )
}
