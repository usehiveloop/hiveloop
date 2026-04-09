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
