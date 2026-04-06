"use client"

import { createContext, useContext, useState, useEffect, useCallback } from "react"
import { useForm, type UseFormReturn } from "react-hook-form"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"
import type { AgentIntegrations } from "@/app/w/agents/_components/manage-integrations-dialog"

type Agent = components["schemas"]["agentResponse"]

export interface EditAgentFormValues {
  name: string
  description: string
  credentialId: string
  model: string
  sandboxType: "shared" | "dedicated"
  systemPrompt: string
  instructions: string
  team: string
  sharedMemory: boolean
}

interface EditAgentContextValue {
  form: UseFormReturn<EditAgentFormValues>
  agent: Agent | null
  integrations: AgentIntegrations
  isSubmitting: boolean
  setIntegrations: (integrations: AgentIntegrations) => void
  removeIntegration: (connectionId: string) => void
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
      systemPrompt: "",
      instructions: "",
      team: "",
      sharedMemory: false,
    },
  })

  const [integrations, setIntegrations] = useState<AgentIntegrations>({})

  // Reset form from agent data when panel opens
  useEffect(() => {
    if (!open || !agent) return
    form.reset({
      name: agent.name ?? "",
      description: agent.description ?? "",
      credentialId: agent.credential_id ?? "",
      model: agent.model ?? "",
      sandboxType: (agent.sandbox_type as "shared" | "dedicated") ?? "shared",
      systemPrompt: agent.system_prompt ?? "",
      instructions: agent.instructions ?? "",
      team: agent.team ?? "",
      sharedMemory: agent.shared_memory ?? false,
    })
    setIntegrations(parseAgentIntegrations(agent.integrations))
  }, [open, agent, form])

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

    updateAgent.mutate(
      {
        params: { path: { id: agent.id } },
        body: {
          name: values.name.trim(),
          description: values.description.trim() || undefined,
          credential_id: values.credentialId || undefined,
          model: values.model || undefined,
          sandbox_type: values.sandboxType,
          system_prompt: values.systemPrompt,
          instructions: values.instructions || undefined,
          integrations: integrationsPayload,
          shared_memory: values.sharedMemory,
          team: values.team.trim() || undefined,
        },
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
  }, [agent, form, integrations, updateAgent, queryClient, onClose])

  return (
    <EditAgentContext.Provider
      value={{
        form,
        agent,
        integrations,
        isSubmitting: updateAgent.isPending,
        setIntegrations,
        removeIntegration,
        handleSave,
      }}
    >
      {children}
    </EditAgentContext.Provider>
  )
}
