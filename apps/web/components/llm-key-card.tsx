import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowRight01Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { ProviderLogo } from "@/components/provider-logo"

interface LlmKeyCardProps {
  label?: string
  providerId: string
  selected?: boolean
  onClick?: () => void
}

export function LlmKeyCard({ label, providerId, selected, onClick }: LlmKeyCardProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`group flex items-center gap-3 w-full rounded-xl p-3 text-left transition-colors cursor-pointer ${
        selected
          ? "bg-primary/5 border border-primary/20"
          : "bg-muted/50 hover:bg-muted border border-transparent"
      }`}
    >
      <ProviderLogo provider={providerId} size={20} className="shrink-0" />
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium text-foreground truncate">{label}</p>
        <p className="text-xs text-muted-foreground">{providerId}</p>
      </div>
      {selected ? (
        <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0" />
      ) : (
        <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0" />
      )}
    </button>
  )
}
