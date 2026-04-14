"use client"

import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

export function BillingSettings() {
  const queryClient = useQueryClient()
  const { data: subscriptionData, isLoading } = $api.useQuery("get", "/v1/billing/subscription", {}, {
    refetchOnWindowFocus: true,
  })
  const checkoutMutation = $api.useMutation("post", "/v1/billing/checkout")
  const portalMutation = $api.useMutation("post", "/v1/billing/portal")

  const subscription = subscriptionData as { plan?: string; status?: string; product_type?: string } | undefined

  function handleUpgrade(productType: string) {
    checkoutMutation.mutate(
      {
        body: {
          product_type: productType,
          success_url: `${window.location.origin}/w?checkout=success`,
        },
      },
      {
        onSuccess: (response) => {
          const checkoutUrl = (response as { checkout_url?: string })?.checkout_url
          if (checkoutUrl) {
            window.location.href = checkoutUrl
          }
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to start checkout"))
        },
      },
    )
  }

  function handleManageBilling() {
    portalMutation.mutate(
      {},
      {
        onSuccess: (response) => {
          const portalUrl = (response as { portal_url?: string })?.portal_url
          if (portalUrl) {
            window.open(portalUrl, "_blank")
            queryClient.invalidateQueries({ queryKey: ["get", "/v1/billing/subscription"] })
          }
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
        <Skeleton className="h-24 w-full" />
      </div>
    )
  }

  const isPro = subscription?.plan === "pro"

  if (isPro) {
    return (
      <div className="space-y-6">
        <div className="flex items-center gap-3">
          <h3 className="text-sm font-medium text-foreground">Current plan</h3>
          <Badge variant="default" className="bg-green-500/10 text-green-600 dark:text-green-400 border-green-500/20">Pro</Badge>
        </div>
        <div className="rounded-xl border border-border p-5 space-y-3">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-foreground">
                {subscription?.product_type === "pro_dedicated" ? "Pro Dedicated" : "Pro Shared"}
              </p>
              <p className="text-xs text-muted-foreground mt-1">
                {subscription?.product_type === "pro_dedicated"
                  ? "$6.99/agent/month — 300 runs included, dedicated sandbox"
                  : "$4.99/agent/month — 300 runs included, shared sandbox"}
              </p>
            </div>
            <Badge variant="outline" className="text-green-600 dark:text-green-400 border-green-500/30">{subscription?.status}</Badge>
          </div>
        </div>
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
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <div className="flex items-center gap-3">
          <h3 className="text-sm font-medium text-foreground">Current plan</h3>
          <Badge variant="secondary">Free</Badge>
        </div>
        <p className="mt-2 text-xs text-muted-foreground">
          1 agent, 100 runs/month, shared sandbox only.
        </p>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div className="rounded-xl border border-border p-5 space-y-3">
          <div>
            <p className="text-sm font-medium text-foreground">Pro Shared</p>
            <p className="text-xs text-muted-foreground mt-1">$4.99/agent/month</p>
          </div>
          <ul className="space-y-1.5 text-xs text-muted-foreground">
            <li>Unlimited agents</li>
            <li>300 runs/agent/month</li>
            <li>$0.01/run overage</li>
          </ul>
          <Button
            size="sm"
            className="w-full"
            onClick={() => handleUpgrade("pro_shared")}
            loading={checkoutMutation.isPending}
          >
            Upgrade
          </Button>
        </div>

        <div className="rounded-xl border-2 border-primary/30 p-5 space-y-3 relative overflow-hidden">
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background: "radial-gradient(circle at 50% 0%, color-mix(in oklch, var(--primary) 6%, transparent) 0%, transparent 60%)",
            }}
          />
          <div className="relative">
            <p className="text-sm font-medium text-foreground">Pro Dedicated</p>
            <p className="text-xs text-muted-foreground mt-1">$6.99/agent/month</p>
          </div>
          <ul className="space-y-1.5 text-xs text-muted-foreground relative">
            <li>Everything in Pro Shared</li>
            <li>Isolated sandbox per run</li>
            <li>Shell, filesystem, git access</li>
          </ul>
          <Button
            size="sm"
            className="w-full relative"
            onClick={() => handleUpgrade("pro_dedicated")}
            loading={checkoutMutation.isPending}
          >
            Upgrade
          </Button>
        </div>
      </div>
    </div>
  )
}
