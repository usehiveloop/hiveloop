import Link from "next/link"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowRight01Icon } from "@hugeicons/core-free-icons"

const logoSizeClasses: Record<number, string> = {
  16: "h-4 w-4",
  20: "h-5 w-5",
  24: "h-6 w-6",
  28: "h-7 w-7",
  32: "h-8 w-8",
  36: "h-9 w-9",
  40: "h-10 w-10",
  48: "h-12 w-12",
}

interface ChoiceCardBaseProps {
  icon?: typeof ArrowRight01Icon
  iconClassName?: string
  logoUrl?: string
  /** Square px size for the leading icon/logo. Defaults to 20 (matches the existing density). */
  logoSize?: number
  title: string
  description: string
  trailing?: React.ReactNode
  selected?: boolean
}

type ChoiceCardProps =
  | (ChoiceCardBaseProps & { onClick: () => void; href?: never })
  | (ChoiceCardBaseProps & { href: string; onClick?: () => void })

export function ChoiceCard({
  icon,
  iconClassName,
  logoUrl,
  logoSize = 20,
  title,
  description,
  trailing,
  selected = false,
  onClick,
  href,
}: ChoiceCardProps) {
  const alignment = logoSize <= 24 ? "items-start" : "items-center"
  const leadingTopOffset = logoSize <= 24 ? "mt-0.5" : ""
  const logoSizeClass = logoSizeClasses[logoSize] ?? "h-5 w-5"

  const className = `group flex ${alignment} gap-4 w-full rounded-xl p-4 text-left transition-colors cursor-pointer ${
    selected
      ? "bg-primary/5 ring-1 ring-primary/40 hover:bg-primary/10"
      : "bg-muted/50 hover:bg-muted"
  }`

  const content = (
    <>
      {logoUrl ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={logoUrl} alt={title} className={`${logoSizeClass} shrink-0 ${leadingTopOffset}`.trim()} />
      ) : icon ? (
        <HugeiconsIcon icon={icon} size={logoSize} className={`shrink-0 ${leadingTopOffset} ${iconClassName ?? "text-muted-foreground"}`.trim()} />
      ) : null}
      <div className="flex-1 min-w-0">
        <p className="text-sm font-semibold text-foreground">{title}</p>
        <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">{description}</p>
      </div>
      {trailing ?? (
        <HugeiconsIcon
          icon={ArrowRight01Icon}
          size={16}
          className={`text-muted-foreground/30 shrink-0 ${leadingTopOffset}`.trim()}
        />
      )}
    </>
  )

  if (href) {
    return (
      <Link href={href} onClick={onClick} className={className}>
        {content}
      </Link>
    )
  }

  return (
    <button onClick={onClick} aria-pressed={selected} className={className}>
      {content}
    </button>
  )
}
