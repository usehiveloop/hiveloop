"use client"

import * as React from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Label } from "@/components/ui/label"
import { ImagePicker } from "@/components/image-picker"
import { useAuth } from "@/lib/auth/auth-context"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"

export function WorkspaceLogoField() {
  const { activeOrg } = useAuth()
  const queryClient = useQueryClient()

  // Seed from /auth/me so an existing logo paints on first render. Local state
  // tracks subsequent picks so the preview stays in sync without waiting for
  // the next /auth/me invalidation.
  const [logoUrl, setLogoUrl] = React.useState<string | undefined>(
    activeOrg?.logo_url || undefined
  )
  React.useEffect(() => {
    setLogoUrl(activeOrg?.logo_url || undefined)
  }, [activeOrg?.logo_url])

  const updateOrg = $api.useMutation("patch", "/v1/orgs/current")

  function handleUploaded(url: string | undefined) {
    setLogoUrl(url)
    if (!url) return

    updateOrg.mutate(
      { body: { logo_url: url } },
      {
        onSuccess: () => {
          toast.success("Workspace logo updated")
          queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to save workspace logo"))
        },
      }
    )
  }

  return (
    <section className="flex items-start justify-between gap-4">
      <div className="min-w-0 flex-1">
        <Label className="text-[13px] font-medium">Workspace logo</Label>
        <p className="mt-0.5 text-[12px] text-muted-foreground">
          Square. PNG, JPEG, WEBP, or GIF. Up to 5 MB.
        </p>
      </div>
      <ImagePicker
        assetType="org_logo"
        orgId={activeOrg?.id}
        value={logoUrl}
        onChange={handleUploaded}
        fallback={activeOrg?.name?.[0]?.toUpperCase() ?? "?"}
        ariaLabel={logoUrl ? "Replace workspace logo" : "Upload workspace logo"}
      />
    </section>
  )
}
