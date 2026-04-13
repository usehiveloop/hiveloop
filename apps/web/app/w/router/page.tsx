"use client"

import { useState } from "react"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import { Add01Icon } from "@hugeicons/core-free-icons"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import { Button } from "@/components/ui/button"
import { RouterEmpty } from "./_components/router-empty"
import { TriggerCard } from "./_components/trigger-card"
import { CreateTriggerDialog } from "./_components/create-trigger-dialog"
import { EditTriggerPanel } from "./_components/edit-trigger-panel"
import { DecisionsTab } from "./_components/decisions-tab"
import { SettingsTab } from "./_components/settings-tab"
import {
  INITIAL_TRIGGERS,
  INITIAL_DECISIONS,
  INITIAL_SETTINGS,
  type MockTrigger,
  type MockRouterSettings,
} from "./_components/mock-data"

export default function RouterPage() {
  const [triggers, setTriggers] = useState<MockTrigger[]>(INITIAL_TRIGGERS)
  const [settings, setSettings] = useState<MockRouterSettings>(INITIAL_SETTINGS)
  const [createOpen, setCreateOpen] = useState(false)
  const [editingTrigger, setEditingTrigger] = useState<MockTrigger | null>(null)

  function handleTriggerCreated(trigger: MockTrigger) {
    setTriggers((previous) => [...previous, trigger])
    toast.success(`Trigger created for ${trigger.connectionName}`)
  }

  function handleTriggerUpdate(updated: MockTrigger) {
    setTriggers((previous) =>
      previous.map((trigger) => trigger.id === updated.id ? updated : trigger)
    )
    setEditingTrigger(updated)
  }

  function handleTriggerDelete(triggerId: string) {
    setTriggers((previous) => previous.filter((trigger) => trigger.id !== triggerId))
    toast.success("Trigger deleted")
  }

  function handleToggleEnabled(triggerId: string) {
    setTriggers((previous) =>
      previous.map((trigger) =>
        trigger.id === triggerId ? { ...trigger, enabled: !trigger.enabled } : trigger
      )
    )
  }

  return (
    <>
      <div className="max-w-464 mx-auto w-full px-4 py-8">
        <div className="mb-6">
          <h1 className="font-heading text-xl font-semibold text-foreground">Router</h1>
          <p className="text-sm text-muted-foreground mt-1">Route incoming events to the right agent.</p>
        </div>

        <Tabs defaultValue="triggers">
          <TabsList variant="line" className="mb-6">
            <TabsTrigger value="triggers">Triggers</TabsTrigger>
            <TabsTrigger value="decisions">Decisions</TabsTrigger>
            <TabsTrigger value="settings">Settings</TabsTrigger>
          </TabsList>

          <TabsContent value="triggers">
            {triggers.length === 0 ? (
              <RouterEmpty onCreateTrigger={() => setCreateOpen(true)} />
            ) : (
              <>
                <div className="flex items-center justify-between mb-4">
                  <p className="text-sm text-muted-foreground">
                    {triggers.length} trigger{triggers.length !== 1 ? "s" : ""} across {new Set(triggers.map((trigger) => trigger.provider)).size} connection{new Set(triggers.map((trigger) => trigger.provider)).size !== 1 ? "s" : ""}
                  </p>
                  <Button size="sm" onClick={() => setCreateOpen(true)}>
                    <HugeiconsIcon icon={Add01Icon} size={14} data-icon="inline-start" />
                    New trigger
                  </Button>
                </div>

                <div className="flex flex-col gap-3">
                  {triggers.map((trigger) => (
                    <TriggerCard
                      key={trigger.id}
                      trigger={trigger}
                      onEdit={setEditingTrigger}
                      onToggleEnabled={handleToggleEnabled}
                      onDelete={handleTriggerDelete}
                    />
                  ))}
                </div>
              </>
            )}
          </TabsContent>

          <TabsContent value="decisions">
            <DecisionsTab decisions={INITIAL_DECISIONS} />
          </TabsContent>

          <TabsContent value="settings">
            <SettingsTab settings={settings} onUpdate={setSettings} />
          </TabsContent>
        </Tabs>
      </div>

      <CreateTriggerDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={handleTriggerCreated}
      />

      <EditTriggerPanel
        open={editingTrigger !== null}
        onOpenChange={(nextOpen) => { if (!nextOpen) setEditingTrigger(null) }}
        trigger={editingTrigger}
        onUpdate={handleTriggerUpdate}
        onDelete={handleTriggerDelete}
      />
    </>
  )
}
