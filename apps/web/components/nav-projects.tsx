"use client"

import {
  SidebarGroup,
  SidebarGroupLabel,
} from "@/components/ui/sidebar"

export function NavProjects() {
  return (
    <SidebarGroup className="group-data-[collapsible=icon]:hidden">
      <SidebarGroupLabel>Projects</SidebarGroupLabel>
      <p className="px-2 pt-1 pb-1 text-[11px] text-muted-foreground/60">
        Coming soon
      </p>
    </SidebarGroup>
  )
}
