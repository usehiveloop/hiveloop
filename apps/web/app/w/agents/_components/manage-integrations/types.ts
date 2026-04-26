/** The shape stored in agent.integrations JSON: connectionId → { actions: string[] } */
export type AgentIntegrations = Record<string, { actions: string[] }>

export const innerVariants = {
  enter: (direction: number) => ({ x: direction > 0 ? 60 : -60, opacity: 0 }),
  center: { x: 0, opacity: 1 },
  exit: (direction: number) => ({ x: direction > 0 ? -60 : 60, opacity: 0 }),
}
