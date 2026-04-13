"use client"

import { useState } from "react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select"
import { MOCK_AGENTS, type MockRouterSettings } from "./mock-data"

interface SettingsTabProps {
  settings: MockRouterSettings
  onUpdate: (settings: MockRouterSettings) => void
}

export function SettingsTab({ settings, onUpdate }: SettingsTabProps) {
  const [persona, setPersona] = useState(settings.persona)
  const [defaultAgentId, setDefaultAgentId] = useState(settings.defaultAgentId)
  const [memoryTeam, setMemoryTeam] = useState(settings.memoryTeam)
  const [dirty, setDirty] = useState(false)

  function handlePersonaChange(value: string) {
    setPersona(value)
    setDirty(true)
  }

  function handleAgentChange(value: string) {
    setDefaultAgentId(value)
    setDirty(true)
  }

  function handleMemoryChange(value: string) {
    setMemoryTeam(value)
    setDirty(true)
  }

  function handleSave() {
    onUpdate({ persona, defaultAgentId, memoryTeam })
    setDirty(false)
    toast.success("Router settings saved")
  }

  const selectedAgentName = MOCK_AGENTS.find((agent) => agent.id === defaultAgentId)?.name ?? "None"

  return (
    <div className="max-w-lg">
      <div className="flex flex-col gap-6">
        {/* Persona */}
        <div className="flex flex-col gap-2">
          <Label className="text-sm font-medium">Persona</Label>
          <p className="text-[12px] text-muted-foreground -mt-1">
            The shared voice injected into every agent&apos;s instructions when handling a routed event.
          </p>
          <Textarea
            value={persona}
            onChange={(event) => handlePersonaChange(event.target.value)}
            placeholder="You are Zira, an engineering operations assistant..."
            className="min-h-32"
          />
        </div>

        {/* Default Agent */}
        <div className="flex flex-col gap-2">
          <Label className="text-sm font-medium">Default agent</Label>
          <p className="text-[12px] text-muted-foreground -mt-1">
            Fallback when no routing rules match and triage selects nothing.
          </p>
          <Select value={defaultAgentId} onValueChange={(value) => handleAgentChange(value ?? "")}>
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Select a default agent..." />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">
                <span className="text-muted-foreground">None (no fallback)</span>
              </SelectItem>
              {MOCK_AGENTS.map((agent) => (
                <SelectItem key={agent.id} value={agent.id}>
                  {agent.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {/* Memory Team */}
        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-2">
            <Label className="text-sm font-medium">Memory team</Label>
            <span className="text-[11px] text-muted-foreground font-normal">(optional)</span>
          </div>
          <p className="text-[12px] text-muted-foreground -mt-1">
            Shared memory namespace across all agents handling routed events. Agents in the same team share context.
          </p>
          <Input
            value={memoryTeam}
            onChange={(event) => handleMemoryChange(event.target.value)}
            placeholder="engineering"
          />
        </div>

        {/* Save */}
        <div>
          <Button onClick={handleSave} disabled={!dirty}>
            Save changes
          </Button>
        </div>
      </div>
    </div>
  )
}
