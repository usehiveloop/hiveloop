// Static mock data for the router configuration UI.
// Replace with $api hooks when the OpenAPI spec is updated.

export interface MockAgent {
  id: string
  name: string
  description: string
}

export interface MockConnection {
  id: string
  provider: string
  displayName: string
}

export interface MockCondition {
  path: string
  operator: string
  value?: string | string[]
}

export interface MockRule {
  id: string
  agentId: string
  agentName: string
  priority: number
  conditions: {
    mode: "all" | "any"
    conditions: MockCondition[]
  } | null
}

export interface MockTrigger {
  id: string
  connectionId: string
  provider: string
  connectionName: string
  triggerKeys: string[]
  routingMode: "rule" | "triage"
  enabled: boolean
  enrichCrossReferences: boolean
  rules: MockRule[]
}

export interface MockDecision {
  id: string
  eventType: string
  routingMode: "rule" | "triage"
  resourceKey: string
  intentSummary: string
  selectedAgents: string[]
  enrichmentSteps: number
  turnCount: number
  latencyMs: number
  createdAt: string
}

export interface MockRouterSettings {
  persona: string
  defaultAgentId: string
  memoryTeam: string
}

export const MOCK_AGENTS: MockAgent[] = [
  { id: "agent-1", name: "CodeReviewer", description: "Reviews PRs for quality, security, and style" },
  { id: "agent-2", name: "TriageBot", description: "Classifies and assigns new issues" },
  { id: "agent-3", name: "OnCallResponder", description: "Handles urgent Slack mentions and escalations" },
  { id: "agent-4", name: "LinearProjectTracker", description: "Tracks project health and status in Linear" },
  { id: "agent-5", name: "CIWatcher", description: "Monitors failed CI runs and posts summaries" },
]

