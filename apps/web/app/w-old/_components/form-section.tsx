import type * as React from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Plug01Icon } from "@hugeicons/core-free-icons"

export function FormSection({
  title,
  description,
  aside,
  children,
}: {
  title: string
  description?: string
  aside?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <section className="flex flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0 flex-1">
          <h2 className="text-[15px] font-medium text-foreground">{title}</h2>
          {description ? (
            <p className="mt-0.5 text-[12px] text-muted-foreground">
              {description}
            </p>
          ) : null}
        </div>
        {aside ? <div className="shrink-0">{aside}</div> : null}
      </div>
      {children}
    </section>
  )
}

export function FormEmptyWell({
  icon = Plug01Icon,
  message,
  action,
}: {
  icon?: typeof Plug01Icon
  message: string
  action?: React.ReactNode
}) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-xl bg-muted/30 px-6 py-9 text-center">
      <div className="flex size-10 items-center justify-center rounded-lg bg-muted text-muted-foreground">
        <HugeiconsIcon icon={icon} strokeWidth={2} className="size-4" />
      </div>
      <p className="text-[13px] text-muted-foreground">{message}</p>
      {action ? <div className="mt-1">{action}</div> : null}
    </div>
  )
}
