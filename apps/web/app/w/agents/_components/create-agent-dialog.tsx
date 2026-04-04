"use client"

import { useState, useRef } from "react"
import { AnimatePresence, motion } from "motion/react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  PencilEdit02Icon,
  SparklesIcon,
  ArrowRight01Icon,
  ArrowLeft01Icon,
  CloudServerIcon,
  LaptopProgrammingIcon,
  Search01Icon,
  Tick02Icon,
  Key01Icon,
  Store01Icon,
  Download04Icon,
  CheckmarkBadge01Icon,
} from "@hugeicons/core-free-icons"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command"

// --- Types ---

type CreationMode = "scratch" | "forge" | "marketplace"
type Step = "mode" | "sandbox" | "integrations" | "llm-key" | "basics" | "system-prompt" | "instructions" | "forge-judge" | "summary" | "marketplace-browse" | "marketplace-detail"

const scratchSteps: Step[] = ["mode", "sandbox", "integrations", "llm-key", "basics", "system-prompt", "instructions", "summary"]
const forgeSteps: Step[] = ["mode", "sandbox", "integrations", "llm-key", "basics", "forge-judge", "summary"]
const marketplaceSteps: Step[] = ["mode", "marketplace-browse", "marketplace-detail"]

type Integration = {
  id: string
  name: string
  logo: string
  description: string
  actions: IntegrationAction[]
}

type IntegrationAction = {
  id: string
  name: string
  description: string
  type: "read" | "write" | "delete"
}

type MarketplaceAgent = {
  slug: string
  name: string
  description: string
  publisher: { name: string; avatar: string }
  installs: number
  integrations: string[]
  verified: boolean
}

