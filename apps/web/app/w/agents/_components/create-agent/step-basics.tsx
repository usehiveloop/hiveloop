"use client"

import { useMemo } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { ModelCombobox } from "./model-combobox"
import { $api } from "@/lib/api/hooks"
import { useCreateAgent } from "./context"

export function StepBasics() {
  const { form, mode, goTo } = useCreateAgent()
  const credentialId = form.watch("credentialId")
  const name = form.watch("name")
  const model = form.watch("model")

  const { data: credentialsData } = $api.useQuery("get", "/v1/credentials")
  const credentials = credentialsData?.data ?? []
  const selectedCredential = credentials.find((credential) => credential.id === credentialId)

  const { data: modelsData, isLoading: modelsLoading } = $api.useQuery(
    "get",
    "/v1/providers/{id}/models",
    { params: { path: { id: selectedCredential?.provider_id ?? "" } } },
    { enabled: !!selectedCredential?.provider_id },
  )

  const modelIds = useMemo(() => {
    return (modelsData ?? []).map((item) => item.id ?? "").filter(Boolean)
  }, [modelsData])

  const canSubmit = name.trim().length > 0 && model.length > 0

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={() => goTo("llm-key")} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Agent details</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Give your agent a name, pick a model, and optionally describe what it does.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-5 mt-4 flex-1">
        <div className="flex flex-col gap-2">
          <Label htmlFor="agent-name" className="text-sm">Name</Label>
          <Input
            id="agent-name"
            {...form.register("name")}
            placeholder="e.g. Issue Triage Agent"
          />
        </div>

        <div className="flex flex-col gap-2">
          <Label className="text-sm">Model</Label>
          <ModelCombobox
            models={modelIds}
            value={model || null}
            onSelect={(value) => form.setValue("model", value)}
            loading={modelsLoading}
            disabled={modelsLoading || modelIds.length === 0}
          />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="agent-description" className="text-sm">
            Description <span className="text-muted-foreground font-normal">(optional)</span>
          </Label>
          <Textarea
            id="agent-description"
            {...form.register("description")}
            placeholder="Briefly describe what this agent does..."
            className="min-h-24"
          />
        </div>
      </div>

      <div className="pt-4 shrink-0">
        <Button
          onClick={() => goTo(mode === "scratch" ? "system-prompt" : "forge-judge")}
          className="w-full"
          disabled={!canSubmit}
        >
          Continue
        </Button>
      </div>
    </div>
  )
}
