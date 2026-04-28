"use client"

import { createContext, useContext, useState, useCallback } from "react"
import { useForm, type UseFormReturn } from "react-hook-form"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"
import type { SkillPreview, SubagentPreview, TriggerConfig } from "./types"

type Agent = components["schemas"]["agentResponse"]

type PermissionLevel = "allow" | "deny" | "require_approval"

export interface CreateAgentFormValues {
  name: string
  description: string
  model: string
  credentialId: string
  sandboxType: "shared" | "dedicated"
  systemPrompt: string
  instructions: string
  permissions: Record<string, PermissionLevel>
  sharedMemory: boolean
  category: string
  avatarUrl: string
}

type Mode = "create" | "edit"

interface CreateAgentContextValue {
  mode: Mode
  agentId: string | null
  form: UseFormReturn<CreateAgentFormValues>
  selectedIntegrations: Set<string>
  selectedActions: Record<string, Set<string>>
  selectedSkills: Map<string, SkillPreview>
  selectedSubagents: Map<string, SubagentPreview>
  triggers: TriggerConfig[]
  isSubmitting: boolean
  setSelectedIntegrations: React.Dispatch<React.SetStateAction<Set<string>>>
  setSelectedActions: React.Dispatch<React.SetStateAction<Record<string, Set<string>>>>
  toggleSkill: (skill: SkillPreview) => void
  toggleSubagent: (subagent: SubagentPreview) => void
  addTrigger: (trigger: TriggerConfig) => void
  removeTrigger: (index: number) => void
  updateTrigger: (index: number, newTriggers: TriggerConfig[]) => void
  handleSubmit: () => void
}

const CreateAgentContext = createContext<CreateAgentContextValue | null>(null)

export function useCreateAgent() {
  const ctx = useContext(CreateAgentContext)
  if (!ctx) throw new Error("useCreateAgent must be used within CreateAgentProvider")
  return ctx
}

interface CreateAgentProviderProps {
  children: React.ReactNode
  onClose: () => void
  agent?: Agent | null
}

function deriveFormValues(agent: Agent | null | undefined): CreateAgentFormValues {
  if (!agent) {
    return {
      name: "",
      description: "",
      model: "",
      credentialId: "",
      sandboxType: "shared",
      systemPrompt: "",
      instructions: "",
      permissions: {},
      sharedMemory: false,
      category: "",
      avatarUrl: "",
    }
  }
  const rawPermissions = (agent.permissions ?? {}) as Record<string, string>
  const permissions: Record<string, PermissionLevel> = {}
  for (const [key, value] of Object.entries(rawPermissions)) {
    if (value === "allow" || value === "deny" || value === "require_approval") {
      permissions[key] = value
    }
  }
  return {
    name: agent.name ?? "",
    description: agent.description ?? "",
    model: agent.model ?? "",
    credentialId: agent.credential_id ?? "",
    sandboxType: "shared",
    systemPrompt: agent.system_prompt ?? "",
    instructions: agent.instructions ?? "",
    permissions,
    sharedMemory: agent.shared_memory ?? false,
    category: agent.category ?? "",
    avatarUrl: agent.avatar_url ?? "",
  }
}

function deriveIntegrations(agent: Agent | null | undefined): {
  ids: Set<string>
  actions: Record<string, Set<string>>
} {
  const ids = new Set<string>()
  const actions: Record<string, Set<string>> = {}
  const raw = (agent?.integrations ?? {}) as Record<string, unknown>
  for (const [connectionId, value] of Object.entries(raw)) {
    ids.add(connectionId)
    const cfg = value as { actions?: unknown } | undefined
    actions[connectionId] = new Set(
      Array.isArray(cfg?.actions) ? (cfg.actions as string[]) : [],
    )
  }
  return { ids, actions }
}

function deriveTriggers(agent: Agent | null | undefined): TriggerConfig[] {
  const list = agent?.triggers ?? []
  return list.map((trigger) => ({
    triggerType: ((trigger.trigger_type as TriggerConfig["triggerType"]) || "webhook"),
    connectionId: trigger.connection_id ?? "",
    connectionName: trigger.provider ?? "",
    provider: trigger.provider ?? "",
    triggerKeys: trigger.trigger_keys ?? [],
    triggerDisplayNames: trigger.trigger_keys ?? [],
    conditions: (trigger.conditions as TriggerConfig["conditions"]) ?? null,
    cronSchedule: trigger.cron_schedule || undefined,
    instructions: trigger.instructions || undefined,
  }))
}

function deriveSkills(agent: Agent | null | undefined): Map<string, SkillPreview> {
  const out = new Map<string, SkillPreview>()
  for (const skill of agent?.attached_skills ?? []) {
    if (!skill.id) continue
    out.set(skill.id, {
      id: skill.id,
      slug: "",
      name: skill.name ?? "",
      description: skill.description ?? "",
      sourceType: skill.source_type === "git" ? "git" : "inline",
      scope: "org",
      tags: [],
      installCount: 0,
      featured: false,
    })
  }
  return out
}

function deriveSubagents(agent: Agent | null | undefined): Map<string, SubagentPreview> {
  const out = new Map<string, SubagentPreview>()
  for (const sub of agent?.subagents ?? []) {
    if (!sub.id) continue
    out.set(sub.id, {
      id: sub.id,
      name: sub.name ?? "",
      description: sub.description ?? "",
      model: sub.model ?? "",
      scope: "org",
    })
  }
  return out
}

