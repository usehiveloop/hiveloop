import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { HugeiconsIcon } from "@hugeicons/react"
import { Search01Icon, Download04Icon, CheckmarkBadge01Icon } from "@hugeicons/core-free-icons"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

/**
 * Variant 1: "App Store"
 * Featured hero agent, category filters, 3-col grid with enhanced cards.
 */

const featuredAgent = {
  name: "PR Review Agent",
  slug: "pr-review-agent",
  icon: "\u{1F50D}",
  description: "Automatically reviews pull requests, checks for code quality issues, security vulnerabilities, and suggests improvements based on your team's standards.",
  publisher: { name: "Sarah Chen", avatar: "https://i.pravatar.cc/80?u=sarah" },
  installs: 12400,
  integrations: ["GitHub", "Slack", "Linear"],
  verified: true,
  highlight: "Most installed agent on the marketplace",
}

const allAgents = [
  { name: "Customer Support Agent", slug: "customer-support-agent", icon: "\u{1F4AC}", description: "Handles incoming support tickets by searching your knowledge base, drafting responses, and escalating complex issues to the right team member.", publisher: { name: "Alex Rivera", avatar: "https://i.pravatar.cc/80?u=alex" }, installs: 8900, integrations: ["Intercom", "Notion", "Slack"], verified: true, category: "Support" },
  { name: "Meeting Summarizer", slug: "meeting-summarizer", icon: "\u{1F4DD}", description: "Joins your calendar meetings, records key decisions and action items, and posts structured summaries to the relevant Notion page.", publisher: { name: "Tom Wilson", avatar: "https://i.pravatar.cc/80?u=tom" }, installs: 7300, integrations: ["Google", "Notion", "Slack"], verified: true, category: "Productivity" },
  { name: "Incident Responder", slug: "incident-responder", icon: "\u{1F6A8}", description: "Monitors your infrastructure alerts, correlates events, creates incident channels, and coordinates response workflows automatically.", publisher: { name: "HiveLoop", avatar: "https://i.pravatar.cc/80?u=hiveloop" }, installs: 6200, integrations: ["Slack", "Linear", "GitHub"], verified: true, category: "DevOps" },
  { name: "Daily Standup Bot", slug: "daily-standup-bot", icon: "\u2615", description: "Collects async standup updates from your team, summarizes blockers and progress, and posts a digest to your team channel every morning.", publisher: { name: "Maria Santos", avatar: "https://i.pravatar.cc/80?u=maria" }, installs: 5100, integrations: ["Slack", "Linear"], verified: false, category: "Productivity" },
  { name: "Release Manager", slug: "release-manager", icon: "\u{1F680}", description: "Tracks your release pipeline, generates changelogs from merged PRs, notifies stakeholders, and manages deployment approvals across environments.", publisher: { name: "HiveLoop", avatar: "https://i.pravatar.cc/80?u=hiveloop2" }, installs: 4500, integrations: ["GitHub", "Slack", "Vercel"], verified: true, category: "DevOps" },
  { name: "Onboarding Agent", slug: "onboarding-agent", icon: "\u{1F44B}", description: "Guides new hires through your onboarding checklist, provisions accounts, shares relevant docs, and answers common questions about your codebase.", publisher: { name: "James Park", avatar: "https://i.pravatar.cc/80?u=james" }, installs: 3800, integrations: ["Notion", "GitHub", "Slack", "Google"], verified: false, category: "Productivity" },
  { name: "Security Scanner", slug: "security-scanner", icon: "\u{1F6E1}\uFE0F", description: "Continuously scans your repositories for dependency vulnerabilities, secret leaks, and misconfigurations, then files issues automatically.", publisher: { name: "HiveLoop", avatar: "https://i.pravatar.cc/80?u=hiveloop3" }, installs: 3400, integrations: ["GitHub", "Linear"], verified: true, category: "Security" },
  { name: "Sales Lead Qualifier", slug: "sales-lead-qualifier", icon: "\u{1F3AF}", description: "Enriches inbound leads with company data, scores them based on your ICP, and routes qualified prospects to the right sales rep.", publisher: { name: "Chris Brown", avatar: "https://i.pravatar.cc/80?u=chris" }, installs: 3100, integrations: ["Intercom", "Slack"], verified: false, category: "Sales" },
  { name: "Bug Triage Agent", slug: "bug-triage-agent", icon: "\u{1F41B}", description: "Classifies incoming bug reports by severity, assigns them to the right team, and links related issues to help your team prioritize faster.", publisher: { name: "David Kim", avatar: "https://i.pravatar.cc/80?u=david" }, installs: 2900, integrations: ["Linear", "GitHub", "Slack"], verified: true, category: "DevOps" },
  { name: "Data Pipeline Monitor", slug: "data-pipeline-monitor", icon: "\u{1F4CA}", description: "Watches your ETL pipelines for failures, alerts the team, and provides root cause analysis with suggested fixes.", publisher: { name: "Emily Zhang", avatar: "https://i.pravatar.cc/80?u=emily" }, installs: 2100, integrations: ["Slack", "GitHub"], verified: true, category: "DevOps" },
  { name: "Compliance Checker", slug: "compliance-checker", icon: "\u2705", description: "Reviews code changes and documentation against your compliance requirements, generates audit reports, and tracks remediation progress.", publisher: { name: "HiveLoop", avatar: "https://i.pravatar.cc/80?u=hiveloop4" }, installs: 1900, integrations: ["GitHub", "Notion", "Linear"], verified: true, category: "Security" },
  { name: "Content Writer", slug: "content-writer", icon: "\u270D\uFE0F", description: "Drafts blog posts, social media content, and documentation based on your product updates and team inputs with consistent brand voice.", publisher: { name: "Nina Patel", avatar: "https://i.pravatar.cc/80?u=nina" }, installs: 1800, integrations: ["Notion", "Slack"], verified: false, category: "Marketing" },
]

