"use client"

import Link from "next/link"
import { LogoMark } from "@/components/logo"
import { usePathname } from "next/navigation"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowDown01Icon,
  Tick02Icon,
  Add01Icon,
  Robot01Icon,
  Plug01Icon,
  Activity01Icon,
  Settings01Icon,
  Logout01Icon,
  CreditCardIcon,
  UserCircleIcon,
} from "@hugeicons/core-free-icons"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Button } from "@/components/ui/button"
import { AuthProvider, useAuth } from "@/lib/auth/auth-context"

const navItems = [
  { label: "Agents", href: "/w/agents", icon: Robot01Icon },
  { label: "Connections", href: "/w/connections", icon: Plug01Icon },
  { label: "Observe", href: "/w/observe", icon: Activity01Icon },
]

function NavItems() {
  const pathname = usePathname()

  return (
    <nav className="hidden items-center gap-1 md:flex">
      {navItems.map((item) => {
        const isActive = pathname.startsWith(item.href)
        return (
          <Button
            key={item.href}
            variant={isActive ? 'secondary' : "ghost"}
            size="sm"
            render={<Link href={item.href} className="flex items-center gap-2 px-1" />}
          >
            <HugeiconsIcon icon={item.icon} size={14} data-icon="inline-start" />
            {item.label}
          </Button>
        )
      })}
    </nav>
  )
}

function WorkspaceHeader() {
  const { user, orgs, activeOrg, setActiveOrg, logout } = useAuth()

  const initials = user?.name
    ? user.name
        .split(" ")
        .map((w) => w[0])
        .join("")
        .slice(0, 2)
        .toUpperCase()
    : "?"

  return (
    <header className="sticky top-0 z-50 flex h-[54px] shrink-0 items-center gap-3 border-b border-border bg-background px-4">
      <Link href="/w">
        <LogoMark className="h-6 w-6" />
      </Link>
      <span className="text-muted-foreground/30">/</span>

      <DropdownMenu>
        <DropdownMenuTrigger className="flex items-center gap-2 rounded-full px-3 py-1 transition-colors hover:bg-muted outline-none">
          <span className="text-sm font-medium text-foreground">
            {activeOrg?.name ?? "Select workspace"}
          </span>
          <HugeiconsIcon icon={ArrowDown01Icon} size={14} className="text-muted-foreground" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" sideOffset={8} className="min-w-60">
          <DropdownMenuGroup>
            <DropdownMenuLabel className="font-mono text-[10px] uppercase tracking-[1.5px]">Workspaces</DropdownMenuLabel>
            <DropdownMenuSeparator />
            {orgs.map((org) => (
              <DropdownMenuItem key={org.id} onClick={() => setActiveOrg(org)}>
                <span className="flex h-5 w-5 items-center justify-center rounded-md bg-muted font-mono text-xs text-muted-foreground">
                  {org.name?.[0]?.toUpperCase() ?? "?"}
                </span>
                {org.name}
                {org.id === activeOrg?.id && (
                  <HugeiconsIcon icon={Tick02Icon} size={14} className="ml-auto text-primary" />
                )}
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem>
            <HugeiconsIcon icon={Add01Icon} size={16} className="text-muted-foreground" />
            Create new workspace
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <NavItems />

      <div className="flex-1" />

      <DropdownMenu>
        <DropdownMenuTrigger className="flex items-center outline-none rounded-full transition-opacity hover:opacity-80">
          <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary/20 text-primary font-mono text-xs font-semibold">
            {initials}
          </div>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" sideOffset={8} className="min-w-56">
          <div className="px-3 py-3 border-b border-border">
            <p className="text-sm font-medium text-foreground">{user?.name ?? "User"}</p>
            <p className="text-xs text-muted-foreground">{user?.email}</p>
          </div>
          <DropdownMenuGroup>
            <DropdownMenuItem>
              <HugeiconsIcon icon={UserCircleIcon} size={16} className="text-muted-foreground" />
              Profile
            </DropdownMenuItem>
            <DropdownMenuItem>
              <HugeiconsIcon icon={Settings01Icon} size={16} className="text-muted-foreground" />
              Settings
            </DropdownMenuItem>
            <DropdownMenuItem>
              <HugeiconsIcon icon={CreditCardIcon} size={16} className="text-muted-foreground" />
              Billing
            </DropdownMenuItem>
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem className="text-muted-foreground" onClick={logout}>
            <HugeiconsIcon icon={Logout01Icon} size={16} />
            Sign out
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </header>
  )
}

export default function WorkspaceLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <AuthProvider>
      <div className="flex min-h-screen flex-col bg-background">
        <WorkspaceHeader />

        <main className="flex-1">{children}</main>

        <footer className="sticky bottom-0 z-50 flex h-9 shrink-0 items-center justify-center border-t border-border bg-background px-4">
          <span className="font-mono text-[10px] text-muted-foreground/50">ziraloop</span>
        </footer>
      </div>
    </AuthProvider>
  )
}
