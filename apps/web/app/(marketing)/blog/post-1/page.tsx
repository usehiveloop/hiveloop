import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

/**
 * Variant 1: "Cinematic"
 * Full-bleed gradient hero, reading progress bar, narrow column, pull quotes, floating back button
 */
export default function BlogPostCinematic() {
  return (
    <div className="w-full bg-background flex flex-col relative min-h-screen">
      {/* Progress bar */}
      <div className="fixed top-0 left-0 right-0 z-200 h-0.5 bg-border">
        <div className="h-full bg-primary w-[35%] transition-all" />
      </div>

      {/* Hero */}
      <div className="relative w-full overflow-hidden">
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            background: "radial-gradient(ellipse at 50% 80%, color-mix(in oklch, var(--primary) 12%, transparent) 0%, transparent 60%)",
          }}
        />
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            backgroundImage:
              "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
            backgroundSize: "60px 60px",
            maskImage: "radial-gradient(ellipse at 50% 100%, black 10%, transparent 60%)",
          }}
        />
        <div className="relative max-w-3xl mx-auto px-4 pt-16 sm:pt-24 pb-16 sm:pb-20 flex flex-col items-center text-center gap-6">
          <span className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
            Engineering
          </span>
          <h1 className="font-heading text-[32px] sm:text-[44px] lg:text-[56px] font-bold text-foreground leading-[1.1] -tracking-[1px]">
            Why We Chose Ephemeral Sandboxes Over Persistent VMs
          </h1>
          <p className="text-lg sm:text-xl text-muted-foreground leading-relaxed max-w-2xl">
            The economics and architecture behind spinning up a fresh sandbox for every agent run — and how Daytona makes it possible in 2 seconds.
          </p>
          <div className="flex items-center gap-4 pt-4">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src="https://i.pravatar.cc/80?u=kati" alt="Kati Frantz" className="w-10 h-10 rounded-full object-cover" />
            <div className="flex flex-col items-start">
              <span className="text-sm font-medium text-foreground">Kati Frantz</span>
              <span className="text-xs text-muted-foreground">Mar 25, 2026 &middot; 6 min read</span>
            </div>
          </div>
        </div>
      </div>

      {/* Content */}
      <article className="max-w-2xl mx-auto px-4 pb-24">
        <div className="prose-custom flex flex-col gap-6">
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            When we started building ZiraLoop, one of the earliest architectural decisions was how to run agent code. The obvious approach — giving each agent a persistent VM that&apos;s always on — seemed natural. But the economics didn&apos;t work.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-[0.5px]">
            The persistent sandbox problem
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            A persistent sandbox needs dedicated resources 24/7. On a Hetzner AX101 (16 cores, 128GB RAM, ~$110/month), you can fit roughly 48 dedicated sandboxes at 2 vCPU and 2GB RAM each. That&apos;s $2.29 per agent per month in raw compute — before any margin.
          </p>

          {/* Pull quote */}
          <blockquote className="my-8 sm:my-12 py-6 border-l-2 border-primary pl-6 sm:pl-8 -ml-2">
            <p className="font-heading text-xl sm:text-2xl font-semibold text-foreground leading-snug -tracking-[0.3px]">
              &ldquo;A code review agent running 25 times a day only uses compute for 8 minutes total. The other 23 hours and 52 minutes, you&apos;re paying for an idle sandbox.&rdquo;
            </p>
          </blockquote>

          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            Most agents are event-driven. A code review agent fires on PR creation, does work for 1-2 minutes, then sits idle until the next PR. A support agent handles a ticket, responds, and waits. The utilization rate for persistent sandboxes in agent workloads is typically under 5%.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-[0.5px]">
            The ephemeral model
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            Instead of reserving resources, we spin up a fresh sandbox for each run. The agent triggers, we boot a container, clone the repo, do the work, and tear it down. With Daytona&apos;s self-hosted runtime, sandbox cold start is roughly 2 seconds.
          </p>

          {/* Code block */}
          <div className="my-6 rounded-xl bg-[oklch(0.14_0.01_55)] border border-white/[0.06] overflow-hidden">
            <div className="flex items-center gap-2 px-4 py-2.5 border-b border-white/[0.06]">
              <span className="w-2.5 h-2.5 rounded-full bg-red-500/70" />
              <span className="w-2.5 h-2.5 rounded-full bg-yellow-500/70" />
              <span className="w-2.5 h-2.5 rounded-full bg-green-500/70" />
              <span className="flex-1 text-center font-mono text-[10px] text-white/25">run-lifecycle.ts</span>
            </div>
            <pre className="p-4 sm:p-5 overflow-x-auto">
              <code className="font-mono text-[13px] leading-relaxed text-white/80">{`// Each agent run follows this lifecycle
const run = await sandbox.create({
  image: agent.sandboxTemplate,
  timeout: 300_000, // 5 min max
});

await run.exec("git clone", { repo: trigger.repo });
const result = await agent.execute(run, trigger);
await run.destroy(); // cleanup immediately`}</code>
            </pre>
          </div>

          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            The math changes dramatically. A single server can now handle ~172,800 runs per month instead of 48 always-on sandboxes. Cost per run drops to $0.003. That&apos;s what makes the $3/month dedicated sandbox add-on possible with healthy margins.
          </p>

          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-[0.5px]">
            What we learned
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            The biggest challenge wasn&apos;t the architecture — it was caching. Cold cloning a large monorepo on every run would kill performance. We built a layer caching system that keeps warm copies of frequently-used repos and dependency trees. Combined with Daytona&apos;s 2-second boot time, most runs feel instant.
          </p>

          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            Ephemeral sandboxes also simplify security. There&apos;s no persistent state to leak between runs. Each agent gets a clean environment, does its work, and disappears. No leftover credentials, no stale data, no cross-contamination between tenants.
          </p>

          {/* Pull quote */}
          <blockquote className="my-8 sm:my-12 py-6 border-l-2 border-primary pl-6 sm:pl-8 -ml-2">
            <p className="font-heading text-xl sm:text-2xl font-semibold text-foreground leading-snug -tracking-[0.3px]">
              &ldquo;Ephemeral sandboxes turned our biggest cost center into our biggest advantage. The same infrastructure that would support 48 persistent agents now handles thousands.&rdquo;
            </p>
          </blockquote>

          <p className="text-base sm:text-lg text-muted-foreground leading-[1.8]">
            If you&apos;re building an agent platform and considering persistent VMs, do the utilization math first. For event-driven workloads — which is most agents — ephemeral is the way to go.
          </p>
        </div>

        {/* Author card */}
        <div className="mt-16 pt-8 border-t border-border flex items-center gap-4">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="https://i.pravatar.cc/80?u=kati" alt="Kati Frantz" className="w-12 h-12 rounded-full object-cover" />
          <div className="flex flex-col">
            <span className="text-sm font-semibold text-foreground">Kati Frantz</span>
            <span className="text-sm text-muted-foreground">Founder at ZiraLoop. Building the future of production AI agents.</span>
          </div>
        </div>

        {/* Newsletter */}
        <div className="mt-12 relative rounded-2xl border border-border overflow-hidden">
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background: "radial-gradient(ellipse at 70% 50%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
            }}
          />
          <div className="relative p-6 sm:p-8 flex flex-col gap-4">
            <span className="font-heading text-lg font-semibold text-foreground">
              Enjoyed this post?
            </span>
            <p className="text-sm text-muted-foreground leading-relaxed">
              Engineering deep dives, product updates, and tutorials delivered to your inbox. No spam, unsubscribe anytime.
            </p>
            <div className="flex flex-col sm:flex-row gap-2.5 pt-1">
              <Input
                type="email"
                placeholder="you@company.com"
                className="h-10 sm:h-11 rounded-full text-sm px-5 flex-1"
              />
              <Button size="default" className="rounded-full h-10 sm:h-11 shrink-0">
                Subscribe
              </Button>
            </div>
          </div>
        </div>
      </article>
    </div>
  )
}
