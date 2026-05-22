"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  LayoutDashboard,
  Plug01Icon,
  BookOpen02Icon,
  CommandIcon,
  Chatting01Icon,
  Settings02Icon,
  CustomerService01Icon,
  AwardIcon,
  UserAdd01Icon,
  Logout04Icon,
} from "@hugeicons/core-free-icons"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from "@/components/ui/sidebar"
import { cn } from "@/lib/utils"

const navItems = [
  { label: "Dashboard", href: "/w-v2", icon: LayoutDashboard },
  { label: "Connections", href: "/w-v2/connections", icon: Plug01Icon },
  { label: "Knowledge", href: "/w-v2/knowledge", icon: BookOpen02Icon },
  { label: "Skills", href: "/w-v2/skills", icon: CommandIcon },
  { label: "Sessions", href: "/w-v2/sessions", icon: Chatting01Icon },
  { label: "Settings", href: "/w-v2/settings", icon: Settings02Icon },
]

const footerLinks = [
  { label: "Support", href: "mailto:hello@usehivy.com", icon: CustomerService01Icon },
  { label: "Get free credits", href: "#", icon: AwardIcon },
  { label: "Invite team members", href: "#", icon: UserAdd01Icon },
  { label: "Sign out", href: "#", icon: Logout04Icon },
]

function GhostLogo({ className, size = 24 }: { className?: string; size?: number }) {
  return (
    <svg
      viewBox="0 0 640 640"
      width={size}
      height={size}
      fill="currentColor"
      className={className}
    >
      <path d="M63.7314 260.875C115.623 104.119 238.334 51.5019 291.736 44.0986C600.403 1.30772 662.211 304.136 543.862 460.66C441.808 595.633 262.075 620.78 154.214 585.754C59.2103 554.903 6.44755 433.92 63.7314 260.875Z" />
      <ellipse cx="318.5" cy="282" rx="45.5" ry="101" fill="var(--background)" />
      <ellipse cx="457.5" cy="282" rx="45.5" ry="101" fill="var(--background)" />
    </svg>
  )
}

function UserAvatar({ name, size = 36 }: { name: string; size?: number }) {
  const initials = name
    .split(" ")
    .map((n) => n[0])
    .join("")
    .toUpperCase()
    .slice(0, 2)

  return (
    <div
      className="flex shrink-0 items-center justify-center rounded-md bg-primary/15 text-primary"
      style={{ width: size, height: size }}
    >
      <span className="text-[11px] font-semibold">{initials}</span>
    </div>
  )
}

export default function WorkspaceV2Layout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset className="relative overflow-hidden">
        {/* Subtle top-right blur glow */}
        <div
          className="pointer-events-none absolute -top-32 -right-32 h-[500px] w-[500px] rounded-full opacity-30 blur-[120px]"
          style={{ backgroundColor: "var(--glow-right)" }}
        />
        <main className="relative z-10 flex flex-1 flex-col p-6 md:p-8">
          {children}
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}

function AppSidebar() {
  const pathname = usePathname()

  return (
    <Sidebar
      collapsible="none"
      className="h-svh min-h-svh border-r border-border"
      style={{ width: 268, minWidth: 268 }}
    >
      <SidebarHeader>
        <div className="flex items-center gap-2.5 px-4 py-4">
          <GhostLogo className="text-primary" />
          <span className="font-heading text-lg font-medium tracking-tight text-sidebar-foreground">
            hivy
          </span>
        </div>
      </SidebarHeader>

      <SidebarContent className="px-3 py-2">
        <SidebarMenu>
          {navItems.map((item) => {
            const active =
              pathname === item.href ||
              (item.href !== "/w-v2" && pathname.startsWith(`${item.href}/`))

            return (
              <SidebarMenuItem key={item.href}>
                <SidebarMenuButton
                  asChild
                  isActive={active}
                  className={cn(
                    "h-11 cursor-pointer items-center gap-3 rounded-md font-display text-base",
                    active && "text-primary"
                  )}
                >
                  <Link href={item.href} className="flex w-full items-center gap-3">
                    <HugeiconsIcon icon={item.icon} size={16} />
                    <span>{item.label}</span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
            )
          })}
        </SidebarMenu>
      </SidebarContent>

      <SidebarFooter className="gap-0 px-3 py-4">
        <SidebarMenu>
          {footerLinks.map((link) => (
            <SidebarMenuItem key={link.label}>
              <SidebarMenuButton
                asChild
                className={cn(
                  "h-11 cursor-pointer items-center gap-3 rounded-md font-display text-base"
                )}
              >
                <Link href={link.href} className="flex w-full items-center gap-3">
                  <HugeiconsIcon icon={link.icon} size={16} />
                  <span>{link.label}</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
        </SidebarMenu>

        <div className="mt-4 flex items-center gap-3 rounded-md border border-sidebar-border/50 bg-sidebar-accent/30 px-2 py-1.5">
          <UserAvatar name="Alex Johnson" />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-display text-sidebar-foreground">
              Alex Johnson
            </p>
            <p className="truncate text-[11px] font-display text-sidebar-foreground/50">
              alex@acme.com
            </p>
          </div>
        </div>
      </SidebarFooter>
    </Sidebar>
  )
}
