"use client"

import * as React from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import { Loading03Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuth } from "@/lib/auth/auth-context"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"

export function WorkspaceNameField() {
  const { activeOrg } = useAuth()
  const queryClient = useQueryClient()
  const updateOrg = $api.useMutation("patch", "/v1/orgs/current")

  const savedName = activeOrg?.name ?? ""
  const [value, setValue] = React.useState(savedName)

  // Re-sync when the active org changes (workspace switch or post-mutation refetch).
  React.useEffect(() => {
    setValue(savedName)
  }, [savedName])

  const trimmed = value.trim()
  const dirty = trimmed.length > 0 && trimmed !== savedName
  const isSaving = updateOrg.isPending

  function handleSave() {
    if (!dirty || isSaving) return
    updateOrg.mutate(
      { body: { name: trimmed } },
      {
        onSuccess: () => {
          toast.success("Workspace name updated")
          queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to update workspace name"))
        },
      }
    )
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter" && dirty && !isSaving) {
      event.preventDefault()
      handleSave()
    }
    if (event.key === "Escape") {
      setValue(savedName)
    }
  }

  return (
    <section className="flex flex-col gap-2.5">
      <div>
        <Label htmlFor="workspace-name" className="text-[13px] font-medium">
          Workspace name
        </Label>
        <p className="mt-0.5 text-[12px] text-muted-foreground">
          Used in URLs, invitations, and email subject lines.
        </p>
      </div>
      <div className="relative max-w-sm">
        <Input
          id="workspace-name"
          value={value}
          onChange={(event) => setValue(event.target.value)}
          onKeyDown={handleKeyDown}
          disabled={isSaving}
          className="pr-10"
        />
        {dirty || isSaving ? (
          <button
            type="button"
            onClick={handleSave}
            disabled={!dirty || isSaving}
            aria-label="Save workspace name"
            className="absolute top-1/2 right-1 flex size-7 -translate-y-1/2 items-center justify-center rounded-full bg-primary text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-60"
          >
            <HugeiconsIcon
              icon={isSaving ? Loading03Icon : Tick02Icon}
              strokeWidth={2.5}
              className={
                "size-3.5 " + (isSaving ? "animate-spin" : "")
              }
            />
          </button>
        ) : null}
      </div>
    </section>
  )
}
