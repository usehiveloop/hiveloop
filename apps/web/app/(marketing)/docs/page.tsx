import Link from "next/link"
import { Input } from "@/components/ui/input"
import { HugeiconsIcon } from "@hugeicons/react"
import { Search01Icon } from "@hugeicons/core-free-icons"

/**
 * Variant 3: "Collapsible Sidebar with Tabs"
 * Sidebar with collapsible sections (accordion-style arrows), top tab bar for major sections,
 * content area with code examples side-by-side. More developer-tool focused.
 */

const tabs = [
  { label: "Guides", href: "/docs", active: true },
  { label: "API Reference", href: "/docs/api" },
  { label: "SDK", href: "/docs/sdk" },
  { label: "Changelog", href: "/docs/changelog" },
]

const sidebarGroups = [
  {
    title: "Getting Started",
    expanded: true,
    items: [
      { label: "Introduction", href: "/docs", active: true },
      { label: "Quickstart", href: "/docs/quickstart" },
      { label: "Installation", href: "/docs/installation" },
    ],
  },
  {
    title: "Agents",
    expanded: true,
    items: [
      { label: "Creating Agents", href: "/docs/agents/creating" },
      { label: "System Prompts", href: "/docs/agents/prompts" },
      { label: "Agent Forge", href: "/docs/agents/forge" },
      { label: "Memory & Hindsight", href: "/docs/agents/memory" },
    ],
  },
  {
    title: "Sandboxes",
    expanded: false,
    items: [
      { label: "Overview", href: "/docs/sandboxes/overview" },
      { label: "Custom Templates", href: "/docs/sandboxes/templates" },
    ],
  },
  {
    title: "Integrations",
    expanded: false,
    items: [
      { label: "Connections", href: "/docs/integrations/connections" },
      { label: "GitHub", href: "/docs/integrations/github" },
      { label: "Slack", href: "/docs/integrations/slack" },
    ],
  },
]

