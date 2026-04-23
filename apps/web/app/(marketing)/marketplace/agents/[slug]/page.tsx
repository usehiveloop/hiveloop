import Link from "next/link"
import { Button } from "@/components/ui/button"
import { HugeiconsIcon } from "@hugeicons/react"
import { Download04Icon, CheckmarkBadge01Icon, ArrowRight01Icon } from "@hugeicons/core-free-icons"

/**
 * Variant 1: "Hero + Sidebar"
 * Gradient hero with agent icon, two-column layout below (content left, install sidebar right).
 * Matches the marketplace-1 card aesthetic with the gradient header.
 */

const agent = {
  name: "PR Review Agent",
  slug: "pr-review-agent",
  icon: "\u{1F50D}",
  shortDescription: "Automatically reviews pull requests for code quality, security vulnerabilities, and style enforcement.",
  publisher: { name: "Sarah Chen", avatar: "https://i.pravatar.cc/80?u=sarah" },
  installs: 12400,
  integrations: ["GitHub", "Slack", "Linear"],
  verified: true,
  version: "2.1.0",
  lastUpdated: "June 2025",
  category: "DevOps",
  price: "$3.99/mo",
}

const sections = [
  {
    title: "How it works",
    content: [
      "When a pull request is opened or updated, the agent receives a webhook from GitHub and begins its review process.",
      "**Code analysis** — reads the full diff and understands context across your codebase.",
      "**Style enforcement** — checks against your linting rules, naming conventions, and patterns.",
      "**Security scanning** — flags SQL injection, XSS, hardcoded secrets, and more.",
      "**Review comments** — posts inline comments with clear explanations and suggested fixes.",
      "**Summary** — a top-level comment summarizes findings with a pass/fail recommendation.",
    ],
  },
  {
    title: "What you need",
    content: [
      "A GitHub integration connected to your HiveLoop workspace.",
      "A Slack integration (optional) for notifications.",
      "A Linear integration (optional) for auto-creating issues from findings.",
    ],
  },
  {
    title: "Configuration",
    content: [
      "**Severity threshold** — flag only critical issues or include warnings and suggestions.",
      "**Auto-approve** — optionally auto-approve PRs that pass all checks.",
      "**Custom rules** — add your own review rules using natural language.",
      "**Ignore patterns** — skip files matching certain globs (generated code, vendor dirs).",
    ],
  },
]

const relatedAgents = [
  { name: "Security Scanner", slug: "security-scanner", icon: "\u{1F6E1}\uFE0F", installs: 3400 },
  { name: "Bug Triage Agent", slug: "bug-triage-agent", icon: "\u{1F41B}", installs: 2900 },
  { name: "Release Manager", slug: "release-manager", icon: "\u{1F680}", installs: 4500 },
]

function formatInstalls(count: number) {
  if (count >= 1000) return `${(count / 1000).toFixed(count % 1000 === 0 ? 0 : 1)}k`
  return count.toString()
}