export const MOCK_CONNECTIONS: MockConnection[] = [
  { id: "conn-gh", provider: "github", displayName: "GitHub" },
  { id: "conn-slack", provider: "slack", displayName: "Slack" },
  { id: "conn-linear", provider: "linear", displayName: "Linear" },
  { id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890", provider: "railway", displayName: "Railway" },
]

export const PROVIDER_TRIGGERS: Record<string, { key: string; displayName: string; description: string; resourceType: string }[]> = {
  github: [
    { key: "issues.opened", displayName: "Issue created", description: "Fires when a new issue is created", resourceType: "issues" },
    { key: "issues.closed", displayName: "Issue closed", description: "Fires when an issue is closed", resourceType: "issues" },
    { key: "issues.labeled", displayName: "Issue labeled", description: "Fires when a label is added to an issue", resourceType: "issues" },
    { key: "issues.reopened", displayName: "Issue reopened", description: "Previously closed issue reopened", resourceType: "issues" },
    { key: "pull_request.opened", displayName: "PR created", description: "Fires when a new pull request is created", resourceType: "pull_request" },
    { key: "pull_request.closed", displayName: "PR closed", description: "PR closed (check merged field)", resourceType: "pull_request" },
    { key: "pull_request.ready_for_review", displayName: "PR ready for review", description: "Draft PR marked ready for review", resourceType: "pull_request" },
    { key: "pull_request.synchronize", displayName: "PR commits pushed", description: "New commits pushed to PR branch", resourceType: "pull_request" },
    { key: "pull_request_review.submitted", displayName: "Review submitted", description: "Review submitted on a PR", resourceType: "pull_request_review" },
    { key: "issue_comment.created", displayName: "Comment created", description: "Comment on issue or PR", resourceType: "comment" },
    { key: "workflow_run.completed", displayName: "Workflow completed", description: "GitHub Actions workflow run completes", resourceType: "workflow" },
    { key: "push", displayName: "Push", description: "Commits pushed to branch or tag", resourceType: "repository" },
  ],
  slack: [
    { key: "app_mention", displayName: "Bot mentioned", description: "Bot @-mentioned in channel or thread", resourceType: "message" },
    { key: "message.im", displayName: "Direct message", description: "Direct message sent to bot", resourceType: "message" },
    { key: "message.channels", displayName: "Channel message", description: "Message in public channel bot is in", resourceType: "message" },
    { key: "message.groups", displayName: "Private channel message", description: "Message in private channel", resourceType: "message" },
    { key: "reaction_added", displayName: "Reaction added", description: "Emoji reaction added to a message", resourceType: "reaction" },
    { key: "member_joined_channel", displayName: "Member joined", description: "User joins a channel", resourceType: "channel" },
    { key: "channel_created", displayName: "Channel created", description: "New channel created in workspace", resourceType: "channel" },
    { key: "team_join", displayName: "Team join", description: "New user joins workspace", resourceType: "team" },
  ],
  linear: [
    { key: "Issue.create", displayName: "Issue created", description: "New Linear issue created", resourceType: "issue" },
    { key: "Issue.update", displayName: "Issue updated", description: "Issue title, status, assignee, or priority changed", resourceType: "issue" },
    { key: "Issue.remove", displayName: "Issue removed", description: "Issue deleted or removed", resourceType: "issue" },
    { key: "Comment.create", displayName: "Comment created", description: "New comment on an issue", resourceType: "comment" },
    { key: "Project.create", displayName: "Project created", description: "New project created", resourceType: "project" },
    { key: "Project.update", displayName: "Project updated", description: "Project name, status, or health changed", resourceType: "project" },
    { key: "ProjectUpdate.create", displayName: "Status update posted", description: "Status update posted on a project", resourceType: "project_update" },
    { key: "Cycle.create", displayName: "Cycle created", description: "New sprint/cycle created", resourceType: "cycle" },
    { key: "Cycle.update", displayName: "Cycle updated", description: "Cycle dates or progress changed", resourceType: "cycle" },
    { key: "IssueSLA.breached", displayName: "SLA breached", description: "Issue breached its SLA deadline", resourceType: "sla" },
    { key: "IssueSLA.highRisk", displayName: "SLA high risk", description: "Issue at high risk of breaching SLA", resourceType: "sla" },
  ],
  railway: [
    { key: "Deployment.success", displayName: "Deployment succeeded", description: "Deployment completed successfully and is live", resourceType: "deployment" },
    { key: "Deployment.failed", displayName: "Deployment failed", description: "Deployment failed during build or startup", resourceType: "deployment" },
    { key: "Deployment.crashed", displayName: "Deployment crashed", description: "Running deployment crashed unexpectedly", resourceType: "deployment" },
    { key: "Deployment.building", displayName: "Deployment building", description: "Deployment started building", resourceType: "deployment" },
    { key: "Deployment.deploying", displayName: "Deployment deploying", description: "Build completed, deployment starting up", resourceType: "deployment" },
    { key: "Deployment.initializing", displayName: "Deployment initializing", description: "Deployment begins initializing", resourceType: "deployment" },
    { key: "Deployment.removed", displayName: "Deployment removed", description: "Deployment was removed", resourceType: "deployment" },
    { key: "Deployment.sleeping", displayName: "Deployment sleeping", description: "Deployment entered sleep mode due to inactivity", resourceType: "deployment" },
    { key: "Deployment.queued", displayName: "Deployment queued", description: "Deployment queued and waiting to start", resourceType: "deployment" },
    { key: "Deployment.needs_approval", displayName: "Deployment needs approval", description: "Deployment waiting for manual approval", resourceType: "deployment" },
  ],
}

// Providers that require manual webhook URL configuration.
// Maps provider name to the configuration notes shown to the user.
export const PROVIDER_WEBHOOK_CONFIG: Record<string, { webhookUrlRequired: boolean; configurationNotes: string }> = {
  railway: {
    webhookUrlRequired: true,
    configurationNotes: "Railway requires manual webhook configuration.\n\n1. Open your Railway project dashboard\n2. Go to **Settings \u2192 Webhooks**\n3. Paste the webhook URL shown below\n4. Click **Save**\n\nRailway will send deployment status events to this URL. Note: Railway does not support webhook signature verification.",
  },
}

export const PAYLOAD_PATHS: Record<string, { path: string; example: string }[]> = {
  github: [
    { path: "action", example: '"opened"' },
    { path: "issue.number", example: "42" },
    { path: "issue.title", example: '"Fix login bug"' },
    { path: "issue.state", example: '"open"' },
    { path: "pull_request.number", example: "17" },
    { path: "pull_request.title", example: '"Add auth refactor"' },
    { path: "pull_request.merged", example: "false" },
    { path: "label.name", example: '"bug"' },
    { path: "repository.full_name", example: '"acme/api"' },
    { path: "sender.login", example: '"alice"' },
    { path: "workflow_run.conclusion", example: '"failure"' },
  ],
  slack: [
    { path: "event.type", example: '"app_mention"' },
    { path: "event.text", example: '"@Zira review the PR"' },
    { path: "event.channel", example: '"C01ABC"' },
    { path: "event.user", example: '"U01ABC"' },
    { path: "event.thread_ts", example: '"1234567890.123"' },
    { path: "event.reaction", example: '"thumbsup"' },
  ],
  linear: [
    { path: "type", example: '"Issue"' },
    { path: "action", example: '"create"' },
    { path: "data.id", example: '"abc-123"' },
    { path: "data.title", example: '"Fix deployment"' },
    { path: "data.priority", example: "1" },
    { path: "data.state.name", example: '"In Progress"' },
    { path: "data.assignee.name", example: '"Alice"' },
    { path: "data.team.key", example: '"ENG"' },
  ],
}

export const CONDITION_OPERATORS = [
  { value: "equals", label: "equals" },
  { value: "not_equals", label: "not equals" },
  { value: "contains", label: "contains" },
  { value: "not_contains", label: "not contains" },
  { value: "one_of", label: "one of" },
  { value: "not_one_of", label: "not one of" },
  { value: "matches", label: "matches (regex)" },
  { value: "exists", label: "exists" },
  { value: "not_exists", label: "not exists" },
]

export const INITIAL_TRIGGERS: MockTrigger[] = [
  {
    id: "trigger-1",
    connectionId: "conn-gh",
    provider: "github",
    connectionName: "GitHub",
    triggerKeys: ["pull_request.opened", "pull_request.ready_for_review", "issues.opened", "issues.labeled", "workflow_run.completed"],
    routingMode: "rule",
    enabled: true,
    enrichCrossReferences: false,
    rules: [
      {
        id: "rule-1",
        agentId: "agent-1",
        agentName: "CodeReviewer",
        priority: 1,
        conditions: {
          mode: "any",
          conditions: [{ path: "pull_request.number", operator: "exists" }],
        },
      },
      {
        id: "rule-2",
        agentId: "agent-2",
        agentName: "TriageBot",
        priority: 1,
        conditions: {
          mode: "all",
          conditions: [
            { path: "issue.number", operator: "exists" },
            { path: "label.name", operator: "equals", value: "bug" },
          ],
        },
      },
      {
        id: "rule-3",
        agentId: "agent-5",
        agentName: "CIWatcher",
        priority: 1,
        conditions: {
          mode: "all",
          conditions: [{ path: "workflow_run.conclusion", operator: "equals", value: "failure" }],
        },
      },
      {
        id: "rule-4",
        agentId: "agent-2",
        agentName: "TriageBot",
        priority: 99,
        conditions: null,
      },
    ],
  },
  {
    id: "trigger-2",
    connectionId: "conn-slack",
    provider: "slack",
    connectionName: "Slack",
    triggerKeys: ["app_mention", "message.im"],
    routingMode: "triage",
    enabled: true,
    enrichCrossReferences: true,
    rules: [],
  },
  {
    id: "trigger-3",
    connectionId: "conn-linear",
    provider: "linear",
    connectionName: "Linear",
    triggerKeys: ["Issue.create", "Issue.update", "IssueSLA.breached", "IssueSLA.highRisk"],
    routingMode: "rule",
    enabled: true,
    enrichCrossReferences: false,
    rules: [
      {
        id: "rule-5",
        agentId: "agent-3",
        agentName: "OnCallResponder",
        priority: 1,
        conditions: {
          mode: "all",
          conditions: [{ path: "type", operator: "equals", value: "IssueSLA" }],
        },
      },
      {
        id: "rule-6",
        agentId: "agent-2",
        agentName: "TriageBot",
        priority: 2,
        conditions: null,
      },
    ],
  },
]

export const INITIAL_DECISIONS: MockDecision[] = [
  {
    id: "dec-1",
    eventType: "issues.opened",
    routingMode: "rule",
    resourceKey: "github:issues:acme/api#142",
    intentSummary: "deterministic rule match",
    selectedAgents: ["TriageBot"],
    enrichmentSteps: 0,
    turnCount: 0,
    latencyMs: 4,
    createdAt: new Date(Date.now() - 2 * 60 * 1000).toISOString(),
  },
  {
    id: "dec-2",
    eventType: "app_mention",
    routingMode: "triage",
    resourceKey: "slack:thread:C01ABC:1234567890.123",
    intentSummary: "User is asking for a PR review on the auth refactor",
    selectedAgents: ["CodeReviewer"],
    enrichmentSteps: 1,
    turnCount: 2,
    latencyMs: 820,
    createdAt: new Date(Date.now() - 14 * 60 * 1000).toISOString(),
  },
  {
    id: "dec-3",
    eventType: "pull_request.opened",
    routingMode: "rule",
    resourceKey: "github:pull_request:acme/api#87",
    intentSummary: "deterministic rule match",
    selectedAgents: ["CodeReviewer", "TriageBot"],
    enrichmentSteps: 0,
    turnCount: 0,
    latencyMs: 3,
    createdAt: new Date(Date.now() - 31 * 60 * 1000).toISOString(),
  },
  {
    id: "dec-4",
    eventType: "Issue.create",
    routingMode: "rule",
    resourceKey: "linear:issue:ENG-421",
    intentSummary: "deterministic rule match",
    selectedAgents: ["TriageBot", "LinearProjectTracker"],
    enrichmentSteps: 0,
    turnCount: 0,
    latencyMs: 2,
    createdAt: new Date(Date.now() - 65 * 60 * 1000).toISOString(),
  },
  {
    id: "dec-5",
    eventType: "IssueSLA.breached",
    routingMode: "rule",
    resourceKey: "linear:issue:ENG-398",
    intentSummary: "SLA breach escalation",
    selectedAgents: ["OnCallResponder", "TriageBot"],
    enrichmentSteps: 0,
    turnCount: 0,
    latencyMs: 3,
    createdAt: new Date(Date.now() - 72 * 60 * 1000).toISOString(),
  },
  {
    id: "dec-6",
    eventType: "app_mention",
    routingMode: "triage",
    resourceKey: "slack:thread:C01ABC:1234567891.456",
    intentSummary: "General question about deployment status",
    selectedAgents: ["LinearProjectTracker"],
    enrichmentSteps: 2,
    turnCount: 3,
    latencyMs: 1240,
    createdAt: new Date(Date.now() - 95 * 60 * 1000).toISOString(),
  },
  {
    id: "dec-7",
    eventType: "workflow_run.completed",
    routingMode: "rule",
    resourceKey: "github:workflow:acme/api:ci.yml:91",
    intentSummary: "deterministic rule match",
    selectedAgents: ["CIWatcher"],
    enrichmentSteps: 0,
    turnCount: 0,
    latencyMs: 5,
    createdAt: new Date(Date.now() - 120 * 60 * 1000).toISOString(),
  },
  {
    id: "dec-8",
    eventType: "app_mention",
    routingMode: "triage",
    resourceKey: "slack:thread:C02DEF:1234567892.789",
    intentSummary: "No relevant agent found for this request",
    selectedAgents: [],
    enrichmentSteps: 0,
    turnCount: 2,
    latencyMs: 650,
    createdAt: new Date(Date.now() - 150 * 60 * 1000).toISOString(),
  },
]

export const INITIAL_SETTINGS: MockRouterSettings = {
  persona: "You are Zira, an engineering operations assistant for the Acme team. You are helpful, concise, and proactive. When responding in Slack, keep messages short and actionable. When reviewing PRs, be thorough but kind.",
  defaultAgentId: "agent-2",
  memoryTeam: "engineering",
}
