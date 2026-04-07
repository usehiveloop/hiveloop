"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { Logo } from "@/components/logo"
import { Button } from "@/components/ui/button"

const navLinks = [
  { label: "Blog", href: "/blog" },
  { label: "Docs", href: "/docs" },
  { label: "Pricing", href: "/pricing" },
  { label: "Marketplace", href: "/marketplace" },
]

export function MarketingNav() {
  const pathname = usePathname()

  return (
    <nav className="w-full h-16 flex items-center justify-between max-w-424 mx-auto sticky top-0 bg-background/80 backdrop-blur-xl z-100 px-4 lg:px-0">
      <Link href="/"><Logo className="h-8" /></Link>
      <div className="hidden md:flex items-center gap-6 lg:gap-9">
        {navLinks.map((link) => {
          const isActive = pathname === link.href || pathname.startsWith(`${link.href}/`)
          return (
            <Link
              key={link.href}
              href={link.href}
              className={`text-sm font-medium transition-colors ${
                isActive ? "text-foreground" : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {link.label}
            </Link>
          )
        })}
      </div>
      <Link href="/auth">
        <Button variant="outline" size="sm">Sign in</Button>
      </Link>
    </nav>
  )
}
