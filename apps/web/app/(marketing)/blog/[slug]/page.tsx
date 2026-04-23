import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { findPostBySlug, getAllPosts } from "@/lib/source"
import { notFound } from "next/navigation"
import type { Metadata } from "next"
import { ReadingProgress } from "./reading-progress"

interface PageProps {
  params: Promise<{ slug: string }>
}

// Custom MDX components that style the post body to match the cinematic
// reading layout. Headings, paragraphs, code blocks, and blockquotes all
// override fumadocs's defaults so MDX content reads as a magazine-style
// long-form article rather than docs.
const proseComponents = {
  h1: (props: React.HTMLAttributes<HTMLHeadingElement>) => (
    <h1
      {...props}
      className="font-heading text-3xl sm:text-4xl font-bold text-foreground mt-12 -tracking-micro"
    />
  ),
  h2: (props: React.HTMLAttributes<HTMLHeadingElement>) => (
    <h2
      {...props}
      className="font-heading text-2xl sm:text-3xl font-bold text-foreground mt-10 -tracking-micro"
    />
  ),
  h3: (props: React.HTMLAttributes<HTMLHeadingElement>) => (
    <h3
      {...props}
      className="font-heading text-xl sm:text-2xl font-semibold text-foreground mt-8 -tracking-[0.3px]"
    />
  ),
  p: (props: React.HTMLAttributes<HTMLParagraphElement>) => (
    <p
      {...props}
      className="text-base sm:text-lg text-muted-foreground leading-[1.8] my-5"
    />
  ),
  ul: (props: React.HTMLAttributes<HTMLUListElement>) => (
    <ul
      {...props}
      className="my-5 ml-6 list-disc space-y-2 text-base sm:text-lg text-muted-foreground leading-[1.8]"
    />
  ),
  ol: (props: React.HTMLAttributes<HTMLOListElement>) => (
    <ol
      {...props}
      className="my-5 ml-6 list-decimal space-y-2 text-base sm:text-lg text-muted-foreground leading-[1.8]"
    />
  ),
  li: (props: React.HTMLAttributes<HTMLLIElement>) => <li {...props} className="leading-[1.8]" />,
  a: (props: React.AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a
      {...props}
      className="text-primary underline-offset-2 hover:underline"
    />
  ),
  strong: (props: React.HTMLAttributes<HTMLElement>) => (
    <strong {...props} className="font-semibold text-foreground" />
  ),
  em: (props: React.HTMLAttributes<HTMLElement>) => (
    <em {...props} className="italic" />
  ),
  blockquote: (props: React.HTMLAttributes<HTMLQuoteElement>) => (
    <blockquote
      {...props}
      className="my-8 sm:my-12 py-6 border-l-2 border-primary pl-6 sm:pl-8 -ml-2 [&>p]:font-heading [&>p]:text-xl sm:[&>p]:text-2xl [&>p]:font-semibold [&>p]:text-foreground [&>p]:leading-snug [&>p]:-tracking-[0.3px] [&>p]:my-0"
    />
  ),
  // Inline code (`code`) — small monospace pill inside paragraphs.
  code: (props: React.HTMLAttributes<HTMLElement>) => {
    const { className, ...rest } = props
    // Block-level code (inside <pre>) gets `language-...` className from MDX —
    // skip our pill styling so the pre/code combo renders as a code block.
    if (className && className.startsWith("language-")) {
      return <code {...props} className={`${className} font-mono text-sm-alt leading-relaxed text-white/80`} />
    }
    return (
      <code
        {...rest}
        className="font-mono text-sm bg-muted px-1.5 py-0.5 rounded"
      />
    )
  },
  // Block-level pre — terminal-style chrome matching the rest of the marketing pages.
  pre: (props: React.HTMLAttributes<HTMLPreElement> & { children?: React.ReactNode }) => {
    const { children, ...rest } = props
    // MDX may pass a `title` data attribute via fumadocs's code block plugin —
    // we read it for the filename pill, falling back to a generic label.
    const dataAttrs = rest as Record<string, unknown>
    const title = (dataAttrs["data-title"] as string) || (dataAttrs.title as string) || ""
    return (
      <div className="my-6 rounded-xl bg-[oklch(0.14_0.01_55)] border border-white/[0.06] overflow-hidden">
        <div className="flex items-center gap-2 px-4 py-2.5 border-b border-white/[0.06]">
          <span className="w-2.5 h-2.5 rounded-full bg-red-500/70" />
          <span className="w-2.5 h-2.5 rounded-full bg-yellow-500/70" />
          <span className="w-2.5 h-2.5 rounded-full bg-green-500/70" />
          {title && (
            <span className="flex-1 text-center font-mono text-2xs text-white/25">{title}</span>
          )}
        </div>
        <pre {...rest} className="p-4 sm:p-5 overflow-x-auto">
          {children}
        </pre>
      </div>
    )
  },
}

