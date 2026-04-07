import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

/**
 * Variant 4: "Editorial"
 * Newspaper-inspired — top meta band, drop cap on first paragraph, wide reading column,
 * related posts sidebar on desktop, newsletter inline
 */

const relatedPosts = [
  { slug: "how-we-built-the-agent-forger", title: "How We Built the Agent Forger", readTime: "8 min" },
  { slug: "byok-changes-everything", title: "Bring Your Own Keys Changes Everything", readTime: "4 min" },
  { slug: "agent-memory-with-hindsight", title: "How Agent Memory Works with Hindsight", readTime: "7 min" },
]

export default function BlogPostEditorial() {
  return (
    <div className="w-full bg-background flex flex-col relative min-h-screen">
      {/* Meta band */}
      <div className="w-full border-b border-border">
        <div className="max-w-4xl mx-auto px-4 py-4 flex flex-wrap items-center justify-between gap-4">
          <div className="flex items-center gap-4 text-sm text-muted-foreground">
            <span className="font-mono text-[11px] font-medium uppercase tracking-wider text-blue-500">Engineering</span>
            <span>March 25, 2026</span>
            <span>6 min read</span>
          </div>
          <div className="flex items-center gap-2">
            <button className="flex items-center justify-center w-8 h-8 rounded-lg border border-border text-muted-foreground hover:text-foreground hover:border-foreground/20 transition-colors cursor-pointer">
              <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor">
                <title>share on x</title>
                <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z" />
              </svg>
            </button>
            <button className="flex items-center justify-center w-8 h-8 rounded-lg border border-border text-muted-foreground hover:text-foreground hover:border-foreground/20 transition-colors cursor-pointer">
              <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <title>copy link</title>
                <path d="M10 13a5 5 0 007.54.54l3-3a5 5 0 00-7.07-7.07l-1.72 1.71" />
                <path d="M14 11a5 5 0 00-7.54-.54l-3 3a5 5 0 007.07 7.07l1.71-1.71" />
              </svg>
            </button>
          </div>
        </div>
      </div>

      <div className="max-w-4xl mx-auto w-full px-4 pb-24">
        {/* Title */}
        <div className="pt-12 sm:pt-16 pb-8 sm:pb-10">
          <h1 className="font-heading text-[32px] sm:text-[44px] lg:text-[52px] font-bold text-foreground leading-[1.06] -tracking-[1px] max-w-3xl">
            Why We Chose Ephemeral Sandboxes Over Persistent VMs
          </h1>
        </div>

        {/* Author */}
        <div className="flex items-center gap-4 pb-10 sm:pb-14 border-b border-border">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="https://i.pravatar.cc/80?u=kati" alt="Kati Frantz" className="w-11 h-11 rounded-full object-cover" />
          <div className="flex flex-col">
            <span className="text-sm font-semibold text-foreground">Kati Frantz</span>
            <span className="text-xs text-muted-foreground">Founder at ZiraLoop</span>
          </div>
        </div>

        <div className="flex gap-12 lg:gap-16 pt-10 sm:pt-14">
          {/* Main content */}
          <article className="flex-1 min-w-0 flex flex-col gap-5">
            {/* Drop cap paragraph */}
            <p className="text-[16px] sm:text-[17px] text-foreground/80 leading-[1.9] first-letter:text-5xl first-letter:font-heading first-letter:font-bold first-letter:text-foreground first-letter:float-left first-letter:mr-3 first-letter:mt-1">
              When we started building ZiraLoop, one of the earliest architectural decisions was how to run agent code. The obvious approach — giving each agent a persistent VM that&apos;s always on — seemed natural. But the economics didn&apos;t work. At scale, idle VMs would consume the majority of our infrastructure budget.
            </p>

            <h2 className="font-heading text-xl sm:text-2xl font-bold text-foreground mt-6 -tracking-[0.3px]">
              The persistent sandbox problem
            </h2>
            <p className="text-[16px] sm:text-[17px] text-foreground/80 leading-[1.9]">
              A persistent sandbox needs dedicated resources 24/7. On a Hetzner AX101 (16 cores, 128GB RAM, ~$110/month), you can fit roughly 48 dedicated sandboxes at 2 vCPU and 2GB RAM each. That&apos;s $2.29 per agent per month in raw compute — before any margin.
            </p>

            {/* Full-width accent line quote */}
            <div className="my-8 sm:my-10 py-8 border-y border-border">
              <p className="font-heading text-2xl sm:text-3xl font-bold text-foreground leading-snug -tracking-[0.5px] text-center max-w-xl mx-auto">
                The utilization rate for persistent sandboxes in agent workloads is typically under 5%.
              </p>
            </div>

            <h2 className="font-heading text-xl sm:text-2xl font-bold text-foreground mt-4 -tracking-[0.3px]">
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

            <p className="text-[16px] sm:text-[17px] text-foreground/80 leading-[1.9]">
              The math changes dramatically. A single server can now handle ~172,800 runs per month instead of 48 always-on sandboxes. Cost per run drops to $0.003. That&apos;s what makes the $3/month dedicated sandbox add-on possible with healthy margins.
            </p>

            <h2 className="font-heading text-xl sm:text-2xl font-bold text-foreground mt-6 -tracking-[0.3px]">
              What we learned
            </h2>
            <p className="text-[16px] sm:text-[17px] text-foreground/80 leading-[1.9]">
              The biggest challenge was caching. Cold cloning a large monorepo on every run would kill performance. We built a layer caching system that keeps warm copies of frequently-used repos and dependency trees. Combined with Daytona&apos;s 2-second boot, most runs feel instant.
            </p>
            <p className="text-[16px] sm:text-[17px] text-foreground/80 leading-[1.9]">
              Ephemeral sandboxes also simplify security. No persistent state to leak between runs. Each agent gets a clean environment, does its work, and disappears. No leftover credentials, no stale data, no cross-contamination between tenants.
            </p>
            <p className="text-[16px] sm:text-[17px] text-foreground/80 leading-[1.9]">
              If you&apos;re building an agent platform and considering persistent VMs, do the utilization math first. For event-driven workloads — which is most agents — ephemeral is the way to go.
            </p>

            {/* Inline newsletter */}
            <div className="my-10 rounded-2xl border border-border p-6 sm:p-8 flex flex-col gap-4">
              <span className="font-heading text-lg font-semibold text-foreground">Enjoyed this post?</span>
              <p className="text-sm text-muted-foreground">Get engineering deep dives and product updates in your inbox. No spam.</p>
              <div className="flex flex-col sm:flex-row gap-2.5">
                <Input
                  type="email"
                  placeholder="you@company.com"
                  className="h-10 rounded-full text-sm px-4 flex-1"
                />
                <Button size="default" className="rounded-full h-10 shrink-0">Subscribe</Button>
              </div>
            </div>
          </article>

          {/* Sidebar — desktop only */}
          <aside className="hidden lg:flex w-56 shrink-0 flex-col gap-8 sticky top-24 self-start">
            <div className="flex flex-col gap-3">
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground">
                Related posts
              </span>
              {relatedPosts.map((post) => (
                <Link
                  key={post.slug}
                  href={`/blog/${post.slug}`}
                  className="group flex flex-col gap-1 py-2"
                >
                  <span className="text-sm font-medium text-foreground group-hover:text-primary transition-colors leading-snug">
                    {post.title}
                  </span>
                  <span className="text-xs text-muted-foreground">{post.readTime} read</span>
                </Link>
              ))}
            </div>
          </aside>
        </div>
      </div>
    </div>
  )
}
