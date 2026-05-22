import { Button } from "@/components/ui/button"
import { HugeiconsIcon } from "@hugeicons/react"
import { Add01Icon } from "@hugeicons/core-free-icons"

interface ConnectionsHeaderProps {
  count: number
  onAddClick: () => void
}

export function ConnectionsHeader({ count, onAddClick }: ConnectionsHeaderProps) {
  return (
    <div className="flex items-center justify-between mb-6">
      <div>
        <h1 className="font-heading text-xl font-semibold text-foreground">Connections</h1>
        <p className="text-sm text-muted-foreground mt-1">{count} connections in this workspace</p>
      </div>
      <Button size="default" onClick={onAddClick}>
        <HugeiconsIcon icon={Add01Icon} size={16} data-icon="inline-start" />
        Add connection
      </Button>
    </div>
  )
}
