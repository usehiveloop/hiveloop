"use client"

import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, Key01Icon } from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { LlmKeyCard } from "@/components/llm-key-card"
import { $api } from "@/lib/api/hooks"
import { AddLlmKeyDialog } from "./add-llm-key-dialog"
import { useCreateAgent } from "./context"

export function StepLlmKey() {
  const { form, goTo } = useCreateAgent()
  const selectedKey = form.watch("credentialId")
  const [addKeyOpen, setAddKeyOpen] = useState(false)

  const { data, isLoading } = $api.useQuery("get", "/v1/credentials")
  const credentials = data?.data ?? []

  function handleSelect(credentialId: string) {
    form.setValue("credentialId", credentialId)
    goTo("basics")
  }

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={() => goTo("integrations")} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Select an llm key</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Choose which AI provider your agent will use. You can add a new key if you haven&apos;t connected one yet.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {isLoading ? (
          Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="h-18 w-full rounded-xl" />
          ))
        ) : credentials.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <p className="text-sm text-muted-foreground">No credentials yet. Add one below.</p>
          </div>
        ) : (
          credentials.map((credential) => (
            <LlmKeyCard
              key={credential.id}
              label={credential.label}
              providerId={credential.provider_id ?? ""}
              selected={selectedKey === credential.id}
              onClick={() => handleSelect(credential.id ?? "")}
            />
          ))
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button variant="outline" className="w-full" onClick={() => setAddKeyOpen(true)}>
          <HugeiconsIcon icon={Key01Icon} size={16} data-icon="inline-start" />
          Add llm key
        </Button>
      </div>

      <AddLlmKeyDialog
        open={addKeyOpen}
        onOpenChange={setAddKeyOpen}
        onCreated={(credentialId) => handleSelect(credentialId)}
      />
    </div>
  )
}
