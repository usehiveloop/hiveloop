import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon } from "@hugeicons/core-free-icons"

export function BackButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors w-fit mb-2"
    >
      <HugeiconsIcon icon={ArrowLeft01Icon} size={14} />
      Back
    </button>
  )
}
