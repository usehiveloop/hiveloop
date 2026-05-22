import Link from "next/link"
import { Button } from "@/components/ui/button"
import { MarketingFooter } from "./footer"

type LegalSection = {
  id: string
  title: string
  body?: string[]
  list?: string[]
}

type RelatedLink = {
  label: string
  href: string
}

interface LegalPageProps {
  title: string
  eyebrow: string
  sections: LegalSection[]
  effectiveDate?: string
  lastUpdated?: string
  version?: string
  notice?: string
  relatedLinks?: RelatedLink[]
}

export function LegalPage({
  title,
  eyebrow,
  sections,
  effectiveDate,
  lastUpdated,
  version,
  notice,
  relatedLinks,
}: LegalPageProps) {
  return (
    <main className="relative flex min-h-screen flex-col items-center overflow-x-hidden bg-background font-display text-foreground">
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="absolute -top-52 -left-28 h-[28rem] w-[28rem] rounded-full bg-[var(--glow-left)] opacity-55 blur-[140px]" />
        <div className="absolute -top-40 left-1/2 h-[28rem] w-[28rem] -translate-x-1/2 rounded-full bg-[var(--glow-center)] opacity-50 blur-[140px]" />
        <div className="absolute -top-52 -right-28 h-[28rem] w-[28rem] rounded-full bg-[var(--glow-right)] opacity-50 blur-[140px]" />
      </div>

      <div className="fixed top-5 right-0 left-0 z-50 mx-auto flex max-w-5xl items-center justify-between px-4 md:px-0">
        <Link
          href="/"
          className="font-heading text-xl font-bold tracking-tight text-foreground"
        >
          hivy
        </Link>
        <nav className="absolute top-1/2 left-1/2 hidden h-11 -translate-x-1/2 -translate-y-1/2 items-center rounded-full border border-[var(--nav-border)] bg-[var(--nav-bg)] px-2 text-sm backdrop-blur-lg md:flex">
          <Link
            href="/legal"
            className="rounded-full px-3 py-2 text-muted-foreground transition-colors hover:text-foreground"
          >
            Legal
          </Link>
          <Link
            href="/terms"
            className="rounded-full px-3 py-2 text-muted-foreground transition-colors hover:text-foreground"
          >
            Terms
          </Link>
        </nav>
        <div className="flex items-center gap-2 sm:gap-3">
          <div className="hidden sm:block">
            <Button variant="ghost" size="sm" render={<a href="mailto:hello@usehivy.com" />}>
              Contact
            </Button>
          </div>
          <Button size="sm" render={<a href="/auth/signup" />}>
            Hire hivy
          </Button>
        </div>
      </div>

      <article className="relative z-10 w-full max-w-3xl px-4 pt-36 pb-24 sm:px-6 sm:pt-44 lg:pt-52">
        <div className="mb-16 sm:mb-20">
          <p className="mb-4 font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
            {eyebrow}
          </p>
          <h1 className="font-heading text-5xl font-normal leading-[1.02] tracking-tight text-foreground sm:text-6xl lg:text-7xl">
            {title}
          </h1>
          {(effectiveDate || lastUpdated || version) ? (
            <div className="mt-8 grid gap-2 border-y border-border py-5 font-mono text-[11px] uppercase tracking-[1.3px] text-muted-foreground sm:grid-cols-3">
              {effectiveDate ? <p>Effective: {effectiveDate}</p> : null}
              {lastUpdated ? <p>Updated: {lastUpdated}</p> : null}
              {version ? <p>Version: {version}</p> : null}
            </div>
          ) : null}
          {notice ? (
            <p className="mt-6 text-sm leading-7 text-muted-foreground">
              {notice}
            </p>
          ) : null}
          {relatedLinks?.length ? (
            <div className="mt-8 flex flex-wrap gap-2">
              {relatedLinks.map((link) => (
                <Link
                  key={link.href}
                  href={link.href}
                  className="rounded-full border border-border px-3 py-1.5 text-xs text-muted-foreground transition-colors hover:text-foreground"
                >
                  {link.label}
                </Link>
              ))}
            </div>
          ) : null}
        </div>

        <div className="space-y-14 sm:space-y-16">
          {sections.map((section, index) => (
            <section key={section.id} id={section.id} className="scroll-mt-28">
              <div className="mb-5 flex items-baseline gap-4">
                <span className="font-mono text-[11px] font-medium tracking-[1.5px] text-primary">
                  {String(index + 1).padStart(2, "0")}
                </span>
                <h2 className="font-heading text-2xl font-semibold leading-tight tracking-tight text-foreground sm:text-3xl">
                  {section.title}
                </h2>
              </div>
              <div className="space-y-5 break-words text-base leading-8 text-muted-foreground">
                {section.body?.map((paragraph) => (
                  <p key={paragraph}>{paragraph}</p>
                ))}
                {section.list ? (
                  <ul className="space-y-3 pl-5">
                    {section.list.map((item) => (
                      <li key={item} className="list-disc pl-2">
                        {item}
                      </li>
                    ))}
                  </ul>
                ) : null}
              </div>
            </section>
          ))}
        </div>
      </article>

      <MarketingFooter />
    </main>
  )
}
