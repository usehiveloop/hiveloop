import Link from "next/link"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowRight01Icon } from "@hugeicons/core-free-icons"

interface ChoiceCardBaseProps {
  icon?: typeof ArrowRight01Icon
  iconClassName?: string
  logoUrl?: string
  title: string
  description: string
  trailing?: React.ReactNode
}

type ChoiceCardProps =
  | (ChoiceCardBaseProps & { onClick: () => void; href?: never })
  | (ChoiceCardBaseProps & { href: string; onClick?: () => void })

export function ChoiceCard({
  icon,
  iconClassName,
  logoUrl,
  title,
  description,
  trailing,
  onClick,
  href,
}: ChoiceCardProps) {
  const className =
    "group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"

  const content = (
    <>
      {logoUrl ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={logoUrl} alt={title} className="h-5 w-5 shrink-0 mt-0.5" />
      ) : icon ? (
        <HugeiconsIcon icon={icon} size={20} className={`shrink-0 mt-0.5 ${iconClassName ?? "text-muted-foreground"}`} />
      ) : null}
      <div className="flex-1 min-w-0">
        <p className="text-sm font-semibold text-foreground">{title}</p>
        <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">{description}</p>
      </div>
      {trailing ?? (
        <HugeiconsIcon
          icon={ArrowRight01Icon}
          size={16}
          className="text-muted-foreground/30 shrink-0 mt-0.5"
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
    <button onClick={onClick} className={className}>
      {content}
    </button>
  )
}
