import { HugeiconsIcon } from "@hugeicons/react"
import { Plug01Icon } from "@hugeicons/core-free-icons"
import {
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { ChoiceCard } from "../create-agent/choice-card"
import type { InConnection } from "./types"

interface ConnectionListViewProps {
  connections: InConnection[]
  getSelectedCount: (connectionId: string) => number
  getOrphanCount: (connectionId: string) => number
  onSelect: (connectionId: string) => void
  onSave: () => void
  saving: boolean
}

export function ConnectionListView({ connections, getSelectedCount, getOrphanCount, onSelect, onSave, saving }: ConnectionListViewProps) {
  return (
    <>
      <DialogHeader>
        <DialogTitle>Configure resources</DialogTitle>
        <DialogDescription>Choose which resources each integration can access.</DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {connections.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-3">
            <div className="flex items-center justify-center size-12 rounded-full bg-muted">
              <HugeiconsIcon icon={Plug01Icon} size={20} className="text-muted-foreground" />
            </div>
            <p className="text-sm text-muted-foreground text-center">
              No integrations with configurable resources.
            </p>
          </div>
        ) : (
          connections.map((connection) => {
            const connectionId = connection.id as string
            const count = getSelectedCount(connectionId)
            const orphans = getOrphanCount(connectionId)
            const baseDescription = count > 0
              ? `${count} resource${count !== 1 ? "s" : ""} selected`
              : "No resources configured"
            const description = orphans > 0
              ? `${baseDescription} · ${orphans} no longer accessible`
              : baseDescription
            return (
              <ChoiceCard
                key={connectionId}
                logoUrl={`https://connections.usehiveloop.com/images/template-logos/${connection.provider ?? ""}.svg`}
                title={connection.display_name ?? connection.provider ?? ""}
                description={description}
                onClick={() => onSelect(connectionId)}
                trailing={
                  orphans > 0 ? (
                    <span className="text-xs font-medium text-amber-700 dark:text-amber-300 bg-amber-500/10 px-2 py-0.5 rounded-full shrink-0">
                      !{orphans}
                    </span>
                  ) : count > 0 ? (
                    <span className="text-xs font-medium text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded-full shrink-0">
                      {count}
                    </span>
                  ) : undefined
                }
              />
            )
          })
        )}
      </div>

      <div className="pt-4 mt-auto">
        <Button className="w-full" onClick={onSave} loading={saving}>
          Save resources
        </Button>
      </div>
    </>
  )
}
