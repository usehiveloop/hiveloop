export function SettingsShell({
  title,
  description,
  action,
  dividers = true,
  children,
}: {
  title: string
  description?: string
  action?: React.ReactNode
  dividers?: boolean
  children: React.ReactNode
}) {
  return (
    <div className="mx-auto w-full max-w-2xl px-6 pb-20 pt-10">
      <div className="mb-9 flex items-end justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
          {description ? (
            <p className="mt-1 text-[13px] text-muted-foreground">{description}</p>
          ) : null}
        </div>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>

      <div
        className={
          dividers
            ? "divide-y divide-border/60 [&>section]:py-7 [&>section:first-child]:pt-0 [&>section:last-child]:pb-0"
            : "flex flex-col gap-10"
        }
      >
        {children}
      </div>
    </div>
  )
}
