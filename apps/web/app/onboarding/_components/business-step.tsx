"use client"

import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Controller, useWatch } from "react-hook-form"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  ArrowLeft01Icon,
  Loading03Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { ImagePicker } from "@/components/image-picker"
import { useAuth } from "@/lib/auth/auth-context"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { StepHeader } from "./step-header"
import { useOnboarding } from "./context"

export function BusinessStep() {
  const { form, goBack } = useOnboarding()
  const { activeOrg } = useAuth()
  const router = useRouter()
  const queryClient = useQueryClient()

  const name = useWatch({ control: form.control, name: "businessName" })
  const website = useWatch({ control: form.control, name: "businessWebsite" })
  const description = useWatch({ control: form.control, name: "businessDescription" })
  const logoUrl = useWatch({ control: form.control, name: "businessLogoUrl" })

  const completeOnboarding = $api.useMutation(
    "post",
    "/v1/orgs/current/onboarding/complete",
  )
  const submitting = completeOnboarding.isPending
  const errorMessage = completeOnboarding.isError
    ? extractErrorMessage(
        completeOnboarding.error,
        "Could not finish onboarding. Try again.",
      )
    : null

  const canSubmit =
    (name?.trim().length ?? 0) > 0 &&
    (website?.trim().length ?? 0) > 0 &&
    (description?.trim().length ?? 0) >= 30 &&
    !submitting

  function handleSubmit() {
    completeOnboarding.mutate(
      {
        body: {
          name: name.trim(),
          website: website.trim(),
          logo_url: logoUrl?.trim() || "",
          description: description.trim(),
        },
      },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/orgs/current"] })
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
          router.push("/w")
        },
      },
    )
  }

  return (
    <div className="mx-auto flex w-full max-w-md flex-col gap-8">
      <StepHeader
        title="Tell your AI employee about your business"
        description="The more they know, the better they answer customers and teammates from day one."
      />

      <div className="flex flex-col gap-5">
        <Controller
          control={form.control}
          name="businessLogoUrl"
          render={({ field }) => (
            <section className="flex items-start justify-between gap-4">
              <div className="min-w-0 flex-1">
                <Label className="text-[13px] font-medium">Logo</Label>
                <p className="mt-0.5 text-[12px] text-muted-foreground">
                  Square. PNG, JPEG, WEBP, or GIF. Up to 5 MB.
                </p>
              </div>
              <ImagePicker
                assetType="org_logo"
                orgId={activeOrg?.id}
                value={field.value || undefined}
                onChange={(url) => field.onChange(url ?? "")}
                fallback={name?.[0]?.toUpperCase() ?? "?"}
                ariaLabel={field.value ? "Replace business logo" : "Upload business logo"}
              />
            </section>
          )}
        />

        <div className="flex flex-col gap-2.5">
          <Label htmlFor="business-name">Business name</Label>
          <Input
            id="business-name"
            placeholder="Acme"
            autoComplete="organization"
            {...form.register("businessName")}
          />
        </div>

        <div className="flex flex-col gap-2.5">
          <Label htmlFor="business-website">Website</Label>
          <Input
            id="business-website"
            type="url"
            placeholder="https://acme.com"
            autoComplete="url"
            {...form.register("businessWebsite")}
          />
        </div>

        <div className="flex flex-col gap-2.5">
          <Label htmlFor="business-description">What does your business do?</Label>
          <Textarea
            id="business-description"
            placeholder="We help small ecommerce brands run loyalty programs. Customers earn points for orders, reviews, and referrals; admins manage rewards in our dashboard…"
            rows={5}
            {...form.register("businessDescription")}
          />
          <p className="text-xs text-muted-foreground">
            Aim for 2–3 sentences. Mention your audience, what problem you solve, and your tone.
          </p>
        </div>
      </div>

      {errorMessage ? (
        <div className="flex items-start gap-2.5 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-[13px] text-destructive">
          <HugeiconsIcon
            icon={Alert02Icon}
            className="mt-0.5 size-4 shrink-0"
            strokeWidth={2}
          />
          <span className="leading-relaxed">{errorMessage}</span>
        </div>
      ) : null}

      <div className="flex items-center justify-between">
        <Button
          variant="ghost"
          onClick={goBack}
          disabled={submitting}
          className="gap-2"
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} className="size-4" />
          Back
        </Button>
        <Button onClick={handleSubmit} disabled={!canSubmit} className="gap-2">
          {submitting ? (
            <>
              <HugeiconsIcon
                icon={Loading03Icon}
                className="size-4 animate-spin"
                strokeWidth={2}
              />
              Finishing…
            </>
          ) : (
            <>
              Finish setup
              <HugeiconsIcon icon={Tick02Icon} className="size-4" />
            </>
          )}
        </Button>
      </div>
    </div>
  )
}
