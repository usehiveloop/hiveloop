export type TriggerView = "choice" | "connections" | "triggers" | "context"

export interface ContextActionConfig {
  as: string
  action: string
  actionDisplayName: string
  ref?: string
  params?: Record<string, string>
  optional?: boolean
}

export interface TriggerConditionConfig {
  path: string
  operator: string
  value: any
}

export interface TriggerConditionsConfig {
  match: "all" | "any"
  rules: TriggerConditionConfig[]
}

export interface TriggerSelection {
  connectionId: string
  connectionName: string
  provider: string
  triggerKeys: string[]
  triggerDisplayNames: string[]
  refs: Record<string, string>
  contextActions: ContextActionConfig[]
  conditions?: TriggerConditionsConfig
  prompt: string
}
