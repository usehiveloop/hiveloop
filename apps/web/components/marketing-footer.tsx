import Link from "next/link"
import { Logo } from "@/components/logo"
import { Button } from "@/components/ui/button"

const footerLinks = {
  product: [
    { label: "Marketplace", href: "/marketplace" },
    { label: "Pricing", href: "/pricing" },
    { label: "Documentation", href: "/docs" },
    { label: "TypeScript SDK", href: "/docs/sdk" },
    { label: "API Reference", href: "/docs/api" },
  ],
  resources: [
    { label: "Blog", href: "/blog" },
    { label: "Changelog", href: "/changelog" },
    { label: "Community", href: "https://discord.gg/hiveloop", external: true },
    { label: "GitHub", href: "https://github.com/hiveloop", external: true },
    { label: "Self-hosting", href: "/docs/self-hosting" },
  ],
  company: [
    { label: "About", href: "/about" },
    { label: "Privacy Policy", href: "/legal/privacy" },
    { label: "Terms of Service", href: "/legal/terms" },
    { label: "Contact", href: "mailto:hello@hiveloop.com", external: true },
  ],
}

const socialLinks = [
  {
    label: "GitHub",
    href: "https://github.com/hiveloop",
    icon: (
      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
        <title>github</title>
        <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
      </svg>
    ),
  },
  {
    label: "X",
    href: "https://x.com/hiveloop",
    icon: (
      <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor">
        <title>x</title>
        <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z" />
      </svg>
    ),
  },
  {
    label: "Discord",
    href: "https://discord.gg/hiveloop",
    icon: (
      <svg className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
        <title>discord</title>
        <path d="M20.317 4.37a19.791 19.791 0 00-4.885-1.515.074.074 0 00-.079.037c-.21.375-.444.864-.608 1.25a18.27 18.27 0 00-5.487 0 12.64 12.64 0 00-.617-1.25.077.077 0 00-.079-.037A19.736 19.736 0 003.677 4.37a.07.07 0 00-.032.027C.533 9.046-.32 13.58.099 18.057a.082.082 0 00.031.057 19.9 19.9 0 005.993 3.03.078.078 0 00.084-.028c.462-.63.874-1.295 1.226-1.994a.076.076 0 00-.041-.106 13.107 13.107 0 01-1.872-.892.077.077 0 01-.008-.128 10.2 10.2 0 00.372-.292.074.074 0 01.077-.01c3.928 1.793 8.18 1.793 12.062 0a.074.074 0 01.078.01c.12.098.246.198.373.292a.077.077 0 01-.006.127 12.299 12.299 0 01-1.873.892.077.077 0 00-.041.107c.36.698.772 1.362 1.225 1.993a.076.076 0 00.084.028 19.839 19.839 0 006.002-3.03.077.077 0 00.032-.054c.5-5.177-.838-9.674-3.549-13.66a.061.061 0 00-.031-.03zM8.02 15.33c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.956-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.956 2.418-2.157 2.418zm7.975 0c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.955-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.946 2.418-2.157 2.418z" />
      </svg>
    ),
  },
]

