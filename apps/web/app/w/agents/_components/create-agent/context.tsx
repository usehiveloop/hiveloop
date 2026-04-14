"use client"

import { createContext, useContext, useState, useRef, useCallback } from "react"
import { useRouter } from "next/navigation"
import { useForm, type UseFormReturn } from "react-hook-form"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { scratchSteps, forgeSteps, marketplaceSteps } from "./types"
import type { CreationMode, Step, SkillPreview, SubagentPreview, TriggerConfig } from "./types"

export interface CreateAgentFormValues {
  name: string
  description: string
  model: string
  credentialId: string
  sandboxType: "shared" | "dedicated"
  systemPrompt: string
  instructions: string
  judgeKeyId: string
  judgeModel: string
  selectedMarketplaceAgent: string
}

interface CreateAgentContextValue {
  form: UseFormReturn<CreateAgentFormValues>
  step: Step
  mode: CreationMode | null
  direction: React.MutableRefObject<1 | -1>
  selectedIntegrations: Set<string>
  selectedActions: Record<string, Set<string>>
  selectedSkills: Map<string, SkillPreview>
  selectedSubagents: Map<string, SubagentPreview>
  triggers: TriggerConfig[]
  isSubmitting: boolean
  setMode: (mode: CreationMode) => void
  goTo: (step: Step) => void
  toggleIntegration: (connectionId: string) => void
  toggleAction: (connectionId: string, actionKey: string) => void
  toggleSkill: (skill: SkillPreview) => void
  clearSkills: () => void
  toggleSubagent: (subagent: SubagentPreview) => void
  clearSubagents: () => void
  addTrigger: (trigger: TriggerConfig) => void
  removeTrigger: (index: number) => void
  handleCreate: () => void
  reset: () => void
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
  initialMode?: CreationMode
}

