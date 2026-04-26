"use client"

import * as React from "react"
import Link from "next/link"
import { AnimatePresence, motion } from "motion/react"
import { usePathname } from "next/navigation"
import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from "@/components/ui/sidebar"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowRight01Icon } from "@hugeicons/core-free-icons"

type NavItem = {
  title: string
  url: string
  icon: React.ReactNode
  isActive?: boolean
  items?: { title: string; url: string }[]
}

const EASE_OUT_QUART: [number, number, number, number] = [0.165, 0.84, 0.44, 1]

export function NavMain({ items }: { items: NavItem[] }) {
  return (
    <SidebarGroup>
      <SidebarGroupLabel>Platform</SidebarGroupLabel>
      <SidebarMenu>
        {items.map((item) => (
          <NavRow key={item.title} item={item} />
        ))}
      </SidebarMenu>
    </SidebarGroup>
  )
}

function NavRow({ item }: { item: NavItem }) {
  const pathname = usePathname()
  const hasChildren = (item.items?.length ?? 0) > 0
  const childActive = item.items?.some((s) => pathname === s.url) ?? false
  const [open, setOpen] = React.useState(Boolean(item.isActive || childActive))

  // Open the group when navigation lands on one of its children.
  React.useEffect(() => {
    if (childActive) setOpen(true)
  }, [childActive])

  if (!hasChildren) {
    return (
      <SidebarMenuItem>
        <SidebarMenuButton
          tooltip={item.title}
          isActive={pathname === item.url}
          render={<Link href={item.url} />}
        >
          {item.icon}
          <span>{item.title}</span>
        </SidebarMenuButton>
      </SidebarMenuItem>
    )
  }

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        tooltip={item.title}
        isActive={childActive}
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        {item.icon}
        <span>{item.title}</span>
        <motion.span
          aria-hidden
          animate={{ rotate: open ? 90 : 0 }}
          transition={{ duration: 0.18, ease: EASE_OUT_QUART }}
          className="ml-auto inline-flex"
        >
          <HugeiconsIcon icon={ArrowRight01Icon} strokeWidth={2} className="size-4" />
        </motion.span>
      </SidebarMenuButton>

      <AnimatePresence initial={false}>
        {open ? (
          <motion.div
            key="sub"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{
              height: { duration: 0.22, ease: EASE_OUT_QUART },
              opacity: { duration: 0.16, ease: EASE_OUT_QUART },
            }}
            style={{ overflow: "hidden" }}
          >
            <SidebarMenuSub>
              {item.items?.map((subItem) => (
                <SidebarMenuSubItem key={subItem.title}>
                  <SidebarMenuSubButton
                    isActive={pathname === subItem.url}
                    render={<Link href={subItem.url} />}
                  >
                    <span>{subItem.title}</span>
                  </SidebarMenuSubButton>
                </SidebarMenuSubItem>
              ))}
            </SidebarMenuSub>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </SidebarMenuItem>
  )
}
