"use client"

import Link from "next/link"
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
  useSidebar,
} from "@/components/ui/sidebar"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  CreditCardIcon,
  LogoutIcon,
  Settings05Icon,
  UnfoldMoreIcon,
} from "@hugeicons/core-free-icons"
import { useAuth } from "@/lib/auth/auth-context"

function initials(name?: string | null) {
  if (!name) return "?"
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return "?"
  if (parts.length === 1) return parts[0].charAt(0).toUpperCase()
  return (parts[0].charAt(0) + parts[parts.length - 1].charAt(0)).toUpperCase()
}

export function NavUser() {
  const { user, logout } = useAuth()
  const { isMobile } = useSidebar()

  const name = user?.name ?? user?.email ?? "Account"
  const email = user?.email ?? ""

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <SidebarMenuButton size="lg" className="aria-expanded:bg-muted" />
            }
          >
            <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary/15 font-mono text-[11px] font-medium text-primary">
              {initials(name)}
            </div>
            <div className="grid flex-1 text-left text-sm leading-tight">
              <span className="truncate font-medium">{name}</span>
              {email ? (
                <span className="truncate text-xs text-muted-foreground">
                  {email}
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
            side={isMobile ? "bottom" : "right"}
            align="end"
            sideOffset={4}
          >
            <DropdownMenuLabel className="p-0 font-normal">
              <div className="flex items-center gap-2 px-1 py-1.5 text-left text-sm">
                <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary/15 font-mono text-[11px] font-medium text-primary">
                  {initials(name)}
                </div>
                <div className="grid flex-1 text-left text-sm leading-tight">
                  <span className="truncate font-medium">{name}</span>
                  {email ? (
                    <span className="truncate text-xs text-muted-foreground">
                      {email}
                    </span>
                  ) : null}
                </div>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem
                render={<Link href="/w/settings/general" />}
              >
                <HugeiconsIcon icon={Settings05Icon} strokeWidth={2} />
                Settings
              </DropdownMenuItem>
              <DropdownMenuItem
                render={<Link href="/w/settings/billing" />}
              >
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
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
