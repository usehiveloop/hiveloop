import {
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { ChoiceCard } from "../create-agent/choice-card"
import { BackButton } from "./back-button"
import type { ConfigurableResource, InConnection } from "./types"

interface ResourceTypeListViewProps {
  connection: InConnection
  resourceTypes: ConfigurableResource[]
  getTypeSelectedCount: (resourceType: string) => number
  onSelect: (resourceType: string) => void
  onBack: () => void
}

export function ResourceTypeListView({ connection, resourceTypes, getTypeSelectedCount, onSelect, onBack }: ResourceTypeListViewProps) {
  return (
    <>
      <DialogHeader>
        <BackButton onClick={onBack} />
        <DialogTitle>{connection.display_name ?? connection.provider}</DialogTitle>
        <DialogDescription>Choose resource types to scope</DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {resourceTypes.map((resource) => {
          const count = getTypeSelectedCount(resource.key)
          return (
            <ChoiceCard
              key={resource.key}
              title={resource.display_name}
              description={resource.description}
              onClick={() => onSelect(resource.key)}
              trailing={
                count > 0 ? (
                  <span className="text-xs font-medium text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded-full shrink-0">
                    {count} selected
                  </span>
                ) : undefined
              }
            />
          )
        })}
      </div>
    </>
  )
}
