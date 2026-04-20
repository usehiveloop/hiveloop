import type { Integration, LlmKey, MarketplaceAgentPreview } from "./types"

export const integrations: Integration[] = [
  {
    id: "slack",
    name: "Slack",
    logo: "https://cdn.simpleicons.org/slack",
    description: "Send messages, manage channels, and react to events",
    actions: [
      { id: "post_message", name: "Post message", description: "Send a message to a channel or DM", type: "write" },
      { id: "list_channels", name: "List channels", description: "Get all channels in the workspace", type: "read" },
      { id: "add_reaction", name: "Add reaction", description: "React to a message with an emoji", type: "write" },
      { id: "get_user", name: "Get user info", description: "Look up a user by ID or email", type: "read" },
      { id: "upload_file", name: "Upload file", description: "Upload a file to a channel", type: "write" },
    ],
  },
  {
    id: "linear",
    name: "Linear",
    logo: "https://cdn.simpleicons.org/linear",
    description: "Create issues, manage projects, and track progress",
    actions: [
      { id: "create_issue", name: "Create issue", description: "Create a new issue in a team", type: "write" },
      { id: "update_issue", name: "Update issue", description: "Update an existing issue's fields", type: "write" },
      { id: "list_issues", name: "List issues", description: "Search and filter issues", type: "read" },
      { id: "get_issue", name: "Get issue", description: "Get details of a specific issue", type: "read" },
      { id: "add_comment", name: "Add comment", description: "Comment on an issue", type: "write" },
      { id: "delete_issue", name: "Delete issue", description: "Permanently delete an issue", type: "delete" },
    ],
  },
  {
    id: "github",
    name: "GitHub",
    logo: "https://cdn.simpleicons.org/github/white",
    description: "Manage repos, PRs, issues, and code reviews",
    actions: [
      { id: "get_issue", name: "Get issue", description: "Fetch a specific issue by number", type: "read" },
      { id: "create_issue", name: "Create issue", description: "Open a new issue in a repository", type: "write" },
      { id: "search_issues", name: "Search issues", description: "Search issues across repositories", type: "read" },
      { id: "add_labels", name: "Add labels", description: "Add labels to an issue or PR", type: "write" },
      { id: "create_comment", name: "Create comment", description: "Comment on an issue or PR", type: "write" },
      { id: "get_pull_request", name: "Get pull request", description: "Fetch PR details and diff", type: "read" },
      { id: "merge_pr", name: "Merge pull request", description: "Merge a pull request", type: "write" },
    ],
  },
  {
    id: "notion",
    name: "Notion",
    logo: "https://cdn.simpleicons.org/notion",
    description: "Read and write pages, databases, and blocks",
    actions: [
      { id: "get_page", name: "Get page", description: "Retrieve a page and its content", type: "read" },
      { id: "create_page", name: "Create page", description: "Create a new page in a database", type: "write" },
      { id: "update_page", name: "Update page", description: "Update page properties", type: "write" },
      { id: "query_database", name: "Query database", description: "Search and filter a database", type: "read" },
      { id: "append_block", name: "Append block", description: "Add content blocks to a page", type: "write" },
    ],
  },
  {
    id: "google",
    name: "Google Calendar",
    logo: "https://cdn.simpleicons.org/googlecalendar",
    description: "Create events, check availability, and manage calendars",
    actions: [
      { id: "list_events", name: "List events", description: "Get upcoming events from a calendar", type: "read" },
      { id: "create_event", name: "Create event", description: "Schedule a new calendar event", type: "write" },
      { id: "update_event", name: "Update event", description: "Modify an existing event", type: "write" },
      { id: "delete_event", name: "Delete event", description: "Remove an event from calendar", type: "delete" },
    ],
  },
  {
    id: "intercom",
    name: "Intercom",
    logo: "https://cdn.simpleicons.org/intercom",
    description: "Manage conversations, contacts, and support tickets",
    actions: [
      { id: "list_conversations", name: "List conversations", description: "Get recent conversations", type: "read" },
      { id: "reply", name: "Reply to conversation", description: "Send a reply in a conversation", type: "write" },
      { id: "get_contact", name: "Get contact", description: "Look up a contact by ID or email", type: "read" },
      { id: "create_note", name: "Create note", description: "Add an internal note to a conversation", type: "write" },
    ],
  },
]

export const llmKeys: LlmKey[] = [
  {
    id: "key-1",
    name: "Production key",
    provider: "Anthropic",
    logo: "https://cdn.simpleicons.org/anthropic",
    models: ["claude-sonnet-4-20250514", "claude-haiku-4-20250414", "claude-opus-4-20250514"],
  },
  {
    id: "key-2",
    name: "Team key",
    provider: "OpenAI",
    logo: "https://cdn.simpleicons.org/openai",
    models: ["gpt-4o", "gpt-4o-mini", "o3-mini"],
  },
  {
    id: "key-3",
    name: "Gemini access",
    provider: "Google",
    logo: "https://cdn.simpleicons.org/google",
    models: ["gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.0-flash"],
  },
]

export const marketplaceAgents: MarketplaceAgentPreview[] = [
  {
    slug: "pr-review-agent",
    name: "PR Review Agent",
    description: "Automatically reviews pull requests, checks for code quality issues, security vulnerabilities, and suggests improvements based on your team's standards.",
    publisher: { name: "Sarah Chen", avatar: "https://i.pravatar.cc/80?u=sarah" },
    installs: 12400,
    integrations: ["GitHub", "Slack", "Linear"],
    verified: true,
  },
  {
    slug: "customer-support-agent",
    name: "Customer Support Agent",
    description: "Handles incoming support tickets by searching your knowledge base, drafting responses, and escalating complex issues to the right team member.",
    publisher: { name: "Alex Rivera", avatar: "https://i.pravatar.cc/80?u=alex" },
    installs: 8900,
    integrations: ["Intercom", "Notion", "Slack"],
    verified: true,
  },
  {
    slug: "incident-responder",
    name: "Incident Responder",
    description: "Monitors your infrastructure alerts, correlates events, creates incident channels, and coordinates response workflows automatically.",
    publisher: { name: "Hiveloop", avatar: "https://i.pravatar.cc/80?u=hiveloop" },
    installs: 6200,
    integrations: ["Slack", "Linear", "GitHub"],
    verified: true,
  },
  {
    slug: "daily-standup-bot",
    name: "Daily Standup Bot",
    description: "Collects async standup updates from your team, summarizes blockers and progress, and posts a digest to your team channel every morning.",
    publisher: { name: "Maria Santos", avatar: "https://i.pravatar.cc/80?u=maria" },
    installs: 5100,
    integrations: ["Slack", "Linear"],
    verified: false,
  },
  {
    slug: "release-manager",
    name: "Release Manager",
    description: "Tracks your release pipeline, generates changelogs from merged PRs, notifies stakeholders, and manages deployment approvals across environments.",
    publisher: { name: "Hiveloop", avatar: "https://i.pravatar.cc/80?u=hiveloop2" },
    installs: 4500,
    integrations: ["GitHub", "Slack"],
    verified: true,
  },
  {
    slug: "meeting-summarizer",
    name: "Meeting Summarizer",
    description: "Joins your calendar meetings, records key decisions and action items, and posts structured summaries to the relevant Notion page.",
    publisher: { name: "Tom Wilson", avatar: "https://i.pravatar.cc/80?u=tom" },
    installs: 7300,
    integrations: ["Google Calendar", "Notion", "Slack"],
    verified: true,
  },
]

export const connectedIntegrations = new Set(["GitHub", "Slack"])
