export function StepHeader({
  title,
  description,
  eyebrow,
  hero,
}: {
  title: string
  description?: string
  eyebrow?: React.ReactNode
  hero?: React.ReactNode
}) {
  return (
    <header className="flex flex-col items-center gap-2 text-center">
      {hero}
      {eyebrow ? (
        <div className="flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
          {eyebrow}
        </div>
      ) : null}
      <h1 className="font-display text-3xl font-semibold tracking-tight">{title}</h1>
      {description ? (
        <p className="max-w-md text-sm leading-relaxed text-muted-foreground">
          {description}
        </p>
      ) : null}
    </header>
  )
}
