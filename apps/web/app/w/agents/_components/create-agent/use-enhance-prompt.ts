"use client"

import { useCallback } from "react"
import { toast } from "sonner"
import {
  useSystemTaskStream,
  isSystemTaskError,
} from "@/lib/api/use-system-task-stream"
import type { CreateAgentFormValues } from "./context"
import type { SkillPreview, SubagentPreview, TriggerConfig } from "./types"

interface BuildArgsInput {
  values: CreateAgentFormValues
  selectedIntegrations: Set<string>
  selectedActions: Record<string, Set<string>>
  selectedSkills: Map<string, SkillPreview>
  selectedSubagents: Map<string, SubagentPreview>
  triggers: TriggerConfig[]
}

// Builds the args object the prompt_writer endpoint accepts. Mirrors the
// shape `handleSubmit` builds for POST /v1/agents — minus the create-only
// fields the resolver doesn't read (credential_id, model, system_prompt,
// description, avatar_url, shared_memory).
function buildEnhanceArgs(input: BuildArgsInput): Record<string, unknown> {
  const { values, selectedIntegrations, selectedActions, selectedSkills, selectedSubagents, triggers } = input

  const integrations: Record<string, { actions: string[] }> = {}
  for (const connectionId of selectedIntegrations) {
    const actions = selectedActions[connectionId]
    integrations[connectionId] = { actions: actions ? Array.from(actions) : [] }
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
    }
    return base
  })

  return {
    name: values.name.trim(),
    category: values.category.trim() || undefined,
    instructions: values.instructions || undefined,
    skill_ids: Array.from(selectedSkills.keys()),
    subagent_ids: Array.from(selectedSubagents.keys()),
    integrations,
    triggers: triggersPayload,
    permissions: values.permissions,
  }
}

export function useEnhancePrompt() {
  const { run, isStreaming, output } = useSystemTaskStream("prompt_writer")

  const enhance = useCallback(
    async (params: BuildArgsInput) => {
      if (!params.values.name.trim()) {
        toast.error("Give the agent a name first")
        return
      }
      if (!params.values.instructions?.trim()) {
        toast.error("Write a few lines of instructions before enhancing")
        return
      }

      try {
        const result = await run(buildEnhanceArgs(params))
        console.log("[enhance-prompt] accumulated prompt:\n", result.text)
        console.log("[enhance-prompt] usage:", result.usage)
      } catch (err) {
        const message = isSystemTaskError(err)
          ? err.error
          : err instanceof Error
            ? err.message
            : "Failed to enhance prompt"
        toast.error(message)
      }
    },
    [run],
  )

  return { enhance, isEnhancing: isStreaming, output }
}