const marketplaceAgents: MarketplaceAgent[] = [
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
    publisher: { name: "Ziraloop", avatar: "https://i.pravatar.cc/80?u=ziraloop" },
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
    publisher: { name: "Ziraloop", avatar: "https://i.pravatar.cc/80?u=ziraloop2" },
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

type LlmKey = {
  id: string
  name: string
  provider: string
  logo: string
  models: string[]
}

const llmKeys: LlmKey[] = [
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

// --- Static data ---

const integrations: Integration[] = [
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

// --- Shared components ---

type ChoiceCardProps = {
  icon?: typeof PencilEdit02Icon
  iconClassName?: string
  logoUrl?: string
  title: string
  description: string
  onClick: () => void
  trailing?: React.ReactNode
}

function ChoiceCard({ icon, iconClassName, logoUrl, title, description, onClick, trailing }: ChoiceCardProps) {
  return (
    <button
      onClick={onClick}
      className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
    >
      {logoUrl ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={logoUrl} alt={title} className="h-5 w-5 shrink-0 mt-0.5" />
      ) : icon ? (
        <HugeiconsIcon icon={icon} size={20} className={`shrink-0 mt-0.5 ${iconClassName ?? "text-muted-foreground"}`} />
      ) : null}
      <div className="flex-1 min-w-0">
        <p className="text-sm font-semibold text-foreground">{title}</p>
        <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">{description}</p>
      </div>
      {trailing ?? (
        <HugeiconsIcon
          icon={ArrowRight01Icon}
          size={16}
          className="text-muted-foreground/30 shrink-0 mt-0.5"
        />
      )}
    </button>
  )
}

// --- Step 1: Choose mode ---

function StepChooseMode({ onSelect }: { onSelect: (mode: CreationMode) => void }) {
  return (
    <div>
      <DialogHeader>
        <DialogTitle>Create a new agent</DialogTitle>
        <DialogDescription className="mt-2">
          Build from scratch, let AI generate one for you, or install a ready-made agent from the marketplace.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3 pt-4">
        <ChoiceCard
          icon={PencilEdit02Icon}
          title="Create from scratch"
          description="Write your own system prompt and configure every detail manually."
          onClick={() => onSelect("scratch")}
        />
        <ChoiceCard
          icon={SparklesIcon}
          title="Forge with AI"
          description="Describe what you want and let AI generate an optimized agent for you."
          onClick={() => onSelect("forge")}
        />
        <ChoiceCard
          icon={Store01Icon}
          title="Install from marketplace"
          description="Browse community-built agents and install one in seconds."
          onClick={() => onSelect("marketplace")}
        />
      </div>
    </div>
  )
}

// --- Step 2: Sandbox type ---

function StepSandboxType({ onSelect, onBack }: { onSelect: (type: "shared" | "dedicated") => void; onBack: () => void }) {
  return (
    <div>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Choose a workspace</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Workspaces are isolated environments where your agent runs. Choose the type that fits your agent&apos;s needs.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3 pt-4">
        <ChoiceCard
          icon={CloudServerIcon}
          title="Shared workspace"
          description="End-to-end encrypted. Best for agents that interact with APIs, process data, and call tools — without needing file system access."
          onClick={() => onSelect("shared")}
        />
        <ChoiceCard
          icon={LaptopProgrammingIcon}
          title="Dedicated workspace"
          description="Full system access. For agents that need to read and write files, run shell commands, use code interpreters, or interact with a development environment."
          onClick={() => onSelect("dedicated")}
        />
      </div>
    </div>
  )
}

// --- Step 3: Integrations ---

function StepIntegrations({
  selected,
  selectedActions,
  onToggleAction,
  onBack,
  onNext,
}: {
  selected: Set<string>
  selectedActions: Record<string, Set<string>>
  onToggleAction: (integrationId: string, actionId: string) => void
  onBack: () => void
  onNext: () => void
}) {
  const [search, setSearch] = useState("")
  const [detailView, setDetailView] = useState<string | null>(null)
  const [actionSearch, setActionSearch] = useState("")
  const detailDirection = useRef<1 | -1>(1)

  const filtered = integrations.filter((i) =>
    i.name.toLowerCase().includes(search.toLowerCase())
  )

  const selectedCount = Object.values(selectedActions).filter((s) => s.size > 0).length

  const innerVariants = {
    enter: (d: number) => ({ x: d > 0 ? 60 : -60, opacity: 0 }),
    center: { x: 0, opacity: 1 },
    exit: (d: number) => ({ x: d > 0 ? -60 : 60, opacity: 0 }),
  }

  function openDetail(id: string) {
    detailDirection.current = 1
    setDetailView(id)
    setActionSearch("")
  }

  function closeDetail() {
    detailDirection.current = -1
    setDetailView(null)
    setActionSearch("")
  }

  return (
    <div className="flex flex-col h-full overflow-hidden">
      <AnimatePresence mode="wait" custom={detailDirection.current}>
        {detailView ? (
          <motion.div
            key={`detail-${detailView}`}
            custom={detailDirection.current}
            variants={innerVariants}
            initial="enter"
            animate="center"
            exit="exit"
            transition={{ duration: 0.15, ease: "easeInOut" }}
            className="flex flex-col h-full"
          >
            {(() => {
              const integration = integrations.find((i) => i.id === detailView)!
              const actions = integration.actions.filter((a) =>
                a.name.toLowerCase().includes(actionSearch.toLowerCase())
              )
              const selectedForThis = selectedActions[integration.id] ?? new Set<string>()
              const allSelected = integration.actions.every((a) => selectedForThis.has(a.id))

              return (
                <>
                  <DialogHeader>
                    <div className="flex items-center gap-2">
                      <button
                        onClick={closeDetail}
                        className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1"
                      >
                        <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
                      </button>
                      <div className="flex items-center gap-2.5">
                        {/* eslint-disable-next-line @next/next/no-img-element */}
                        <img src={integration.logo} alt={integration.name} className="h-5 w-5" />
                        <DialogTitle>{integration.name}</DialogTitle>
                      </div>
                    </div>
                    <DialogDescription className="mt-2">
                      Select which actions this agent can use. You can always change this later.
                    </DialogDescription>
                  </DialogHeader>

                  <div className="relative mt-4">
                    <HugeiconsIcon icon={Search01Icon} size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      placeholder="Search actions..."
                      value={actionSearch}
                      onChange={(e) => setActionSearch(e.target.value)}
                      className="pl-9 h-9"
                    />
                  </div>

                  <button
                    onClick={() => {
                      integration.actions.forEach((a) => {
                        if (!allSelected) {
                          if (!selectedForThis.has(a.id)) onToggleAction(integration.id, a.id)
                        } else {
                          if (selectedForThis.has(a.id)) onToggleAction(integration.id, a.id)
                        }
                      })
                    }}
                    className="flex items-center justify-between px-1 py-2 mt-3 text-xs font-medium text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                  >
                    <span>{allSelected ? "Deselect all" : "Select all"}</span>
                    <span className="tabular-nums">{selectedForThis.size}/{integration.actions.length}</span>
                  </button>

                  <div className="flex flex-col gap-1 mt-1 flex-1 overflow-y-auto">
                    {actions.map((action) => {
                      const isSelected = selectedForThis.has(action.id)
                      return (
                        <button
                          key={action.id}
                          onClick={() => onToggleAction(integration.id, action.id)}
                          className={`flex items-start gap-3 w-full rounded-xl p-3 text-left transition-colors cursor-pointer ${
                            isSelected ? "bg-primary/5 border border-primary/20" : "bg-muted/50 hover:bg-muted border border-transparent"
                          }`}
                        >
                          <div className="flex-1 min-w-0">
                            <div className="flex items-center gap-2">
                              <span className="text-sm font-medium text-foreground">{action.name}</span>
                              <span className={`font-mono text-[9px] uppercase tracking-[0.5px] px-1.5 py-0.5 rounded-full ${
                                action.type === "read" ? "bg-blue-500/10 text-blue-500" :
                                action.type === "write" ? "bg-green-500/10 text-green-500" :
                                "bg-destructive/10 text-destructive"
                              }`}>
                                {action.type}
                              </span>
                            </div>
                            <p className="text-[12px] text-muted-foreground mt-0.5">{action.description}</p>
                          </div>
                          {isSelected && (
                            <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0 mt-0.5" />
                          )}
                        </button>
                      )
                    })}
                  </div>
                </>
              )
            })()}
          </motion.div>
        ) : (
          <motion.div
            key="integration-list"
            custom={detailDirection.current}
            variants={innerVariants}
            initial="enter"
            animate="center"
            exit="exit"
            transition={{ duration: 0.15, ease: "easeInOut" }}
            className="flex flex-col h-full"
          >
            <DialogHeader>
              <div className="flex items-center gap-2">
                <button onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
                  <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
                </button>
                <DialogTitle>Connect integrations</DialogTitle>
              </div>
              <DialogDescription className="mt-2">
                Choose which integrations your agent can access. You&apos;ll pick specific actions for each one.
              </DialogDescription>
            </DialogHeader>

            <div className="relative mt-4">
              <HugeiconsIcon icon={Search01Icon} size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search integrations..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-9 h-9"
              />
            </div>

            <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
              {filtered.map((integration) => {
                const actionCount = selectedActions[integration.id]?.size ?? 0
                return (
                  <ChoiceCard
                    key={integration.id}
                    logoUrl={integration.logo}
                    title={integration.name}
                    description={integration.description}
                    onClick={() => openDetail(integration.id)}
                    trailing={
                      actionCount > 0 ? (
                        <span className="flex items-center gap-1.5 shrink-0 mt-0.5">
                          <span className="font-mono text-[11px] text-primary">{actionCount}</span>
                          <HugeiconsIcon icon={ArrowRight01Icon} size={14} className="text-muted-foreground/30" />
                        </span>
                      ) : (
                        <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
                      )
                    }
                  />
                )
              })}
            </div>

            <div className="pt-4 shrink-0">
              <Button onClick={onNext} className="w-full">
                {selectedCount > 0 ? `Continue with ${selectedCount} integration${selectedCount > 1 ? "s" : ""}` : "Skip for now"}
              </Button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

// --- Step 4: LLM Key ---

function StepLlmKey({
  selectedKey,
  onSelect,
  onBack,
}: {
  selectedKey: string | null
  onSelect: (keyId: string) => void
  onBack: () => void
}) {
  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Select an LLM key</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Choose which AI provider your agent will use. You can add a new key if you haven&apos;t connected one yet.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {llmKeys.map((key) => (
          <ChoiceCard
            key={key.id}
            logoUrl={key.logo}
            title={key.name}
            description={`${key.provider} · ${key.models.length} models available`}
            onClick={() => onSelect(key.id)}
            trailing={
              selectedKey === key.id ? (
                <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0 mt-0.5" />
              ) : (
                <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
              )
            }
          />
        ))}
      </div>

      <div className="pt-4 shrink-0">
        <Button variant="outline" className="w-full">
          <HugeiconsIcon icon={Key01Icon} size={16} data-icon="inline-start" />
          Add LLM key
        </Button>
      </div>
    </div>
  )
}

// --- Step 5: Basics ---

function ModelCombobox({ models, value, onSelect: onSelectProp }: { models: string[]; value?: string | null; onSelect?: (model: string) => void }) {
  const [open, setOpen] = useState(false)
  const [internal, setInternal] = useState(models[0] ?? "")
  const selected = value !== undefined ? (value ?? "") : internal

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <button className="flex w-full items-center justify-between rounded-2xl border border-input bg-input/50 px-3 py-2 text-sm transition-colors hover:bg-input/70 outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30">
            <span className={`font-mono text-sm ${selected ? "text-foreground" : "text-muted-foreground"}`}>
              {selected || "Select a model..."}
            </span>
            <HugeiconsIcon icon={ArrowRight01Icon} size={14} className={`text-muted-foreground/40 transition-transform ${open ? "rotate-90" : ""}`} />
          </button>
        }
      />
      <PopoverContent className="w-[var(--anchor-width)] p-0" align="start">
        <Command>
          <CommandInput placeholder="Search models..." />
          <CommandList>
            <CommandEmpty>No models found.</CommandEmpty>
            <CommandGroup>
              {models.map((model) => (
                <CommandItem
                  key={model}
                  value={model}
                  onSelect={() => {
                    if (onSelectProp) onSelectProp(model)
                    else setInternal(model)
                    setOpen(false)
                  }}
                  className="font-mono text-sm"
                >
                  {model}
                  {selected === model && (
                    <HugeiconsIcon icon={Tick02Icon} size={14} className="ml-auto text-primary" />
                  )}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

function StepBasics({
  selectedKeyId,
  onBack,
  onSubmit,
}: {
  selectedKeyId: string | null
  onBack: () => void
  onSubmit: () => void
}) {
  const key = llmKeys.find((k) => k.id === selectedKeyId)

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Agent details</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Give your agent a name, pick a model, and optionally describe what it does.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-5 mt-4 flex-1">
        <div className="flex flex-col gap-2">
          <Label htmlFor="agent-name" className="text-sm">Name</Label>
          <Input id="agent-name" placeholder="e.g. Issue Triage Agent" />
        </div>

        <div className="flex flex-col gap-2">
          <Label className="text-sm">Model</Label>
          {key ? (
            <ModelCombobox models={key.models} />
          ) : (
            <Input disabled placeholder="Select an LLM key first" />
          )}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="agent-description" className="text-sm">
            Description <span className="text-muted-foreground font-normal">(optional)</span>
          </Label>
          <Textarea id="agent-description" placeholder="Briefly describe what this agent does..." className="min-h-24" />
        </div>
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onSubmit} className="w-full">
          Continue
        </Button>
      </div>
    </div>
  )
}

// --- Step: Forge Judge (forge only) ---

function StepForgeJudge({
  selectedKeyId,
  judgeKeyId,
  onSelectKey,
  judgeModel,
  onSelectModel,
  onBack,
  onNext,
  onSkip,
}: {
  selectedKeyId: string | null
  judgeKeyId: string | null
  onSelectKey: (keyId: string) => void
  judgeModel: string | null
  onSelectModel: (model: string) => void
  onBack: () => void
  onNext: () => void
  onSkip: () => void
}) {
  const selectedKey = llmKeys.find((k) => k.id === judgeKeyId)
  const agentKey = llmKeys.find((k) => k.id === selectedKeyId)
  const isSameProvider = agentKey && selectedKey && agentKey.provider === selectedKey.provider

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Forge judge</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Pick an LLM to evaluate and score your agent during the forge process. A different provider from your agent&apos;s LLM is recommended.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-4 mt-4 flex-1 overflow-y-auto">
        <div className="flex flex-col gap-2">
          <Label className="text-sm">Provider</Label>
          <div className="flex flex-col gap-2">
            {llmKeys.map((key) => (
              <ChoiceCard
                key={key.id}
                logoUrl={key.logo}
                title={key.name}
                description={key.provider}
                onClick={() => onSelectKey(key.id)}
                trailing={
                  judgeKeyId === key.id ? (
                    <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0 mt-0.5" />
                  ) : (
                    <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
                  )
                }
              />
            ))}
          </div>
        </div>

        {selectedKey && (
          <div className="flex flex-col gap-2">
            <Label className="text-sm">Model</Label>
            <ModelCombobox
              models={selectedKey.models}
              value={judgeModel}
              onSelect={onSelectModel}
            />
          </div>
        )}

        {isSameProvider && (
          <div className="rounded-xl border border-amber-500/20 bg-amber-500/5 px-4 py-3 flex gap-3 items-start">
            <span className="text-amber-500 text-sm leading-none mt-0.5">!</span>
            <p className="text-sm text-amber-500/90 leading-snug">
              Using a different AI model for the forge judge reduces bias and can lead to a more efficient agent.
            </p>
          </div>
        )}
      </div>

      <div className="flex flex-col gap-2 pt-4 shrink-0">
        <Button onClick={onNext} disabled={!judgeKeyId || !judgeModel} className="w-full">
          Continue
        </Button>
        <Button variant="ghost" onClick={onSkip} className="w-full text-muted-foreground">
          Skip — use default judge
        </Button>
      </div>
    </div>
  )
}

// --- Step 6 (scratch only): System prompt ---

function StepSystemPrompt({ onBack, onNext }: { onBack: () => void; onNext: () => void }) {
  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>System prompt</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Define your agent&apos;s core behavior, personality, and constraints. This is the main instruction that shapes how your agent responds.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1">
        <Textarea
          placeholder={"You are a helpful assistant that triages GitHub issues.\n\nYour responsibilities:\n- Read and classify incoming issues\n- Assign appropriate labels and priority\n- Route to the correct team\n- Notify stakeholders of urgent issues"}
          className="flex-1 min-h-48 font-mono text-sm"
        />
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onNext} className="w-full">Continue</Button>
      </div>
    </div>
  )
}

// --- Step 7 (scratch only): Instructions ---

function StepInstructions({ onBack, onNext }: { onBack: () => void; onNext: () => void }) {
  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Instructions</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Add specific rules and guidelines your agent should follow. These are additional constraints on top of the system prompt.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1">
        <Textarea
          placeholder={"- Always check for duplicate issues before creating new ones\n- Never close issues without team lead approval\n- Escalate security-related issues to P1 immediately\n- Use professional, concise language in all communications"}
          className="flex-1 min-h-48 font-mono text-sm"
        />
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onNext} className="w-full">Continue</Button>
      </div>
    </div>
  )
}

// --- Step: Marketplace Browse ---

function formatInstalls(n: number) {
  if (n >= 1000) return `${(n / 1000).toFixed(n % 1000 === 0 ? 0 : 1)}k`
  return n.toString()
}

function StepMarketplaceBrowse({
  onBack,
  onSelect,
}: {
  onBack: () => void
  onSelect: (slug: string) => void
}) {
  const [search, setSearch] = useState("")

  const filtered = marketplaceAgents.filter(
    (a) =>
      a.name.toLowerCase().includes(search.toLowerCase()) ||
      a.description.toLowerCase().includes(search.toLowerCase()) ||
      a.integrations.some((i) => i.toLowerCase().includes(search.toLowerCase()))
  )

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Marketplace</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Browse community-built agents. Install one to get started instantly.
        </DialogDescription>
      </DialogHeader>

      <div className="relative mt-4">
        <HugeiconsIcon icon={Search01Icon} size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="Search agents..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="pl-9 h-9"
        />
      </div>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {filtered.map((agent) => (
          <button
            key={agent.slug}
            onClick={() => onSelect(agent.slug)}
            className="group flex items-start gap-3 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
          >
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <p className="text-sm font-semibold text-foreground truncate">{agent.name}</p>
                {agent.verified && (
                  <HugeiconsIcon icon={CheckmarkBadge01Icon} size={13} className="text-green-500 shrink-0" />
                )}
              </div>
              <p className="text-xs text-muted-foreground mt-0.5 line-clamp-2 leading-relaxed">{agent.description}</p>
              <div className="flex items-center gap-3 mt-2">
                <span className="flex items-center gap-1.5 text-[10px] text-muted-foreground/40">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src={agent.publisher.avatar} alt="" className="h-3.5 w-3.5 rounded-full" />
                  {agent.publisher.name}
                </span>
                <span className="text-[10px] text-muted-foreground/30">·</span>
                <span className="font-mono text-[10px] text-muted-foreground/40">
                  {formatInstalls(agent.installs)} installs
                </span>
                <span className="flex items-center -space-x-1.5 ml-auto">
                  {agent.integrations.map((name) => {
                    const integ = integrations.find((i) => i.name === name)
                    return integ ? (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img key={name} src={integ.logo} alt={name} className="h-4 w-4 rounded-full border-2 border-muted/50 dark:invert" />
                    ) : (
                      <span key={name} className="flex h-4 w-4 items-center justify-center rounded-full border-2 border-muted/50 bg-muted text-[7px] font-bold text-muted-foreground">
                        {name[0]}
                      </span>
                    )
                  })}
                </span>
              </div>
            </div>
            <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
          </button>
        ))}
        {filtered.length === 0 && (
          <div className="flex items-center justify-center py-12">
            <p className="text-sm text-muted-foreground">No agents found</p>
          </div>
        )}
      </div>
    </div>
  )
}

