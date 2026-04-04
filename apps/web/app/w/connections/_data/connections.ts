export type Connection = {
  id: string
  provider: string
  displayName: string
  logo: string
  status: "active" | "error" | "expired"
  connectedAt: string
  agentsUsing: number
}

export const connections: Connection[] = [
  {
    id: "conn-1",
    provider: "github",
    displayName: "GitHub",
    logo: "https://cdn.simpleicons.org/github/white",
    status: "active",
    connectedAt: "2026-03-18",
    agentsUsing: 3,
  },
  {
    id: "conn-2",
    provider: "slack",
    displayName: "Slack",
    logo: "https://cdn.simpleicons.org/slack",
    status: "active",
    connectedAt: "2026-03-20",
    agentsUsing: 2,
  },
  {
    id: "conn-3",
    provider: "linear",
    displayName: "Linear",
    logo: "https://cdn.simpleicons.org/linear",
    status: "active",
    connectedAt: "2026-03-22",
    agentsUsing: 2,
  },
  {
    id: "conn-4",
    provider: "notion",
    displayName: "Notion",
    logo: "https://cdn.simpleicons.org/notion",
    status: "error",
    connectedAt: "2026-02-10",
    agentsUsing: 0,
  },
  {
    id: "conn-5",
    provider: "intercom",
    displayName: "Intercom",
    logo: "https://cdn.simpleicons.org/intercom",
    status: "active",
    connectedAt: "2026-03-28",
    agentsUsing: 1,
  },
]