export function MarketingFooter() {
  return (
    <footer className="w-full relative overflow-hidden">
      {/* Top CTA band */}
      <div className="w-full border-t border-border bg-muted/40">
        <div className="w-full max-w-424 mx-auto px-4 lg:px-0 py-10 sm:py-14">
          <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-6">
            <div className="flex flex-col gap-1.5">
              <span className="font-heading text-lg sm:text-xl font-semibold text-foreground">
                Ready to replace your agent subscriptions?
              </span>
              <span className="text-sm text-muted-foreground">
                Start free. Deploy your first agent in minutes.
              </span>
            </div>
            <div className="flex items-center gap-3 shrink-0">
              <Link href="/auth">
                <Button size="lg">Get started</Button>
              </Link>
              <Link href="/docs">
                <Button variant="outline" size="lg">
                  View docs
                </Button>
              </Link>
            </div>
          </div>
        </div>
      </div>

      {/* Main footer */}
      <div className="w-full border-t border-border">
        <div className="w-full max-w-424 mx-auto px-4 lg:px-0 py-14 sm:py-16">
          <div className="grid grid-cols-2 gap-y-10 gap-x-8 sm:grid-cols-12">
            {/* Brand — full width on mobile, 5 cols on desktop */}
            <div className="col-span-2 sm:col-span-5 flex flex-col gap-6 pr-0 sm:pr-8">
              <Logo className="h-7" />
              <p className="text-[13px] text-muted-foreground leading-relaxed max-w-80">
                Build, run, and monitor production-grade AI agents. Bring your
                own keys, pick your models, install from the marketplace or
                create from scratch.
              </p>
              <div className="flex items-center gap-0.5">
                {socialLinks.map((social) => (
                  <a
                    key={social.label}
                    href={social.href}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center justify-center w-9 h-9 rounded-xl text-muted-foreground hover:text-foreground hover:bg-foreground/[0.05] transition-colors"
                    aria-label={social.label}
                  >
                    {social.icon}
                  </a>
                ))}
              </div>
            </div>

            {/* Link columns — 2 cols each on a 12 col grid, leave 1 col gap */}
            <div className="col-span-1 sm:col-span-2 sm:col-start-7">
              <FooterColumn title="Product" links={footerLinks.product} />
            </div>
            <div className="col-span-1 sm:col-span-2">
              <FooterColumn title="Resources" links={footerLinks.resources} />
            </div>
            <div className="col-span-1 sm:col-span-2">
              <FooterColumn title="Company" links={footerLinks.company} />
            </div>
          </div>
        </div>
      </div>

      {/* Bottom bar */}
      <div className="w-full border-t border-border">
        <div className="w-full max-w-424 mx-auto px-4 lg:px-0 py-5 flex flex-col-reverse sm:flex-row items-center justify-between gap-4">
          <div className="flex items-center gap-4">
            <span className="text-xs text-muted-foreground">
              &copy; {new Date().getFullYear()} HiveLoop Inc.
            </span>
            <span className="hidden sm:inline text-border">|</span>
            <a
              href="https://status.hiveloop.com"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors"
            >
              <span className="relative flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75" />
                <span className="relative inline-flex rounded-full h-2 w-2 bg-green-500" />
              </span>
              All systems operational
            </a>
          </div>
          <div className="flex items-center gap-5">
            <Link
              href="/legal/privacy"
              className="text-xs text-muted-foreground hover:text-foreground transition-colors"
            >
              Privacy
            </Link>
            <Link
              href="/legal/terms"
              className="text-xs text-muted-foreground hover:text-foreground transition-colors"
            >
              Terms
            </Link>
            <Link
              href="/docs/security"
              className="text-xs text-muted-foreground hover:text-foreground transition-colors"
            >
              Security
            </Link>
          </div>
        </div>
      </div>
    </footer>
  )
}

interface FooterLink {
  label: string
  href: string
  external?: boolean
}

function FooterColumn({
  title,
  links,
}: {
  title: string
  links: FooterLink[]
}) {
  return (
    <div className="flex flex-col gap-3.5">
      <span className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-foreground">
        {title}
      </span>
      <div className="flex flex-col gap-2">
        {links.map((link) =>
          link.external ? (
            <a
              key={link.label}
              href={link.href}
              target="_blank"
              rel="noopener noreferrer"
              className="text-[13px] text-muted-foreground hover:text-foreground transition-colors w-fit"
            >
              {link.label}
            </a>
          ) : (
            <Link
              key={link.label}
              href={link.href}
              className="text-[13px] text-muted-foreground hover:text-foreground transition-colors w-fit"
            >
              {link.label}
            </Link>
          ),
        )}
      </div>
    </div>
  )
}
