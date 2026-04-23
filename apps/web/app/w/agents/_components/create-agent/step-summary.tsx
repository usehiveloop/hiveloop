"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, SparklesIcon, GithubIcon, FileEditIcon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { IntegrationLogo } from "@/components/integration-logo"
import { ProviderLogo } from "@/components/provider-logo"
import { $api } from "@/lib/api/hooks"
import { useCreateAgent } from "./context"

interface SummaryRowProps {
  label: string
  children: React.ReactNode
}

function SummaryRow({ label, children }: SummaryRowProps) {
  return (
    <div className="flex items-center justify-between rounded-xl bg-muted/50 px-4 py-3">
      <span className="text-sm text-muted-foreground">{label}</span>
      {children}
    </div>
  )
}

export function StepSummary() {
  const { form, mode, selectedIntegrations, selectedActions, selectedSkills, isSubmitting, goTo, handleCreate } = useCreateAgent()
  const credentialId = form.watch("credentialId")

  const selectedSkillsList = Array.from(selectedSkills.values())

  const { data: credentialsData } = $api.useQuery("get", "/v1/credentials")
  const credentials = credentialsData?.data ?? []
  const selectedCredential = credentials.find((credential) => credential.id === credentialId)

  const { data: connectionsData } = $api.useQuery("get", "/v1/in/connections")
  const connections = connectionsData?.data ?? []

  const selectedConnections = connections.filter(
    (connection) => selectedIntegrations.has(connection.id!)
  )

  const totalSelectedActions = Object.values(selectedActions).reduce(
    (sum, actions) => sum + actions.size, 0
  )
  const totalAvailableActions = selectedConnections.reduce(
    (sum, connection) => sum + (connection.actions_count ?? 0), 0
  )

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => goTo("skills")}
            className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1"
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Review & create</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Review your configuration before creating your agent.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-3 mt-4 flex-1 overflow-y-auto">
        <SummaryRow label="LLM provider">
          {selectedCredential ? (
            <span className="flex items-center gap-2">
              <ProviderLogo provider={selectedCredential.provider_id ?? ""} size={16} />
              <span className="text-sm font-medium text-foreground">{selectedCredential.label}</span>
            </span>
          ) : (
            <span className="text-sm font-medium text-foreground">None selected</span>
          )}
        </SummaryRow>

        <SummaryRow label="Integrations">
          <span className="text-sm font-medium text-foreground">
            {selectedConnections.length > 0
              ? `${selectedConnections.length} connected · ${totalSelectedActions}/${totalAvailableActions} actions`
              : "None"}
          </span>
        </SummaryRow>

        {selectedConnections.length > 0 && (
          <div className="rounded-xl bg-muted/50 px-4 py-3">
            <div className="flex flex-col gap-2">
              {selectedConnections.map((connection) => {
                const userSelected = selectedActions[connection.id!]?.size ?? 0
                const total = connection.actions_count ?? 0
                return (
                  <div key={connection.id} className="flex items-center gap-3 py-1">
                    <IntegrationLogo provider={connection.provider ?? ""} size={20} className="shrink-0" />
                    <span className="text-sm font-medium text-foreground">{connection.display_name}</span>
                    <span className="text-xs text-muted-foreground ml-auto font-mono">
                      {userSelected}/{total} actions
                    </span>
                  </div>
                )
              })}
            </div>
          </div>
        )}

        <SummaryRow label="Skills">
          <span className="text-sm font-medium text-foreground">
            {selectedSkillsList.length > 0 ? `${selectedSkillsList.length} attached` : "None"}
          </span>
        </SummaryRow>

        {selectedSkillsList.length > 0 && (
          <div className="rounded-xl bg-muted/50 px-4 py-3">
            <div className="flex flex-col gap-2">
              {selectedSkillsList.map((skill) => (
                <div key={skill.id} className="flex items-center gap-3 py-1">
                  <div className={`flex items-center justify-center size-6 rounded-md shrink-0 ${
                    skill.sourceType === "git" ? "bg-foreground/5" : "bg-blue-500/10"
                  }`}>
                    <HugeiconsIcon
                      icon={skill.sourceType === "git" ? GithubIcon : FileEditIcon}
                      size={12}
                      className={skill.sourceType === "git" ? "text-foreground" : "text-blue-500"}
                    />
                  </div>
                  <span className="text-sm font-medium text-foreground truncate">{skill.name}</span>
                  <span className="text-2xs font-medium uppercase tracking-wide text-muted-foreground ml-auto">
                    {skill.scope === "public" ? "Public" : "Your org"}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={handleCreate} className="w-full" loading={isSubmitting}>
          Create agent
        </Button>
      </div>
    </div>
  )
}
