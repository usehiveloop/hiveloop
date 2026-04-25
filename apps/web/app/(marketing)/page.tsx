import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { HugeiconsIcon } from "@hugeicons/react"
import { Download04Icon, CheckmarkBadge01Icon, ArrowRight01Icon, CpuIcon } from "@hugeicons/core-free-icons"
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

export default function Home() {
  return (
    <div className="w-full bg-background flex flex-col relative">
      <div className="flex flex-1 px-4 sm:px-6 lg:px-0">
        <div className="w-full max-w-424 lg:min-h-425 mx-auto relative overflow-hidden">
          {/* Grid background */}
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              backgroundImage:
                "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
              backgroundSize: "40px 40px",
              maskImage:
                "radial-gradient(ellipse at center, black, transparent 80%)",
            }}
          />
          {/* Hero glow */}
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background:
                "radial-gradient(circle at 50% 40%, color-mix(in oklch, var(--primary) 12%, transparent) 0%, transparent 70%)",
            }}
          />
          <div className="relative flex flex-col items-center gap-6 sm:gap-8 pt-12 sm:pt-16 lg:pt-25 px-4 sm:px-8 lg:px-0">
            <div className="flex items-center gap-2 px-4 py-2 bg-muted border border-border rounded-full">
              <span className="w-1.5 h-1.5 rounded-full bg-green-500" />
              <span className="font-mono text-[11px] font-medium uppercase tracking-[0.5px] text-muted-foreground">
                introducing the marketplace with revenue sharing
              </span>
            </div>
            <h1 className="font-heading text-[28px] sm:text-[40px] lg:text-[56px] font-bold text-foreground text-center leading-[1.15] -tracking-[0.5px] sm:-tracking-[1px]">
              Run production-grade <br className="hidden sm:block" /> agents for your team
            </h1>
            <p className="text-base sm:text-lg lg:text-xl text-muted-foreground text-center leading-relaxed max-w-160">
              The complete platform for building, running and monitoring real world agents
              with memory, observability and access control.
            </p>
            <div className="flex flex-col sm:flex-row gap-2.5 pt-2 w-full sm:w-auto">
              <Input
                type="email"
                placeholder="Enter your email"
                className="h-10 sm:h-12 sm:w-72 rounded-full text-sm sm:text-base px-5"
              />
              <Button size="default" className="sm:hidden rounded-full h-10">
                Join the waitlist
              </Button>
              <Button size="lg" className="hidden sm:inline-flex rounded-full h-12">
                Join the waitlist
              </Button>
            </div>
          </div>

          <div className="px-4 lg:px-0">
            <div className="relative z-10 w-full max-w-5xl bg-black dark:bg-card min-h-60 sm:min-h-80 lg:min-h-180 mt-8 sm:mt-12 lg:mt-16 lg:mx-auto border border-border rounded-4xl shadow-[0_0_60px_-20px_color-mix(in_oklch,var(--primary)_12%,transparent),0_0_20px_-10px_color-mix(in_oklch,var(--primary)_8%,transparent)] flex items-center justify-center">
              <div className="relative flex items-center justify-center">
                {/* Pulse rings */}
                <span className="absolute w-12 h-12 lg:w-32 lg:h-32 rounded-full border border-foreground/20 lg:border-2 animate-[ping_2.5s_ease-out_infinite]" />
                <span className="absolute w-12 h-12 lg:w-32 lg:h-32 rounded-full border border-foreground/12 lg:border-2 animate-[ping_2.5s_ease-out_0.8s_infinite]" />
                <span className="absolute w-12 h-12 lg:w-32 lg:h-32 rounded-full bg-foreground/5 animate-[ping_2.5s_ease-out_0.4s_infinite]" />
                <svg
                  className="relative w-8 h-8 lg:w-20 lg:h-20 text-muted-foreground"
                  viewBox="0 0 24 24"
                  fill="none"
                  xmlns="http://www.w3.org/2000/svg"
                >
                  <title>play</title>
                  <path
                    fillRule="evenodd"
                    clipRule="evenodd"
                    d="M7.23832 3.04445C5.65196 2.1818 3.75 3.31957 3.75 5.03299L3.75 18.9672C3.75 20.6806 5.65196 21.8184 7.23832 20.9557L20.0503 13.9886C21.6499 13.1188 21.6499 10.8814 20.0503 10.0116L7.23832 3.04445ZM2.25 5.03299C2.25 2.12798 5.41674 0.346438 7.95491 1.72669L20.7669 8.6938C23.411 10.1317 23.411 13.8685 20.7669 15.3064L7.95491 22.2735C5.41674 23.6537 2.25 21.8722 2.25 18.9672L2.25 5.03299Z"
                    fill="currentColor"
                  />
                </svg>
              </div>
            </div>
          </div>

          {/* Trusted by */}
          <div className="relative z-10 flex flex-col items-center gap-8 py-20 sm:py-28 px-4">
            <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
              Trusted by companies of all sizes
            </p>
            <div className="flex flex-wrap items-center justify-center gap-x-12 gap-y-6 sm:gap-x-16 opacity-40">
              <span className="text-lg sm:text-xl font-semibold text-muted-foreground tracking-tight">
                Vercel
              </span>
              <span className="text-lg sm:text-xl font-semibold text-muted-foreground tracking-tight">
                Linear
              </span>
              <span className="text-lg sm:text-xl font-semibold text-muted-foreground tracking-tight">
                Stripe
              </span>
              <span className="text-lg sm:text-xl font-semibold text-muted-foreground tracking-tight">
                Notion
              </span>
              <span className="text-lg sm:text-xl font-semibold text-muted-foreground tracking-tight">
                Raycast
              </span>
              <span className="text-lg sm:text-xl font-semibold text-muted-foreground tracking-tight">
                Supabase
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Value proposition: Replace subscriptions */}
      <section className="w-full px-4 sm:px-6 lg:px-0">
        <div className="w-full max-w-424 mx-auto relative">
          {/* Grid background */}
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
                "radial-gradient(circle at 50% 50%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
            }}
          />

          <div className="relative flex flex-col items-center gap-10 sm:gap-14 pb-20 sm:pb-28 lg:pb-36">
            {/* Section header */}
            <div className="flex flex-col items-center gap-5 sm:gap-6 max-w-3xl text-center px-4">
              <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
                Why Hiveloop
              </p>
              <h2 className="font-heading text-[24px] sm:text-[32px] lg:text-[44px] font-bold text-foreground leading-[1.15] -tracking-[0.5px] sm:-tracking-[1px]">
                Stop paying for 10 subscriptions.{" "}
                <br className="hidden sm:block" />
                Build your own agents instead.
              </h2>
              <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-2xl">
                Every AI tool is another $20–50/month. Hiveloop gives you the
                building blocks to run your own — for a fraction of the cost.
              </p>
            </div>

            {/* Comparison card */}
            <div className="w-full max-w-5xl mx-auto rounded-4xl border border-border ring-1 ring-foreground/5 shadow-[0_0_60px_-20px_color-mix(in_oklch,var(--primary)_12%,transparent),0_0_20px_-10px_color-mix(in_oklch,var(--primary)_8%,transparent)] overflow-hidden">
              <div className="grid grid-cols-1 lg:grid-cols-2">
                {/* Left: Subscriptions */}
                <div className="p-6 sm:p-8 lg:p-10 bg-muted/50 dark:bg-card/50 border-b lg:border-b-0 lg:border-r border-border">
                  <div className="flex items-center gap-2 mb-6">
                    <span className="w-2 h-2 rounded-full bg-destructive" />
                    <span className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-destructive">
                      What you&apos;re paying today
                    </span>
                  </div>

                  <div className="flex flex-col gap-3">
                    {[
                      { name: "CodeRabbit", desc: "Code review", price: "$30" },
                      { name: "Cursor", desc: "AI coding", price: "$20" },
                      { name: "Lovable", desc: "UI generation", price: "$25" },
                      { name: "Devin", desc: "Autonomous dev", price: "$500" },
                      { name: "Jasper", desc: "Content writing", price: "$49" },
                      {
                        name: "Intercom Fin",
                        desc: "Support agent",
                        price: "$99",
                      },
                    ].map((tool) => (
                      <div
                        key={tool.name}
                        className="flex items-center justify-between py-3 px-4 rounded-2xl bg-background/60 dark:bg-background/30 border border-border/60"
                      >
                        <div className="flex flex-col">
                          <span className="text-sm font-semibold text-foreground">
                            {tool.name}
                          </span>
                          <span className="text-xs text-muted-foreground">
                            {tool.desc}
                          </span>
                        </div>
                        <span className="text-sm font-mono text-muted-foreground line-through decoration-destructive/60">
                          {tool.price}/mo
                        </span>
                      </div>
                    ))}
                  </div>

                  <div className="mt-6 pt-4 border-t border-border/60 flex flex-col gap-3">
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-medium text-muted-foreground">
                        Monthly total
                      </span>
                      <span className="text-2xl font-heading font-bold text-destructive -tracking-[0.5px]">
                        $723/mo
                      </span>
                    </div>
                    <div className="flex items-center justify-between">
                      <span className="text-xs text-muted-foreground">
                        6 separate bills. No shared context.
                      </span>
                      <span className="text-xs text-muted-foreground">
                        and growing...
                      </span>
                    </div>
                  </div>

                  <div className="mt-5 flex flex-col gap-2.5">
                    {[
                      "Locked into each vendor's model",
                      "No control over prompts or behavior",
                      "Separate billing for every tool",
                      "Can't customize or extend features",
                    ].map((item) => (
                      <div key={item} className="flex items-center gap-2.5">
                        <svg className="w-4 h-4 shrink-0 text-destructive" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                          <path d="M18 6L6 18M6 6l12 12" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                        </svg>
                        <span className="text-sm text-muted-foreground">{item}</span>
                      </div>
                    ))}
                  </div>
                </div>

                {/* Right: Hiveloop agents */}
                <div className="p-6 sm:p-8 lg:p-10 bg-background dark:bg-[oklch(0.14_0.01_55)]">
                  <div className="flex items-center gap-2 mb-6">
                    <span className="w-2 h-2 rounded-full bg-green-500" />
                    <span className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-foreground">
                      What you build on Hiveloop
                    </span>
                  </div>

                  <div className="flex flex-col gap-3">
                    {[
                      {
                        name: "code-reviewer",
                        desc: "Reviews PRs on every push",
                      },
                      {
                        name: "ui-builder",
                        desc: "Generates components from designs",
                      },
                      {
                        name: "content-writer",
                        desc: "Drafts blog posts from outlines",
                      },
                      {
                        name: "support-agent",
                        desc: "Answers tickets from your docs",
                      },
                      {
                        name: "code-assistant",
                        desc: "Helps across the codebase",
                      },
                      {
                        name: "deploy-monitor",
                        desc: "Watches deploys, alerts on failure",
                      },
                    ].map((agent) => (
                      <div
                        key={agent.name}
                        className="flex items-center justify-between py-3 px-4 rounded-2xl bg-muted/40 dark:bg-white/[0.04] border border-border/60"
                      >
                        <div className="flex items-center gap-3">
                          <span className="w-1.5 h-1.5 rounded-full bg-green-500" />
                          <div className="flex flex-col">
                            <span className="text-sm font-semibold font-mono text-foreground">
                              {agent.name}
                            </span>
                            <span className="text-xs text-muted-foreground">
                              {agent.desc}
                            </span>
                          </div>
                        </div>
                        <span className="text-xs font-mono text-green-600 dark:text-green-400 bg-green-500/10 px-2 py-0.5 rounded-full">
                          running
                        </span>
                      </div>
                    ))}
                  </div>

                  <div className="mt-6 pt-4 border-t border-border/60 flex flex-col gap-3">
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-medium text-muted-foreground">
                        6 agents x $4.99/mo
                      </span>
                      <span className="text-2xl font-heading font-bold text-foreground -tracking-[0.5px]">
                        $29.94/mo
                      </span>
                    </div>
                    <div className="flex items-center justify-between">
                      <span className="text-xs text-muted-foreground">
                        Same capabilities. You own the agents.
                      </span>
                      <span className="text-xs font-mono text-green-600 dark:text-green-400">
                        saving $693/mo
                      </span>
                    </div>
                  </div>

                  <div className="mt-5 flex flex-col gap-2.5">
                    {[
                      "Bring your own API keys",
                      "Pick any model — GPT, Claude, Gemini, open-source",
                      "Install agents from the marketplace",
                      "Full control over prompts and behavior",
                    ].map((item) => (
                      <div key={item} className="flex items-center gap-2.5">
                        <svg className="w-4 h-4 shrink-0 text-green-600 dark:text-green-400" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                          <path d="M20 6L9 17l-5-5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                        </svg>
                        <span className="text-sm text-foreground">{item}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </div>

            {/* Stats */}
            <div className="grid grid-cols-3 gap-6 sm:gap-12 lg:gap-20 pt-4 sm:pt-8 px-4">
              {[
                {
                  value: "10+",
                  label: "subscriptions replaced",
                },
                {
                  value: "90%",
                  label: "cost savings vs. individual tools",
                },
                {
                  value: "Minutes",
                  label: "to deploy a new agent",
                },
              ].map((stat) => (
                <div
                  key={stat.value}
                  className="flex flex-col items-center text-center gap-1.5"
                >
                  <span className="font-heading text-2xl sm:text-3xl lg:text-4xl font-bold text-foreground -tracking-[0.5px]">
                    {stat.value}
                  </span>
                  <span className="text-xs sm:text-sm text-muted-foreground leading-snug max-w-32">
                    {stat.label}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </section>

      {/* Marketplace */}
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

      {/* Agent Forger */}
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
                "radial-gradient(circle at 50% 30%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
            }}
          />

          <div className="relative flex flex-col items-center gap-10 sm:gap-14 pb-20 sm:pb-28 lg:pb-36">
            {/* Section header */}
            <div className="flex flex-col items-center gap-5 sm:gap-6 max-w-3xl text-center px-4">
              <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
                The Agent Forger
              </p>
              <h2 className="font-heading text-[24px] sm:text-[32px] lg:text-[44px] font-bold text-foreground leading-[1.15] -tracking-[0.5px] sm:-tracking-[1px]">
                From zero to a running agent{" "}
                <br className="hidden sm:block" />
                in under five minutes.
              </h2>
              <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-2xl">
                Build from scratch, let AI forge one for you, or install from
                the marketplace. Three paths, same result — a production-ready
                agent.
              </p>
            </div>

            {/* Three creation modes */}
            <div className="w-full max-w-4xl mx-auto grid grid-cols-1 sm:grid-cols-3 gap-4 px-4 lg:px-0">
              {[
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Create from scratch",
                  description:
                    "Write your own system prompt, pick a model, connect integrations, and deploy.",
                },
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09zM18.259 8.715L18 9.75l-.259-1.035a3.375 3.375 0 00-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 002.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 002.455 2.456L21.75 6l-1.036.259a3.375 3.375 0 00-2.455 2.456zM16.894 20.567L16.5 21.75l-.394-1.183a2.25 2.25 0 00-1.423-1.423L13.5 18.75l1.183-.394a2.25 2.25 0 001.423-1.423l.394-1.183.394 1.183a2.25 2.25 0 001.423 1.423l1.183.394-1.183.394a2.25 2.25 0 00-1.423 1.423z" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Forge with AI",
                  description:
                    "Describe what you need. AI generates an optimized agent with the right prompt and config.",
                },
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M13.5 21v-7.5a.75.75 0 01.75-.75h3a.75.75 0 01.75.75V21m-4.5 0H2.36m11.14 0H18m0 0h3.64m-1.39 0V9.349m-16.5 11.65V9.35m0 0a3.001 3.001 0 003.75-.615A2.993 2.993 0 009.75 9.75c.896 0 1.7-.393 2.25-1.016a2.993 2.993 0 002.25 1.016c.896 0 1.7-.393 2.25-1.016A3.001 3.001 0 0021 9.349m-18 0a2.999 2.999 0 00.97-1.599L5.49 3h13.02l1.52 4.75A2.999 2.999 0 0021 9.349" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Install from marketplace",
                  description:
                    "Browse community agents. One click to install, connect your keys, and run.",
                },
              ].map((mode) => (
                <div
                  key={mode.title}
                  className="flex flex-col gap-4 rounded-2xl border border-border bg-background p-6 transition-colors"
                >
                  <div className="flex items-center justify-center w-10 h-10 rounded-xl bg-muted text-foreground">
                    {mode.icon}
                  </div>
                  <h3 className="font-heading text-base font-semibold text-foreground">
                    {mode.title}
                  </h3>
                  <p className="text-sm text-muted-foreground leading-relaxed">
                    {mode.description}
                  </p>
                </div>
              ))}
            </div>

            {/* Steps preview */}
            <div className="w-full max-w-5xl mx-auto rounded-4xl border border-border ring-1 ring-foreground/5 shadow-[0_0_60px_-20px_color-mix(in_oklch,var(--primary)_12%,transparent),0_0_20px_-10px_color-mix(in_oklch,var(--primary)_8%,transparent)] overflow-hidden bg-background">
              <div className="p-6 sm:p-8 lg:p-10">
                <div className="flex items-center gap-2 mb-8">
                  <span className="w-2 h-2 rounded-full bg-primary" />
                  <span className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-foreground">
                    How it works
                  </span>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-6 lg:gap-8">
                  {[
                    {
                      step: "01",
                      title: "Pick your model",
                      description:
                        "Bring your own API key. Choose from GPT, Claude, Gemini, Llama, or any provider.",
                    },
                    {
                      step: "02",
                      title: "Add skills & tools",
                      description:
                        "Connect GitHub, Slack, Linear, and more. Select exactly which actions the agent can take.",
                    },
                    {
                      step: "03",
                      title: "Write the prompt",
                      description:
                        "Define behavior with a system prompt and instructions — or let AI generate them.",
                    },
                    {
                      step: "04",
                      title: "Deploy & run",
                      description:
                        "Your agent goes live in a sandboxed environment with full observability from day one.",
                    },
                  ].map((item) => (
                    <div key={item.step} className="flex flex-col gap-3">
                      <span className="font-mono text-xs text-primary font-medium">
                        {item.step}
                      </span>
                      <h3 className="font-heading text-base font-semibold text-foreground">
                        {item.title}
                      </h3>
                      <p className="text-sm text-muted-foreground leading-relaxed">
                        {item.description}
                      </p>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Platform features */}
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
                "radial-gradient(circle at 50% 50%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
            }}
          />

          <div className="relative flex flex-col items-center gap-10 sm:gap-14 pb-20 sm:pb-28 lg:pb-36">
            {/* Section header */}
            <div className="flex flex-col items-center gap-5 sm:gap-6 max-w-3xl text-center px-4">
              <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
                Platform
              </p>
              <h2 className="font-heading text-[24px] sm:text-[32px] lg:text-[44px] font-bold text-foreground leading-[1.15] -tracking-[0.5px] sm:-tracking-[1px]">
                Everything your agents need{" "}
                <br className="hidden sm:block" />
                to run in production.
              </h2>
              <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-2xl">
                Every agent you build or install inherits the full power of the
                platform — memory, observability, access control, and more.
              </p>
            </div>

            {/* Bento grid */}
            <div className="w-full max-w-5xl mx-auto grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 px-4 lg:px-0">
              {[
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M20.25 6.375c0 2.278-3.694 4.125-8.25 4.125S3.75 8.653 3.75 6.375m16.5 0c0-2.278-3.694-4.125-8.25-4.125S3.75 4.097 3.75 6.375m16.5 0v11.25c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125V6.375m16.5 0v3.75m-16.5-3.75v3.75m16.5 0v3.75C20.25 16.153 16.556 18 12 18s-8.25-1.847-8.25-4.125v-3.75m16.5 0c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Long-term memory",
                  description:
                    "Agents remember context across conversations. Persistent memory that grows smarter over time.",
                },
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M14.25 6.087c0-.355.186-.676.401-.959.221-.29.349-.634.349-1.003 0-1.036-1.007-1.875-2.25-1.875s-2.25.84-2.25 1.875c0 .369.128.713.349 1.003.215.283.401.604.401.959v0a.64.64 0 01-.657.643 48.39 48.39 0 01-4.163-.3c.186 1.613.293 3.25.315 4.907a.656.656 0 01-.658.663v0c-.355 0-.676-.186-.959-.401a1.647 1.647 0 00-1.003-.349c-1.036 0-1.875 1.007-1.875 2.25s.84 2.25 1.875 2.25c.369 0 .713-.128 1.003-.349.283-.215.604-.401.959-.401v0c.31 0 .555.26.532.57a48.039 48.039 0 01-.642 5.056c1.518.19 3.058.309 4.616.354a.64.64 0 00.657-.643v0c0-.355-.186-.676-.401-.959a1.647 1.647 0 01-.349-1.003c0-1.035 1.008-1.875 2.25-1.875 1.243 0 2.25.84 2.25 1.875 0 .369-.128.713-.349 1.003-.215.283-.4.604-.4.959v0c0 .333.277.599.61.58a48.1 48.1 0 005.427-.63 48.05 48.05 0 00.582-4.717.532.532 0 00-.533-.57v0c-.355 0-.676.186-.959.401-.29.221-.634.349-1.003.349-1.035 0-1.875-1.007-1.875-2.25s.84-2.25 1.875-2.25c.37 0 .713.128 1.003.349.283.215.604.401.96.401v0a.656.656 0 00.657-.663 48.422 48.422 0 00-.37-5.36c-1.886.342-3.81.574-5.766.689a.578.578 0 01-.61-.58v0z" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Skills & tools",
                  description:
                    "Plug in reusable capabilities — API calls, workflows, code execution. Compose agents from building blocks.",
                },
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M3.75 3v11.25A2.25 2.25 0 006 16.5h2.25M3.75 3h-1.5m1.5 0h16.5m0 0h1.5m-1.5 0v11.25A2.25 2.25 0 0118 16.5h-2.25m-7.5 0h7.5m-7.5 0l-1 3m8.5-3l1 3m0 0l.5 1.5m-.5-1.5h-9.5m0 0l-.5 1.5m.75-9l3-3 2.148 2.148A12.061 12.061 0 0116.5 7.605" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Observability",
                  description:
                    "Traces, logs, and cost tracking for every run. Know exactly what your agents are doing and what they cost.",
                },
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Access control",
                  description:
                    "Fine-grained permissions for every agent. Scope API keys, assign team roles, and control what each agent can access.",
                },
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.86-2.814a4.5 4.5 0 00-1.242-7.244l4.5-4.5a4.5 4.5 0 116.364 6.364l-1.757 1.757" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Connections",
                  description:
                    "Native integrations with GitHub, Slack, Linear, Notion, and more. Select exactly which actions each agent can perform.",
                },
                {
                  icon: (
                    <svg className="w-6 h-6" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                      <path d="M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3m-19.5 0a4.5 4.5 0 01.9-2.7L5.737 5.1a3.375 3.375 0 012.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 01.9 2.7m0 0a3 3 0 013 3m0 3h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008zm-3 6h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008z" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  ),
                  title: "Sandboxed execution",
                  description:
                    "Every agent runs in an isolated environment. Shared for API-only or dedicated for full system access.",
                },
              ].map((feature) => (
                <div
                  key={feature.title}
                  className="flex flex-col gap-4 rounded-2xl border border-border bg-background p-6 transition-colors"
                >
                  <div className="flex items-center justify-center w-10 h-10 rounded-xl bg-muted text-foreground">
                    {feature.icon}
                  </div>
                  <h3 className="font-heading text-base font-semibold text-foreground">
                    {feature.title}
                  </h3>
                  <p className="text-sm text-muted-foreground leading-relaxed">
                    {feature.description}
                  </p>
                </div>
              ))}
            </div>
          </div>
        </div>
      </section>

      {/* Start with 150 free credits */}
      <section className="w-full px-4 sm:px-6 lg:px-0">
        <div className="w-full max-w-424 mx-auto relative pb-20 sm:pb-28 lg:pb-36">
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
                "radial-gradient(ellipse 50% 60% at 50% 40%, color-mix(in oklch, var(--primary) 12%, transparent) 0%, transparent 70%)",
            }}
          />

          <div className="relative flex flex-col items-center gap-6 sm:gap-7 pt-20 sm:pt-28 text-center">
            <HugeiconsIcon
              icon={CpuIcon}
              size={22}
              className="text-primary"
            />
            <p className="font-mono text-[11px] font-medium uppercase tracking-[2px] text-primary">
              Pricing · Credits-based
            </p>
            <h2 className="font-heading text-[36px] sm:text-[52px] lg:text-[64px] font-bold text-foreground leading-[1.03] -tracking-[1px] sm:-tracking-[1.4px] max-w-3xl">
              Start with{" "}
              <span className="italic font-medium text-primary">
                150 free credits.
              </span>
            </h2>
            <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-xl">
              Enough to build your first agent, watch it run, and decide
              whether this is the platform for you — before you spend a dollar.
            </p>
            <div className="flex flex-col items-center gap-3">
              <Link href="/auth">
                <Button size="lg" className="group cursor-pointer">
                  Get started free
                  <HugeiconsIcon
                    icon={ArrowRight01Icon}
                    size={15}
                    className="ml-1.5 opacity-80 group-hover:translate-x-0.5 transition-transform"
                  />
                </Button>
              </Link>
              <p className="font-mono text-[10px] uppercase tracking-[1.8px] text-muted-foreground/70">
                No card required · Free trial expires after 30 days
              </p>
            </div>
            <Link
              href="/pricing"
              className="font-mono text-[11px] uppercase tracking-[1.5px] text-muted-foreground hover:text-foreground transition-colors mt-4"
            >
              See full pricing →
            </Link>
          </div>
        </div>
      </section>

    </div>
  )
}
