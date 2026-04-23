import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { getAllPosts } from "@/lib/source"
import type { Metadata } from "next"

// All post metadata is read at build time from MDX frontmatter via the
// fumadocs source. Adding a new post = dropping an .mdx file in
// content/blog. The blog index automatically picks it up.

const categoryColors: Record<string, string> = {
  Product: "text-primary",
  Engineering: "text-blue-500",
  Guides: "text-green-500",
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  })
}

export const metadata: Metadata = {
  title: "Blog — HiveLoop",
  description:
    "Engineering deep dives, product updates, and tutorials from the HiveLoop team.",
  alternates: { canonical: "/blog" },
  openGraph: {
    title: "Blog — HiveLoop",
    description:
      "Engineering deep dives, product updates, and tutorials from the HiveLoop team.",
    url: "/blog",
    type: "website",
    siteName: "HiveLoop",
  },
}

export default function BlogIndex() {
  const allPosts = getAllPosts().map((post) => ({
    slug: post.slug,
    title: post.data.title,
    description: post.data.description,
    date: post.data.date,
    author: post.data.author,
    authorAvatar: post.data.authorAvatar,
    category: post.data.category,
    readTime: post.data.readTime,
    url: `/blog/${post.slug}`,
  }))
  const featured = allPosts[0]
  const rest = allPosts.slice(1)

  return (
    <div className="w-full bg-background flex flex-col relative min-h-screen">
      <div className="max-w-6xl mx-auto w-full px-4 pb-24">
        {/* Header */}
        <div className="flex flex-col gap-2 pt-12 sm:pt-20 pb-10 sm:pb-14">
          <p className="font-mono text-mini font-medium uppercase tracking-medium text-primary">
            Blog
          </p>
          <h1 className="font-heading text-[28px] sm:text-[36px] lg:text-[44px] font-bold text-foreground leading-[1.15] -tracking-micro">
            Stories from the team
          </h1>
        </div>

        {/* Featured post — only renders when at least one post exists */}
        {featured && (
          <Link href={featured.url} className="group block mb-12 sm:mb-16">
            <div className="relative rounded-3xl overflow-hidden border border-border hover:border-primary/40 transition-colors">
              <div
                className="absolute inset-0 pointer-events-none"
                style={{
                  background:
                    "radial-gradient(ellipse at 30% 50%, color-mix(in oklch, var(--primary) 10%, transparent) 0%, transparent 70%)",
                }}
              />
              <div
                className="absolute inset-0 pointer-events-none"
                style={{
                  backgroundImage:
                    "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
                  backgroundSize: "60px 60px",
                  maskImage:
                    "radial-gradient(ellipse at 30% 50%, black 10%, transparent 60%)",
                }}
              />

              <div className="relative p-8 sm:p-12 lg:p-16 flex flex-col gap-5 max-w-3xl">
                <div className="flex items-center gap-3">
                  <span
                    className={`font-mono text-mini font-medium uppercase tracking-wider ${
                      categoryColors[featured.category] ?? "text-primary"
                    }`}
                  >
                    {featured.category}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {formatDate(featured.date)}
                  </span>
                </div>
                <h2 className="font-heading text-[24px] sm:text-[32px] lg:text-[40px] font-bold text-foreground leading-[1.15] -tracking-micro group-hover:text-primary transition-colors">
                  {featured.title}
                </h2>
                <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-2xl">
                  {featured.description}
                </p>
                <div className="flex items-center gap-3 pt-2">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={featured.authorAvatar}
                    alt={featured.author}
                    className="w-8 h-8 rounded-full object-cover"
                  />
                  <div className="flex flex-col">
                    <span className="text-sm font-medium text-foreground">
                      {featured.author}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {featured.readTime} read
                    </span>
                  </div>
                </div>
              </div>
            </div>
          </Link>
        )}

        {/* Two-column grid — only renders when there are non-featured posts */}
        {rest.length > 0 && (
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 lg:gap-12">
            <div className="lg:col-span-7 flex flex-col gap-6">
              {rest.map((post) => (
                <Link
                  key={post.slug}
                  href={post.url}
                  className="group flex flex-col gap-4 rounded-2xl border border-border p-6 sm:p-8 hover:border-primary/40 transition-colors"
                >
                  <div className="flex items-center gap-3">
                    <span
                      className={`font-mono text-mini font-medium uppercase tracking-wider ${
                        categoryColors[post.category] ?? "text-primary"
                      }`}
                    >
                      {post.category}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {formatDate(post.date)}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      &middot; {post.readTime}
                    </span>
                  </div>
                  <h3 className="font-heading text-xl sm:text-2xl font-semibold text-foreground group-hover:text-primary transition-colors leading-snug">
                    {post.title}
                  </h3>
                  <p className="text-sm text-muted-foreground leading-relaxed">
                    {post.description}
                  </p>
                  <div className="flex items-center gap-2 pt-1">
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={post.authorAvatar}
                      alt={post.author}
                      className="w-5 h-5 rounded-full object-cover"
                    />
                    <span className="text-xs text-muted-foreground">
                      {post.author}
                    </span>
                  </div>
                </Link>
              ))}
            </div>
          </div>
        )}

        {/* Empty state when no posts beyond the featured one */}
        {!featured && (
          <div className="rounded-3xl border border-border p-12 text-center">
            <h2 className="font-heading text-2xl font-bold text-foreground">
              No posts yet
            </h2>
            <p className="text-muted-foreground mt-2">
              Check back soon. We&apos;re working on something good.
            </p>
          </div>
        )}

        {/* Newsletter */}
        <div className="mt-16 sm:mt-24 mb-16 sm:mb-24 relative rounded-3xl border border-border overflow-hidden">
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background:
                "radial-gradient(ellipse at 70% 50%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
            }}
          />
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              backgroundImage:
                "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
              backgroundSize: "40px 40px",
              maskImage:
                "radial-gradient(ellipse at 70% 50%, black 10%, transparent 50%)",
            }}
          />
          <div className="relative p-8 sm:p-12 lg:p-16 flex flex-col lg:flex-row lg:items-center gap-8 lg:gap-16">
            <div className="flex flex-col gap-3 lg:flex-1">
              <span className="font-mono text-mini font-medium uppercase tracking-medium text-primary">
                Newsletter
              </span>
              <h2 className="font-heading text-[22px] sm:text-[28px] font-bold text-foreground leading-[1.2] -tracking-micro">
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
      </div>
    </div>
  )
}
