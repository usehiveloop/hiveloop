import Link from "next/link"
import { Logo } from "@/components/logo"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { HugeiconsIcon } from "@hugeicons/react"
import { Search01Icon, ArrowDown02Icon, Download04Icon, CheckmarkBadge01Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"

const agents = [
  {
    name: "PR Review Agent",
    slug: "pr-review-agent",
    description: "Automatically reviews pull requests, checks for code quality issues, security vulnerabilities, and suggests improvements based on your team's standards.",
    publisher: { name: "Sarah Chen", avatar: "https://i.pravatar.cc/80?u=sarah" },
    installs: 12400,
    integrations: ["GitHub", "Slack", "Linear"],
    verified: true,
  },
  {
    name: "Customer Support Agent",
    slug: "customer-support-agent",
    description: "Handles incoming support tickets by searching your knowledge base, drafting responses, and escalating complex issues to the right team member.",
    publisher: { name: "Alex Rivera", avatar: "https://i.pravatar.cc/80?u=alex" },
    installs: 8900,
    integrations: ["Intercom", "Notion", "Slack"],
    verified: true,
  },
  {
    name: "Incident Responder",
    slug: "incident-responder",
    description: "Monitors your infrastructure alerts, correlates events, creates incident channels, and coordinates response workflows automatically.",
    publisher: { name: "LLMVault", avatar: "https://i.pravatar.cc/80?u=llmvault" },
    installs: 6200,
    integrations: ["Slack", "Linear", "GitHub"],
    verified: true,
  },
  {
    name: "Daily Standup Bot",
    slug: "daily-standup-bot",
    description: "Collects async standup updates from your team, summarizes blockers and progress, and posts a digest to your team channel every morning.",
    publisher: { name: "Maria Santos", avatar: "https://i.pravatar.cc/80?u=maria" },
    installs: 5100,
    integrations: ["Slack", "Linear"],
    verified: false,
  },
  {
    name: "Onboarding Agent",
    slug: "onboarding-agent",
    description: "Guides new hires through your onboarding checklist, provisions accounts, shares relevant docs, and answers common questions about your codebase.",
    publisher: { name: "James Park", avatar: "https://i.pravatar.cc/80?u=james" },
    installs: 3800,
    integrations: ["Notion", "GitHub", "Slack", "Google"],
    verified: false,
  },
  {
    name: "Release Manager",
    slug: "release-manager",
    description: "Tracks your release pipeline, generates changelogs from merged PRs, notifies stakeholders, and manages deployment approvals across environments.",
    publisher: { name: "LLMVault", avatar: "https://i.pravatar.cc/80?u=llmvault2" },
    installs: 4500,
    integrations: ["GitHub", "Slack", "Vercel"],
    verified: true,
  },
]

const allAgents = [
  ...agents,
  {
    name: "Data Pipeline Monitor",
    slug: "data-pipeline-monitor",
    description: "Watches your ETL pipelines for failures, alerts the team, and provides root cause analysis with suggested fixes.",
    publisher: { name: "Emily Zhang", avatar: "https://i.pravatar.cc/80?u=emily" },
    installs: 2100,
    integrations: ["Slack", "GitHub"],
    verified: true,
    category: "devops",
  },
  {
    name: "Meeting Summarizer",
    slug: "meeting-summarizer",
    description: "Joins your calendar meetings, records key decisions and action items, and posts structured summaries to the relevant Notion page.",
    publisher: { name: "Tom Wilson", avatar: "https://i.pravatar.cc/80?u=tom" },
    installs: 7300,
    integrations: ["Google", "Notion", "Slack"],
    verified: true,
    category: "productivity",
  },
  {
    name: "Security Scanner",
    slug: "security-scanner",
    description: "Continuously scans your repositories for dependency vulnerabilities, secret leaks, and misconfigurations, then files issues automatically.",
    publisher: { name: "LLMVault", avatar: "https://i.pravatar.cc/80?u=llmvault3" },
    installs: 3400,
    integrations: ["GitHub", "Linear"],
    verified: true,
    category: "security",
  },
  {
    name: "Content Writer",
    slug: "content-writer",
    description: "Drafts blog posts, social media content, and documentation based on your product updates and team inputs with consistent brand voice.",
    publisher: { name: "Nina Patel", avatar: "https://i.pravatar.cc/80?u=nina" },
    installs: 1800,
    integrations: ["Notion", "Slack"],
    verified: false,
    category: "marketing",
  },
  {
    name: "Bug Triage Agent",
    slug: "bug-triage-agent",
    description: "Classifies incoming bug reports by severity, assigns them to the right team, and links related issues to help your team prioritize faster.",
    publisher: { name: "David Kim", avatar: "https://i.pravatar.cc/80?u=david" },
    installs: 2900,
    integrations: ["Linear", "GitHub", "Slack"],
    verified: true,
    category: "devops",
  },
  {
    name: "Invoice Processor",
    slug: "invoice-processor",
    description: "Extracts data from invoices, reconciles with your accounting system, flags discrepancies, and routes approvals to the finance team.",
    publisher: { name: "Rachel Lee", avatar: "https://i.pravatar.cc/80?u=rachel" },
    installs: 1200,
    integrations: ["Google", "Slack"],
    verified: false,
    category: "finance",
  },
  {
    name: "Compliance Checker",
    slug: "compliance-checker",
    description: "Reviews code changes and documentation against your compliance requirements, generates audit reports, and tracks remediation progress.",
    publisher: { name: "LLMVault", avatar: "https://i.pravatar.cc/80?u=llmvault4" },
    installs: 1900,
    integrations: ["GitHub", "Notion", "Linear"],
    verified: true,
    category: "security",
  },
  {
    name: "Sales Lead Qualifier",
    slug: "sales-lead-qualifier",
    description: "Enriches inbound leads with company data, scores them based on your ICP, and routes qualified prospects to the right sales rep.",
    publisher: { name: "Chris Brown", avatar: "https://i.pravatar.cc/80?u=chris" },
    installs: 3100,
    integrations: ["Intercom", "Slack"],
    verified: false,
    category: "sales",
  },
]

const categories = [
  { label: "All categories", value: "all" },
  { label: "DevOps", value: "devops" },
  { label: "Productivity", value: "productivity" },
  { label: "Security", value: "security" },
  { label: "Marketing", value: "marketing" },
  { label: "Sales", value: "sales" },
  { label: "Finance", value: "finance" },
  { label: "Support", value: "support" },
]

function formatInstalls(n: number) {
  if (n >= 1000) return `${(n / 1000).toFixed(n % 1000 === 0 ? 0 : 1)}k`
  return n.toString()
}

export default function MarketplacePage() {
  return (
    <div className="w-full bg-background flex flex-col relative">
      <nav className="w-full h-16 flex items-center justify-between max-w-424 mx-auto sticky top-0 bg-background z-100 px-4 lg:px-0">
        <Link href="/">
          <Logo className="h-8" />
        </Link>
        <div className="hidden md:flex items-center gap-6 lg:gap-9">
          <Link href="/docs" className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors">Docs</Link>
          <Link href="/pricing" className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors">Pricing</Link>
          <Link href="/marketplace" className="text-sm font-medium text-foreground">Marketplace</Link>
        </div>
        <Link href="/auth">
          <Button variant="outline" size="sm">Sign in</Button>
        </Link>
      </nav>

      <div className="max-w-6xl mx-auto w-full px-4 pt-16 sm:pt-24 pb-12 flex flex-col items-center gap-6">
        <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">Marketplace</p>
        <h1 className="font-heading text-[28px] sm:text-[40px] lg:text-[48px] font-bold text-foreground text-center leading-[1.15] -tracking-[0.5px]">
          Discover production-ready agents
        </h1>
        <p className="text-base sm:text-lg text-muted-foreground text-center max-w-lg">
          Browse community-built agents, MCP tools, and integration templates ready to deploy.
        </p>

        <div className="relative w-full max-w-2xl mt-4">
          <HugeiconsIcon icon={Search01Icon} size={18} className="absolute left-4 top-1/2 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search agents, integrations, tools..."
            className="pl-11 h-12 rounded-full text-base"
          />
        </div>
      </div>

      <div className="max-w-6xl mx-auto w-full px-4 pb-8">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-1">
            <Button variant="secondary" size="sm">Popular</Button>
            <Button variant="ghost" size="sm">Verified</Button>
            <Button variant="ghost" size="sm">Featured</Button>
          </div>
          <Button variant="ghost" size="sm">
            All agents
            <HugeiconsIcon icon={ArrowDown02Icon} size={14} data-icon="inline-end" />
          </Button>
        </div>
      </div>

      <div className="max-w-6xl mx-auto w-full px-4 pb-24">
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {agents.map((agent) => (
            <Link
              href={`/marketplace/agents/${agent.slug}`}
              key={agent.slug}
              className="group flex flex-col gap-4 rounded-2xl border border-border p-5 transition-colors hover:border-primary"
            >
              {/* Stacked integration logos + install count */}
              <div className="flex items-center justify-between">
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <div className="flex items-center cursor-default">
                        {agent.integrations.map((integration, i) => (
                          <div
                            key={integration}
                            className="flex h-7 w-7 items-center justify-center rounded-full border-2 border-background bg-muted text-[9px] font-bold text-muted-foreground"
                            style={{ marginLeft: i > 0 ? "-8px" : 0, zIndex: agent.integrations.length - i }}
                          >
                            {integration[0]}
                          </div>
                        ))}
                      </div>
                    }
                  />
                  <TooltipContent>
                    {agent.integrations.join(", ")}
                  </TooltipContent>
                </Tooltip>
                <div className="flex items-center gap-1 text-xs text-muted-foreground">
                  <HugeiconsIcon icon={Download04Icon} size={12} />
                  {formatInstalls(agent.installs)}
                </div>
              </div>

              {/* Agent name + verified badge */}
              <div className="flex items-center gap-1.5">
                <h3 className="font-heading text-sm font-semibold text-foreground transition-colors">
                  {agent.name}
                </h3>
                {agent.verified && (
                  <HugeiconsIcon icon={CheckmarkBadge01Icon} size={15} className="text-green-500 shrink-0" />
                )}
              </div>

              {/* Description */}
              <p className="text-[13px] leading-relaxed text-muted-foreground line-clamp-2">
                {agent.description}
              </p>

              {/* Publisher */}
              <div className="flex items-center gap-2 mt-auto pt-2 border-t border-border/50">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={agent.publisher.avatar}
                  alt={agent.publisher.name}
                  className="h-5 w-5 rounded-full object-cover"
                />
                <span className="text-xs text-muted-foreground">{agent.publisher.name}</span>
              </div>
            </Link>
          ))}
        </div>
      </div>

      {/* Explore all agents */}
      <div className="max-w-6xl mx-auto w-full px-4 pt-8">
        <div className="flex items-center justify-between">
          <h2 className="font-heading text-lg font-semibold text-foreground">Explore all agents</h2>
          <Select defaultValue="popularity">
            <SelectTrigger size="sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="popularity">Sort by popularity</SelectItem>
              <SelectItem value="date">Sort by date</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <Separator className="mt-4" />
      </div>

      {/* Filter sidebar + agent grid */}
      <div className="max-w-6xl mx-auto w-full px-4 py-8 pb-24">
        <div className="flex gap-8">
          {/* Filters sidebar */}
          <aside className="hidden md:flex w-52 shrink-0 flex-col gap-8 sticky top-20 self-start">
            {/* Agents filter */}
            <div className="flex flex-col gap-3">
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground">Agents</span>
              <div className="flex flex-col gap-1">
                <FilterItem label="All" active />
                <FilterItem label="Verified only" />
              </div>
            </div>

            <Separator />

            {/* Category filter */}
            <div className="flex flex-col gap-3">
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground">Category</span>
              <div className="flex flex-col gap-1">
                {categories.map((cat) => (
                  <FilterItem key={cat.value} label={cat.label} active={cat.value === "all"} />
                ))}
              </div>
            </div>
          </aside>

          {/* Agent grid */}
          <div className="flex-1">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              {allAgents.map((agent) => (
                <Link
                  href={`/marketplace/agents/${agent.slug}`}
                  key={agent.slug}
                  className="group flex flex-col gap-4 rounded-2xl border border-border p-5 transition-colors hover:border-primary"
                >
                  <div className="flex items-center justify-between">
                    <Tooltip>
                      <TooltipTrigger
                        render={
                          <div className="flex items-center cursor-default">
                            {agent.integrations.map((integration, i) => (
                              <div
                                key={integration}
                                className="flex h-7 w-7 items-center justify-center rounded-full border-2 border-background bg-muted text-[9px] font-bold text-muted-foreground"
                                style={{ marginLeft: i > 0 ? "-8px" : 0, zIndex: agent.integrations.length - i }}
                              >
                                {integration[0]}
                              </div>
                            ))}
                          </div>
                        }
                      />
                      <TooltipContent>
                        {agent.integrations.join(", ")}
                      </TooltipContent>
                    </Tooltip>
                    <div className="flex items-center gap-1 text-xs text-muted-foreground">
                      <HugeiconsIcon icon={Download04Icon} size={12} />
                      {formatInstalls(agent.installs)}
                    </div>
                  </div>

                  <div className="flex items-center gap-1.5">
                    <h3 className="font-heading text-sm font-semibold text-foreground transition-colors">
                      {agent.name}
                    </h3>
                    {agent.verified && (
                      <HugeiconsIcon icon={CheckmarkBadge01Icon} size={15} className="text-green-500 shrink-0" />
                    )}
                  </div>

                  <p className="text-[13px] leading-relaxed text-muted-foreground line-clamp-2">
                    {agent.description}
                  </p>

                  <div className="flex items-center gap-2 mt-auto pt-2 border-t border-border/50">
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={agent.publisher.avatar}
                      alt={agent.publisher.name}
                      className="h-5 w-5 rounded-full object-cover"
                    />
                    <span className="text-xs text-muted-foreground">{agent.publisher.name}</span>
                  </div>
                </Link>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function FilterItem({ label, active = false }: { label: string; active?: boolean }) {
  return (
    <button className={`flex items-center justify-between w-full rounded-lg px-2.5 py-1.5 text-sm transition-colors cursor-pointer ${active ? "bg-muted text-foreground" : "text-muted-foreground hover:text-foreground hover:bg-muted/50"}`}>
      {label}
      {active && <HugeiconsIcon icon={Tick02Icon} size={14} className="text-primary" />}
    </button>
  )
}
