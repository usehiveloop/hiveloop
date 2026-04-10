"use client"

import { useMemo } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, ArrowRight01Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { ProviderLogo } from "@/components/provider-logo"
import { ModelCombobox } from "./model-combobox"
import { $api } from "@/lib/api/hooks"
import { useCreateAgent } from "./context"

export function StepForgeJudge() {
  const { form, goTo } = useCreateAgent()
  const agentCredentialId = form.watch("credentialId")
  const judgeKeyId = form.watch("judgeKeyId")
  const judgeModel = form.watch("judgeModel")

  const { data: credentialsData, isLoading: credentialsLoading } = $api.useQuery("get", "/v1/credentials")
  const credentials = credentialsData?.data ?? []

  const selectedCredential = credentials.find((credential) => credential.id === judgeKeyId)
  const agentCredential = credentials.find((credential) => credential.id === agentCredentialId)
  const isSameProvider = agentCredential && selectedCredential && agentCredential.provider_id === selectedCredential.provider_id

  const { data: modelsData } = $api.useQuery(
    "get",
    "/v1/providers/{id}/models",
    { params: { path: { id: selectedCredential?.provider_id ?? "" } } },
    { enabled: !!selectedCredential?.provider_id },
  )

  const modelsForSelected = useMemo(() => {
    if (!modelsData) return []
    return (modelsData as { id?: string }[]).map((model) => model.id ?? "").filter(Boolean)
  }, [modelsData])

  function handleSelectCredential(credentialId: string) {
    form.setValue("judgeKeyId", credentialId)
    form.setValue("judgeModel", "")
  }

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={() => goTo("basics")} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Forge judge</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Pick an LLM to evaluate and score your agent during the forge process. A different provider from your agent&apos;s LLM is recommended.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-4 mt-4 flex-1 overflow-y-auto">
        <div className="flex flex-col gap-2">
          <Label className="text-sm">Provider</Label>
          <div className="flex flex-col gap-2">
            {credentialsLoading ? (
              Array.from({ length: 3 }).map((_, index) => (
                <Skeleton key={index} className="h-13 w-full rounded-xl" />
              ))
            ) : credentials.length === 0 ? (
              <p className="text-sm text-muted-foreground py-4 text-center">No LLM keys available. Add one first.</p>
            ) : (
              credentials.map((credential) => (
                <button
                  key={credential.id}
                  type="button"
                  onClick={() => credential.id ? handleSelectCredential(credential.id): null}
                  className={`group flex items-center gap-3 w-full rounded-xl p-3 text-left transition-colors cursor-pointer ${
                    judgeKeyId === credential.id
                      ? "bg-primary/5 border border-primary/20"
                      : "bg-muted/50 hover:bg-muted border border-transparent"
                  }`}
                >
                  <ProviderLogo provider={credential.provider_id ?? ""} size={24} className="shrink-0" />
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-foreground truncate">{credential.label}</p>
                    <p className="text-xs text-muted-foreground">{credential.provider_id}</p>
                  </div>
                  {judgeKeyId === credential.id ? (
                    <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0" />
                  ) : (
                    <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0" />
                  )}
                </button>
              ))
            )}
          </div>
        </div>

        {selectedCredential && (
          <div className="flex flex-col gap-2">
            <Label className="text-sm">Model</Label>
            <ModelCombobox
              models={modelsForSelected}
              value={judgeModel || null}
              onSelect={(model) => form.setValue("judgeModel", model)}
            />
          </div>
        )}

        {isSameProvider && (
          <div className="rounded-xl border border-amber-500/20 bg-amber-500/5 px-4 py-3 flex gap-3 items-start">
            <span className="text-amber-500 text-sm leading-none mt-0.5">!</span>
            <p className="text-sm text-amber-500/90 leading-snug">
              Using a different AI model for the forge judge reduces bias and can lead to a more efficient agent.
            </p>
          </div>
        )}
      </div>

      <div className="flex flex-col gap-2 pt-4 shrink-0">
        <Button onClick={() => goTo("skills")} disabled={!judgeKeyId || !judgeModel} className="w-full">Continue</Button>
        <Button variant="ghost" onClick={() => goTo("skills")} className="w-full text-muted-foreground">
          Skip — use default judge
        </Button>
      </div>
    </div>
  )
}
