import Link from "next/link"
import { Logo } from "@/components/logo"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Download04Icon,
  CheckmarkBadge01Icon,
  Calendar03Icon,
  ArrowRight01Icon,
} from "@hugeicons/core-free-icons"

// Static agent data — will be dynamic later
const agent = {
  name: "PR Review Agent",
  slug: "pr-review-agent",
  description: `
The PR Review Agent automatically reviews every pull request opened in your connected repositories. It analyzes code quality, checks for common security vulnerabilities, enforces your team's style guidelines, and provides actionable suggestions — all before a human reviewer needs to look at it.

## How it works

When a pull request is opened or updated, the agent receives a webhook from GitHub and begins its review process:

1. **Code analysis** — The agent reads the full diff and understands the context of changes across your codebase.
2. **Style enforcement** — It checks against your configured linting rules, naming conventions, and architectural patterns.
3. **Security scanning** — Common vulnerability patterns (SQL injection, XSS, hardcoded secrets) are flagged immediately.
4. **Review comments** — The agent posts inline comments on specific lines with clear explanations and suggested fixes.
5. **Summary** — A top-level review comment summarizes the findings with a pass/fail recommendation.

## What you need

- A GitHub integration connected to your ZiraLoop workspace
- A Slack integration (optional) for notifications
- A Linear integration (optional) for auto-creating issues from review findings

## Configuration

The agent ships with sensible defaults but can be customized:

- **Severity threshold** — Choose whether to flag only critical issues or include warnings and suggestions
- **Auto-approve** — Optionally auto-approve PRs that pass all checks
- **Custom rules** — Add your own review rules using natural language descriptions
- **Ignore patterns** — Skip files matching certain glob patterns (e.g., generated code, vendor directories)

## Who uses this

Engineering teams of all sizes use the PR Review Agent to catch issues early, reduce review cycles, and maintain consistent code quality across their repositories.
  `.trim(),
  publisher: { name: "Sarah Chen", avatar: "https://i.pravatar.cc/80?u=sarah" },
  installs: 12400,
  integrations: ["GitHub", "Slack", "Linear"],
  verified: true,
  publishedAt: "March 2025",
  lastUpdated: "June 2025",
  version: "2.1.0",
}

function formatInstalls(n: number) {
  if (n >= 1000) return `${(n / 1000).toFixed(n % 1000 === 0 ? 0 : 1)}k`
  return n.toString()
}

export default function AgentDetailPage() {
  return (
    <div className="w-full bg-background flex flex-col relative">
      {/* Nav */}
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

      <div className="max-w-4xl mx-auto w-full px-4 pt-8 pb-24">
        {/* Breadcrumb */}
        <div className="flex items-center gap-1.5 text-sm text-muted-foreground mb-8">
          <Link href="/marketplace" className="hover:text-foreground transition-colors">Marketplace</Link>
          <HugeiconsIcon icon={ArrowRight01Icon} size={12} />
          <span className="text-foreground">{agent.name}</span>
        </div>

        {/* Agent name + verified */}
        <div className="flex items-center gap-2 mb-6">
          <h1 className="font-heading text-[28px] sm:text-[36px] font-bold text-foreground leading-tight -tracking-[0.5px]">
            {agent.name}
          </h1>
          {agent.verified && (
            <HugeiconsIcon icon={CheckmarkBadge01Icon} size={22} className="text-green-500 shrink-0" />
          )}
        </div>

        {/* Cover image */}
        <div className="w-full aspect-[2.4/1] rounded-2xl border border-border bg-card flex items-center justify-center mb-8">
          <span className="font-mono text-sm text-muted-foreground">Cover image</span>
        </div>

        {/* Stats row */}
        <div className="flex flex-wrap items-center justify-center gap-6 mb-8">
          <div className="flex items-center gap-2.5">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={agent.publisher.avatar} alt={agent.publisher.name} className="h-7 w-7 rounded-full object-cover" />
            <span className="text-sm font-medium text-foreground">{agent.publisher.name}</span>
          </div>

          <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <HugeiconsIcon icon={Download04Icon} size={14} />
            <span>{formatInstalls(agent.installs)} installs</span>
          </div>

          <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <HugeiconsIcon icon={Calendar03Icon} size={14} />
            <span>Updated {agent.lastUpdated}</span>
          </div>

          <div className="flex items-center gap-1.5">
            {agent.integrations.map((name) => (
              <span key={name} className="inline-flex items-center rounded-full bg-muted px-2.5 py-0.5 text-xs font-medium text-muted-foreground">
                {name}
              </span>
            ))}
          </div>
        </div>

        <Separator className="mb-8" />

        {/* Description — blog post style */}
        <article className="prose prose-sm dark:prose-invert max-w-none mb-12">
          {agent.description.split("\n\n").map((block, i) => {
            if (block.startsWith("## ")) {
              return <h2 key={i} className="font-heading text-xl font-semibold text-foreground mt-10 mb-4">{block.replace("## ", "")}</h2>
            }
            if (block.startsWith("1. ") || block.startsWith("- ")) {
              const items = block.split("\n").filter(Boolean)
              const isOrdered = block.startsWith("1. ")
              const ListTag = isOrdered ? "ol" : "ul"
              return (
                <ListTag key={i} className={`${isOrdered ? "list-decimal" : "list-disc"} pl-5 flex flex-col gap-2 my-4`}>
                  {items.map((item, j) => {
                    const text = item.replace(/^\d+\.\s+/, "").replace(/^-\s+/, "")
                    return (
                      <li key={j} className="text-sm leading-relaxed text-muted-foreground">
                        <span dangerouslySetInnerHTML={{ __html: text.replace(/\*\*(.+?)\*\*/g, "<strong class='text-foreground font-medium'>$1</strong>") }} />
                      </li>
                    )
                  })}
                </ListTag>
              )
            }
            return (
              <p key={i} className="text-sm leading-relaxed text-muted-foreground my-4">
                <span dangerouslySetInnerHTML={{ __html: block.replace(/\*\*(.+?)\*\*/g, "<strong class='text-foreground font-medium'>$1</strong>") }} />
              </p>
            )
          })}
        </article>

        {/* Install CTA */}
        <div className="flex flex-col sm:flex-row items-start sm:items-center gap-4 rounded-2xl border border-border p-6">
          <div className="flex-1">
            <h3 className="font-heading text-lg font-semibold text-foreground">Ready to use this agent?</h3>
            <p className="text-sm text-muted-foreground mt-1">Install it in your workspace and start running in minutes.</p>
          </div>
          <Button size="lg">Install agent</Button>
        </div>
      </div>
    </div>
  )
}