export default function DocsVariant3() {
  return (
    <div className="w-full bg-background flex flex-col relative min-h-screen">
      {/* Tab bar */}
      <div className="w-full border-b border-border bg-background sticky top-16 z-50">
        <div className="max-w-424 mx-auto px-4 lg:px-0 flex items-center gap-0">
          {tabs.map((tab) => (
            <Link
              key={tab.label}
              href={tab.href}
              className={`px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors ${
                tab.active
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`}
            >
              {tab.label}
            </Link>
          ))}
          <div className="flex-1" />
          <div className="relative w-56 hidden sm:block">
            <HugeiconsIcon icon={Search01Icon} size={13} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <Input placeholder="Search..." className="pl-8 h-8 rounded-lg text-xs bg-muted/50" />
          </div>
        </div>
      </div>

      <div className="flex flex-1 max-w-424 mx-auto w-full px-4 lg:px-0">
        {/* Sidebar */}
        <aside className="hidden lg:flex w-64 shrink-0 border-r border-border flex-col sticky top-[7.5rem] h-[calc(100vh-7.5rem)] overflow-y-auto py-6 pr-4">
          <nav className="flex flex-col gap-4">
            {sidebarGroups.map((group) => (
              <div key={group.title} className="flex flex-col">
                <button className="flex items-center justify-between px-2 py-1.5 text-sm font-medium text-foreground cursor-pointer">
                  {group.title}
                  <svg className={`w-3 h-3 text-muted-foreground transition-transform ${group.expanded ? "rotate-90" : ""}`} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <path d="M9 18l6-6-6-6" />
                  </svg>
                </button>
                {group.expanded && (
                  <div className="flex flex-col gap-0.5 ml-2 mt-1 border-l border-border pl-3">
                    {group.items.map((item) => (
                      <Link
                        key={item.href}
                        href={item.href}
                        className={`text-[13px] px-2 py-1 rounded transition-colors ${
                          item.active
                            ? "text-primary font-medium"
                            : "text-muted-foreground hover:text-foreground"
                        }`}
                      >
                        {item.label}
                      </Link>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </nav>
        </aside>

        {/* Main content */}
        <main className="flex-1 min-w-0 px-6 sm:px-10 lg:px-14 py-10">
          <div className="max-w-4xl mx-auto flex gap-12">
            <article className="flex-1 min-w-0">
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-6">
              <span>Guides</span>
              <span>/</span>
              <span>Getting Started</span>
              <span>/</span>
              <span className="text-foreground">Introduction</span>
            </div>

            <h1 className="font-heading text-[32px] sm:text-[38px] font-bold text-foreground -tracking-[0.5px] mb-3">
              Introduction
            </h1>
            <p className="text-lg text-muted-foreground leading-relaxed mb-10">
              ZiraLoop is the complete platform for building, running, and monitoring production-grade AI agents.
            </p>

            {/* Info callout */}
            <div className="flex gap-3 rounded-xl bg-blue-500/[0.06] border border-blue-500/20 p-4 mb-8">
              <span className="text-lg shrink-0">&#128161;</span>
              <div className="flex flex-col gap-1">
                <span className="text-sm font-medium text-foreground">New to ZiraLoop?</span>
                <span className="text-sm text-muted-foreground">
                  Start with the <Link href="/docs/quickstart" className="text-primary hover:underline">Quickstart guide</Link> to deploy your first agent in under 5 minutes.
                </span>
              </div>
            </div>

            <h2 className="font-heading text-2xl font-semibold text-foreground mt-10 mb-4">
              Core concepts
            </h2>
            <p className="text-sm text-muted-foreground leading-[1.8] mb-6">
              ZiraLoop revolves around a few key primitives that work together to power production agent workflows.
            </p>

            {/* Concepts as a definition list style */}
            <div className="flex flex-col gap-0 mb-8">
              {[
                { term: "Agents", description: "Autonomous programs with a system prompt, model, skills, and integrations. They execute tasks in sandboxed environments." },
                { term: "Sandboxes", description: "Isolated runtimes for agent execution. Shared for API work, dedicated for shell/filesystem access. ~2s cold start." },
                { term: "Connections", description: "Scoped OAuth integrations. You choose exactly which actions each agent can perform on GitHub, Slack, Linear, etc." },
                { term: "Credentials", description: "Your LLM API keys stored with AES-256-GCM envelope encryption. Bring any provider — OpenAI, Anthropic, Google, or open-source." },
              ].map((item) => (
                <div key={item.term} className="flex flex-col sm:flex-row gap-1 sm:gap-4 py-4 border-b border-border/50">
                  <span className="text-sm font-semibold text-foreground w-32 shrink-0">{item.term}</span>
                  <span className="text-sm text-muted-foreground leading-relaxed">{item.description}</span>
                </div>
              ))}
            </div>

            {/* Code example */}
            <h2 className="font-heading text-2xl font-semibold text-foreground mt-10 mb-4">
              Quick example
            </h2>
            <p className="text-sm text-muted-foreground leading-[1.8] mb-4">
              Create and run an agent with the TypeScript SDK:
            </p>
            <div className="rounded-xl bg-[oklch(0.14_0.01_55)] border border-white/[0.06] overflow-hidden mb-8">
              <div className="flex items-center justify-between px-4 py-2.5 border-b border-white/[0.06]">
                <span className="font-mono text-[11px] text-white/40">TypeScript</span>
                <button className="text-[10px] font-mono text-white/30 hover:text-white/60 transition-colors cursor-pointer">Copy</button>
              </div>
              <pre className="p-5 overflow-x-auto">
                <code className="font-mono text-[13px] leading-[1.7] text-white/80">{`import { ZiraLoop } from "@ziraloop/sdk";

const zira = new ZiraLoop({ apiKey: "zl_..." });

const agent = await zira.agents.create({
  name: "PR Reviewer",
  model: "claude-sonnet-4-20250514",
  systemPrompt: "Review pull requests for quality and security.",
  connections: ["github", "slack"],
});

const conversation = await agent.converse("Review PR #42");
console.log(conversation.response);`}</code>
              </pre>
            </div>

            {/* Prev / Next */}
            <div className="flex items-center justify-between pt-8 border-t border-border">
              <div />
              <Link href="/docs/quickstart" className="group flex flex-col items-end gap-0.5">
                <span className="text-xs text-muted-foreground">Next</span>
                <span className="text-sm font-medium text-foreground group-hover:text-primary transition-colors">Quickstart &rarr;</span>
              </Link>
            </div>
            </article>

            {/* TOC */}
            <div className="hidden xl:flex w-44 shrink-0">
              <div className="sticky top-[8.5rem] flex flex-col gap-1">
                <span className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground mb-2">
                  On this page
                </span>
                {[
                  { label: "Introduction", id: "introduction", active: true },
                  { label: "Core concepts", id: "core-concepts" },
                  { label: "Quick example", id: "quick-example" },
                ].map((item) => (
                  <a
                    key={item.id}
                    href={`#${item.id}`}
                    className={`text-[13px] py-1 pl-3 border-l-2 transition-colors ${
                      item.active
                        ? "border-primary text-foreground font-medium"
                        : "border-transparent text-muted-foreground hover:text-foreground hover:border-foreground/20"
                    }`}
                  >
                    {item.label}
                  </a>
                ))}
              </div>
            </div>
          </div>
        </main>
      </div>
    </div>
  )
}
