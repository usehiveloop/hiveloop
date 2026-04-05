"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, SparklesIcon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { $api } from "@/lib/api/hooks"
import { IntegrationLogo } from "@/components/integration-logo"
import { llmKeys } from "./data"
import type { CreationMode } from "./types"

interface SummaryRowProps {
  label: string
  value: string
}

function SummaryRow({ label, value }: SummaryRowProps) {
  return (
    <div className="flex items-center justify-between rounded-xl bg-muted/50 px-4 py-3">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-sm font-medium text-foreground">{value}</span>
    </div>
  )
}

interface StepSummaryProps {
  mode: CreationMode
  selectedKeyId: string | null
  selectedIntegrations: Set<string>
  onBack: () => void
  onSubmit: () => void
}

export function StepSummary({ mode, selectedKeyId, selectedIntegrations, onBack, onSubmit }: StepSummaryProps) {
  const key = llmKeys.find((item) => item.id === selectedKeyId)
  const { data: connectionsData } = $api.useQuery("get", "/v1/in/connections")
  const connections = connectionsData?.data ?? []

  const selectedConnections = connections.filter(
    (connection) => selectedIntegrations.has(connection.id!)
  )

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Review & create</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          {mode === "forge"
            ? "Review your configuration. Forge will generate and optimize your agent's system prompt automatically."
            : "Review your configuration before creating your agent."}
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3 mt-4 flex-1 overflow-y-auto">
        <SummaryRow label="LLM provider" value={key ? `${key.provider} — ${key.name}` : "None selected"} />
        <SummaryRow
          label="Integrations"
          value={selectedConnections.length > 0 ? `${selectedConnections.length} connected` : "None"}
        />

        {selectedConnections.length > 0 && (
          <div className="rounded-xl bg-muted/50 px-4 py-3">
            <div className="flex flex-col gap-2">
              {selectedConnections.map((connection) => (
                <div key={connection.id} className="flex items-center gap-3 py-1">
                  <IntegrationLogo provider={connection.provider ?? ""} size={20} className="shrink-0" />
                  <span className="text-sm font-medium text-foreground">{connection.display_name}</span>
                  <span className="text-xs text-muted-foreground ml-auto font-mono">
                    {connection.actions_count ?? 0} actions
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onSubmit} className="w-full">
          {mode === "forge" ? (
            <>
              <HugeiconsIcon icon={SparklesIcon} size={16} data-icon="inline-start" />
              Forge agent
            </>
          ) : (
            "Create agent"
          )}
        </Button>
      </div>
    </div>
  )
}
