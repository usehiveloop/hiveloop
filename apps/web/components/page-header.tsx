import * as React from "react"

export function PageHeader({
  title,
  breadcrumb,
  actions,
}: {
  title: React.ReactNode
  breadcrumb?: React.ReactNode
  actions?: React.ReactNode
}) {
  return (
    <header className="sticky top-0 z-10 flex h-14 shrink-0 items-center justify-between gap-4 border-b border-border/60 bg-background/80 px-6 backdrop-blur">
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
