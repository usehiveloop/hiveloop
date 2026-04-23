const subscriptionTools = [
  { name: "CodeRabbit", desc: "Code review", price: "$30" },
  { name: "Cursor", desc: "AI coding", price: "$20" },
  { name: "Lovable", desc: "UI generation", price: "$25" },
  { name: "Devin", desc: "Autonomous dev", price: "$500" },
  { name: "Jasper", desc: "Content writing", price: "$49" },
  { name: "Intercom Fin", desc: "Support agent", price: "$99" },
]

const hiveloopAgents = [
  { name: "code-reviewer", desc: "Reviews PRs on every push" },
  { name: "ui-builder", desc: "Generates components from designs" },
  { name: "content-writer", desc: "Drafts blog posts from outlines" },
  { name: "support-agent", desc: "Answers tickets from your docs" },
  { name: "code-assistant", desc: "Helps across the codebase" },
  { name: "deploy-monitor", desc: "Watches deploys, alerts on failure" },
]

const subscriptionDownsides = [
  "Locked into each vendor's model",
  "No control over prompts or behavior",
  "Separate billing for every tool",
  "Can't customize or extend features",
]

const hiveloopBenefits = [
  "Bring your own API keys",
  "Pick any model — GPT, Claude, Gemini, open-source",
  "Install agents from the marketplace",
  "Full control over prompts and behavior",
]

const stats = [
  { value: "10+", label: "subscriptions replaced" },
  { value: "90%", label: "cost savings vs. individual tools" },
  { value: "Minutes", label: "to deploy a new agent" },
]

export function ValuePropositionSection() {
  return (
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
                  {subscriptionTools.map((tool) => (
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
                  {subscriptionDownsides.map((item) => (
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
                  {hiveloopAgents.map((agent) => (
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
                  {hiveloopBenefits.map((item) => (
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
            {stats.map((stat) => (
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
  )
}