export function CreateAgentProvider({ children, onClose, agent }: CreateAgentProviderProps) {
  const queryClient = useQueryClient()
  const createAgent = $api.useMutation("post", "/v1/agents")
  const updateAgent = $api.useMutation("put", "/v1/agents/{id}")

  const mode: Mode = agent?.id ? "edit" : "create"
  const agentId = agent?.id ?? null

  const form = useForm<CreateAgentFormValues>({
    defaultValues: deriveFormValues(agent),
  })

  const initialIntegrations = deriveIntegrations(agent)
  const [selectedIntegrations, setSelectedIntegrations] = useState<Set<string>>(initialIntegrations.ids)
  const [selectedActions, setSelectedActions] = useState<Record<string, Set<string>>>(initialIntegrations.actions)
  const [selectedSkills, setSelectedSkills] = useState<Map<string, SkillPreview>>(() => deriveSkills(agent))
  const [selectedSubagents, setSelectedSubagents] = useState<Map<string, SubagentPreview>>(() => deriveSubagents(agent))
  const [triggers, setTriggers] = useState<TriggerConfig[]>(() => deriveTriggers(agent))

  const toggleSkill = useCallback((skill: SkillPreview) => {
    setSelectedSkills((prev) => {
      const next = new Map(prev)
      if (next.has(skill.id)) {
        next.delete(skill.id)
      } else {
        next.set(skill.id, skill)
      }
      return next
    })
  }, [])

  const toggleSubagent = useCallback((subagent: SubagentPreview) => {
    setSelectedSubagents((prev) => {
      const next = new Map(prev)
      if (next.has(subagent.id)) {
        next.delete(subagent.id)
      } else {
        next.set(subagent.id, subagent)
      }
      return next
    })
  }, [])

  const addTrigger = useCallback((trigger: TriggerConfig) => {
    setTriggers((previous) => [...previous, trigger])
  }, [])

  const removeTrigger = useCallback((index: number) => {
    setTriggers((previous) => previous.filter((_, triggerIndex) => triggerIndex !== index))
  }, [])

  const updateTrigger = useCallback((index: number, newTriggers: TriggerConfig[]) => {
    setTriggers((previous) => {
      const next = [...previous]
      next.splice(index, 1, ...newTriggers)
      return next
    })
  }, [])

  const handleSubmit = useCallback(() => {
    const values = form.getValues()
    if (!values.name.trim()) return

    const integrationsPayload: Record<string, { actions: string[] }> = {}
    for (const connectionId of selectedIntegrations) {
      const actions = selectedActions[connectionId]
      integrationsPayload[connectionId] = {
        actions: actions ? Array.from(actions) : [],
      }
    }

    const triggersPayload = triggers.map((trigger) => {
      const base: Record<string, unknown> = { trigger_type: trigger.triggerType }
      if (trigger.instructions) base.instructions = trigger.instructions
      if (trigger.triggerType === "webhook") {
        base.connection_id = trigger.connectionId
        base.trigger_keys = trigger.triggerKeys
        base.conditions = trigger.conditions
      } else if (trigger.triggerType === "cron") {
        base.cron_schedule = trigger.cronSchedule
      } else if (trigger.triggerType === "http") {
        if (trigger.secretKey) base.secret_key = trigger.secretKey
      }
      return base
    })

    const sharedBody: Record<string, unknown> = {
      name: values.name.trim(),
      description: values.description.trim() || undefined,
      credential_id: values.credentialId || undefined,
      model: values.model || undefined,
      system_prompt: values.systemPrompt.trim(),
      instructions: values.instructions || undefined,
      integrations: integrationsPayload,
      triggers: triggersPayload,
      skill_ids: Array.from(selectedSkills.keys()),
      subagent_ids: Array.from(selectedSubagents.keys()),
      permissions: values.permissions,
      shared_memory: values.sharedMemory,
      category: values.category.trim() || undefined,
      avatar_url: values.avatarUrl || undefined,
    }

    if (mode === "edit" && agentId) {
      updateAgent.mutate(
        { params: { path: { id: agentId } }, body: sharedBody as never },
        {
          onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents"] })
            queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents/{id}", { params: { path: { id: agentId } } }] })
            toast.success(`Agent "${values.name}" updated`)
            onClose()
          },
          onError: (error) => {
            toast.error(extractErrorMessage(error, "Failed to update agent"))
          },
        },
      )
      return
    }

    createAgent.mutate(
      { body: sharedBody as never },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents"] })
          toast.success(`Agent "${values.name}" created`)
          onClose()
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to create agent"))
        },
      },
    )
  }, [
    mode,
    agentId,
    form,
    selectedIntegrations,
    selectedActions,
    selectedSkills,
    selectedSubagents,
    triggers,
    createAgent,
    updateAgent,
    queryClient,
    onClose,
  ])

  return (
    <CreateAgentContext.Provider
      value={{
        mode,
        agentId,
        form,
        selectedIntegrations,
        selectedActions,
        selectedSkills,
        selectedSubagents,
        triggers,
        isSubmitting: mode === "edit" ? updateAgent.isPending : createAgent.isPending,
        setSelectedIntegrations,
        setSelectedActions,
        toggleSkill,
        toggleSubagent,
        addTrigger,
        removeTrigger,
        updateTrigger,
        handleSubmit,
      }}
    >
      {children}
    </CreateAgentContext.Provider>
  )
}
