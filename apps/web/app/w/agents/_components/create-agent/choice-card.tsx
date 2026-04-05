import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowRight01Icon } from "@hugeicons/core-free-icons"

interface ChoiceCardProps {
  icon?: typeof ArrowRight01Icon
  iconClassName?: string
  logoUrl?: string
  title: string
  description: string
  onClick: () => void
  trailing?: React.ReactNode
}

export function ChoiceCard({ icon, iconClassName, logoUrl, title, description, onClick, trailing }: ChoiceCardProps) {
  return (
    <button
      onClick={onClick}
      className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
    >
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
    </button>
  )
}
