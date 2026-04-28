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

export interface SubagentPreview {
  id: string
  name: string
  description: string
  model: string
  scope: "public" | "org"
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

export type TriggerType = "webhook" | "http" | "cron"

export interface TriggerConfig {
  triggerType: TriggerType
  connectionId: string
  connectionName: string
  provider: string
  triggerKeys: string[]
  triggerDisplayNames: string[]
  conditions: TriggerConditionsConfig | null
  cronSchedule?: string
  instructions?: string
  secretKey?: string
}
