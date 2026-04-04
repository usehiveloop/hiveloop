export type AgentStatus = "active" | "forging" | "draft" | "archived"

export type Agent = {
  id: string
  name: string
  integrations: string[]
  hasMemory: boolean
  status: AgentStatus
  totalRuns: number
  totalSpend: number
  totalTokens: number
}

export const agents: Agent[] = [
  {
    id: "1",
    name: "PR Review Agent",
    integrations: ["GitHub", "Slack", "Linear"],
    hasMemory: true,
    status: "active",
    totalRuns: 1243,
    totalSpend: 48.72,
    totalTokens: 2_840_000,
  },
  {
    id: "2",
    name: "Customer Support Agent",
    integrations: ["Intercom", "Notion", "Slack"],
    hasMemory: true,
    status: "active",
    totalRuns: 892,
    totalSpend: 32.15,
    totalTokens: 1_920_000,
  },
  {
    id: "3",
    name: "Incident Responder",
    integrations: ["Slack", "Linear", "GitHub"],
    hasMemory: false,
    status: "forging",
    totalRuns: 0,
    totalSpend: 12.40,
    totalTokens: 580_000,
  },
  {
    id: "4",
    name: "Daily Standup Bot",
    integrations: ["Slack", "Linear"],
    hasMemory: false,
    status: "active",
    totalRuns: 467,
    totalSpend: 8.90,
    totalTokens: 640_000,
  },
  {
    id: "5",
    name: "Onboarding Agent",
    integrations: ["Notion", "GitHub", "Slack", "Google"],
    hasMemory: true,
    status: "draft",
    totalRuns: 0,
    totalSpend: 0,
    totalTokens: 0,
  },
  {
    id: "6",
    name: "Release Manager",
    integrations: ["GitHub", "Slack", "Vercel"],
    hasMemory: false,
    status: "archived",
    totalRuns: 312,
    totalSpend: 14.55,
    totalTokens: 890_000,
  },
  {
    id: "7",
    name: "Security Scanner",
    integrations: ["GitHub", "Linear"],
    hasMemory: false,
    status: "active",
    totalRuns: 156,
    totalSpend: 6.20,
    totalTokens: 420_000,
  },
]

export const statusConfig: Record<AgentStatus, { label: string; color: string }> = {
  active: { label: "Active", color: "bg-green-500" },
  forging: { label: "Forging", color: "bg-primary" },
  draft: { label: "Draft", color: "bg-muted-foreground/50" },
  archived: { label: "Archived", color: "bg-destructive" },
}
