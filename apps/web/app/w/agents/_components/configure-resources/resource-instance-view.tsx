import { useMemo } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Tick02Icon } from "@hugeicons/core-free-icons"
import {
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { ChoiceCard } from "../create-agent/choice-card"
import { $api } from "@/lib/api/hooks"
import { BackButton } from "./back-button"
import type { ResourceItem } from "./types"

interface ResourceInstanceListViewProps {
  connectionId: string
  resourceType: string
  selectedItems: ResourceItem[]
  isSelected: (resourceId: string) => boolean
  onToggle: (item: ResourceItem) => void
  onBack: () => void
}

export function ResourceInstanceListView({ connectionId, resourceType, selectedItems, isSelected, onToggle, onBack }: ResourceInstanceListViewProps) {
  const { data, isLoading } = $api.useQuery("get", "/v1/in/connections/{id}/resources/{type}", {
    params: { path: { id: connectionId, type: resourceType } },
  })

  const items: ResourceItem[] = ((data as Record<string, unknown> | undefined)?.resources as ResourceItem[] | undefined) ?? []
  const label = resourceType.replace(/_/g, " ")

  // Orphans: items selected in the agent's state but not returned by the live
  // item list. Only computed once the live list has resolved to avoid a false
  // warning during load.
  const reachableIds = useMemo(() => new Set(items.map((item) => item.id)), [items])
  const orphans = useMemo(
    () => (isLoading ? [] : selectedItems.filter((item) => !reachableIds.has(item.id))),
    [isLoading, selectedItems, reachableIds],
  )

  return (
    <>
      <DialogHeader>
        <BackButton onClick={onBack} />
        <DialogTitle className="capitalize">{label}s</DialogTitle>
        <DialogDescription>Select which {label}s this agent can access</DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {orphans.length > 0 && (
          <div className="rounded-xl border border-amber-300/60 bg-amber-500/10 px-3 py-2.5">
            <p className="text-xs font-medium text-amber-800 dark:text-amber-200">
              {orphans.length} {label}{orphans.length !== 1 ? "s" : ""} no longer accessible
            </p>
            <p className="text-[11px] text-amber-800/80 dark:text-amber-200/80 mt-0.5">
              These will be removed from this agent when you save.
            </p>
            <ul className="mt-2 space-y-0.5">
              {orphans.map((item) => (
                <li key={item.id} className="font-mono text-[11px] text-amber-900 dark:text-amber-100">
                  {item.id}
                </li>
              ))}
            </ul>
          </div>
        )}
        {isLoading ? (
          Array.from({ length: 5 }).map((_, index) => (
            <Skeleton key={index} className="h-13 w-full rounded-xl" />
          ))
        ) : items.length === 0 ? (
          <p className="text-sm text-muted-foreground py-8 text-center">
            No {label}s found.
          </p>
        ) : (
          items.map((item) => {
            const selected = isSelected(item.id)
            return (
              <ChoiceCard
                key={item.id}
                title={item.name}
                description={item.id !== item.name ? item.id : ""}
                onClick={() => onToggle(item)}
                trailing={
                  selected ? (
                    <HugeiconsIcon icon={Tick02Icon} size={16} className="text-emerald-600 dark:text-emerald-400 shrink-0" />
                  ) : (
                    <span className="h-4 w-4 rounded-full border border-border shrink-0" />
                  )
                }
              />
            )
          })
        )}
      </div>
    </>
  )
}
