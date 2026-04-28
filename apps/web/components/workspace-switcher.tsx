"use client"

import * as React from "react"
import Image from "next/image"

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

function OrgAvatar({
  name,
  logoUrl,
  size = 8,
}: {
  name?: string
  logoUrl?: string
  size?: 6 | 8
}) {
  const px = size === 6 ? 24 : 32
  const sizeClass = size === 6 ? "size-6" : "size-8"
  const radiusClass = "rounded-md"
  const fontClass = size === 6 ? "text-[11px]" : "text-sm"

  if (logoUrl) {
    return (
      <Image
        src={logoUrl}
        alt=""
        width={px}
        height={px}
        sizes={`${px}px`}
        className={`${sizeClass} ${radiusClass} aspect-square shrink-0 object-cover`}
      />
    )
  }
  return (
    <div
      className={`flex aspect-square ${sizeClass} ${radiusClass} shrink-0 items-center justify-center bg-sidebar-primary font-mono ${fontClass} text-sidebar-primary-foreground`}
    >
      {initial(name)}
    </div>
  )
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
              <OrgAvatar
                name={activeOrg?.name}
                logoUrl={activeOrg?.logo_url}
                size={8}
              />
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
                      <OrgAvatar
                        name={org.name}
                        logoUrl={org.logo_url}
                        size={6}
                      />
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
