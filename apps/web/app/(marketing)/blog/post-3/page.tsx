import Link from "next/link"

/**
 * Variant 3: "Notion-style"
 * Clean, spacious, icon-accented headings, callout blocks, wide with subtle page chrome
 */
export default function BlogPostNotion() {
  return (
    <div className="w-full bg-background flex flex-col relative min-h-screen">
      <article className="max-w-3xl mx-auto w-full px-4 pb-24">
        {/* Breadcrumb */}
        <div className="flex items-center gap-2 pt-8 sm:pt-12 pb-8 text-sm text-muted-foreground">
          <Link href="/blog" className="hover:text-foreground transition-colors">Blog</Link>
          <span>/</span>
          <span className="text-foreground">Engineering</span>
        </div>

        {/* Title block */}
        <div className="flex flex-col gap-4 pb-8">
          {/* Icon */}
          <div className="w-14 h-14 sm:w-16 sm:h-16 rounded-2xl bg-blue-500/10 flex items-center justify-center text-2xl sm:text-3xl">
            <span>&#9881;</span>
          </div>
          <h1 className="font-heading text-[32px] sm:text-[40px] lg:text-[48px] font-bold text-foreground leading-[1.08] -tracking-[1px]">
            Why We Chose Ephemeral Sandboxes Over Persistent VMs
          </h1>
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-muted-foreground">
            <div className="flex items-center gap-2">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src="https://i.pravatar.cc/80?u=kati" alt="Kati Frantz" className="w-5 h-5 rounded-full" />
              <span>Kati Frantz</span>
            </div>
            <span>&middot;</span>
            <span>March 25, 2026</span>
            <span>&middot;</span>
            <span>6 min read</span>
          </div>
        </div>

        <div className="h-px bg-border mb-10" />

        {/* Content */}
        <div className="flex flex-col gap-5">
          <p className="text-[16px] sm:text-[17px] text-foreground/80 leading-[1.9]">
            When we started building ZiraLoop, one of the earliest architectural decisions was how to run agent code. The obvious approach — giving each agent a persistent VM that&apos;s always on — seemed natural. But the economics didn&apos;t work.
          </p>

          <h2 className="font-heading text-2xl font-bold text-foreground mt-6 flex items-center gap-3">
            <span className="text-muted-foreground/30">#</span>
            The persistent sandbox problem
          </h2>
          <p className="text-[16px] sm:text-[17px] text-foreground/80 leading-[1.9]">
            A persistent sandbox needs dedicated resources 24/7. On a Hetzner AX101 (16 cores, 128GB RAM, ~$110/month), you can fit roughly 48 dedicated sandboxes at 2 vCPU and 2GB RAM each. That&apos;s $2.29 per agent per month in raw compute — before any margin.
          </p>

          {/* Callout block */}
          <div className="my-4 flex gap-3 rounded-xl bg-amber-500/[0.06] border border-amber-500/20 p-4 sm:p-5">
            <span className="text-lg shrink-0 mt-0.5">&#128161;</span>
            <div className="flex flex-col gap-1">
              <span className="text-sm font-medium text-foreground">Key insight</span>
              <span className="text-sm text-foreground/70 leading-relaxed">
                A code review agent running 25 times a day only uses compute for ~8 minutes. The other 23 hours and 52 minutes, the sandbox sits completely idle — but you&apos;re still paying for it.
              </span>
            </div>
          </div>

          <p className="text-[16px] sm:text-[17px] text-foreground/80 leading-[1.9]">
            Most agents are event-driven. A code review agent fires on PR creation, does work for 1-2 minutes, then sits idle. A support agent handles a ticket, responds, and waits. The utilization rate for persistent sandboxes in agent workloads is typically under 5%.
          </p>

          <h2 className="font-heading text-2xl font-bold text-foreground mt-6 flex items-center gap-3">
            <span className="text-muted-foreground/30">#</span>
            The ephemeral model
          </h2>
          <p className="text-[16px] sm:text-[17px] text-foreground/80 leading-[1.9]">
            Instead of reserving resources, we spin up a fresh sandbox for each run. The agent triggers, we boot a container, clone the repo, do the work, and tear it down. With Daytona&apos;s self-hosted runtime, sandbox cold start is roughly 2 seconds.
          </p>

          <div className="my-4 rounded-xl bg-[oklch(0.14_0.01_55)] border border-white/[0.06] overflow-hidden">
            <pre className="p-5 overflow-x-auto">
              <code className="font-mono text-[13px] leading-[1.7] text-white/80">{`const run = await sandbox.create({
  image: agent.sandboxTemplate,
  timeout: 300_000,
});

await run.exec("git clone", { repo: trigger.repo });
const result = await agent.execute(run, trigger);
await run.destroy();`}</code>
            </pre>
          </div>

          {/* Info callout */}
          <div className="my-4 flex gap-3 rounded-xl bg-blue-500/[0.06] border border-blue-500/20 p-4 sm:p-5">
            <span className="text-lg shrink-0 mt-0.5">&#128202;</span>
            <div className="flex flex-col gap-1">
              <span className="text-sm font-medium text-foreground">By the numbers</span>
              <span className="text-sm text-foreground/70 leading-relaxed">
                A single server handles ~172,800 runs/month with ephemeral sandboxes vs. 48 agents with persistent. Cost per run: $0.003 vs. $2.29/agent/month.
              </span>
            </div>
          </div>

          <h2 className="font-heading text-2xl font-bold text-foreground mt-6 flex items-center gap-3">
            <span className="text-muted-foreground/30">#</span>
            What we learned
          </h2>
          <p className="text-[16px] sm:text-[17px] text-foreground/80 leading-[1.9]">
            The biggest challenge wasn&apos;t the architecture — it was caching. Cold cloning a large monorepo on every run would kill performance. We built a layer caching system that keeps warm copies of frequently-used repos and dependency trees. Combined with Daytona&apos;s 2-second boot time, most runs feel instant.
          </p>
          <p className="text-[16px] sm:text-[17px] text-foreground/80 leading-[1.9]">
            Ephemeral sandboxes also simplify security. No persistent state to leak between runs. Each agent gets a clean environment, does its work, and disappears. No leftover credentials, no stale data, no cross-contamination between tenants.
          </p>

          {/* Success callout */}
          <div className="my-4 flex gap-3 rounded-xl bg-green-500/[0.06] border border-green-500/20 p-4 sm:p-5">
            <span className="text-lg shrink-0 mt-0.5">&#9989;</span>
            <div className="flex flex-col gap-1">
              <span className="text-sm font-medium text-foreground">Result</span>
              <span className="text-sm text-foreground/70 leading-relaxed">
                Ephemeral sandboxes turned our biggest cost center into our biggest advantage. The same infrastructure that would support 48 persistent agents now handles thousands — and makes $3/month dedicated sandbox pricing possible.
              </span>
            </div>
          </div>
        </div>

        {/* Tags */}
        <div className="flex items-center gap-2 mt-12 pt-8 border-t border-border">
          {["infrastructure", "sandboxes", "daytona", "architecture"].map((tag) => (
            <span key={tag} className="px-3 py-1 rounded-full bg-muted text-xs text-muted-foreground font-medium">
              {tag}
            </span>
          ))}
        </div>

        {/* Author */}
        <div className="mt-8 p-6 rounded-2xl border border-border flex items-start gap-4">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="https://i.pravatar.cc/80?u=kati" alt="Kati Frantz" className="w-12 h-12 rounded-full object-cover shrink-0" />
          <div className="flex flex-col gap-1.5">
            <span className="text-sm font-semibold text-foreground">Written by Kati Frantz</span>
            <span className="text-sm text-muted-foreground leading-relaxed">Founder at ZiraLoop. Building the future of production AI agents. Previously built Kibamail.</span>
          </div>
        </div>
      </article>
    </div>
  )
}
