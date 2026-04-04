export type RunStatus = "running" | "waiting_approval" | "completed" | "error"

export type RunEvent = {
  id: string
  type: "system" | "user" | "agent" | "tool_call" | "approval" | "error" | "thinking"
  content?: string
  toolName?: string
  toolParams?: Record<string, string>
  toolResult?: { status: "success" | "failed" | "running"; duration: string; response?: string }
  approvalStatus?: "pending" | "approved" | "denied"
  timestamp: string
}

export type Run = {
  id: string
  identity: string
  subject: string
  status: RunStatus
  duration: string
  tokensIn: number
  tokensOut: number
  cost: number
  startedAt: string
  events: RunEvent[]
}

export const agent = {
  id: "agt_triage_01",
  name: "Issue Triage Agent",
  provider: "anthropic",
  model: "claude-sonnet-4-20250514",
  sandboxType: "Shared",
  memoryEnabled: true,
  status: "active" as const,
  integrations: ["GitHub", "Linear", "Slack"],
  stats: {
    totalRuns: 2847,
    totalRunsTrend: 14,
    activeNow: 2,
    spendThisMonth: 124.58,
    spendTrend: 8,
    tokensThisMonth: 8_420_000,
    tokensTrend: 11,
    avgCostPerRun: 0.036,
    avgCostTrend: -12,
  },
}

export const activeRuns: Run[] = [
  {
    id: "run_001",
    identity: "github_webhook",
    subject: "Issue #1024: Login fails on Safari",
    status: "running",
    duration: "32s",
    tokensIn: 840,
    tokensOut: 320,
    cost: 0.02,
    startedAt: "Just now",
    events: [
      {
        id: "e1",
        type: "system",
        content: "Triage new GitHub issue #1024 \"Login fails on Safari 17.2 after latest deploy\". Read the issue, check for related issues, assign labels and priority, then notify the team.",
        timestamp: "32s ago",
      },
      {
        id: "e2",
        type: "tool_call",
        toolName: "github.get_issue",
        toolParams: { repo: "acme/webapp", issue: "1024" },
        toolResult: { status: "success", duration: "280ms", response: '{"number":1024,"title":"Login fails on Safari 17.2 after latest deploy","state":"open","user":"jsmith","labels":[],"body":"After upgrading to v2.8.0, login no longer works on Safari 17.2. Console shows SameSite cookie error...","created_at":"2025-06-15T10:23:00Z"}' },
        timestamp: "30s ago",
      },
      {
        id: "e3",
        type: "agent",
        content: "I've read issue #1024. The reporter describes a login failure specifically on Safari 17.2 after the v2.8.0 deploy. They include a console screenshot showing a `SameSite` cookie error. Let me check for related issues.",
        timestamp: "28s ago",
      },
      {
        id: "e4",
        type: "tool_call",
        toolName: "github.search_issues",
        toolParams: { repo: "acme/webapp", query: "Safari cookie SameSite login" },
        toolResult: { status: "success", duration: "450ms", response: '{"total_count":2,"items":[{"number":987,"title":"Safari SameSite cookie fix","state":"closed"},{"number":1002,"title":"Login redirect loop on WebKit","state":"open"}]}' },
        timestamp: "25s ago",
      },
      {
        id: "e5",
        type: "agent",
        content: "Found 2 related issues: #987 (closed, fixed in v2.7.1) and #1002 (open, different symptoms). This appears to be a regression. I'll label it as `bug`, `P2`, `auth`, and `browser-compat`.",
        timestamp: "22s ago",
      },
      {
        id: "e5b",
        type: "user",
        content: "This is affecting production logins. Please escalate to P1 and make sure the auth team gets notified on Slack.",
        timestamp: "20s ago",
      },
      {
        id: "e5c",
        type: "agent",
        content: "Understood — escalating to P1. I'll label it as `bug`, `P1`, `auth`, and `browser-compat`, create an urgent Linear issue for the auth team, and notify them on Slack.",
        timestamp: "19s ago",
      },
      {
        id: "e6",
        type: "tool_call",
        toolName: "github.add_labels",
        toolParams: { repo: "acme/webapp", issue: "1024", labels: "bug, P1, auth, browser-compat" },
        toolResult: { status: "success", duration: "190ms", response: '{"labels":["bug","P1","auth","browser-compat"]}' },
        timestamp: "18s ago",
      },
      {
        id: "e7",
        type: "tool_call",
        toolName: "linear.create_issue",
        toolParams: { team: "AUTH", title: "Safari 17.2 login regression", priority: "urgent" },
        toolResult: { status: "success", duration: "340ms", response: '{"id":"AUTH-1842","url":"https://linear.app/acme/issue/AUTH-1842"}' },
        timestamp: "15s ago",
      },
      {
        id: "e8",
        type: "approval",
        toolName: "slack.post_message",
        toolParams: { channel: "#eng-auth", message: "🚨 P1 regression: Safari 17.2 login broken after v2.8.0 deploy.\n\nIssue: #1024\nSeverity: P1\nLabels: bug, auth, browser-compat\nLinear: AUTH-1842\n\nSameSite cookie regression — likely related to closed issue #987. Auth team please investigate urgently." },
        approvalStatus: "pending",
        timestamp: "12s ago",
      },
    ],
  },
  {
    id: "run_002",
    identity: "github_webhook",
    subject: "Issue #1025: Add dark mode to settings",
    status: "running",
    duration: "14s",
    tokensIn: 420,
    tokensOut: 180,
    cost: 0.01,
    startedAt: "14s ago",
    events: [
      {
        id: "e20",
        type: "system",
        content: "Triage new GitHub issue #1025 \"Add dark mode support to settings page\".",
        timestamp: "14s ago",
      },
      {
        id: "e21",
        type: "tool_call",
        toolName: "github.get_issue",
        toolParams: { repo: "acme/webapp", issue: "1025" },
        toolResult: { status: "success", duration: "240ms" },
        timestamp: "12s ago",
      },
      {
        id: "e22",
        type: "tool_call",
        toolName: "github.search_issues",
        toolParams: { repo: "acme/webapp", query: "dark mode settings" },
        toolResult: { status: "running", duration: "..." },
        timestamp: "now",
      },
    ],
  },
]

