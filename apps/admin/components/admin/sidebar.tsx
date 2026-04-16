"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { useAuth } from "@/lib/auth/auth-context"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { Button } from "@/components/ui/button"

const sections = [
  {
    label: "Overview",
    items: [{ label: "Dashboard", href: "/dashboard" }],
  },
  {
    label: "Users & Orgs",
    items: [
      { label: "Users", href: "/dashboard/users" },
      { label: "Organizations", href: "/dashboard/orgs" },
    ],
  },
  {
    label: "Credentials & Access",
    items: [
      { label: "Credentials", href: "/dashboard/credentials" },
      { label: "API Keys", href: "/dashboard/api-keys" },
      { label: "Tokens", href: "/dashboard/tokens" },
    ],
  },
  {
    label: "Agents & Sandboxes",
    items: [
      { label: "Agents", href: "/dashboard/agents" },
      { label: "Skills", href: "/dashboard/skills" },
      { label: "Sandboxes", href: "/dashboard/sandboxes" },
      { label: "Templates", href: "/dashboard/sandbox-templates" },
      { label: "Conversations", href: "/dashboard/conversations" },
      { label: "Forge Runs", href: "/dashboard/forge-runs" },
    ],
  },
  {
    label: "Integrations",
    items: [
      { label: "Platform Integrations", href: "/dashboard/in-integrations" },
    ],
  },
  {
    label: "Observability",
    items: [
      { label: "Generations", href: "/dashboard/generations" },
      { label: "Audit Log", href: "/dashboard/audit" },
      { label: "Admin Audit", href: "/dashboard/admin-audit" },
    ],
  },
  {
    label: "Infrastructure",
    items: [
      { label: "Custom Domains", href: "/dashboard/custom-domains" },
      { label: "Workspace Storage", href: "/dashboard/workspace-storage" },
    ],
  },
]

export function AdminSidebar() {
  const pathname = usePathname()
  const { logout } = useAuth()

  return (
    <Sidebar>
      <SidebarHeader>
        <Link
          href="/dashboard"
          className="flex items-center gap-2 px-2 py-1 text-lg font-semibold"
        >
          Zeus Admin
        </Link>
      </SidebarHeader>
      <SidebarContent>
        {sections.map((section) => (
          <SidebarGroup key={section.label}>
            <SidebarGroupLabel>{section.label}</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {section.items.map((item) => {
                  const active =
                    item.href === "/dashboard"
                      ? pathname === "/dashboard"
                      : pathname.startsWith(item.href)
                  return (
                    <SidebarMenuItem key={item.href}>
                      <SidebarMenuButton asChild isActive={active}>
                        <Link href={item.href}>{item.label}</Link>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  )
                })}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ))}
      </SidebarContent>
      <SidebarFooter>
        <Button variant="outline" size="sm" className="w-full" onClick={logout}>
          Sign out
        </Button>
      </SidebarFooter>
    </Sidebar>
  )
}
