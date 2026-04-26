import type { components } from "@/lib/api/schema"

export type Agent = components["schemas"]["agentResponse"]
export type InConnection = components["schemas"]["inConnectionResponse"]

export interface ResourceItem {
  id: string
  name: string
}

export type AgentResources = Record<string, Record<string, ResourceItem[]>>

export interface ConfigurableResource {
  key: string
  display_name: string
  description: string
}

export function parseAgentResources(raw: unknown): AgentResources {
  if (!raw || typeof raw !== "object") return {}
  const result: AgentResources = {}
  for (const [connId, resourceTypes] of Object.entries(raw as Record<string, unknown>)) {
    if (!resourceTypes || typeof resourceTypes !== "object") continue
    const parsed: Record<string, ResourceItem[]> = {}
    for (const [resourceKey, items] of Object.entries(resourceTypes as Record<string, unknown>)) {
      if (!Array.isArray(items)) continue
      parsed[resourceKey] = items.filter(
        (item): item is ResourceItem =>
          typeof item === "object" && item !== null && "id" in item && "name" in item,
      )
    }
    result[connId] = parsed
  }
  return result
}

export function getConfigurableResources(connection: InConnection): ConfigurableResource[] {
  const raw = (connection as Record<string, unknown>).configurable_resources
  if (!Array.isArray(raw)) return []
  return raw as ConfigurableResource[]
}

export const slideVariants = {
  enter: (direction: number) => ({ x: direction > 0 ? 60 : -60, opacity: 0 }),
  center: { x: 0, opacity: 1 },
  exit: (direction: number) => ({ x: direction > 0 ? -60 : 60, opacity: 0 }),
}