const categories = ["All", "DevOps", "Productivity", "Security", "Support", "Sales", "Marketing"]

function formatInstalls(count: number) {
  if (count >= 1000) return `${(count / 1000).toFixed(count % 1000 === 0 ? 0 : 1)}k`
  return count.toString()
}

export default function MarketplaceAppStore() {
  return (
    <div className="w-full bg-background flex flex-col relative min-h-screen">
      {/* Hero */}
      <div className="max-w-6xl mx-auto w-full px-4 pt-12 sm:pt-20 pb-8">
        <div className="flex flex-col gap-2 mb-10">
          <p className="font-mono text-mini font-medium uppercase tracking-medium text-primary">Marketplace</p>
          <h1 className="font-heading text-[28px] sm:text-[40px] lg:text-5xl font-bold text-foreground leading-[1.15] -tracking-micro">
            Discover production-ready agents
          </h1>
          <p className="text-base sm:text-lg text-muted-foreground max-w-lg">
            Install community-built agents in one click. Or build your own and earn revenue.
          </p>
        </div>

        {/* Search */}
        <div className="relative w-full max-w-2xl">
          <HugeiconsIcon icon={Search01Icon} size={18} className="absolute left-4 top-1/2 -translate-y-1/2 text-muted-foreground" />
          <Input placeholder="Search agents, integrations, tools..." className="pl-11 h-12 rounded-full text-base" />
        </div>
      </div>

      {/* Featured agent */}
      <div className="max-w-6xl mx-auto w-full px-4 pb-12">
        <Link href={`/marketplace/agents/${featuredAgent.slug}`} className="group block">
          <div className="relative rounded-3xl border border-border overflow-hidden hover:border-primary/40 transition-colors">
            <div
              className="absolute inset-0 pointer-events-none"
              style={{
                background: "radial-gradient(ellipse at 80% 50%, color-mix(in oklch, var(--primary) 10%, transparent) 0%, transparent 60%)",
              }}
            />
            <div
              className="absolute inset-0 pointer-events-none"
              style={{
                backgroundImage: "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
                backgroundSize: "60px 60px",
                maskImage: "radial-gradient(ellipse at 80% 50%, black 10%, transparent 50%)",
              }}
            />
            <div className="relative p-8 sm:p-10 lg:p-14 flex flex-col gap-4 max-w-2xl">
              <div className="flex items-center justify-center w-14 h-14 rounded-2xl bg-background border border-border shadow-sm text-2xl mb-1">
                {featuredAgent.icon}
              </div>
              <span className="font-mono text-2xs font-medium uppercase tracking-wider text-primary bg-primary/10 px-2.5 py-1 rounded-full w-fit">
                {featuredAgent.highlight}
              </span>
              <h2 className="font-heading text-2xl sm:text-3xl lg:text-4xl font-bold text-foreground group-hover:text-primary transition-colors -tracking-micro">
                {featuredAgent.name}
              </h2>
              <p className="text-base sm:text-lg text-muted-foreground leading-relaxed">
                {featuredAgent.description}
              </p>
              <div className="flex items-center gap-4 pt-2">
                <div className="flex items-center gap-2">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src={featuredAgent.publisher.avatar} alt={featuredAgent.publisher.name} className="w-6 h-6 rounded-full" />
                  <span className="text-sm text-muted-foreground">{featuredAgent.publisher.name}</span>
                </div>
                <span className="text-sm text-muted-foreground">{formatInstalls(featuredAgent.installs)} installs</span>
                {featuredAgent.verified && (
                  <HugeiconsIcon icon={CheckmarkBadge01Icon} size={16} className="text-green-500" />
                )}
              </div>
            </div>
          </div>
        </Link>
      </div>

      {/* Popular agents */}
      <div className="max-w-6xl mx-auto w-full px-4 pb-14">
        <div className="flex flex-col gap-2 mb-8">
          <h2 className="font-heading text-xl sm:text-2xl font-bold text-foreground">
            Popular agents
          </h2>
          <p className="text-sm text-muted-foreground">
            The most installed agents on the platform — trusted by thousands of teams.
          </p>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-5">
          {allAgents.slice(0, 3).map((agent) => (
            <AgentCard key={agent.slug} agent={agent} />
          ))}
        </div>
      </div>

      {/* Featured agents */}
      <div className="max-w-6xl mx-auto w-full px-4 pb-14">
        <div className="flex flex-col gap-2 mb-8">
          <h2 className="font-heading text-xl sm:text-2xl font-bold text-foreground">
            Featured by HiveLoop
          </h2>
          <p className="text-sm text-muted-foreground">
            Hand-picked by our team for quality, reliability, and creative use of the platform.
          </p>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-5">
          {allAgents.filter((agent) => agent.verified).slice(0, 3).map((agent) => (
            <AgentCard key={agent.slug} agent={agent} />
          ))}
        </div>
      </div>

      {/* Category filter + grid */}
      <div className="max-w-6xl mx-auto w-full px-4 pb-24">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
          <div className="flex items-center gap-1.5 flex-wrap">
            {categories.map((category, index) => (
              <button
                key={category}
                className={`px-3.5 py-1.5 rounded-full text-sm font-medium transition-colors cursor-pointer ${
                  index === 0
                    ? "bg-foreground text-background"
                    : "bg-muted text-muted-foreground hover:text-foreground hover:bg-muted/80"
                }`}
              >
                {category}
              </button>
            ))}
          </div>
          <span className="text-sm text-muted-foreground shrink-0">{allAgents.length} agents</span>
        </div>

        {/* Agent grid — 3 columns */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-5">
          {allAgents.map((agent) => (
            <AgentCard key={agent.slug} agent={agent} />
          ))}
        </div>
      </div>

      {/* CTA */}
      <div className="max-w-6xl mx-auto w-full px-4 pb-24">
        <div className="flex flex-col items-center gap-6 rounded-3xl border border-border p-12 text-center relative overflow-hidden">
          <div className="absolute inset-0 pointer-events-none" style={{ background: "radial-gradient(circle at 50% 100%, color-mix(in oklch, var(--primary) 6%, transparent) 0%, transparent 60%)" }} />
          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground relative">Build an agent, earn revenue</h2>
          <p className="text-muted-foreground max-w-md relative">Publish your agent to the marketplace and earn 50% of every install. Build once, earn forever.</p>
          <div className="flex gap-3 relative">
            <Link href="/docs"><Button size="lg">Start building</Button></Link>
            <Link href="/docs/marketplace"><Button variant="outline" size="lg">Learn more</Button></Link>
          </div>
        </div>
      </div>
    </div>
  )
}

interface AgentCardProps {
  agent: {
    name: string
    slug: string
    icon: string
    description: string
    publisher: { name: string; avatar: string }
    installs: number
    integrations: string[]
    verified: boolean
    category: string
  }
}

const categoryGradients: Record<string, string> = {
  DevOps: "radial-gradient(circle at 70% 50%, oklch(0.55 0.15 250 / 14%) 0%, transparent 70%)",
  Security: "radial-gradient(circle at 70% 50%, oklch(0.55 0.2 27 / 14%) 0%, transparent 70%)",
  Support: "radial-gradient(circle at 70% 50%, oklch(0.55 0.18 145 / 14%) 0%, transparent 70%)",
  Productivity: "radial-gradient(circle at 70% 50%, oklch(0.55 0.18 290 / 14%) 0%, transparent 70%)",
  Sales: "radial-gradient(circle at 70% 50%, oklch(0.6 0.16 70 / 14%) 0%, transparent 70%)",
  Marketing: "radial-gradient(circle at 70% 50%, oklch(0.55 0.2 340 / 14%) 0%, transparent 70%)",
  Finance: "radial-gradient(circle at 70% 50%, oklch(0.55 0.15 180 / 14%) 0%, transparent 70%)",
}

const defaultGradient = "radial-gradient(circle at 70% 50%, color-mix(in oklch, var(--primary) 12%, transparent) 0%, transparent 70%)"

function AgentCard({ agent }: AgentCardProps) {
  return (
    <Link
      href={`/marketplace/agents/${agent.slug}`}
      className="group flex flex-col rounded-2xl border border-border overflow-hidden hover:border-primary/40 hover:shadow-[0_0_40px_-15px_color-mix(in_oklch,var(--primary)_10%,transparent)] transition-all"
    >
      {/* Card header with gradient */}
      <div className="relative h-28 bg-muted/30 overflow-hidden border-b border-border/50">
        <div
          className="absolute inset-0"
          style={{ background: categoryGradients[agent.category] || defaultGradient }}
        />
        <div
          className="absolute inset-0"
          style={{
            backgroundImage: "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
            backgroundSize: "24px 24px",
            maskImage: "radial-gradient(ellipse at 70% 50%, black 10%, transparent 60%)",
          }}
        />
        {/* Agent icon */}
        <div className="absolute top-4 left-5 flex items-center justify-center w-11 h-11 rounded-xl bg-background border border-border shadow-sm text-xl">
          {agent.icon}
        </div>
        <div className="absolute bottom-3 left-5">
          <Tooltip>
            <TooltipTrigger
              render={
                <div className="flex items-center cursor-default">
                  {agent.integrations.slice(0, 4).map((integration, index) => (
                    <div
                      key={integration}
                      className="flex h-8 w-8 items-center justify-center rounded-full border-2 border-background bg-background text-2xs font-bold text-muted-foreground shadow-sm"
                      style={{ marginLeft: index > 0 ? "-6px" : 0, zIndex: agent.integrations.length - index }}
                    >
                      {integration[0]}
                    </div>
                  ))}
                </div>
              }
            />
            <TooltipContent>{agent.integrations.join(", ")}</TooltipContent>
          </Tooltip>
        </div>
        <div className="absolute bottom-3 right-5 flex items-center gap-1 text-xs text-muted-foreground bg-background/80 backdrop-blur-sm px-2.5 py-1 rounded-full border border-border/50">
          <HugeiconsIcon icon={Download04Icon} size={11} />
          {formatInstalls(agent.installs)}
        </div>
      </div>

      {/* Card body */}
      <div className="flex flex-col flex-1 p-5 gap-3">
        <div className="flex items-center gap-2">
          <h3 className="font-heading text-base font-semibold text-foreground group-hover:text-primary transition-colors">
            {agent.name}
          </h3>
          {agent.verified && (
            <HugeiconsIcon icon={CheckmarkBadge01Icon} size={15} className="text-green-500 shrink-0" />
          )}
        </div>
        <p className="text-sm-alt text-muted-foreground leading-relaxed line-clamp-2 flex-1">
          {agent.description}
        </p>
        <div className="flex items-center justify-between pt-3 border-t border-border/50 mt-auto">
          <div className="flex items-center gap-2">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={agent.publisher.avatar} alt={agent.publisher.name} className="h-5 w-5 rounded-full object-cover" />
            <span className="text-xs text-muted-foreground">{agent.publisher.name}</span>
          </div>
          <span className="text-2xs font-mono font-medium uppercase tracking-wider text-muted-foreground">
            {agent.category}
          </span>
        </div>
      </div>
    </Link>
  )
}