export default async function BlogPostPage(props: PageProps) {
  const { slug } = await props.params
  const post = findPostBySlug(slug)
  if (!post) notFound()

  const MDXContent = post.body
  const { title, description, date, author, authorAvatar, category, readTime } = post.data

  return (
    <div className="w-full bg-background flex flex-col relative min-h-screen">
      {/* Reading-progress bar — client component, tracks window scroll */}
      <ReadingProgress />

      {/* Hero */}
      <div className="relative w-full overflow-hidden">
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            background:
              "radial-gradient(ellipse at 50% 80%, color-mix(in oklch, var(--primary) 12%, transparent) 0%, transparent 60%)",
          }}
        />
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            backgroundImage:
              "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
            backgroundSize: "60px 60px",
            maskImage:
              "radial-gradient(ellipse at 50% 100%, black 10%, transparent 60%)",
          }}
        />
        <div className="relative max-w-3xl mx-auto px-4 pt-16 sm:pt-24 pb-16 sm:pb-20 flex flex-col items-center text-center gap-6">
          <span className="font-mono text-mini font-medium uppercase tracking-medium text-primary">
            {category}
          </span>
          <h1 className="font-heading text-[32px] sm:text-[44px] lg:text-[56px] font-bold text-foreground leading-[1.1] -tracking-small">
            {title}
          </h1>
          <p className="text-lg sm:text-xl text-muted-foreground leading-relaxed max-w-2xl">
            {description}
          </p>
          <div className="flex items-center gap-4 pt-4">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={authorAvatar} alt={author} className="w-10 h-10 rounded-full object-cover" />
            <div className="flex flex-col items-start">
              <span className="text-sm font-medium text-foreground">{author}</span>
              <span className="text-xs text-muted-foreground">
                {formatDate(date)} &middot; {readTime} read
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Article body */}
      <article className="max-w-2xl mx-auto px-4 pb-24">
        <div className="flex flex-col gap-1">
          <MDXContent components={proseComponents} />
        </div>

        {/* Author card */}
        <div className="mt-16 pt-8 border-t border-border flex items-center gap-4">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src={authorAvatar} alt={author} className="w-12 h-12 rounded-full object-cover" />
          <div className="flex flex-col">
            <span className="text-sm font-semibold text-foreground">{author}</span>
            <span className="text-sm text-muted-foreground">
              Founder at HiveLoop. Building the future of production AI agents.
            </span>
          </div>
        </div>

        {/* Newsletter */}
        <div className="mt-12 relative rounded-2xl border border-border overflow-hidden">
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background:
                "radial-gradient(ellipse at 70% 50%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
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
              <Input type="email" placeholder="you@company.com" className="h-10 sm:h-11 rounded-full text-sm px-5 flex-1" />
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

// Static-generation: pre-render every blog post at build time so the dynamic
// route still benefits from static delivery and instant TTFB.
export async function generateStaticParams() {
  return getAllPosts().map((post) => ({
    slug: post.slug,
  }))
}

// SEO metadata is generated per slug from the MDX frontmatter. Title,
// description, and Open Graph tags all flow from the file's front matter.
export async function generateMetadata(props: PageProps): Promise<Metadata> {
  const { slug } = await props.params
  const post = findPostBySlug(slug)
  if (!post) return {}

  const { title, description, date, author, authorAvatar, category } = post.data
  const url = `/blog/${slug}`

  return {
    title,
    description,
    authors: [{ name: author }],
    keywords: [category, "ai agents", "context engineering", "hiveloop"],
    openGraph: {
      title,
      description,
      type: "article",
      publishedTime: date,
      authors: [author],
      url,
      siteName: "HiveLoop",
      images: authorAvatar ? [{ url: authorAvatar }] : undefined,
    },
    twitter: {
      card: "summary_large_image",
      title,
      description,
      creator: `@${author.replace(/\s+/g, "").toLowerCase()}`,
    },
    alternates: {
      canonical: url,
    },
  }
}

// Format an ISO date as "Apr 11, 2026". Plain helper, no library.
function formatDate(iso: string): string {
  const date = new Date(iso)
  return date.toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  })
}
