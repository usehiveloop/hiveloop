import Link from "next/link"
import { Button } from "@/components/ui/button"
import { HugeiconsIcon } from "@hugeicons/react"
import { Download04Icon, CheckmarkBadge01Icon } from "@hugeicons/core-free-icons"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

const marketplaceAgents = [
  {
    name: "PR Review Agent",
    slug: "pr-review-agent",
    description: "Reviews pull requests for code quality, security issues, and suggests improvements based on your standards.",
    publisher: { name: "Sarah Chen", avatar: "https://i.pravatar.cc/80?u=sarah" },
    installs: 12400,
    integrations: ["GitHub", "Slack", "Linear"],
    verified: true,
  },
  {
    name: "Customer Support Agent",
    slug: "customer-support-agent",
    description: "Handles support tickets by searching your knowledge base, drafting responses, and escalating complex issues.",
    publisher: { name: "Alex Rivera", avatar: "https://i.pravatar.cc/80?u=alex" },
    installs: 8900,
    integrations: ["Intercom", "Notion", "Slack"],
    verified: true,
  },
  {
    name: "Incident Responder",
    slug: "incident-responder",
    description: "Monitors infrastructure alerts, correlates events, creates incident channels, and coordinates response workflows.",
    publisher: { name: "HiveLoop", avatar: "https://i.pravatar.cc/80?u=hiveloop" },
    installs: 6200,
    integrations: ["Slack", "Linear", "GitHub"],
    verified: true,
  },
  {
    name: "Meeting Summarizer",
    slug: "meeting-summarizer",
    description: "Joins calendar meetings, records key decisions and action items, and posts structured summaries to Notion.",
    publisher: { name: "Tom Wilson", avatar: "https://i.pravatar.cc/80?u=tom" },
    installs: 7300,
    integrations: ["Google", "Notion", "Slack"],
    verified: true,
  },
  {
    name: "Release Manager",
    slug: "release-manager",
    description: "Tracks your release pipeline, generates changelogs from merged PRs, and manages deployment approvals.",
    publisher: { name: "HiveLoop", avatar: "https://i.pravatar.cc/80?u=hiveloop2" },
    installs: 4500,
    integrations: ["GitHub", "Slack", "Vercel"],
    verified: true,
  },
  {
    name: "Security Scanner",
    slug: "security-scanner",
    description: "Scans repositories for dependency vulnerabilities, secret leaks, and misconfigurations, then files issues.",
    publisher: { name: "HiveLoop", avatar: "https://i.pravatar.cc/80?u=hiveloop3" },
    installs: 3400,
    integrations: ["GitHub", "Linear"],
    verified: true,
  },
]

function formatInstalls(count: number) {
  if (count >= 1000) return `${(count / 1000).toFixed(count % 1000 === 0 ? 0 : 1)}k`
  return count.toString()
}

export function MarketplaceSection() {
  return (
    <section className="w-full px-4 sm:px-6 lg:px-0">
      <div className="w-full max-w-424 mx-auto relative">
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            backgroundImage:
              "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
            backgroundSize: "40px 40px",
            maskImage:
              "radial-gradient(ellipse at center, black, transparent 70%)",
          }}
        />
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            background:
              "radial-gradient(circle at 50% 40%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
          }}
        />

        <div className="relative flex flex-col items-center gap-10 sm:gap-14 pb-20 sm:pb-28 lg:pb-36">
          {/* Section header */}
          <div className="flex flex-col items-center gap-5 sm:gap-6 max-w-3xl text-center px-4">
            <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
              Marketplace
            </p>
            <h2 className="font-heading text-[24px] sm:text-[32px] lg:text-[44px] font-bold text-foreground leading-[1.15] -tracking-[0.5px] sm:-tracking-[1px]">
              Install an agent in seconds.{" "}
              <br className="hidden sm:block" />
              Or build one and get paid.
            </h2>
            <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-2xl">
              Browse pre-built agents from the community — code review,
              support, monitoring, and more. Builders earn revenue on every
              install.
            </p>
          </div>

          {/* Agent grid */}
          <div className="w-full max-w-5xl mx-auto grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 px-4 lg:px-0">
            {marketplaceAgents.map((agent) => (
              <Link
                href={`/marketplace/agents/${agent.slug}`}
                key={agent.slug}
                className="group flex flex-col gap-4 rounded-2xl border border-border bg-background p-5 transition-colors hover:border-primary"
              >
                {/* Stacked integration logos + install count */}
                <div className="flex items-center justify-between">
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <div className="flex items-center cursor-default">
                          {agent.integrations.map((integration, index) => (
                            <div
                              key={integration}
                              className="flex h-7 w-7 items-center justify-center rounded-full border-2 border-background bg-muted text-[9px] font-bold text-muted-foreground"
                              style={{ marginLeft: index > 0 ? "-8px" : 0, zIndex: agent.integrations.length - index }}
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

          {/* CTAs */}
          <div className="flex flex-col sm:flex-row items-center gap-3 sm:gap-4 pt-2">
            <Link href="/marketplace">
              <Button size="lg">Browse the marketplace</Button>
            </Link>
            <Link href="/docs">
              <Button variant="outline" size="lg">
                Start building
              </Button>
            </Link>
          </div>
        </div>
      </div>
    </section>
  )
}
