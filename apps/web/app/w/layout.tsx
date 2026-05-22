"use client"

import Link from "next/link"
import { usePathname, useRouter } from "next/navigation"
import { useEffect, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  LayoutDashboard,
  Plug01Icon,
  CommandIcon,
  Chatting01Icon,
  Settings02Icon,
  TimeScheduleIcon,
  DriveIcon,
  GridViewIcon,
  Chart01Icon,
  CreditCardIcon,
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
import { FullPageLoader } from "@/components/full-page-loader"
import { api } from "@/lib/api/client"
import { AuthProvider, useAuth } from "@/lib/auth/auth-context"
import { cn } from "@/lib/utils"

const navSections = [
  {
    label: "Workspace",
    items: [
      { label: "Dashboard", href: "/w", icon: LayoutDashboard },
      { label: "Sessions", href: "/w/sessions", icon: Chatting01Icon },
      {
        label: "Scheduled tasks",
        href: "/w/scheduled-tasks",
        icon: TimeScheduleIcon,
      },
    ],
  },
  {
    label: "Resources",
    items: [
      { label: "Drive", href: "/w/drive", icon: DriveIcon },
      { label: "Skills", href: "/w/skills", icon: CommandIcon },
      { label: "Apps", href: "/w/apps", icon: GridViewIcon },
    ],
  },
  {
    label: "Integrations",
    items: [{ label: "Connections", href: "/w/connections", icon: Plug01Icon }],
  },
  {
    label: "Admin",
    items: [
      { label: "Usage", href: "/w/usage", icon: Chart01Icon },
      { label: "Credits", href: "/w/credits", icon: CreditCardIcon },
      { label: "Settings", href: "/w/settings", icon: Settings02Icon },
    ],
  },
]

const footerLinks = [
  {
    label: "Support",
    href: "mailto:hello@usehivy.com",
    icon: CustomerService01Icon,
  },
  { label: "Get free credits", href: "#", icon: AwardIcon },
  { label: "Invite team members", href: "#", icon: UserAdd01Icon },
]

function GhostLogo({
  className,
  size = 24,
}: {
  className?: string
  size?: number
}) {
  return (
    <svg
      viewBox="0 0 640 640"
      width={size}
      height={size}
      fill="currentColor"
      className={className}
    >
      <path d="M63.7314 260.875C115.623 104.119 238.334 51.5019 291.736 44.0986C600.403 1.30772 662.211 304.136 543.862 460.66C441.808 595.633 262.075 620.78 154.214 585.754C59.2103 554.903 6.44755 433.92 63.7314 260.875Z" />
      <ellipse
        cx="318.5"
        cy="282"
        rx="45.5"
        ry="101"
        fill="var(--background)"
      />
      <ellipse
        cx="457.5"
        cy="282"
        rx="45.5"
        ry="101"
        fill="var(--background)"
      />
    </svg>
  )
}

function UserAvatar({
  name,
  email,
  size = 36,
}: {
  name: string
  email?: string
  size?: number
}) {
  const label = name || email || "User"
  const initials = label
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
    <AuthProvider>
      <WorkspaceGate>
        <SidebarProvider>
          <AppSidebar />
          <SidebarInset className="relative h-screen overflow-hidden">
            {/* Subtle top-right blur glow */}
            <div
              className="pointer-events-none absolute -top-32 -right-32 h-[500px] w-[500px] rounded-full opacity-30 blur-[120px]"
              style={{ backgroundColor: "var(--glow-right)" }}
            />
            <main className="relative z-10 flex h-full flex-1 flex-col overflow-y-auto p-6 md:p-8">
              {children}
            </main>
          </SidebarInset>
        </SidebarProvider>
      </WorkspaceGate>
    </AuthProvider>
  )
}

function WorkspaceGate({ children }: { children: React.ReactNode }) {
  const { user, activeOrg, isLoading } = useAuth()
  const router = useRouter()

  const needsOnboarding = activeOrg !== null && !activeOrg.onboarded

  useEffect(() => {
    if (needsOnboarding) {
      router.replace("/onboarding")
    }
  }, [needsOnboarding, router])

  if (isLoading || !user || needsOnboarding) {
    return <FullPageLoader description="Loading workspace" />
  }

  return <>{children}</>
}

function AppSidebar() {
  const pathname = usePathname()
  const router = useRouter()
  const queryClient = useQueryClient()
  const [isLoggingOut, setIsLoggingOut] = useState(false)
  const { user } = useAuth()
  const displayName =
    user?.name || user?.email?.split("@")[0] || "Workspace member"
  const displayEmail = user?.email || "Signed in"

  async function handleLogout() {
    if (isLoggingOut) return
    setIsLoggingOut(true)
    await api.POST("/auth/logout", { body: {} })
    queryClient.clear()
    router.replace("/auth/signin")
  }

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

      <SidebarContent className="gap-5 px-3 py-2">
        {navSections.map((section) => (
          <div key={section.label}>
            <p className="px-3 pb-1.5 text-[11px] font-semibold tracking-wide text-sidebar-foreground/40 uppercase">
              {section.label}
            </p>
            <SidebarMenu>
              {section.items.map((item) => {
                const active =
                  pathname === item.href ||
                  (item.href !== "/w" && pathname.startsWith(`${item.href}/`))

                return (
                  <SidebarMenuItem key={item.href}>
                    <SidebarMenuButton
                      render={<Link href={item.href} />}
                      isActive={active}
                      className={cn(
                        "h-11 cursor-pointer items-center gap-3 rounded-md font-display text-base",
                        active && "text-primary"
                      )}
                    >
                      <HugeiconsIcon icon={item.icon} size={16} />
                      <span>{item.label}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                )
              })}
            </SidebarMenu>
          </div>
        ))}
      </SidebarContent>

      <SidebarFooter className="gap-0 px-3 py-4">
        <SidebarMenu>
          {footerLinks.map((link) => (
            <SidebarMenuItem key={link.label}>
              <SidebarMenuButton
                render={<Link href={link.href} />}
                className={cn(
                  "h-11 cursor-pointer items-center gap-3 rounded-md font-display text-base"
                )}
              >
                <HugeiconsIcon icon={link.icon} size={16} />
                <span>{link.label}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
          <SidebarMenuItem>
            <SidebarMenuButton
              onClick={handleLogout}
              disabled={isLoggingOut}
              className={cn(
                "h-11 cursor-pointer items-center gap-3 rounded-md font-display text-base"
              )}
            >
              <HugeiconsIcon icon={Logout04Icon} size={16} />
              <span>{isLoggingOut ? "Signing out..." : "Sign out"}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>

        <div className="mt-4 flex items-center gap-3 rounded-md border border-sidebar-border/50 bg-sidebar-accent/30 px-2 py-1.5">
          <UserAvatar name={displayName} email={displayEmail} />
          <div className="min-w-0 flex-1">
            <p className="truncate font-display text-sm text-sidebar-foreground">
              {displayName}
            </p>
            <p className="truncate font-display text-[11px] text-sidebar-foreground/50">
              {displayEmail}
            </p>
          </div>
        </div>
      </SidebarFooter>
    </Sidebar>
  )
}
