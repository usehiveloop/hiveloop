import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

const featuredPost = {
  slug: "introducing-the-marketplace",
  title: "Introducing the Marketplace with Revenue Sharing",
  date: "Mar 18, 2026",
  author: { name: "Kati Frantz", avatar: "https://i.pravatar.cc/80?u=kati" },
  readTime: "5 min",
  category: "Product",
  excerpt: "Build an agent, publish it, and earn 50% of every install. Here's how the marketplace works and why we built it. We designed the revenue model to attract quality builders while keeping the platform sustainable.",
}

const leftPosts = [
  {
    slug: "how-we-built-the-agent-forger",
    title: "How We Built the Agent Forger",
    date: "Apr 1, 2026",
    author: { name: "Kati Frantz", avatar: "https://i.pravatar.cc/80?u=kati" },
    readTime: "8 min",
    category: "Engineering",
    excerpt: "A deep dive into the AI-powered agent creation system that generates optimized prompts, selects tools, and configures sandboxes automatically.",
  },
  {
    slug: "why-we-chose-ephemeral-sandboxes",
    title: "Why We Chose Ephemeral Sandboxes Over Persistent VMs",
    date: "Mar 25, 2026",
    author: { name: "Kati Frantz", avatar: "https://i.pravatar.cc/80?u=kati" },
    readTime: "6 min",
    category: "Engineering",
    excerpt: "The economics and architecture behind spinning up a fresh sandbox for every agent run — and how Daytona makes it possible in 2 seconds.",
  },
  {
    slug: "byok-changes-everything",
    title: "Bring Your Own Keys Changes Everything",
    date: "Mar 10, 2026",
    author: { name: "Kati Frantz", avatar: "https://i.pravatar.cc/80?u=kati" },
    readTime: "4 min",
    category: "Product",
    excerpt: "Why we don't charge for inference and how BYOK unlocks $3.99/agent pricing that would be impossible otherwise.",
  },
]

const rightPosts = [
  {
    slug: "building-a-pr-review-agent",
    title: "Tutorial: Building a PR Review Agent in 5 Minutes",
    date: "Mar 5, 2026",
    readTime: "10 min",
    category: "Guides",
  },
  {
    slug: "agent-memory-with-hindsight",
    title: "How Agent Memory Works with Hindsight",
    date: "Feb 28, 2026",
    readTime: "7 min",
    category: "Engineering",
  },
  {
    slug: "observability-for-agents",
    title: "Why Observability Is Non-Negotiable for Production Agents",
    date: "Feb 20, 2026",
    readTime: "5 min",
    category: "Engineering",
  },
  {
    slug: "access-control-deep-dive",
    title: "Fine-Grained Access Control for AI Agents",
    date: "Feb 14, 2026",
    readTime: "6 min",
    category: "Engineering",
  },
  {
    slug: "mcp-tools-explained",
    title: "MCP Tools: What They Are and Why Agents Need Them",
    date: "Feb 8, 2026",
    readTime: "5 min",
    category: "Product",
  },
  {
    slug: "self-hosting-ziraloop",
    title: "Self-Hosting ZiraLoop: A Complete Guide",
    date: "Feb 1, 2026",
    readTime: "12 min",
    category: "Guides",
  },
]

const allPosts = [
  {
    slug: "connection-scoping",
    title: "Connection Scoping: Giving Agents the Minimum Access They Need",
    date: "Jan 25, 2026",
    author: { name: "Kati Frantz", avatar: "https://i.pravatar.cc/80?u=kati" },
    readTime: "6 min",
    category: "Engineering",
    excerpt: "How integration action scoping works under the hood and why it matters for security-conscious teams.",
  },
  {
    slug: "support-agent-case-study",
    title: "Case Study: Replacing Intercom Fin with a $3.99 Agent",
    date: "Jan 18, 2026",
    author: { name: "Kati Frantz", avatar: "https://i.pravatar.cc/80?u=kati" },
    readTime: "8 min",
    category: "Product",
    excerpt: "How one team built a customer support agent that handles 80% of tickets automatically, for a fraction of the cost.",
  },
  {
    slug: "typescript-sdk-v1",
    title: "Announcing the TypeScript SDK v1.0",
    date: "Jan 12, 2026",
    author: { name: "Kati Frantz", avatar: "https://i.pravatar.cc/80?u=kati" },
    readTime: "4 min",
    category: "Product",
    excerpt: "The official TypeScript SDK for ZiraLoop is now stable. Create agents, manage conversations, and stream responses with type safety.",
  },
  {
    slug: "sandbox-templates",
    title: "Custom Sandbox Templates: Pre-Configure Your Agent's Environment",
    date: "Jan 5, 2026",
    author: { name: "Kati Frantz", avatar: "https://i.pravatar.cc/80?u=kati" },
    readTime: "7 min",
    category: "Engineering",
    excerpt: "Build reusable sandbox templates with pre-installed tools, cached dependencies, and custom configurations for faster cold starts.",
  },
  {
    slug: "building-a-deploy-monitor",
    title: "Tutorial: Building a Deploy Monitor Agent",
    date: "Dec 28, 2025",
    author: { name: "Kati Frantz", avatar: "https://i.pravatar.cc/80?u=kati" },
    readTime: "9 min",
    category: "Guides",
    excerpt: "Step-by-step guide to creating an agent that watches your deployments, runs health checks, and alerts your team on failure.",
  },
  {
    slug: "encryption-at-rest",
    title: "How We Encrypt Credentials with AES-256-GCM Envelope Encryption",
    date: "Dec 20, 2025",
    author: { name: "Kati Frantz", avatar: "https://i.pravatar.cc/80?u=kati" },
    readTime: "6 min",
    category: "Engineering",
    excerpt: "A look at our credential storage architecture — envelope encryption, key rotation, and why we never see your API keys.",
  },
]

