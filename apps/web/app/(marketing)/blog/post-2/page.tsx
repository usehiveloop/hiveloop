import Link from "next/link"

/**
 * Variant 2: "Split Screen"
 * Left panel fixed with title/meta/TOC. Right panel scrolls with content.
 */

const tocItems = [
  { id: "problem", label: "The persistent sandbox problem" },
  { id: "ephemeral", label: "The ephemeral model" },
  { id: "learned", label: "What we learned" },
  { id: "conclusion", label: "Conclusion" },
]

export default function BlogPostSplit() {
  return (
    <div className="w-full bg-background flex flex-col relative min-h-screen">
      <div className="max-w-6xl mx-auto w-full px-4 pb-24">
        <div className="flex flex-col lg:flex-row gap-12 lg:gap-16">
          {/* Left panel — sticky on desktop */}
          <div className="lg:w-96 lg:shrink-0">
            <div className="lg:sticky lg:top-24 flex flex-col gap-8 pt-8 sm:pt-12 lg:pt-16">
              <Link href="/blog" className="text-xs font-mono uppercase tracking-wider text-primary hover:underline w-fit">
                &larr; All posts
              </Link>

              <div className="flex flex-col gap-4">
                <span className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-blue-500">
                  Engineering
                </span>
                <h1 className="font-heading text-[28px] sm:text-[36px] font-bold text-foreground leading-[1.1] -tracking-[0.5px]">
                  Why We Chose Ephemeral Sandboxes Over Persistent VMs
                </h1>
                <p className="text-sm text-muted-foreground leading-relaxed">
                  The economics and architecture behind spinning up a fresh sandbox for every agent run.
                </p>
              </div>

              <div className="flex items-center gap-3">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img src="https://i.pravatar.cc/80?u=kati" alt="Kati Frantz" className="w-9 h-9 rounded-full object-cover" />
                <div className="flex flex-col">
                  <span className="text-sm font-medium text-foreground">Kati Frantz</span>
                  <span className="text-xs text-muted-foreground">Mar 25, 2026 &middot; 6 min read</span>
                </div>
              </div>

              {/* TOC */}
              <div className="hidden lg:flex flex-col gap-1 mt-4">
                <span className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground mb-2">
                  On this page
                </span>
                {tocItems.map((item, index) => (
                  <a
                    key={item.id}
                    href={`#${item.id}`}
                    className={`text-sm py-1.5 pl-3 border-l-2 transition-colors ${
                      index === 0
                        ? "border-primary text-foreground font-medium"
                        : "border-border text-muted-foreground hover:text-foreground hover:border-foreground/30"
                    }`}
                  >
                    {item.label}
                  </a>
                ))}
              </div>

              {/* Share */}
              <div className="hidden lg:flex flex-col gap-3 mt-4 pt-6 border-t border-border">
                <span className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground">
                  Share
                </span>
                <div className="flex items-center gap-2">
                  <button className="flex items-center justify-center w-8 h-8 rounded-lg bg-muted text-muted-foreground hover:text-foreground transition-colors cursor-pointer">
                    <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor">
                      <title>x</title>
                      <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z" />
                    </svg>
                  </button>
                  <button className="flex items-center justify-center w-8 h-8 rounded-lg bg-muted text-muted-foreground hover:text-foreground transition-colors cursor-pointer">
                    <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                      <title>copy link</title>
                      <path d="M10 13a5 5 0 007.54.54l3-3a5 5 0 00-7.07-7.07l-1.72 1.71" />
                      <path d="M14 11a5 5 0 00-7.54-.54l-3 3a5 5 0 007.07 7.07l1.71-1.71" />
                    </svg>
                  </button>
                </div>
              </div>
            </div>
          </div>

          {/* Right panel — scrolls */}
          <article className="flex-1 min-w-0 pt-8 sm:pt-12 lg:pt-16">
            <div className="flex flex-col gap-6 max-w-2xl">
              <p className="text-base sm:text-[17px] text-muted-foreground leading-[1.85]">
                When we started building ZiraLoop, one of the earliest architectural decisions was how to run agent code. The obvious approach — giving each agent a persistent VM that&apos;s always on — seemed natural. But the economics didn&apos;t work.
              </p>

              <h2 id="problem" className="font-heading text-xl sm:text-2xl font-bold text-foreground mt-6 -tracking-[0.3px] scroll-mt-24">
                The persistent sandbox problem
              </h2>
              <p className="text-base sm:text-[17px] text-muted-foreground leading-[1.85]">
                A persistent sandbox needs dedicated resources 24/7. On a Hetzner AX101 (16 cores, 128GB RAM, ~$110/month), you can fit roughly 48 dedicated sandboxes at 2 vCPU and 2GB RAM each. That&apos;s $2.29 per agent per month in raw compute — before any margin.
              </p>
              <p className="text-base sm:text-[17px] text-muted-foreground leading-[1.85]">
                Most agents are event-driven. A code review agent fires on PR creation, does work for 1-2 minutes, then sits idle until the next PR. The utilization rate for persistent sandboxes in agent workloads is typically under 5%.
              </p>

              {/* Highlighted stat */}
              <div className="my-6 rounded-2xl bg-muted/50 border border-border p-6 sm:p-8 flex flex-col items-center text-center gap-2">
                <span className="font-heading text-4xl sm:text-5xl font-bold text-foreground -tracking-[1px]">5%</span>
                <span className="text-sm text-muted-foreground">average utilization for persistent agent sandboxes</span>
              </div>

              <h2 id="ephemeral" className="font-heading text-xl sm:text-2xl font-bold text-foreground mt-6 -tracking-[0.3px] scroll-mt-24">
                The ephemeral model
              </h2>
              <p className="text-base sm:text-[17px] text-muted-foreground leading-[1.85]">
                Instead of reserving resources, we spin up a fresh sandbox for each run. The agent triggers, we boot a container, clone the repo, do the work, and tear it down. With Daytona&apos;s self-hosted runtime, sandbox cold start is roughly 2 seconds.
              </p>

              <div className="my-6 rounded-xl bg-[oklch(0.14_0.01_55)] border border-white/[0.06] overflow-hidden">
                <div className="flex items-center justify-between px-4 py-2.5 border-b border-white/[0.06]">
                  <span className="font-mono text-[11px] text-white/40">run-lifecycle.ts</span>
                  <button className="text-[10px] font-mono text-white/30 hover:text-white/60 transition-colors cursor-pointer">Copy</button>
                </div>
                <pre className="p-4 sm:p-5 overflow-x-auto">
                  <code className="font-mono text-[13px] leading-relaxed text-white/80">{`const run = await sandbox.create({
  image: agent.sandboxTemplate,
  timeout: 300_000,
});

await run.exec("git clone", { repo: trigger.repo });
const result = await agent.execute(run, trigger);
await run.destroy();`}</code>
                </pre>
              </div>

              <p className="text-base sm:text-[17px] text-muted-foreground leading-[1.85]">
                The math changes dramatically. A single server can now handle ~172,800 runs per month instead of 48 always-on sandboxes. Cost per run drops to $0.003.
              </p>

              <h2 id="learned" className="font-heading text-xl sm:text-2xl font-bold text-foreground mt-6 -tracking-[0.3px] scroll-mt-24">
                What we learned
              </h2>
              <p className="text-base sm:text-[17px] text-muted-foreground leading-[1.85]">
                The biggest challenge wasn&apos;t the architecture — it was caching. Cold cloning a large monorepo on every run would kill performance. We built a layer caching system that keeps warm copies of frequently-used repos and dependency trees.
              </p>
              <p className="text-base sm:text-[17px] text-muted-foreground leading-[1.85]">
                Ephemeral sandboxes also simplify security. There&apos;s no persistent state to leak between runs. Each agent gets a clean environment, does its work, and disappears. No leftover credentials, no stale data, no cross-contamination.
              </p>

              <h2 id="conclusion" className="font-heading text-xl sm:text-2xl font-bold text-foreground mt-6 -tracking-[0.3px] scroll-mt-24">
                Conclusion
              </h2>
              <p className="text-base sm:text-[17px] text-muted-foreground leading-[1.85]">
                Ephemeral sandboxes turned our biggest cost center into our biggest advantage. The same infrastructure that would support 48 persistent agents now handles thousands. If you&apos;re building an agent platform, do the utilization math first.
              </p>

              {/* Author card */}
              <div className="mt-12 pt-8 border-t border-border flex items-center gap-4">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img src="https://i.pravatar.cc/80?u=kati" alt="Kati Frantz" className="w-12 h-12 rounded-full object-cover" />
                <div className="flex flex-col">
                  <span className="text-sm font-semibold text-foreground">Kati Frantz</span>
                  <span className="text-sm text-muted-foreground">Founder at ZiraLoop. Building the future of production AI agents.</span>
                </div>
              </div>
            </div>
          </article>
        </div>
      </div>
    </div>
  )
}
