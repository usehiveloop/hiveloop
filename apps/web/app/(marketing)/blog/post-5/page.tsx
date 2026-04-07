import Link from "next/link"

/**
 * Variant 5: "Immersive Dark"
 * Dark hero that takes full viewport, content transitions to light (or stays dark in dark mode),
 * large typography, floating nav, parallax-ish feel
 */

const relatedPosts = [
  {
    slug: "how-we-built-the-agent-forger",
    title: "How We Built the Agent Forger",
    category: "Engineering",
    readTime: "8 min",
  },
  {
    slug: "byok-changes-everything",
    title: "Bring Your Own Keys Changes Everything",
    category: "Product",
    readTime: "4 min",
  },
  {
    slug: "building-a-pr-review-agent",
    title: "Tutorial: Building a PR Review Agent in 5 Minutes",
    category: "Guides",
    readTime: "10 min",
  },
]

export default function BlogPostImmersive() {
  return (
    <div className="w-full flex flex-col relative min-h-screen">
      {/* Dark immersive hero */}
      <div className="relative w-full bg-[oklch(0.12_0.01_55)] min-h-[85vh] sm:min-h-[90vh] flex items-end overflow-hidden">
        {/* Gradient orbs */}
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            background: "radial-gradient(circle at 20% 80%, oklch(0.35 0.15 250 / 20%) 0%, transparent 50%), radial-gradient(circle at 80% 20%, color-mix(in oklch, var(--primary) 15%, transparent) 0%, transparent 40%)",
          }}
        />
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            backgroundImage:
              "linear-gradient(oklch(1 0 0 / 3%) 1px, transparent 1px), linear-gradient(90deg, oklch(1 0 0 / 3%) 1px, transparent 1px)",
            backgroundSize: "80px 80px",
            maskImage: "radial-gradient(ellipse at 50% 100%, black 20%, transparent 60%)",
          }}
        />
        {/* Fade to content */}
        <div className="absolute bottom-0 left-0 right-0 h-32 bg-gradient-to-t from-background to-transparent" />

        <div className="relative w-full max-w-3xl mx-auto px-4 pb-24 sm:pb-32 pt-32">
          <div className="flex flex-col gap-6">
            <div className="flex items-center gap-3">
              <span className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-blue-400">
                Engineering
              </span>
              <span className="text-sm text-white/40">Mar 25, 2026</span>
            </div>
            <h1 className="font-heading text-[36px] sm:text-[52px] lg:text-[64px] font-bold text-white leading-[1.04] -tracking-[1.5px]">
              Why We Chose Ephemeral Sandboxes Over Persistent VMs
            </h1>
            <p className="text-lg sm:text-xl text-white/50 leading-relaxed max-w-2xl">
              The economics and architecture behind spinning up a fresh sandbox for every agent run — and how Daytona makes it possible in 2 seconds.
            </p>
            <div className="flex items-center gap-3 pt-4">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src="https://i.pravatar.cc/80?u=kati" alt="Kati Frantz" className="w-10 h-10 rounded-full object-cover ring-2 ring-white/10" />
              <div className="flex flex-col">
                <span className="text-sm font-medium text-white/90">Kati Frantz</span>
                <span className="text-xs text-white/40">6 min read</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Content — light bg */}
      <div className="w-full bg-background">
        <article className="max-w-2xl mx-auto px-4 py-16 sm:py-20">
          <div className="flex flex-col gap-6">
            <p className="text-[17px] sm:text-lg text-muted-foreground leading-[1.85]">
              When we started building ZiraLoop, one of the earliest architectural decisions was how to run agent code. The obvious approach — giving each agent a persistent VM that&apos;s always on — seemed natural. But the economics didn&apos;t work.
            </p>

            <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-[0.5px]">
              The persistent sandbox problem
            </h2>
            <p className="text-[17px] sm:text-lg text-muted-foreground leading-[1.85]">
              A persistent sandbox needs dedicated resources 24/7. On a Hetzner AX101 (16 cores, 128GB RAM, ~$110/month), you can fit roughly 48 dedicated sandboxes at 2 vCPU and 2GB RAM each. That&apos;s $2.29 per agent per month in raw compute — before any margin.
            </p>

            {/* Full-bleed stat */}
            <div className="my-8 -mx-4 sm:-mx-8 lg:-mx-16 px-4 sm:px-8 lg:px-16 py-10 bg-muted/40 border-y border-border flex flex-col items-center text-center gap-3">
              <span className="font-heading text-5xl sm:text-6xl font-bold text-foreground -tracking-[1px]">48 → 172,800</span>
              <span className="text-sm text-muted-foreground max-w-md">agents per box (persistent) vs. runs per month (ephemeral) on identical hardware</span>
            </div>

            <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-4 -tracking-[0.5px]">
              The ephemeral model
            </h2>
            <p className="text-[17px] sm:text-lg text-muted-foreground leading-[1.85]">
              Instead of reserving resources, we spin up a fresh sandbox for each run. The agent triggers, we boot a container, clone the repo, do the work, and tear it down. With Daytona&apos;s self-hosted runtime, sandbox cold start is roughly 2 seconds.
            </p>

            <div className="my-4 rounded-xl bg-[oklch(0.14_0.01_55)] border border-white/[0.06] overflow-hidden">
              <div className="flex items-center gap-2 px-4 py-2.5 border-b border-white/[0.06]">
                <span className="w-2.5 h-2.5 rounded-full bg-red-500/70" />
                <span className="w-2.5 h-2.5 rounded-full bg-yellow-500/70" />
                <span className="w-2.5 h-2.5 rounded-full bg-green-500/70" />
              </div>
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

            <p className="text-[17px] sm:text-lg text-muted-foreground leading-[1.85]">
              The math changes dramatically. Cost per run drops to $0.003. That&apos;s what makes the $3/month dedicated sandbox add-on possible with healthy margins.
            </p>

            <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-8 -tracking-[0.5px]">
              What we learned
            </h2>
            <p className="text-[17px] sm:text-lg text-muted-foreground leading-[1.85]">
              The biggest challenge wasn&apos;t the architecture — it was caching. Cold cloning a large monorepo on every run would kill performance. We built a layer caching system that keeps warm copies of frequently-used repos and dependency trees.
            </p>
            <p className="text-[17px] sm:text-lg text-muted-foreground leading-[1.85]">
              Ephemeral sandboxes also simplify security. There&apos;s no persistent state to leak between runs. Each agent gets a clean environment, does its work, and disappears.
            </p>
            <p className="text-[17px] sm:text-lg text-muted-foreground leading-[1.85]">
              If you&apos;re building an agent platform and considering persistent VMs, do the utilization math first. For event-driven workloads — which is most agents — ephemeral is the way to go.
            </p>
          </div>

          {/* Author */}
          <div className="mt-16 pt-8 border-t border-border flex items-center gap-4">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src="https://i.pravatar.cc/80?u=kati" alt="Kati Frantz" className="w-12 h-12 rounded-full object-cover" />
            <div className="flex flex-col">
              <span className="text-sm font-semibold text-foreground">Kati Frantz</span>
              <span className="text-sm text-muted-foreground">Founder at ZiraLoop. Building the future of production AI agents.</span>
            </div>
          </div>
        </article>

        {/* Related posts */}
        <div className="max-w-2xl mx-auto px-4 pb-24">
          <div className="pt-8 border-t border-border">
            <span className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground">
              Continue reading
            </span>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mt-6">
              {relatedPosts.map((post) => (
                <Link
                  key={post.slug}
                  href={`/blog/${post.slug}`}
                  className="group flex flex-col gap-2 p-4 rounded-xl border border-border hover:border-primary/40 transition-colors"
                >
                  <span className="font-mono text-[10px] font-medium uppercase tracking-wider text-blue-500">
                    {post.category}
                  </span>
                  <span className="text-sm font-semibold text-foreground group-hover:text-primary transition-colors leading-snug">
                    {post.title}
                  </span>
                  <span className="text-xs text-muted-foreground">{post.readTime}</span>
                </Link>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
