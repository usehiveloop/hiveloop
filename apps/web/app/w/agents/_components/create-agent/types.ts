export type CreationMode = "scratch" | "forge" | "marketplace"

export type Step =
  | "mode"
  | "sandbox"
  | "integrations"
  | "trigger"
  | "llm-key"
  | "basics"
  | "system-prompt"
  | "instructions"
  | "forge-judge"
  | "skills"
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
  "summary",
]

export const forgeSteps: Step[] = [
  "mode",
  "sandbox",
  "integrations",
  "trigger",
  "llm-key",
  "basics",
  "forge-judge",
  "skills",
  "summary",
]


export const marketplaceSteps: Step[] = [
  "mode",
  "marketplace-browse",
  "marketplace-detail",
]

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

export interface MarketplaceAgentPreview {
  slug: string
  name: string
  description: string
  publisher: { name: string; avatar: string }
  installs: number
  integrations: string[]
  verified: boolean
}

export interface LlmKey {
  id: string
  name: string
  provider: string
  logo: string
  models: string[]
}

export interface SkillPreview {
  id: string
  slug: string
  name: string
  description: string
  sourceType: "inline" | "git"
  scope: "public" | "org"
  tags: string[]
  installCount: number
  featured: boolean
}

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
