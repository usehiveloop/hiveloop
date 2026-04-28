"use client"

import { useCallback, useState } from "react"
import { toast } from "sonner"
import { useQueryClient } from "@tanstack/react-query"
import PaystackPop from "@paystack/inline-js"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useAuth } from "@/lib/auth/auth-context"
import type { components } from "@/lib/api/schema"

type Plan = components["schemas"]["planDTO"]

interface UsePaystackPopOptions {
  /** Called after the verify endpoint confirms the subscription is active. */
  onSubscribed?: (planSlug: string) => void
}

interface UsePaystackPopHandlers {
  /**
   * Kick off the full subscribe flow for a plan:
   *   1. POST /v1/billing/checkout — backend initialises a Paystack
   *      transaction and returns access_code + reference.
   *   2. PaystackPop.resumeTransaction(access_code) — opens the popup.
   *   3. POST /v1/billing/verify — backend resolves the reference against
   *      Paystack and upserts the Subscription synchronously.
   */
  subscribe: (plan: Plan) => void
  /** Slug of the plan currently mid-flight, or null. */
  pendingSlug: string | null
  /** True iff any subscribe flow is in progress. */
  isPending: boolean
}

export function usePaystackPop(
  options: UsePaystackPopOptions = {},
): UsePaystackPopHandlers {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const checkout = $api.useMutation("post", "/v1/billing/checkout")
  const verify = $api.useMutation("post", "/v1/billing/verify")
  const [pendingSlug, setPendingSlug] = useState<string | null>(null)

  const subscribe = useCallback(
    (plan: Plan) => {
      if (!plan.slug) {
        toast.error("Plan is missing a slug")
        return
      }
      if (!user?.email) {
        toast.error("Please sign in again before subscribing")
        return
      }
      if (!plan.currency) {
        toast.error("Plan is missing a currency")
        return
      }

      const returnURL =
        typeof window !== "undefined" ? window.location.href : ""

      setPendingSlug(plan.slug)

      checkout.mutate(
        {
          body: {
            provider: "paystack",
            plan_slug: plan.slug,
            currency: plan.currency,
            cycle: "monthly",
            success_url: returnURL,
            cancel_url: returnURL,
          } as never,
        },
        {
          onSuccess: (data) => {
            if (!data.access_code) {
              toast.error("Provider did not return an access code")
              setPendingSlug(null)
              return
            }
            const reference = data.reference
            if (!reference) {
              toast.error("Provider did not return a reference")
              setPendingSlug(null)
              return
            }
            const popup = new PaystackPop()
            popup.resumeTransaction(data.access_code, {
              onSuccess: () => {
                verify.mutate(
                  {
                    body: {
                      provider: "paystack",
                      reference,
                    },
                  },
                  {
                    onSuccess: (resp) => {
                      if (resp.status === "active") {
                        toast.success(`Subscribed to ${plan.name ?? plan.slug}`)
                      } else {
                        toast.message(
                          "Payment received. Your subscription will activate momentarily.",
                        )
                      }
                      queryClient.invalidateQueries({
                        queryKey: ["get", "/v1/billing/subscription"],
                      })
                      queryClient.invalidateQueries({
                        queryKey: ["get", "/auth/me"],
                      })
                      options.onSubscribed?.(plan.slug!)
                      setPendingSlug(null)
                    },
                    onError: (err) => {
                      toast.error(
                        extractErrorMessage(
                          err,
                          "Could not confirm subscription. Refresh in a moment.",
                        ),
                      )
                      setPendingSlug(null)
                    },
                  },
                )
              },
              onCancel: () => {
                // Customer dismissed the popup without paying — silent.
                setPendingSlug(null)
              },
            })
          },
          onError: (err) => {
            toast.error(extractErrorMessage(err, "Could not start checkout"))
            setPendingSlug(null)
          },
        },
      )
    },
    [user?.email, checkout, verify, queryClient, options],
  )

  return {
    subscribe,
    pendingSlug,
    isPending: pendingSlug !== null,
  }
}