const categoryColors: Record<string, string> = {
  Product: "text-primary",
  Engineering: "text-blue-500",
  Guides: "text-green-500",
}

export default function BlogMagazine() {
  return (
    <div className="w-full bg-background flex flex-col relative min-h-screen">
      <div className="max-w-6xl mx-auto w-full px-4 pb-24">
        {/* Header */}
        <div className="flex flex-col gap-2 pt-12 sm:pt-20 pb-10 sm:pb-14">
          <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">Blog</p>
          <h1 className="font-heading text-[28px] sm:text-[36px] lg:text-[44px] font-bold text-foreground leading-[1.15] -tracking-[0.5px]">
            Stories from the team
          </h1>
        </div>

        {/* Featured post */}
        <Link
          href={`/blog/${featuredPost.slug}`}
          className="group block mb-12 sm:mb-16"
        >
          <div className="relative rounded-3xl overflow-hidden border border-border hover:border-primary/40 transition-colors">
            {/* Gradient background */}
            <div
              className="absolute inset-0 pointer-events-none"
              style={{
                background: "radial-gradient(ellipse at 30% 50%, color-mix(in oklch, var(--primary) 10%, transparent) 0%, transparent 70%)",
              }}
            />
            <div
              className="absolute inset-0 pointer-events-none"
              style={{
                backgroundImage:
                  "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
                backgroundSize: "60px 60px",
                maskImage: "radial-gradient(ellipse at 30% 50%, black 10%, transparent 60%)",
              }}
            />

            <div className="relative p-8 sm:p-12 lg:p-16 flex flex-col gap-5 max-w-3xl">
              <div className="flex items-center gap-3">
                <span className="font-mono text-[11px] font-medium uppercase tracking-wider text-primary">
                  {featuredPost.category}
                </span>
                <span className="text-xs text-muted-foreground">{featuredPost.date}</span>
              </div>
              <h2 className="font-heading text-[24px] sm:text-[32px] lg:text-[40px] font-bold text-foreground leading-[1.15] -tracking-[0.5px] group-hover:text-primary transition-colors">
                {featuredPost.title}
              </h2>
              <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-2xl">
                {featuredPost.excerpt}
              </p>
              <div className="flex items-center gap-3 pt-2">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={featuredPost.author.avatar}
                  alt={featuredPost.author.name}
                  className="w-8 h-8 rounded-full object-cover"
                />
                <div className="flex flex-col">
                  <span className="text-sm font-medium text-foreground">{featuredPost.author.name}</span>
                  <span className="text-xs text-muted-foreground">{featuredPost.readTime} read</span>
                </div>
              </div>
            </div>
          </div>
        </Link>

        {/* Two-column grid */}
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 lg:gap-12">
          {/* Left: Feature cards */}
          <div className="lg:col-span-7 flex flex-col gap-6">
            {leftPosts.map((post) => (
              <Link
                key={post.slug}
                href={`/blog/${post.slug}`}
                className="group flex flex-col gap-4 rounded-2xl border border-border p-6 sm:p-8 hover:border-primary/40 transition-colors"
              >
                <div className="flex items-center gap-3">
                  <span className={`font-mono text-[11px] font-medium uppercase tracking-wider ${categoryColors[post.category]}`}>
                    {post.category}
                  </span>
                  <span className="text-xs text-muted-foreground">{post.date}</span>
                  <span className="text-xs text-muted-foreground">&middot; {post.readTime}</span>
                </div>
                <h3 className="font-heading text-xl sm:text-2xl font-semibold text-foreground group-hover:text-primary transition-colors leading-snug">
                  {post.title}
                </h3>
                <p className="text-sm text-muted-foreground leading-relaxed">
                  {post.excerpt}
                </p>
                <div className="flex items-center gap-2 pt-1">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={post.author.avatar}
                    alt={post.author.name}
                    className="w-5 h-5 rounded-full object-cover"
                  />
                  <span className="text-xs text-muted-foreground">{post.author.name}</span>
                </div>
              </Link>
            ))}
          </div>

          {/* Right: Compact list */}
          <div className="lg:col-span-5">
            <div className="sticky top-20">
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground mb-4 block">
                More posts
              </span>
              <div className="flex flex-col">
                {rightPosts.map((post) => (
                  <Link
                    key={post.slug}
                    href={`/blog/${post.slug}`}
                    className="group flex flex-col gap-1.5 py-4 border-b border-border/50 last:border-0"
                  >
                    <div className="flex items-center gap-2">
                      <span className={`font-mono text-[10px] font-medium uppercase tracking-wider ${categoryColors[post.category]}`}>
                        {post.category}
                      </span>
                      <span className="text-[10px] text-muted-foreground">&middot; {post.date}</span>
                    </div>
                    <h4 className="text-sm font-semibold text-foreground group-hover:text-primary transition-colors leading-snug">
                      {post.title}
                    </h4>
                    <span className="text-xs text-muted-foreground">{post.readTime} read</span>
                  </Link>
                ))}
              </div>
            </div>
          </div>
        </div>
        {/* Newsletter */}
        <div className="mt-16 sm:mt-24 mb-16 sm:mb-24 relative rounded-3xl border border-border overflow-hidden">
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background: "radial-gradient(ellipse at 70% 50%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
            }}
          />
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              backgroundImage:
                "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
              backgroundSize: "40px 40px",
              maskImage: "radial-gradient(ellipse at 70% 50%, black 10%, transparent 50%)",
            }}
          />
          <div className="relative p-8 sm:p-12 lg:p-16 flex flex-col lg:flex-row lg:items-center gap-8 lg:gap-16">
            <div className="flex flex-col gap-3 lg:flex-1">
              <span className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
                Newsletter
              </span>
              <h2 className="font-heading text-[22px] sm:text-[28px] font-bold text-foreground leading-[1.2] -tracking-[0.5px]">
                Stay in the loop
              </h2>
              <p className="text-sm sm:text-base text-muted-foreground leading-relaxed max-w-md">
                Engineering deep dives, product updates, and tutorials delivered to your inbox. No spam, unsubscribe anytime.
              </p>
            </div>
            <div className="flex flex-col sm:flex-row gap-2.5 lg:flex-1 lg:max-w-md">
              <Input
                type="email"
                placeholder="you@company.com"
                className="h-11 sm:h-12 rounded-full text-sm sm:text-base px-5 flex-1"
              />
              <Button size="default" className="sm:hidden rounded-full h-11">
                Subscribe
              </Button>
              <Button size="lg" className="hidden sm:inline-flex rounded-full h-12 shrink-0">
                Subscribe
              </Button>
            </div>
          </div>
        </div>

        {/* All posts */}
        <div className="flex flex-col gap-8">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <h2 className="font-heading text-xl sm:text-2xl font-bold text-foreground">
              All posts
            </h2>
            <div className="flex items-center gap-1.5 flex-wrap">
              {["All", "Engineering", "Product", "Guides"].map((category, index) => (
                <button
                  key={category}
                  className={`px-3 py-1.5 rounded-full text-xs font-medium transition-colors cursor-pointer ${
                    index === 0
                      ? "bg-foreground text-background"
                      : "bg-muted text-muted-foreground hover:text-foreground hover:bg-muted/80"
                  }`}
                >
                  {category}
                </button>
              ))}
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
            {allPosts.map((post) => (
              <Link
                key={post.slug}
                href={`/blog/${post.slug}`}
                className="group flex flex-col gap-4 rounded-2xl border border-border p-5 sm:p-6 hover:border-primary/40 transition-colors"
              >
                <div className="flex items-center gap-2">
                  <span className={`font-mono text-[10px] font-medium uppercase tracking-wider ${categoryColors[post.category]}`}>
                    {post.category}
                  </span>
                  <span className="text-[10px] text-muted-foreground">&middot; {post.date}</span>
                </div>
                <h3 className="font-heading text-base font-semibold text-foreground leading-snug group-hover:text-primary transition-colors">
                  {post.title}
                </h3>
                <p className="text-sm text-muted-foreground leading-relaxed line-clamp-2 flex-1">
                  {post.excerpt}
                </p>
                <div className="flex items-center justify-between pt-3 border-t border-border/50 mt-auto">
                  <div className="flex items-center gap-2">
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={post.author.avatar}
                      alt={post.author.name}
                      className="w-5 h-5 rounded-full object-cover"
                    />
                    <span className="text-xs text-muted-foreground">{post.author.name}</span>
                  </div>
                  <span className="text-xs text-muted-foreground">{post.readTime}</span>
                </div>
              </Link>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
