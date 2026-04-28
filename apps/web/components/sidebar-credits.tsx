"use client"

import * as React from "react"
import Link from "next/link"
import { useAuth } from "@/lib/auth/auth-context"
import { Progress } from "@/components/ui/progress"

function formatK(n: number) {
  if (n >= 10_000) return `${(n / 1000).toFixed(1).replace(/\.0$/, "")}k`
  return n.toLocaleString("en-US")
}

function tone(pct: number) {
  if (pct < 0.2) return { label: "text-destructive" }
  return { label: "text-sidebar-foreground/55" }
}

export function SidebarCredits() {
  const { activeOrg, isLoading } = useAuth()

  if (isLoading || !activeOrg) {
    return <SidebarCreditsSkeleton />
  }

  const planName = activeOrg.plan?.name ?? "Free"
  const remaining = activeOrg.credits ?? 0
  const total =
    (activeOrg.plan?.monthly_credits ?? 0) +
    (activeOrg.plan?.welcome_credits ?? 0)

  const pctRemaining = total > 0 ? Math.min(1, Math.max(0, remaining / total)) : 0
  const t = tone(pctRemaining)

  return (
    <Link
      href="/w/settings/billing"
      className="group-data-[collapsible=icon]:hidden mx-2 flex flex-col gap-1.5 rounded-xl px-2 py-2 transition-colors hover:bg-sidebar-accent/40"
    >
      <div className="flex items-baseline justify-between text-[11px]">
        <span className="text-sidebar-foreground/85">{planName}</span>
        <span className={`font-mono tabular-nums ${t.label}`}>
          <span className="text-sidebar-foreground/85">{formatK(remaining)}</span>
          {total > 0 ? (
            <>
              <span className="text-sidebar-foreground/30">/</span>
              {formatK(total)}
            </>
          ) : null}
        </span>
      </div>
      <Progress
        value={Math.round(pctRemaining * 100)}
        max={100}
        aria-label={`${planName} credits remaining`}
      />
    </Link>
  )
}

function SidebarCreditsSkeleton() {
  return (
    <div className="group-data-[collapsible=icon]:hidden mx-2 flex flex-col gap-1.5 rounded-xl px-2 py-2">
      <div className="flex items-baseline justify-between text-[11px]">
        <span className="h-3 w-10 rounded bg-sidebar-foreground/10" />
        <span className="h-3 w-16 rounded bg-sidebar-foreground/10" />
      </div>
      <div className="h-2 rounded-full bg-sidebar-foreground/10" />
    </div>
  )
}
