"use client"

import { useEffect, useState } from "react"
import { toast } from "sonner"
import { useQueryClient } from "@tanstack/react-query"
import { HugeiconsIcon } from "@hugeicons/react"
import { Loading03Icon, ArrowUp01Icon, ArrowDown01Icon } from "@hugeicons/core-free-icons"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { usePaystackPop } from "@/hooks/use-paystack-pop"
import type { components } from "@/lib/api/schema"

type Plan = components["schemas"]["planDTO"]
type Preview = components["schemas"]["previewChangeResponse"]

interface ChangePlanDialogProps {
  /** The plan the user wants to switch to. Null hides the dialog. */
  targetPlan: Plan | null
  onClose: () => void
  /** Called after the change is applied so the page can show a celebration. */
  onApplied?: (kind: "upgrade" | "downgrade", planSlug: string) => void
}

function formatMoney(minor: number, currency: string) {
  const value = minor / 100
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
      maximumFractionDigits: currency === "NGN" || currency === "JPY" ? 0 : 2,
    }).format(value)
  } catch {
    return `${value} ${currency}`
  }
}

function formatDate(iso?: string) {
  if (!iso) return ""
  try {
    return new Date(iso).toLocaleDateString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
    })
  } catch {
    return iso
  }
}

export function ChangePlanDialog({ targetPlan, onClose, onApplied }: ChangePlanDialogProps) {
  const open = targetPlan !== null
  const queryClient = useQueryClient()
  const previewMutation = $api.useMutation("post", "/v1/billing/subscription/preview-change")
  const checkoutMutation = $api.useMutation("post", "/v1/billing/checkout")
  const applyMutation = $api.useMutation("post", "/v1/billing/subscription/apply-change")
  const { openPopup } = usePaystackPop()

  const [preview, setPreview] = useState<Preview | null>(null)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [confirming, setConfirming] = useState(false)

  // Fetch a fresh preview each time the dialog opens for a different plan.
  useEffect(() => {
    if (!targetPlan?.slug) return
    setPreview(null)
    setPreviewError(null)
    previewMutation.mutate(
      { body: { plan_slug: targetPlan.slug } },
      {
        onSuccess: (data) => setPreview(data),
        onError: (err) =>
          setPreviewError(extractErrorMessage(err, "Could not compute change")),
      },
    )
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [targetPlan?.slug])

  const isUpgrade = preview?.kind === "upgrade"

  function handleConfirm() {
    if (!preview || !targetPlan?.slug || !targetPlan.currency) return
    setConfirming(true)

    if (preview.kind === "downgrade") {
      applyMutation.mutate(
        { body: { quote_id: preview.quote_id! } },
        {
          onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["get", "/v1/billing/subscription"] })
            toast.success(`Downgrade scheduled for ${formatDate(preview.effective_at)}`)
            onApplied?.("downgrade", targetPlan.slug!)
            setConfirming(false)
            onClose()
          },
          onError: (err) => {
            toast.error(extractErrorMessage(err, "Could not schedule downgrade"))
            setConfirming(false)
          },
        },
      )
      return
    }

    // Upgrade: spin up a Paystack transaction for the prorated amount, then
    // hand the reference to apply-change once the popup completes.
    const returnURL = typeof window !== "undefined" ? window.location.href : ""
    checkoutMutation.mutate(
      {
        body: {
          provider: "paystack",
          plan_slug: targetPlan.slug,
          currency: targetPlan.currency,
          cycle: "monthly",
          success_url: returnURL,
          cancel_url: returnURL,
        } as never,
      },
      {
        onSuccess: (data) => {
          if (!data.access_code) {
            toast.error("Provider did not return an access code")
            setConfirming(false)
            return
          }
          openPopup(
            data.access_code,
            (reference) => {
              applyMutation.mutate(
                { body: { quote_id: preview.quote_id!, paystack_reference: reference } },
                {
                  onSuccess: () => {
                    queryClient.invalidateQueries({ queryKey: ["get", "/v1/billing/subscription"] })
                    queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
                    onApplied?.("upgrade", targetPlan.slug!)
                    setConfirming(false)
                    onClose()
                  },
                  onError: (err) => {
                    toast.error(extractErrorMessage(err, "Payment received but change failed to apply"))
                    setConfirming(false)
                  },
                },
              )
            },
            () => setConfirming(false),
          )
        },
        onError: (err) => {
          toast.error(extractErrorMessage(err, "Could not start payment"))
          setConfirming(false)
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next) onClose() }}>
      <DialogContent showCloseButton className="sm:max-w-md">
        <DialogTitle className="flex items-center gap-2 text-[15px]">
          {isUpgrade ? (
            <HugeiconsIcon icon={ArrowUp01Icon} size={16} className="text-emerald-500" />
          ) : (
            <HugeiconsIcon icon={ArrowDown01Icon} size={16} className="text-muted-foreground" />
          )}
          Switch to {targetPlan?.name ?? "new plan"}
        </DialogTitle>
        <DialogDescription className="text-[13px]">
          {isUpgrade
            ? "We've prorated the price difference for the rest of this period."
            : preview?.kind === "downgrade"
              ? `Your plan changes on ${formatDate(preview.effective_at)} — you'll keep your current benefits until then.`
              : "Loading change preview…"}
        </DialogDescription>

        {previewError ? (
          <p className="rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-[13px] text-destructive">
            {previewError}
          </p>
        ) : !preview ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-4 w-1/2" />
          </div>
        ) : (
          <div className="rounded-lg border border-border/60 bg-muted/30 p-4">
            <div className="flex items-baseline justify-between">
              <span className="text-[12px] text-muted-foreground">
                {isUpgrade ? "Charged today" : "Charged today"}
              </span>
              <span className="font-mono text-[16px] tabular-nums">
                {formatMoney(preview.amount_minor ?? 0, preview.currency ?? "USD")}
              </span>
            </div>
            {isUpgrade && (preview.credit_grant_minor ?? 0) > 0 ? (
              <div className="mt-2 flex items-baseline justify-between text-[12px]">
                <span className="text-muted-foreground">Bonus credits this period</span>
                <span className="font-mono tabular-nums">
                  {(preview.credit_grant_minor ?? 0).toLocaleString("en-US")}
                </span>
              </div>
            ) : null}
            <div className="mt-2 flex items-baseline justify-between text-[12px]">
              <span className="text-muted-foreground">Effective</span>
              <span>{formatDate(preview.effective_at)}</span>
            </div>
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={confirming}>
            Cancel
          </Button>
          <Button
            onClick={handleConfirm}
            disabled={!preview || confirming}
            className="min-w-[120px]"
          >
            {confirming ? (
              <HugeiconsIcon icon={Loading03Icon} size={14} className="animate-spin" />
            ) : isUpgrade ? (
              "Pay & switch"
            ) : (
              "Schedule downgrade"
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