// --- Step: Marketplace Detail ---

// TODO: replace with real connected integrations from API
const connectedIntegrations = new Set(["GitHub", "Slack"])

function StepMarketplaceDetail({
  slug,
  onBack,
  onInstall,
}: {
  slug: string
  onBack: () => void
  onInstall: () => void
}) {
  const agent = marketplaceAgents.find((a) => a.slug === slug)

  if (!agent) return null

  const missing = agent.integrations.filter((i) => !connectedIntegrations.has(i))
  const canInstall = missing.length === 0

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Agent details</DialogTitle>
        </div>
      </DialogHeader>

      <div className="flex flex-col gap-6 mt-6 flex-1 overflow-y-auto">
        {/* Agent header */}
        <div className="flex flex-col gap-1">
          <div className="flex items-center gap-2">
            <h3 className="text-base font-semibold text-foreground">{agent.name}</h3>
            {agent.verified && (
              <HugeiconsIcon icon={CheckmarkBadge01Icon} size={14} className="text-green-500 shrink-0" />
            )}
          </div>
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground/50">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={agent.publisher.avatar} alt="" className="h-4 w-4 rounded-full" />
            {agent.publisher.name}
          </span>
        </div>

        {/* Description */}
        <p className="text-sm text-muted-foreground leading-relaxed">
          {agent.description}
        </p>

        {/* Stats */}
        <div className="flex gap-6">
          <div className="flex flex-col">
            <span className="font-mono text-xs text-muted-foreground/60 uppercase tracking-wide">Installs</span>
            <span className="text-sm font-semibold text-foreground mt-0.5">{formatInstalls(agent.installs)}</span>
          </div>
          <div className="flex flex-col">
            <span className="font-mono text-xs text-muted-foreground/60 uppercase tracking-wide">Integrations</span>
            <span className="text-sm font-semibold text-foreground mt-0.5">{agent.integrations.length}</span>
          </div>
        </div>

        {/* Integrations */}
        <div className="flex flex-col gap-2">
          <span className="font-mono text-[10px] text-muted-foreground/60 uppercase tracking-[1px]">Required integrations</span>
          <div className="flex flex-wrap gap-2">
            {agent.integrations.map((name) => {
              const connected = connectedIntegrations.has(name)
              return (
                <span
                  key={name}
                  className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium ${
                    connected
                      ? "bg-muted text-foreground"
                      : "bg-destructive/5 text-destructive border border-destructive/10"
                  }`}
                >
                  {name}
                  {connected && (
                    <HugeiconsIcon icon={Tick02Icon} size={12} className="text-green-500" />
                  )}
                </span>
              )
            })}
          </div>
        </div>

        {/* Missing integrations warning */}
        {missing.length > 0 && (
          <div className="rounded-xl border border-amber-500/20 bg-amber-500/5 px-4 py-3">
            <p className="text-sm text-amber-500/90 leading-snug">
              You are missing {missing.length} {missing.length === 1 ? "integration" : "integrations"} to install this agent. Please connect {missing.join(", ")} and try again.
            </p>
          </div>
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onInstall} disabled={!canInstall} className="w-full">
          <HugeiconsIcon icon={Download04Icon} size={16} data-icon="inline-start" />
          Install agent
        </Button>
      </div>
    </div>
  )
}

// --- Step: Summary ---

function StepSummary({
  mode,
  selectedKeyId,
  selectedActions,
  onBack,
  onSubmit,
}: {
  mode: CreationMode
  selectedKeyId: string | null
  selectedActions: Record<string, Set<string>>
  onBack: () => void
  onSubmit: () => void
}) {
  const key = llmKeys.find((k) => k.id === selectedKeyId)
  const integrationCount = Object.values(selectedActions).filter((s) => s.size > 0).length
  const totalActions = Object.values(selectedActions).reduce((sum, s) => sum + s.size, 0)
  const [expandedIntegrations, setExpandedIntegrations] = useState(false)

  const activeIntegrations = integrations.filter(
    (i) => selectedActions[i.id] && selectedActions[i.id].size > 0
  )

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Review & create</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          {mode === "forge"
            ? "Review your configuration. Forge will generate and optimize your agent's system prompt automatically."
            : "Review your configuration before creating your agent."}
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3 mt-4 flex-1 overflow-y-auto">
        <SummaryRow label="LLM provider" value={key ? `${key.provider} — ${key.name}` : "None selected"} />

        <div className="rounded-xl bg-muted/50 overflow-hidden">
          <button
            type="button"
            onClick={() => integrationCount > 0 && setExpandedIntegrations((v) => !v)}
            className="flex items-center justify-between w-full px-4 py-3 text-left"
          >
            <span className="text-sm text-muted-foreground">Integrations</span>
            <span className="flex items-center gap-2">
              <span className="text-sm font-medium text-foreground">
                {integrationCount > 0 ? `${integrationCount} connected · ${totalActions} actions` : "None"}
              </span>
              {integrationCount > 0 && (
                <HugeiconsIcon
                  icon={ArrowRight01Icon}
                  size={14}
                  className={`text-muted-foreground transition-transform duration-200 ${expandedIntegrations ? "rotate-90" : ""}`}
                />
              )}
            </span>
          </button>

          <AnimatePresence initial={false}>
            {expandedIntegrations && activeIntegrations.length > 0 && (
              <motion.div
                initial={{ height: 0, opacity: 0 }}
                animate={{ height: "auto", opacity: 1 }}
                exit={{ height: 0, opacity: 0 }}
                transition={{ duration: 0.2, ease: "easeInOut" }}
                className="overflow-hidden"
              >
                <div className="border-t border-border px-4 pb-3">
                  {activeIntegrations.map((integration) => {
                    const count = selectedActions[integration.id].size
                    return (
                      <div key={integration.id} className="flex items-center gap-3 py-2.5 first:pt-3">
                        <img
                          src={integration.logo}
                          alt={integration.name}
                          className="h-5 w-5 shrink-0 rounded dark:invert"
                        />
                        <span className="text-sm font-medium text-foreground">{integration.name}</span>
                        <span className="text-xs text-muted-foreground ml-auto font-mono">
                          {count} {count === 1 ? "action" : "actions"}
                        </span>
                      </div>
                    )
                  })}
                </div>
              </motion.div>
            )}
          </AnimatePresence>
        </div>
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onSubmit} className="w-full">
          {mode === "forge" ? (
            <>
              <HugeiconsIcon icon={SparklesIcon} size={16} data-icon="inline-start" />
              Forge agent
            </>
          ) : (
            "Create agent"
          )}
        </Button>
      </div>
    </div>
  )
}

function SummaryRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between rounded-xl bg-muted/50 px-4 py-3">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-sm font-medium text-foreground">{value}</span>
    </div>
  )
}

// --- Main dialog ---

export function CreateAgentDialog() {
  const [step, setStep] = useState<Step>("mode")
  const [mode, setMode] = useState<CreationMode | null>(null)
  const [open, setOpen] = useState(false)
  const [selectedActions, setSelectedActions] = useState<Record<string, Set<string>>>({})
  const [selectedKeyId, setSelectedKeyId] = useState<string | null>(null)
  const [judgeKeyId, setJudgeKeyId] = useState<string | null>(null)
  const [judgeModel, setJudgeModel] = useState<string | null>(null)
  const [selectedMarketplaceAgent, setSelectedMarketplaceAgent] = useState<string | null>(null)
  const direction = useRef<1 | -1>(1)

  const currentSteps = mode === "marketplace" ? marketplaceSteps : mode === "forge" ? forgeSteps : scratchSteps

  function goTo(next: Step) {
    direction.current = currentSteps.indexOf(next) > currentSteps.indexOf(step) ? 1 : -1
    setStep(next)
  }

  function toggleAction(integrationId: string, actionId: string) {
    setSelectedActions((prev) => {
      const current = new Set(prev[integrationId] ?? [])
      if (current.has(actionId)) {
        current.delete(actionId)
      } else {
        current.add(actionId)
      }
      return { ...prev, [integrationId]: current }
    })
  }

  function reset() {
    setStep("mode")
    setMode(null)
    setSelectedActions({})
    setSelectedKeyId(null)
    setJudgeKeyId(null)
    setJudgeModel(null)
    setSelectedMarketplaceAgent(null)
  }

  const variants = {
    enter: (d: number) => ({ x: d > 0 ? 80 : -80, opacity: 0 }),
    center: { x: 0, opacity: 1 },
    exit: (d: number) => ({ x: d > 0 ? -80 : 80, opacity: 0 }),
  }

  const selected = new Set(
    Object.entries(selectedActions).filter(([, s]) => s.size > 0).map(([id]) => id)
  )

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen)
        if (!nextOpen) reset()
      }}
    >
      <DialogTrigger
        render={
          <Button size="default">
            <HugeiconsIcon icon={Add01Icon} size={16} data-icon="inline-start" />
            Create agent
          </Button>
        }
      />
      <DialogContent className="sm:max-w-md h-[780px] overflow-hidden flex flex-col">
        <div className="flex-1 min-h-0 flex flex-col">
          <AnimatePresence mode="wait" custom={direction.current}>
            <motion.div
              key={step}
              custom={direction.current}
              variants={variants}
              initial="enter"
              animate="center"
              exit="exit"
              transition={{ duration: 0.2, ease: "easeInOut" }}
              className="flex-1 flex flex-col min-h-0"
            >
              {step === "mode" && (
                <StepChooseMode
                  onSelect={(m) => {
                    setMode(m)
                    direction.current = 1
                    if (m === "marketplace") {
                      setStep("marketplace-browse")
                    } else {
                      setStep("sandbox")
                    }
                  }}
                />
              )}
              {step === "marketplace-browse" && (
                <StepMarketplaceBrowse
                  onBack={() => goTo("mode")}
                  onSelect={(slug) => {
                    setSelectedMarketplaceAgent(slug)
                    goTo("marketplace-detail")
                  }}
                />
              )}
              {step === "marketplace-detail" && selectedMarketplaceAgent && (
                <StepMarketplaceDetail
                  slug={selectedMarketplaceAgent}
                  onBack={() => goTo("marketplace-browse")}
                  onInstall={() => setOpen(false)}
                />
              )}
              {step === "sandbox" && (
                <StepSandboxType
                  onBack={() => goTo("mode")}
                  onSelect={() => goTo("integrations")}
                />
              )}
              {step === "integrations" && (
                <StepIntegrations
                  selected={selected}
                  selectedActions={selectedActions}
                  onToggleAction={toggleAction}
                  onBack={() => goTo("sandbox")}
                  onNext={() => goTo("llm-key")}
                />
              )}
              {step === "llm-key" && (
                <StepLlmKey
                  selectedKey={selectedKeyId}
                  onSelect={(id) => {
                    setSelectedKeyId(id)
                    goTo("basics")
                  }}
                  onBack={() => goTo("integrations")}
                />
              )}
              {step === "basics" && (
                <StepBasics
                  selectedKeyId={selectedKeyId}
                  onBack={() => goTo("llm-key")}
                  onSubmit={() => {
                    if (mode === "scratch") {
                      goTo("system-prompt")
                    } else {
                      goTo("forge-judge")
                    }
                  }}
                />
              )}
              {step === "forge-judge" && (
                <StepForgeJudge
                  selectedKeyId={selectedKeyId}
                  judgeKeyId={judgeKeyId}
                  onSelectKey={(id) => {
                    setJudgeKeyId(id)
                    setJudgeModel(null)
                  }}
                  judgeModel={judgeModel}
                  onSelectModel={setJudgeModel}
                  onBack={() => goTo("basics")}
                  onNext={() => goTo("summary")}
                  onSkip={() => goTo("summary")}
                />
              )}
              {step === "system-prompt" && (
                <StepSystemPrompt
                  onBack={() => goTo("basics")}
                  onNext={() => goTo("instructions")}
                />
              )}
              {step === "instructions" && (
                <StepInstructions
                  onBack={() => goTo("system-prompt")}
                  onNext={() => goTo("summary")}
                />
              )}
              {step === "summary" && (
                <StepSummary
                  mode={mode!}
                  selectedKeyId={selectedKeyId}
                  selectedActions={selectedActions}
                  onBack={() => {
                    if (mode === "scratch") {
                      goTo("instructions")
                    } else {
                      goTo("forge-judge")
                    }
                  }}
                  onSubmit={() => setOpen(false)}
                />
              )}
            </motion.div>
          </AnimatePresence>
        </div>

        {/* Step indicator */}
        <div className="flex items-center justify-center gap-1.5 pb-2 shrink-0">
          {currentSteps.map((s) => (
            <span
              key={s}
              className={`rounded-full transition-all duration-200 ${
                s === step
                  ? "h-2 w-2 bg-foreground"
                  : "h-1.5 w-1.5 bg-muted-foreground/30"
              }`}
            />
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}
