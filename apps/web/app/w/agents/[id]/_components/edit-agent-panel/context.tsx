"use client"

import { createContext, useContext, useState, useEffect, useCallback } from "react"
import { useForm, type UseFormReturn } from "react-hook-form"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"
import type { AgentIntegrations } from "@/app/w/agents/_components/manage-integrations-dialog"
import type { TriggerConfig } from "@/app/w/agents/_components/create-agent/types"

type Agent = components["schemas"]["agentResponse"]

type PermissionLevel = "allow" | "deny" | "require_approval"

export interface EditAgentFormValues {
  name: string
  description: string
  credentialId: string
  model: string
  sandboxType: "shared" | "dedicated"
  providerPrompts: Record<string, string>
  instructions: string
  team: string
  sharedMemory: boolean
  permissions: Record<string, PermissionLevel>
}

interface EditAgentContextValue {
  form: UseFormReturn<EditAgentFormValues>
  agent: Agent | null
  integrations: AgentIntegrations
  triggers: TriggerConfig[]
  isSubmitting: boolean
  setIntegrations: (integrations: AgentIntegrations) => void
  removeIntegration: (connectionId: string) => void
  addTrigger: (trigger: TriggerConfig) => void
  removeTrigger: (index: number) => void
  handleSave: () => void
}

const EditAgentContext = createContext<EditAgentContextValue | null>(null)

export function useEditAgent() {
  const ctx = useContext(EditAgentContext)
  if (!ctx) throw new Error("useEditAgent must be used within EditAgentProvider")
  return ctx
}

interface EditAgentProviderProps {
  children: React.ReactNode
  agent: Agent | null
  open: boolean
  onClose: () => void
}

function parseAgentTriggers(agent: Agent): TriggerConfig[] {
  const rawTriggers = (agent as Record<string, unknown>).triggers
  if (!Array.isArray(rawTriggers)) return []
  return rawTriggers.map((trigger: Record<string, unknown>) => ({
    connectionId: (trigger.connection_id as string) ?? "",
    connectionName: (trigger.provider as string) ?? "",
    provider: (trigger.provider as string) ?? "",
    triggerKeys: Array.isArray(trigger.trigger_keys) ? trigger.trigger_keys : [],
    triggerDisplayNames: Array.isArray(trigger.trigger_keys) ? trigger.trigger_keys : [],
    conditions: trigger.conditions as TriggerConfig["conditions"] ?? null,
  }))
}

function parseAgentIntegrations(raw: unknown): AgentIntegrations {
  if (!raw || typeof raw !== "object") return {}
  const result: AgentIntegrations = {}
  for (const [id, config] of Object.entries(raw as Record<string, unknown>)) {
    const cfg = config as { actions?: unknown } | undefined
    result[id] = {
      actions: Array.isArray(cfg?.actions) ? cfg.actions : [],
    }
  }
  return result
}

export function EditAgentProvider({ children, agent, open, onClose }: EditAgentProviderProps) {
  const queryClient = useQueryClient()
  const updateAgent = $api.useMutation("put", "/v1/agents/{id}")

  const form = useForm<EditAgentFormValues>({
    defaultValues: {
      name: "",
      description: "",
      credentialId: "",
      model: "",
      sandboxType: "shared",
      providerPrompts: {},
      instructions: "",
      team: "",
      sharedMemory: false,
      permissions: {},
    },
  })

  const [integrations, setIntegrations] = useState<AgentIntegrations>({})
  const [triggers, setTriggers] = useState<TriggerConfig[]>([])

  // Reset form from agent data when panel opens
  useEffect(() => {
    if (!open || !agent) return

    const rawPrompts = agent.provider_prompts ?? {}
    const providerPrompts: Record<string, string> = {}
    for (const [provider, config] of Object.entries(rawPrompts)) {
      providerPrompts[provider] = config.system_prompt ?? ""
    }

    const rawPermissions = (agent.permissions ?? {}) as Record<string, string>
    const parsedPermissions: Record<string, PermissionLevel> = {}
    for (const [key, value] of Object.entries(rawPermissions)) {
      if (value === "allow" || value === "deny" || value === "require_approval") {
        parsedPermissions[key] = value
      }
    }

    form.reset({
      name: agent.name ?? "",
      description: agent.description ?? "",
      credentialId: agent.credential_id ?? "",
      model: agent.model ?? "",
      sandboxType: (agent.sandbox_type as "shared" | "dedicated") ?? "shared",
      providerPrompts,
      instructions: agent.instructions ?? "",
      team: agent.team ?? "",
      sharedMemory: agent.shared_memory ?? false,
      permissions: parsedPermissions,
    })
    setIntegrations(parseAgentIntegrations(agent.integrations))
    setTriggers(parseAgentTriggers(agent))
  }, [open, agent, form])

  const addTrigger = useCallback((trigger: TriggerConfig) => {
    setTriggers((previous) => [...previous, trigger])
  }, [])

  const removeTrigger = useCallback((index: number) => {
    setTriggers((previous) => previous.filter((_, triggerIndex) => triggerIndex !== index))
  }, [])

  const removeIntegration = useCallback((connectionId: string) => {
    setIntegrations((prev) => {
      const next = { ...prev }
      delete next[connectionId]
      return next
    })
  }, [])

  const handleSave = useCallback(() => {
    if (!agent?.id) return
    const values = form.getValues()

    const integrationsPayload: Record<string, { actions: string[] }> = {}
    for (const [id, config] of Object.entries(integrations)) {
      integrationsPayload[id] = { actions: config.actions }
    }

    const firstPrompt = Object.values(values.providerPrompts).find((prompt) => prompt.trim()) ?? ""

    const providerPromptsPayload: Record<string, { system_prompt: string }> = {}
    for (const [provider, prompt] of Object.entries(values.providerPrompts)) {
      if (prompt.trim()) {
        providerPromptsPayload[provider] = { system_prompt: prompt }
      }
    }

    const body: Record<string, unknown> = {
      name: values.name.trim(),
      description: values.description.trim() || undefined,
      credential_id: values.credentialId || undefined,
      model: values.model || undefined,
      sandbox_type: values.sandboxType,
      system_prompt: firstPrompt,
      provider_prompts: providerPromptsPayload,
      instructions: values.instructions || undefined,
      integrations: integrationsPayload,
      triggers: triggers.map((trigger) => ({
        connection_id: trigger.connectionId,
        trigger_keys: trigger.triggerKeys,
        conditions: trigger.conditions,
      })),
      shared_memory: values.sharedMemory,
      team: values.team.trim() || undefined,
      permissions: values.permissions,
    }

    updateAgent.mutate(
      {
        params: { path: { id: agent.id } },
        body: body as never,
      },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents"] })
          toast.success(`Agent "${values.name}" updated`)
          onClose()
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to update agent"))
        },
      },
    )
  }, [agent, form, integrations, triggers, updateAgent, queryClient, onClose])

  return (
    <EditAgentContext.Provider
      value={{
        form,
        agent,
        integrations,
        triggers,
        isSubmitting: updateAgent.isPending,
        setIntegrations,
        removeIntegration,
        addTrigger,
        removeTrigger,
        handleSave,
      }}
    >
      {children}
    </EditAgentContext.Provider>
  )
}
