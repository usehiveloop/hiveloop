"use client"

import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { toast } from "sonner"

type Subscription = {
  plan_slug?: string
  status?: string
  provider?: string
  credits_balance?: number
  current_period_end?: string
}

export function BillingSettings() {
  const { data: subscriptionData, isLoading } = $api.useQuery(
    "get",
    "/v1/billing/subscription",
    {},
    { refetchOnWindowFocus: true },
  )
  const portalMutation = $api.useMutation("post", "/v1/billing/portal")

  const subscription = subscriptionData as Subscription | undefined

  function handleManageBilling() {
    if (!subscription?.provider) {
      toast.error("No active billing provider for this org")
      return
    }
    portalMutation.mutate(
      { body: { provider: subscription.provider } } as any,
      {
        onSuccess: (response) => {
          const portalUrl = (response as { portal_url?: string })?.portal_url
          if (portalUrl) window.open(portalUrl, "_blank")
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to open billing portal"))
        },
      },
    )
  }

  if (isLoading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-6 w-32" />
        <Skeleton className="h-24 w-full" />
      </div>
    )
  }

  const planSlug = subscription?.plan_slug ?? "free"
  const balance = subscription?.credits_balance ?? 0
  const hasActiveSubscription = Boolean(subscription?.provider)

  return (
    <div className="space-y-6">
      <div>
        <div className="flex items-center gap-3">
          <h3 className="text-sm font-medium text-foreground">Current plan</h3>
          <Badge variant={planSlug === "free" ? "secondary" : "default"}>
            {planSlug}
          </Badge>
          {subscription?.status && (
            <Badge variant="outline">{subscription.status}</Badge>
          )}
        </div>
        {subscription?.current_period_end && (
          <p className="mt-2 text-xs text-muted-foreground">
            Renews {new Date(subscription.current_period_end).toLocaleDateString()}
          </p>
        )}
      </div>

      <div className="rounded-xl border border-border p-5 space-y-1">
        <p className="text-xs text-muted-foreground">Credits balance</p>
        <p className="text-2xl font-semibold text-foreground">
          {balance.toLocaleString()}
        </p>
        <p className="text-xs text-muted-foreground">
          Credits are consumed per conversation and per LLM call.
        </p>
      </div>

      {hasActiveSubscription && (
        <div>
          <Button
            variant="outline"
            size="sm"
            onClick={handleManageBilling}
            loading={portalMutation.isPending}
          >
            Manage billing
          </Button>
          <p className="mt-2 text-xs text-muted-foreground">
            View invoices, update payment methods, or cancel your subscription.
          </p>
        </div>
      )}
    </div>
  )
}
