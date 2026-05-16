import * as React from "react"
import { cn } from "@/lib/utils"

export function PageHeader({
  title,
  breadcrumb,
  actions,
  sticky = false,
}: {
  title: React.ReactNode
  breadcrumb?: React.ReactNode
  actions?: React.ReactNode
  sticky?: boolean
}) {
  return (
    <header
      className={cn(
        "flex h-14 shrink-0 items-center justify-between gap-4 border-b border-border/60 bg-background px-6",
        sticky && "sticky top-[65px] z-20 bg-background/95 backdrop-blur"
      )}
    >
      <div className="flex min-w-0 items-center gap-2">
        <h1 className="truncate text-[15px] font-medium text-foreground">
          {title}
        </h1>
        {breadcrumb ? (
          <span className="flex min-w-0 items-center gap-2 text-[13px] text-muted-foreground">
            <span className="text-muted-foreground/40">/</span>
            {breadcrumb}
          </span>
        ) : null}
      </div>
      {actions ? (
        <div className="flex shrink-0 items-center gap-2">{actions}</div>
      ) : null}
    </header>
  )
}