export default function DetailVariant1() {
  return (
    <div className="w-full bg-background flex flex-col relative min-h-screen">
      {/* Hero — DevOps category-tinted backdrop */}
      <div className="w-full relative overflow-hidden border-b border-border">
        <div
          aria-hidden="true"
          className="absolute inset-0 pointer-events-none agent-detail-glow-devops"
        />
        <div
          aria-hidden="true"
          className="absolute inset-0 pointer-events-none marketing-grid marketing-grid-lg marketing-grid-mask-left-50"
        />

        <div className="max-w-6xl mx-auto w-full px-4 relative py-10 sm:py-14">
          {/* Breadcrumb */}
          <div className="flex items-center gap-1.5 text-sm text-muted-foreground mb-6">
            <Link href="/marketplace" className="hover:text-foreground transition-colors">Marketplace</Link>
            <HugeiconsIcon icon={ArrowRight01Icon} size={12} />
            <span className="text-foreground">{agent.name}</span>
          </div>

          <div className="flex items-start gap-5">
            <div className="flex items-center justify-center w-16 h-16 sm:w-20 sm:h-20 rounded-2xl bg-background border border-border shadow-sm text-3xl sm:text-4xl shrink-0">
              {agent.icon}
            </div>
            <div className="flex flex-col gap-2">
              <div className="flex items-center gap-2.5">
                <h1 className="font-heading text-[28px] sm:text-[36px] font-bold text-foreground leading-tight -tracking-[0.5px]">
                  {agent.name}
                </h1>
                {agent.verified && <HugeiconsIcon icon={CheckmarkBadge01Icon} size={22} className="text-green-500 shrink-0" />}
              </div>
              <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-2xl">
                {agent.shortDescription}
              </p>
              <div className="flex flex-wrap items-center gap-3 pt-1 text-sm text-muted-foreground">
                <div className="flex items-center gap-2">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src={agent.publisher.avatar} alt={agent.publisher.name} className="h-5 w-5 rounded-full" />
                  <span>{agent.publisher.name}</span>
                </div>
                <span>&middot;</span>
                <div className="flex items-center gap-1">
                  <HugeiconsIcon icon={Download04Icon} size={13} />
                  {formatInstalls(agent.installs)} installs
                </div>
                <span>&middot;</span>
                <span>v{agent.version}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Content + Sidebar */}
      <div className="max-w-6xl mx-auto w-full px-4 py-10 sm:py-14">
        <div className="flex flex-col lg:flex-row gap-10 lg:gap-14">
          {/* Main content */}
          <article className="flex-1 min-w-0">
            {sections.map((section) => (
              <div key={section.title} className="mb-10">
                <h2 className="font-heading text-xl font-semibold text-foreground mb-4">{section.title}</h2>
                <div className="flex flex-col gap-3">
                  {section.content.map((line, index) => (
                    <p
                      key={index}
                      className="text-sm leading-relaxed text-muted-foreground"
                      dangerouslySetInnerHTML={{ __html: line.replace(/\*\*(.+?)\*\*/g, "<strong class='text-foreground font-medium'>$1</strong>") }}
                    />
                  ))}
                </div>
              </div>
            ))}

            {/* Related agents */}
            <div className="mt-12 pt-8 border-t border-border">
              <h2 className="font-heading text-lg font-semibold text-foreground mb-6">Related agents</h2>
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                {relatedAgents.map((related) => (
                  <Link
                    key={related.slug}
                    href={`/marketplace/agents/${related.slug}`}
                    className="group flex items-center gap-3 rounded-xl border border-border p-4 hover:border-primary/40 transition-colors"
                  >
                    <span className="flex items-center justify-center w-10 h-10 rounded-xl bg-muted text-lg">{related.icon}</span>
                    <div className="flex flex-col">
                      <span className="text-sm font-semibold text-foreground group-hover:text-primary transition-colors">{related.name}</span>
                      <span className="text-xs text-muted-foreground">{formatInstalls(related.installs)} installs</span>
                    </div>
                  </Link>
                ))}
              </div>
            </div>
          </article>

          {/* Sidebar */}
          <aside className="lg:w-72 shrink-0">
            <div className="lg:sticky lg:top-24 flex flex-col gap-6">
              {/* Install card */}
              <div className="rounded-2xl border border-border p-6 flex flex-col gap-4">
                <div className="flex items-center justify-between">
                  <span className="font-heading text-2xl font-bold text-foreground">{agent.price}</span>
                  <span className="text-xs text-muted-foreground">per agent</span>
                </div>
                <Button size="lg" className="w-full">Install agent</Button>
                <p className="text-xs text-muted-foreground text-center">
                  Installs to your workspace. Requires a Pro plan.
                </p>
              </div>

              {/* Details */}
              <div className="rounded-2xl border border-border p-6 flex flex-col gap-3">
                <span className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground">Details</span>
                <div className="flex flex-col gap-2.5">
                  {[
                    { label: "Category", value: agent.category },
                    { label: "Version", value: agent.version },
                    { label: "Updated", value: agent.lastUpdated },
                  ].map((detail) => (
                    <div key={detail.label} className="flex items-center justify-between">
                      <span className="text-xs text-muted-foreground">{detail.label}</span>
                      <span className="text-xs font-medium text-foreground">{detail.value}</span>
                    </div>
                  ))}
                </div>
              </div>

              {/* Integrations */}
              <div className="rounded-2xl border border-border p-6 flex flex-col gap-3">
                <span className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground">Integrations</span>
                <div className="flex flex-col gap-2">
                  {agent.integrations.map((integration) => (
                    <div key={integration} className="flex items-center gap-2.5">
                      <div className="flex h-7 w-7 items-center justify-center rounded-full bg-muted text-[10px] font-bold text-muted-foreground">
                        {integration[0]}
                      </div>
                      <span className="text-sm text-foreground">{integration}</span>
                    </div>
                  ))}
                </div>
              </div>

              {/* Publisher */}
              <div className="rounded-2xl border border-border p-6 flex flex-col gap-3">
                <span className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground">Publisher</span>
                <div className="flex items-center gap-3">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src={agent.publisher.avatar} alt={agent.publisher.name} className="w-10 h-10 rounded-full" />
                  <div className="flex flex-col">
                    <span className="text-sm font-medium text-foreground">{agent.publisher.name}</span>
                    <span className="text-xs text-muted-foreground">Agent builder</span>
                  </div>
                </div>
              </div>
            </div>
          </aside>
        </div>
      </div>
    </div>
  )
}
