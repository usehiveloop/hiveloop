"use client"

import * as React from "react"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  Tick02Icon,
  UnfoldMoreIcon,
} from "@hugeicons/core-free-icons"
import { useAuth } from "@/lib/auth/auth-context"
import { CreateWorkspaceDialog } from "@/components/create-workspace-dialog"

function initial(name?: string) {
  return name?.trim().charAt(0).toUpperCase() || "?"
}

export function WorkspaceSwitcher() {
  const { orgs, activeOrg, setActiveOrg } = useAuth()
  const [createOpen, setCreateOpen] = React.useState(false)

  return (
    <>
      <SidebarMenu>
        <SidebarMenuItem>
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <SidebarMenuButton
                  size="lg"
                  className="aria-expanded:bg-muted"
                />
              }
            >
              <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary font-mono text-sm text-sidebar-primary-foreground">
                {initial(activeOrg?.name)}
              </div>
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-medium">
                  {activeOrg?.name ?? "Select workspace"}
                </span>
                {activeOrg?.role ? (
                  <span className="truncate text-xs capitalize text-muted-foreground">
                    {activeOrg.role}
                  </span>
                ) : null}
              </div>
              <HugeiconsIcon
                icon={UnfoldMoreIcon}
                strokeWidth={2}
                className="ml-auto size-4"
              />
            </DropdownMenuTrigger>
            <DropdownMenuContent
              className="min-w-56"
              side="bottom"
              align="start"
              sideOffset={4}
            >
              <DropdownMenuGroup>
                <DropdownMenuLabel className="text-muted-foreground text-xs">
                  Workspaces
                </DropdownMenuLabel>
                {orgs.map((org) => {
                  const isActive = org.id === activeOrg?.id
                  return (
                    <DropdownMenuItem
                      key={org.id ?? org.name}
                      onClick={() => setActiveOrg(org)}
                      className="gap-2 p-2"
                    >
                      <div className="flex size-6 items-center justify-center rounded-md border font-mono text-[11px] text-muted-foreground">
                        {initial(org.name)}
                      </div>
                      <span className="flex-1 truncate">{org.name}</span>
                      {isActive ? (
                        <HugeiconsIcon
                          icon={Tick02Icon}
                          strokeWidth={2}
                          className="size-4 text-primary"
                        />
                      ) : null}
                    </DropdownMenuItem>
                  )
                })}
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="gap-2 p-2"
                onClick={() => setCreateOpen(true)}
              >
                <div className="flex size-6 items-center justify-center rounded-md border bg-background">
                  <HugeiconsIcon
                    icon={Add01Icon}
                    strokeWidth={2}
                    className="size-4"
                  />
                </div>
                <span className="text-muted-foreground font-medium">
                  Add workspace
                </span>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </SidebarMenuItem>
      </SidebarMenu>

      <CreateWorkspaceDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
      />
    </>
  )
}