export const recentRuns: Run[] = [
  {
    id: "run_100",
    identity: "github_webhook",
    subject: "Issue #1023: Optimize dashboard queries",
    status: "completed",
    duration: "1m 12s",
    tokensIn: 1_840,
    tokensOut: 920,
    cost: 0.05,
    startedAt: "8m ago",
    events: [],
  },
  {
    id: "run_101",
    identity: "github_webhook",
    subject: "Issue #1022: TypeError in billing webhook",
    status: "completed",
    duration: "48s",
    tokensIn: 1_200,
    tokensOut: 680,
    cost: 0.03,
    startedAt: "22m ago",
    events: [],
  },
  {
    id: "run_102",
    identity: "github_webhook",
    subject: "Issue #1021: Update API rate limit docs",
    status: "completed",
    duration: "35s",
    tokensIn: 780,
    tokensOut: 440,
    cost: 0.02,
    startedAt: "1h ago",
    events: [],
  },
  {
    id: "run_103",
    identity: "cron_daily",
    subject: "Daily stale issue cleanup",
    status: "error",
    duration: "2m 5s",
    tokensIn: 3_200,
    tokensOut: 0,
    cost: 0.04,
    startedAt: "3h ago",
    events: [],
  },
  {
    id: "run_104",
    identity: "github_webhook",
    subject: "Issue #1020: Add SSO support",
    status: "completed",
    duration: "52s",
    tokensIn: 1_100,
    tokensOut: 620,
    cost: 0.03,
    startedAt: "5h ago",
    events: [],
  },
  {
    id: "run_105",
    identity: "github_webhook",
    subject: "Issue #1019: Mobile nav broken on iOS",
    status: "completed",
    duration: "1m 8s",
    tokensIn: 1_600,
    tokensOut: 780,
    cost: 0.04,
    startedAt: "6h ago",
    events: [],
  },
]
