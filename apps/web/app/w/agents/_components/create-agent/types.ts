import type { components } from "@/lib/api/schema"

export type CreationMode = "scratch" | "marketplace"

export type Step =
  | "mode"
  | "sandbox"
  | "integrations"
  | "trigger"
  | "llm-key"
  | "basics"
  | "system-prompt"
  | "instructions"
  | "skills"
  | "subagents"
  | "summary"
  | "marketplace-browse"
  | "marketplace-detail"

export const scratchSteps: Step[] = [
  "mode",
  "sandbox",
  "integrations",
  "trigger",
  "llm-key",
  "basics",
  "system-prompt",
  "instructions",
  "skills",
  "subagents",
  "summary",
]

export const marketplaceSteps: Step[] = [
  "mode",
  "marketplace-browse",
  "marketplace-detail",
]

/**
 * UI-only mock shape used by `./data.ts` to render the marketplace browse /
 * detail preview before the backend marketplace endpoints return real data.
 * Once wired up, prefer `components["schemas"]["marketplaceAgentResponse"]`
 * directly and delete this alongside `./data.ts`.
 */
export interface MarketplaceAgentPreview {
  slug: string
  name: string
  description: string
  publisher: { name: string; avatar: string }
  installs: number
  integrations: string[]
  verified: boolean
}

/**
 * UI-only mock shape used by `./data.ts` for the integration / action picker.
 * Real integrations come from `components["schemas"]["integrationSummary"]`
 * and `components["schemas"]["actionSummary"]` via `/v1/catalog/integrations`.
 */
export interface Integration {
  id: string
  name: string
  logo: string
  description: string
  actions: IntegrationAction[]
}

export interface IntegrationAction {
  id: string
  name: string
  description: string
  type: "read" | "write" | "delete"
}

/**
 * UI-only mock shape for the LLM key picker seed data in `./data.ts`.
 * Real keys come from `components["schemas"]["credentialResponse"]`.
 */
export interface LlmKey {
  id: string
  name: string
  provider: string
  logo: string
  models: string[]
}

/**
 * UI projection of `components["schemas"]["skillResponse"]` with camel-cased
 * field names and a narrowed `scope` derived from `org_id`. Built via
 * `toSkillPreview` in `step-skills.tsx`.
 */
export type SkillPreview = {
  id: NonNullable<components["schemas"]["skillResponse"]["id"]>
  slug: NonNullable<components["schemas"]["skillResponse"]["slug"]>
  name: NonNullable<components["schemas"]["skillResponse"]["name"]>
  description: string
  sourceType: "inline" | "git"
  scope: "public" | "org"
  tags: string[]
  installCount: number
  featured: boolean
}

/**
 * UI projection of `components["schemas"]["subagentResponse"]` with a narrowed
 * `scope` derived from `org_id`. Built via `toSubagentPreview` in
 * `step-subagents.tsx`.
 */
export type SubagentPreview = {
  id: NonNullable<components["schemas"]["subagentResponse"]["id"]>
  name: NonNullable<components["schemas"]["subagentResponse"]["name"]>
  description: string
  model: string
  scope: "public" | "org"
}

/**
 * UI state for the trigger picker. Collapses the backend
 * `components["schemas"]["agentTriggerResponse"]` (snake_case, opaque
 * `conditions`) into a shape that's ergonomic for dialog state: we keep the
 * resolved display name alongside the raw key, and represent conditions as a
 * typed mode + list instead of an opaque `unknown`.
 */
export interface TriggerConditionConfig {
  path: string
  operator: string
  value: unknown
}

export interface TriggerConditionsConfig {
  mode: "all" | "any"
  conditions: TriggerConditionConfig[]
}

export interface TriggerConfig {
  connectionId: string
  connectionName: string
  provider: string
  triggerKeys: string[]
  triggerDisplayNames: string[]
  conditions: TriggerConditionsConfig | null
}
