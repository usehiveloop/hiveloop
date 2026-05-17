"use client"

import { useEffect, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Plug01Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { ChoiceCard } from "@/components/agent-form-shared/choice-card"
import { FormEmptyWell, FormSection } from "@/app/w/_components/form-section"
import {
  IntegrationLogo,
  integrationLogoURL,
} from "@/components/integration-logo"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { ListRowsSkeleton } from "./list-rows-skeleton"
import type { components } from "@/lib/api/schema"

type Connection = components["schemas"]["employeeConnectionResponse"]

export function EmployeeConnectionsSection({
  connections,
  loading,
  selectedIDs,
  dialogOpen,
  onDialogOpenChange,
  onSelectionChange,
  onRemove,
}: {
  connections: Connection[]
  loading: boolean
  selectedIDs: Set<string>
  dialogOpen: boolean
  onDialogOpenChange: (open: boolean) => void
  onSelectionChange: (ids: Set<string>) => void
  onRemove: (id: string) => void
}) {
  const selectedConnections = connections.filter(
    (connection) => connection.id && selectedIDs.has(connection.id)
  )

  return (
    <>
      <FormSection
        title="Connections"
        description="Attach existing workspace connections. Matching global skills are added automatically."
      >
        <div className="flex flex-col gap-2">
          {loading ? (
            <ListRowsSkeleton />
          ) : selectedConnections.length === 0 ? (
            <FormEmptyWell
              icon={Plug01Icon}
              message="No connections attached."
              action={
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => onDialogOpenChange(true)}
                >
                  Manage connections
                </Button>
              }
            />
          ) : (
            <>
              {selectedConnections.map((connection) => {
                if (!connection.id) return null
                return (
                  <SelectedConnectionRow
                    key={connection.id}
                    connection={connection}
                    onRemove={() => onRemove(connection.id!)}
                  />
                )
              })}
              <Button
                variant="outline"
                size="sm"
                className="mt-1 w-fit"
                onClick={() => onDialogOpenChange(true)}
              >
                Manage connections
              </Button>
            </>
          )}
        </div>
      </FormSection>

      <EmployeeConnectionsDialog
        open={dialogOpen}
        onOpenChange={onDialogOpenChange}
        connections={connections}
        loading={loading}
        selectedIDs={selectedIDs}
        onSave={onSelectionChange}
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
  onSave,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  connections: Connection[]
  loading: boolean
  selectedIDs: Set<string>
  onSave: (ids: Set<string>) => void
}) {
  const [draftSelectedIDs, setDraftSelectedIDs] = useState<Set<string>>(
    new Set()
  )

  useEffect(() => {
    if (open) {
      queueMicrotask(() => {
        setDraftSelectedIDs(new Set(selectedIDs))
      })
    }
  }, [open, selectedIDs])

  function toggle(connectionID: string) {
    setDraftSelectedIDs((prev) => {
      const next = new Set(prev)
      if (next.has(connectionID)) {
        next.delete(connectionID)
      } else {
        next.add(connectionID)
      }
      return next
    })
  }

  function saveSelection() {
    onSave(new Set(draftSelectedIDs))
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[min(680px,85vh)] flex-col overflow-hidden p-6 sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Manage connections</DialogTitle>
          <DialogDescription>
            Choose the workspace connections this employee can use.
          </DialogDescription>
        </DialogHeader>

        <div className="mt-4 flex flex-1 flex-col gap-2 overflow-y-auto">
          {loading ? (
            <ListRowsSkeleton />
          ) : connections.length === 0 ? (
            <div className="flex flex-col items-center justify-center gap-3 py-12">
              <div className="flex size-12 items-center justify-center rounded-full bg-muted">
                <HugeiconsIcon
                  icon={Plug01Icon}
                  size={20}
                  className="text-muted-foreground"
                />
              </div>
              <div className="text-center">
                <p className="text-sm font-medium text-foreground">
                  No connections available
                </p>
                <p className="mt-1 max-w-[260px] text-xs text-muted-foreground">
                  Connect an app from Settings, then return here to attach it.
                </p>
              </div>
            </div>
          ) : (
            connections.map((connection) => {
              if (!connection.id) return null
              const selected = draftSelectedIDs.has(connection.id)
              return (
                <ChoiceCard
                  key={connection.id}
                  logoUrl={integrationLogoURL(connection.provider ?? "")}
                  logoSize={32}
                  title={connection.display_name ?? connection.provider ?? ""}
                  description={connectionDescription(connection)}
                  selected={selected}
                  onClick={() => toggle(connection.id!)}
                  trailing={
                    selected ? (
                      <HugeiconsIcon
                        icon={Tick02Icon}
                        size={16}
                        className="mt-0.5 shrink-0 text-primary"
                      />
                    ) : (
                      <span className="size-4 shrink-0" />
                    )
                  }
                />
              )
            })
          )}
        </div>

        <div className="shrink-0 pt-4">
          <Button className="w-full" onClick={saveSelection}>
            {draftSelectedIDs.size > 0
              ? `Save with ${draftSelectedIDs.size} connection${draftSelectedIDs.size > 1 ? "s" : ""}`
              : "Save with no connections"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function SelectedConnectionRow({
  connection,
  onRemove,
}: {
  connection: Connection
  onRemove: () => void
}) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-xl border border-border bg-muted/50 p-3">
      <div className="flex min-w-0 items-center gap-3">
        {connection.provider ? (
          <IntegrationLogo provider={connection.provider} size={32} />
        ) : (
          <div className="flex size-8 items-center justify-center rounded-md bg-muted text-muted-foreground">
            <HugeiconsIcon
              icon={Plug01Icon}
              className="size-4"
              strokeWidth={2}
            />
          </div>
        )}
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-foreground">
            {connection.display_name ?? connection.provider ?? "Connection"}
          </p>
          <p className="truncate text-xs text-muted-foreground">
            {connectionDescription(connection)}
          </p>
        </div>
      </div>
      <Button
        variant="ghost"
        size="sm"
        className="shrink-0 text-destructive hover:text-destructive"
        onClick={onRemove}
      >
        Remove
      </Button>
    </div>
  )
}

function connectionDescription(connection: Connection) {
  return [connection.provider, connection.nango_connection_id]
    .filter(Boolean)
    .join(" · ")
}
