import Link from "next/link"

const linkGroups = [
  {
    title: "Product",
    links: [
      { label: "Features", href: "/" },
      { label: "Pricing", href: "/pricing" },
      { label: "Security", href: "/legal#security-addendum" },
      { label: "Changelog", href: "#" },
    ],
  },
  {
    title: "Company",
    links: [
      { label: "About", href: "#" },
      { label: "Blog", href: "#" },
      { label: "Careers", href: "#" },
      { label: "Contact", href: "#" },
    ],
  },
  {
    title: "Resources",
    links: [
      { label: "Documentation", href: "#" },
      { label: "API Reference", href: "#" },
      { label: "Community", href: "#" },
      { label: "Status", href: "#" },
    ],
  },
  {
    title: "Legal",
    links: [
      { label: "Terms", href: "/terms" },
      { label: "Legal", href: "/legal" },
      { label: "DPA", href: "/legal#data-processing-addendum" },
      { label: "Subprocessors", href: "/legal#subprocessor-list" },
      { label: "Cookies", href: "/legal#cookie-notice" },
    ],
  },
]

export function MarketingFooter() {
  return (
    <footer className="relative z-10 w-full">
      <div className="mx-auto max-w-5xl px-4 py-16 sm:px-0">
        <div className="grid grid-cols-2 gap-8 sm:grid-cols-4">
          {linkGroups.map((group) => (
            <div key={group.title}>
              <div className="mb-4 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
                {group.title}
              </div>
              <div className="flex flex-col gap-3">
                {group.links.map((link) => (
                  <Link
                    key={link.label}
                    href={link.href}
                    className="text-sm text-foreground transition-colors hover:opacity-70"
                  >
                    {link.label}
                  </Link>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
      <div className="overflow-hidden">
        <div
          className="flex items-start justify-center pt-8 sm:pt-12"
          style={{ height: "clamp(100px, 15vw, 680px)" }}
        >
          <span
            className="font-heading font-bold tracking-tighter text-foreground"
            style={{ fontSize: "clamp(120px, 18vw, 800px)", lineHeight: 1 }}
          >
            hire hivy
          </span>
        </div>
      </div>
    </footer>
  )
}
