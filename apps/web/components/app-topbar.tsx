"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  CreditCardIcon,
  LogoutIcon,
  Settings05Icon,
} from "@hugeicons/core-free-icons"
import { Logo } from "@/components/logo"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { HeaderWorkspaceSwitcher } from "@/components/workspace-switcher"
import { useAuth } from "@/lib/auth/auth-context"
import { cn } from "@/lib/utils"

const PRIMARY_NAV = [
  { label: "Dashboard", href: "/w", match: "/w" },
  { label: "Connections", href: "/w/connections", match: "/w/connections" },
  { label: "Knowledge", href: "/w/knowledge", match: "/w/knowledge" },
  { label: "Skills", href: "/w/skills", match: "/w/skills" },
  { label: "Sessions", href: "/w/sessions", match: "/w/sessions" },
  { label: "Settings", href: "/w/settings/general", match: "/w/settings" },
]

function initials(name?: string | null) {
  if (!name) return "?"
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return "?"
  if (parts.length === 1) return parts[0].charAt(0).toUpperCase()
  return (parts[0].charAt(0) + parts[parts.length - 1].charAt(0)).toUpperCase()
}

export function AppTopbar({
  showPrimaryNav = true,
  onboarding = false,
  logoHref = "/w",
}: {
  showPrimaryNav?: boolean
  onboarding?: boolean
  logoHref?: string
}) {
  const pathname = usePathname()
  const { logout } = useAuth()

  return (
    <>
      <header className="sticky top-0 z-30 flex w-full items-center justify-between gap-4 border-b border-border/60 bg-background/90 px-6 py-4 backdrop-blur sm:px-10">
        <div className="flex min-w-0 items-center gap-4">
          <Link href={logoHref} aria-label="Hivy home" className="shrink-0">
            <Logo className="h-8" />
          </Link>
          <HeaderWorkspaceSwitcher className="hidden sm:flex" />
          {showPrimaryNav ? (
            <nav
              aria-label="Primary"
              className="ml-1 hidden items-center gap-1 lg:flex"
            >
              {PRIMARY_NAV.map((item) => {
                const active =
                  pathname === item.href ||
                  (item.match !== "/w" && pathname.startsWith(`${item.match}/`))
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    aria-current={active ? "page" : undefined}
                    className={cn(
                      "rounded-full px-3 py-1.5 text-sm font-medium transition-colors",
                      active
                        ? "bg-muted text-foreground"
                        : "text-muted-foreground hover:bg-muted/60 hover:text-foreground"
                    )}
                  >
                    {item.label}
                  </Link>
                )
              })}
            </nav>
          ) : null}
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            render={<a href="mailto:support@usehivy.com" />}
          >
            Support
          </Button>
          {onboarding ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => logout()}
            >
              Log out
            </Button>
          ) : (
            <>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Settings"
                render={<Link href="/w/settings/general" />}
              >
                <HugeiconsIcon icon={Settings05Icon} strokeWidth={2} />
              </Button>
              <HeaderAccountMenu />
            </>
          )}
        </div>
      </header>

      <div className="border-b border-border/60 px-6 py-2 sm:hidden">
        <HeaderWorkspaceSwitcher className="w-full max-w-full" />
      </div>

      {showPrimaryNav ? (
        <nav
          aria-label="Primary"
          className="flex items-center gap-1 overflow-x-auto border-b border-border/60 px-6 py-2 lg:hidden"
        >
          {PRIMARY_NAV.map((item) => {
            const active =
              pathname === item.href ||
              (item.match !== "/w" && pathname.startsWith(`${item.match}/`))
            return (
              <Link
                key={item.href}
                href={item.href}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "whitespace-nowrap rounded-full px-3 py-1.5 text-sm font-medium transition-colors",
                  active
                    ? "bg-muted text-foreground"
                    : "text-muted-foreground hover:bg-muted/60 hover:text-foreground"
                )}
              >
                {item.label}
              </Link>
            )
          })}
        </nav>
      ) : null}
    </>
  )
}

function HeaderAccountMenu() {
  const { user, logout } = useAuth()
  const name = user?.name ?? user?.email ?? "Account"
  const email = user?.email ?? ""

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="sm"
            className="h-9 gap-2 rounded-full px-2"
            aria-label="Account menu"
          />
        }
      >
        <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-primary/15 font-mono text-[11px] font-medium text-primary">
          {initials(name)}
        </span>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        className="min-w-64"
        side="bottom"
        align="end"
        sideOffset={8}
      >
        <DropdownMenuGroup>
          <DropdownMenuLabel className="p-0 font-normal">
            <div className="flex items-center gap-2 px-2 py-2 text-left text-sm">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary/15 font-mono text-[11px] font-medium text-primary">
                {initials(name)}
              </span>
              <span className="grid min-w-0 flex-1 leading-tight">
                <span className="truncate font-medium">{name}</span>
                {email ? (
                  <span className="truncate text-xs text-muted-foreground">
                    {email}
                  </span>
                ) : null}
              </span>
            </div>
          </DropdownMenuLabel>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem render={<Link href="/w/settings/general" />}>
            <HugeiconsIcon icon={Settings05Icon} strokeWidth={2} />
            Settings
          </DropdownMenuItem>
          <DropdownMenuItem render={<Link href="/w/settings/billing" />}>
            <HugeiconsIcon icon={CreditCardIcon} strokeWidth={2} />
            Billing
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => logout()}>
          <HugeiconsIcon icon={LogoutIcon} strokeWidth={2} />
          Log out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
