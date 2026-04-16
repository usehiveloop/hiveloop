import { HugeiconsIcon } from "@hugeicons/react"
import { Robot01Icon } from "@hugeicons/core-free-icons"

export default function ConversationsIndexPage() {
  return (
    <div className="flex flex-1 items-center justify-center">
      <div className="flex flex-col items-center gap-3 text-muted-foreground/40">
        <div className="h-12 w-12 rounded-2xl bg-muted/30 flex items-center justify-center">
          <HugeiconsIcon icon={Robot01Icon} size={24} className="text-muted-foreground/30" />
        </div>
        <p className="text-[13px]">Select a conversation to view</p>
      </div>
    </div>
  )
}
