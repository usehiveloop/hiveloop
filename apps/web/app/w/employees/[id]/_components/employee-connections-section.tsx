import { HugeiconsIcon } from "@hugeicons/react"
import { Add01Icon, Plug01Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { FormEmptyWell, FormSection } from "@/app/w/_components/form-section"
import { IntegrationLogo } from "@/components/integration-logo"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { cn } from "@/lib/utils"
import { ListRowsSkeleton } from "./list-rows-skeleton"
import type { components } from "@/lib/api/schema"

type Connection = components["schemas"]["inConnectionResponse"]

export function EmployeeConnectionsSection({
  connections,
  loading,
  selectedIDs,
  dialogOpen,
  onDialogOpenChange,
  onToggle,
}: {
  connections: Connection[]
  loading: boolean
  selectedIDs: Set<string>
  dialogOpen: boolean
  onDialogOpenChange: (open: boolean) => void
  onToggle: (id: string) => void
}) {
  const selectedConnections = connections.filter(
    (connection) => connection.id && selectedIDs.has(connection.id)
  )

  return (
    <>
      <FormSection
        title="Connections"
        description="Attach existing workspace connections. Matching global skills are added automatically."
        aside={
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => onDialogOpenChange(true)}
          >
            <HugeiconsIcon icon={Add01Icon} data-icon="inline-start" />
            Manage
          </Button>
        }
      >
        {loading ? (
          <ListRowsSkeleton />
        ) : selectedConnections.length === 0 ? (
          <FormEmptyWell message="No connections attached." />
        ) : (
          <div className="divide-y divide-border overflow-hidden rounded-xl border border-border">
            {selectedConnections.map((connection) => (
              <ConnectionRow key={connection.id} connection={connection} />
            ))}
          </div>
        )}
      </FormSection>

      <EmployeeConnectionsDialog
        open={dialogOpen}
        onOpenChange={onDialogOpenChange}
        connections={connections}
        loading={loading}
        selectedIDs={selectedIDs}
        onToggle={onToggle}
      />
    </>
  )
}

function EmployeeConnectionsDialog({
  open,
  onOpenChange,
  connections,
  loading,
  selectedIDs,
  onToggle,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  connections: Connection[]
  loading: boolean
  selectedIDs: Set<string>
  onToggle: (id: string) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Employee connections</DialogTitle>
          <DialogDescription>
            Select existing workspace connections for this employee.
          </DialogDescription>
        </DialogHeader>

        {loading ? (
          <ListRowsSkeleton />
        ) : connections.length === 0 ? (
          <FormEmptyWell message="No workspace connections are available." />
        ) : (
          <div className="max-h-[min(520px,65vh)] overflow-y-auto pr-1">
            <div className="divide-y divide-border overflow-hidden rounded-2xl border border-border">
              {connections.map((connection) => {
                if (!connection.id) return null
                const selected = selectedIDs.has(connection.id)
                return (
                  <button
                    key={connection.id}
                    type="button"
                    onClick={() => onToggle(connection.id!)}
                    className="grid w-full grid-cols-[1rem_1fr] gap-4 px-4 py-3 text-left transition-colors hover:bg-muted/50"
                  >
                    <SelectionDot selected={selected} />
                    <ConnectionRow connection={connection} />
                  </button>
                )
              })}
            </div>
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Done
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function SelectionDot({ selected }: { selected: boolean }) {
  return (
    <span
      className={cn(
        "mt-2 flex size-4 items-center justify-center rounded-[5px] border",
        selected
          ? "border-primary bg-primary text-primary-foreground"
          : "border-border bg-input/70"
      )}
    >
      {selected ? (
        <HugeiconsIcon icon={Tick02Icon} className="size-3" strokeWidth={2.5} />
      ) : null}
    </span>
  )
}

function ConnectionRow({ connection }: { connection: Connection }) {
  return (
    <div className="flex min-w-0 items-center gap-3 py-1">
      {connection.provider ? (
        <IntegrationLogo provider={connection.provider} size={32} />
      ) : (
        <div className="flex size-8 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <HugeiconsIcon icon={Plug01Icon} className="size-4" strokeWidth={2} />
        </div>
      )}
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-foreground">
          {connection.display_name ?? connection.provider ?? "Connection"}
        </p>
        <p className="truncate text-xs text-muted-foreground">
          {connection.provider}
          {connection.nango_connection_id
            ? ` · ${connection.nango_connection_id}`
            : ""}
        </p>
      </div>
    </div>
  )
}
