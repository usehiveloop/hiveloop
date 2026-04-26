"use client"

import * as React from "react"

const PLAN = { name: "Pro", total: 39_000, used: 18_569 }
const REMAINING = PLAN.total - PLAN.used
const PCT_REMAINING = REMAINING / PLAN.total

function formatK(n: number) {
  if (n >= 10_000) return `${(n / 1000).toFixed(1).replace(/\.0$/, "")}k`
  return n.toLocaleString("en-US")
}

function tone(pct: number) {
  if (pct < 0.2) {
    return { fill: "bg-destructive", label: "text-destructive" }
  }
  return { fill: "bg-primary", label: "text-sidebar-foreground/55" }
}

export function SidebarCredits() {
  const cells = 10
  const filled = Math.max(1, Math.round(PCT_REMAINING * cells))
  const t = tone(PCT_REMAINING)

  return (
    <a
      href="#"
      className="group-data-[collapsible=icon]:hidden mx-2 flex flex-col gap-1.5 rounded-xl px-2 py-2 transition-colors hover:bg-sidebar-accent/40"
    >
      <div className="flex items-baseline justify-between text-[11px]">
        <span className="text-sidebar-foreground/85">{PLAN.name}</span>
        <span className={`font-mono tabular-nums ${t.label}`}>
          <span className="text-sidebar-foreground/85">{formatK(REMAINING)}</span>
          <span className="text-sidebar-foreground/30">/</span>
          {formatK(PLAN.total)}
        </span>
      </div>
      <div
        className="flex h-2 gap-px"
        role="meter"
        aria-valuenow={Math.round(PCT_REMAINING * 100)}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={`${PLAN.name} credits remaining`}
      >
        {Array.from({ length: cells }).map((_, i) => (
          <span
            key={i}
            className={
              i < filled
                ? `flex-1 rounded-xs ${t.fill}`
                : "flex-1 rounded-xs bg-sidebar-foreground/10"
            }
          />
        ))}
      </div>
    </a>
  )
}