export function CreateAgentProvider({ children, onClose, initialMode }: CreateAgentProviderProps) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const createAgent = $api.useMutation("post", "/v1/agents")

  const form = useForm<CreateAgentFormValues>({
    defaultValues: {
      name: "",
      description: "",
      model: "",
      credentialId: "",
      sandboxType: "shared",
      systemPrompt: "",
      instructions: "",
      judgeKeyId: "",
      judgeModel: "",
      selectedMarketplaceAgent: "",
    },
  })

  const initialStep: Step = initialMode === "marketplace" ? "marketplace-browse" : initialMode ? "sandbox" : "mode"
  const [step, setStep] = useState<Step>(initialStep)
  const [mode, setModeState] = useState<CreationMode | null>(initialMode ?? null)
  const [selectedIntegrations, setSelectedIntegrations] = useState<Set<string>>(new Set())
  const [selectedActions, setSelectedActions] = useState<Record<string, Set<string>>>({})
  const [selectedSkills, setSelectedSkills] = useState<Map<string, SkillPreview>>(new Map())
  const [selectedSubagents, setSelectedSubagents] = useState<Map<string, SubagentPreview>>(new Map())
  const [triggers, setTriggers] = useState<TriggerConfig[]>([])
  const direction = useRef<1 | -1>(1)

  const currentSteps = mode === "marketplace" ? marketplaceSteps : mode === "forge" ? forgeSteps : scratchSteps

  const goTo = useCallback((next: Step) => {
    direction.current = currentSteps.indexOf(next) > currentSteps.indexOf(step) ? 1 : -1
    setStep(next)
  }, [currentSteps, step])

  const setMode = useCallback((newMode: CreationMode) => {
    setModeState(newMode)
    direction.current = 1
    if (newMode === "marketplace") {
      setStep("marketplace-browse")
    } else {
      setStep("sandbox")
    }
  }, [])

  const toggleIntegration = useCallback((connectionId: string) => {
    setSelectedIntegrations((prev) => {
      const next = new Set(prev)
      if (next.has(connectionId)) {
        next.delete(connectionId)
        setSelectedActions((prevActions) => {
          const nextActions = { ...prevActions }
          delete nextActions[connectionId]
          return nextActions
        })
      } else {
        next.add(connectionId)
      }
      return next
    })
  }, [])

  const toggleAction = useCallback((connectionId: string, actionKey: string) => {
    setSelectedActions((prev) => {
      const current = new Set(prev[connectionId] ?? [])
      if (current.has(actionKey)) {
        current.delete(actionKey)
      } else {
        current.add(actionKey)
      }
      return { ...prev, [connectionId]: current }
    })
  }, [])

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

  const clearSkills = useCallback(() => {
    setSelectedSkills(new Map())
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

  const clearSubagents = useCallback(() => {
    setSelectedSubagents(new Map())
  }, [])

  const addTrigger = useCallback((trigger: TriggerConfig) => {
    setTriggers((previous) => [...previous, trigger])
  }, [])

  const removeTrigger = useCallback((index: number) => {
    setTriggers((previous) => previous.filter((_, triggerIndex) => triggerIndex !== index))
  }, [])

  const reset = useCallback(() => {
    setStep("mode")
    setModeState(null)
    setSelectedIntegrations(new Set())
    setSelectedActions({})
    setSelectedSkills(new Map())
    setSelectedSubagents(new Map())
    setTriggers([])
    form.reset()
  }, [form])

  const handleCreate = useCallback(() => {
    const values = form.getValues()
    if (!values.credentialId || !values.model || !values.sandboxType) return

    const integrationsPayload: Record<string, { actions: string[] }> = {}
    for (const connectionId of selectedIntegrations) {
      const actions = selectedActions[connectionId]
      integrationsPayload[connectionId] = {
        actions: actions ? Array.from(actions) : [],
      }
    }

    const body: Record<string, unknown> = {
      name: values.name.trim(),
      description: values.description.trim() || undefined,
      credential_id: values.credentialId,
      model: values.model,
      sandbox_type: values.sandboxType,
      system_prompt: values.systemPrompt || "",
      instructions: values.instructions || undefined,
      integrations: integrationsPayload,
    }

    if (selectedSkills.size > 0) {
      body.skill_ids = Array.from(selectedSkills.keys())
    }

    if (selectedSubagents.size > 0) {
      body.subagent_ids = Array.from(selectedSubagents.keys())
    }

    if (triggers.length > 0) {
      body.triggers = triggers.map((trigger) => ({
        connection_id: trigger.connectionId,
        trigger_keys: trigger.triggerKeys,
        conditions: trigger.conditions,
      }))
    }

    if (mode === "forge" && values.judgeKeyId && values.judgeModel) {
      body.forge = {
        judge_credential_id: values.judgeKeyId,
        judge_model: values.judgeModel,
      }
    }

    createAgent.mutate(
      { body: body as never },
      {
        onSuccess: (data) => {
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents"] })
          const agentData = data as { id?: string; forge_run_id?: string }
          if (mode === "forge" && agentData.id) {
            toast.success(`Agent "${values.name}" created — starting forge`)
            onClose()
            router.push(`/w/agents/${agentData.id}/forge`)
          } else {
            toast.success(`Agent "${values.name}" created`)
            onClose()
          }
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to create agent"))
        },
      },
    )
  }, [form, mode, selectedIntegrations, selectedActions, selectedSkills, selectedSubagents, triggers, createAgent, queryClient, onClose, router])

  return (
    <CreateAgentContext.Provider
      value={{
        form,
        step,
        mode,
        direction,
        selectedIntegrations,
        selectedActions,
        selectedSkills,
        selectedSubagents,
        triggers,
        isSubmitting: createAgent.isPending,
        setMode,
        goTo,
        toggleIntegration,
        toggleAction,
        toggleSkill,
        clearSkills,
        toggleSubagent,
        clearSubagents,
        addTrigger,
        removeTrigger,
        handleCreate,
        reset,
      }}
    >
      {children}
    </CreateAgentContext.Provider>
  )
}
