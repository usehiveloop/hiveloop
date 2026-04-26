"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  CreditCardIcon,
  Plug01Icon,
  Settings01Icon,
  UserGroupIcon,
} from "@hugeicons/core-free-icons"
import { useAuth } from "@/lib/auth/auth-context"

const SECTIONS = [
  {
    label: "Workspace",
    items: [
      {
        title: "General",
        href: "/w/settings/general",
        icon: Settings01Icon,
      },
      {
        title: "Members",
        href: "/w/settings/members",
        icon: UserGroupIcon,
      },
      {
        title: "Connections",
        href: "/w/settings/connections",
        icon: Plug01Icon,
      },
      {
        title: "Billing",
        href: "/w/settings/billing",
        icon: CreditCardIcon,
      },
    ],
  },
]

export default function SettingsLayout({
  children,
}: {
  children: React.ReactNode
}) {
  const pathname = usePathname()
  const { activeOrg } = useAuth()

  return (
    <div className="flex min-h-0 flex-1">
      <aside className="sticky top-0 hidden h-svh w-56 shrink-0 self-start overflow-y-auto border-r border-border/60 bg-muted/20 md:block">
        <div className="px-4 pt-5 pb-3">
          <h2 className="text-[13px] font-medium text-foreground">Settings</h2>
          <p className="text-[11px] text-muted-foreground">
            {activeOrg?.name ?? ""}
          </p>
        </div>

        <nav className="flex flex-col gap-3 px-2 pb-6">
          {SECTIONS.map((section) => (
            <div key={section.label}>
              <h3 className="px-2 pt-2 pb-1 font-mono text-[10px] uppercase tracking-[1.5px] text-muted-foreground/60">
                {section.label}
              </h3>
              <ul className="flex flex-col">
                {section.items.map((item) => {
                  const active = pathname === item.href
                  return (
                    <li key={item.href}>
                      <Link
                        href={item.href}
                        className={
                          "flex items-center gap-2 rounded-md px-2 py-1.5 text-[13px] transition-colors " +
                          (active
                            ? "bg-muted text-foreground"
                            : "text-muted-foreground hover:bg-muted/60 hover:text-foreground")
                        }
                      >
                        <HugeiconsIcon
                          icon={item.icon}
                          strokeWidth={2}
                          className="size-4"
                        />
                        {item.title}
                      </Link>
                    </li>
                  )
                })}
              </ul>
            </div>
          ))}
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">{children}</div>
    </div>
  )
}
