"use client"

import { useState } from "react"
import { toast } from "sonner"
import { useQueryClient } from "@tanstack/react-query"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Progress } from "@/components/ui/progress"
import { Skeleton } from "@/components/ui/skeleton"
import { SettingsShell } from "@/components/settings-shell"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Loading03Icon,
  Tick02Icon,
  CreditCardIcon,
  BankIcon,
} from "@hugeicons/core-free-icons"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useAuth } from "@/lib/auth/auth-context"
import { usePaystackPop } from "@/hooks/use-paystack-pop"
import type { components } from "@/lib/api/schema"
import { ChangePlanDialog } from "./_components/change-plan-dialog"
import { SubscriptionSuccessDialog } from "./_components/subscription-success-dialog"

type Plan = components["schemas"]["planDTO"]

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

function formatDate(iso: string | undefined) {
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

export default function Page() {
  const { activeOrg } = useAuth()
  const fallbackSlug = activeOrg?.plan?.slug ?? "free"

  const subscriptionQuery = $api.useQuery("get", "/v1/billing/subscription")
  const plansQuery = $api.useQuery("get", "/v1/plans")
  const queryClient = useQueryClient()

  const subscription = subscriptionQuery.data
  const plans = (plansQuery.data ?? []) as Plan[]

  const currentSlug = subscription?.plan_slug || fallbackSlug
  const currentPlan = plans.find((p) => p.slug === currentSlug) ?? null
  const pendingPlan = subscription?.pending_plan_slug
    ? plans.find((p) => p.slug === subscription.pending_plan_slug) ?? null
    : null
  const onPaidPlan = (currentPlan?.price_cents ?? 0) > 0

  const credits = subscription?.credits_balance ?? activeOrg?.credits ?? 0
  const total = currentPlan
    ? (currentPlan.monthly_credits ?? 0) + (currentPlan.welcome_credits ?? 0)
    : 0
  const pctRemaining = total > 0 ? Math.min(1, Math.max(0, credits / total)) : 0

  const cancelMutation = $api.useMutation("post", "/v1/billing/subscription/cancel")
  const resumeMutation = $api.useMutation("post", "/v1/billing/subscription/resume")

  const [subscribedPlan, setSubscribedPlan] = useState<Plan | null>(null)
  const [changeTarget, setChangeTarget] = useState<Plan | null>(null)

  const { subscribe, pendingSlug, isPending: subscribing } = usePaystackPop({
    onSubscribed: (slug) => {
      const next = plans.find((p) => p.slug === slug)
      if (next) setSubscribedPlan(next)
    },
  })

  function handlePlanClick(plan: Plan) {
    if (plan.slug === currentSlug) return
    if (!onPaidPlan) {
      subscribe(plan)
      return
    }
    setChangeTarget(plan)
  }

  function handleCancel() {
    cancelMutation.mutate(
      { body: { at_period_end: true } },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/billing/subscription"] })
          toast.message("Your plan will end at the next renewal.")
        },
        onError: (err) => toast.error(extractErrorMessage(err, "Could not cancel")),
      },
    )
  }

  function handleResume() {
    resumeMutation.mutate(
      {},
      {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/billing/subscription"] })
          toast.success("Subscription resumed.")
        },
        onError: (err) => toast.error(extractErrorMessage(err, "Could not resume")),
      },
    )
  }

  return (
    <SettingsShell
      title="Billing"
      description="Plan and credits."
    >
      {/* Current plan */}
      <section>
        {subscriptionQuery.isLoading || plansQuery.isLoading ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-3 w-64" />
          </div>
        ) : currentPlan ? (
          <>
            <div className="flex items-baseline justify-between gap-4">
              <div className="flex items-baseline gap-2.5">
                <h2 className="text-[15px] font-medium">{currentPlan.name}</h2>
                <span className="text-[12px] text-muted-foreground">
                  {(currentPlan.price_cents ?? 0) > 0
                    ? `${formatMoney(currentPlan.price_cents ?? 0, currentPlan.currency ?? "USD")} / month`
                    : "Free"}
                </span>
              </div>
              <span
                className={
                  "rounded-full px-2 py-0.5 text-[11px] " +
                  (subscription?.status === "active" || (currentPlan.price_cents ?? 0) === 0
                    ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                    : "bg-muted text-muted-foreground")
                }
              >
                {subscription?.status ?? "active"}
              </span>
            </div>
            {subscription?.current_period_end ? (
              <p className="mt-1.5 text-[12px] text-muted-foreground">
                Renews <span className="text-foreground">{formatDate(subscription.current_period_end)}</span>.
              </p>
            ) : null}

            {pendingPlan ? (
              <div className="mt-3 rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-[12px] text-amber-700 dark:text-amber-400">
                Switches to <span className="font-medium">{pendingPlan.name}</span> on{" "}
                {formatDate(subscription?.pending_change_at ?? undefined)}.
              </div>
            ) : null}

            {subscription?.cancel_at_period_end ? (
              <div className="mt-3 flex items-center justify-between rounded-lg border border-border/60 bg-muted/40 px-3 py-2 text-[12px]">
                <span>
                  Cancels at the end of this period — {formatDate(subscription.current_period_end)}.
                </span>
                <Button size="sm" variant="ghost" onClick={handleResume} loading={resumeMutation.isPending}>
                  Resume
                </Button>
              </div>
            ) : onPaidPlan ? (
              <div className="mt-3 flex justify-end">
                <Button size="sm" variant="ghost" onClick={handleCancel} loading={cancelMutation.isPending}>
                  Cancel subscription
                </Button>
              </div>
            ) : null}

            {(subscription?.card_last4 || subscription?.payment_bank_name) ? (
              <div className="mt-3 flex items-center gap-2 text-[12px] text-muted-foreground">
                {subscription.payment_channel === "bank" ? (
                  <HugeiconsIcon icon={BankIcon} size={14} />
                ) : (
                  <HugeiconsIcon icon={CreditCardIcon} size={14} />
                )}
                <span>
                  {subscription.payment_channel === "bank"
                    ? `${subscription.payment_bank_name ?? "Bank"} · ${subscription.payment_account_name ?? ""}`
                    : `${(subscription.card_brand ?? "Card").toUpperCase()} ending ${subscription.card_last4}`}
                </span>
              </div>
            ) : null}
          </>
        ) : (
          <p className="text-[13px] text-muted-foreground">
            No active plan. Pick one below to get started.
          </p>
        )}
      </section>

      {/* Credits */}
      <section>
        <div className="flex items-baseline justify-between">
          <Label className="text-[13px] font-medium">Credits this period</Label>
          <span className="font-mono text-[12px] tabular-nums text-muted-foreground">
            <span className="text-foreground">{credits.toLocaleString("en-US")}</span>
            {total > 0 ? (
              <>
                <span className="px-1 text-muted-foreground/50">/</span>
                {total.toLocaleString("en-US")}
              </>
            ) : null}
          </span>
        </div>
        <Progress value={Math.round(pctRemaining * 100)} max={100} className="mt-2" aria-label="Credits remaining" />
        {subscription?.current_period_end && total > 0 ? (
          <p className="mt-2 text-[12px] text-muted-foreground">
            Resets {formatDate(subscription.current_period_end)}.
          </p>
        ) : null}
      </section>

      {/* Available plans */}
      <section>
        <h2 className="mb-3 text-[13px] font-medium">Plans</h2>
        {plansQuery.isLoading ? (
          <div className="flex flex-col gap-2">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-32 w-full rounded-lg" />
            ))}
          </div>
        ) : plans.length === 0 ? (
          <p className="text-[13px] text-muted-foreground">No plans available.</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {plans.map((plan) => {
              const active = plan.slug === currentSlug
              const isFree = (plan.price_cents ?? 0) === 0
              const features = plan.features ?? []
              const clickable = !active && (onPaidPlan || !isFree)
              const loading = pendingSlug === plan.slug
              return (
                <li key={plan.slug}>
                  <button
                    type="button"
                    disabled={!clickable || subscribing}
                    onClick={() => clickable && handlePlanClick(plan)}
                    className={
                      "relative flex w-full flex-col gap-3 rounded-lg border px-3.5 py-3 text-left transition-colors disabled:cursor-default " +
                      (active
                        ? "border-primary/40 bg-primary/5"
                        : loading
                          ? "border-primary"
                          : clickable
                            ? "cursor-pointer border-border/60 hover:border-primary"
                            : "border-border/60")
                    }
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <p className="text-[13px] font-medium">{plan.name}</p>
                          {active ? (
                            <span className="flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] text-emerald-600 dark:text-emerald-400">
                              <HugeiconsIcon icon={Tick02Icon} size={10} />
                              Current
                            </span>
                          ) : null}
                        </div>
                        <p className="mt-0.5 text-[12px] text-muted-foreground">
                          {(plan.monthly_credits ?? 0).toLocaleString("en-US")} credits / month
                          {(plan.welcome_credits ?? 0) > 0
                            ? ` · ${(plan.welcome_credits ?? 0).toLocaleString("en-US")} welcome`
                            : ""}
                        </p>
                      </div>
                      <span className="shrink-0 font-mono text-[13px] tabular-nums">
                        {isFree ? "Free" : formatMoney(plan.price_cents ?? 0, plan.currency ?? "USD")}
                      </span>
                    </div>

                    {loading ? (
                      <HugeiconsIcon
                        icon={Loading03Icon}
                        size={14}
                        className="absolute bottom-3 right-3 animate-spin text-muted-foreground"
                      />
                    ) : null}

                    {features.length > 0 ? (
                      <ul className="grid grid-cols-1 gap-x-4 gap-y-1.5 sm:grid-cols-2">
                        {features.map((feature) => (
                          <li
                            key={feature}
                            className="flex items-center gap-2 text-[12px] text-muted-foreground"
                          >
                            <HugeiconsIcon
                              icon={Tick02Icon}
                              size={12}
                              className="shrink-0 text-emerald-500"
                            />
                            <span className="truncate">{feature}</span>
                          </li>
                        ))}
                      </ul>
                    ) : null}
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </section>

      <ChangePlanDialog
        targetPlan={changeTarget}
        onClose={() => setChangeTarget(null)}
        onApplied={(kind, slug) => {
          if (kind === "upgrade") {
            const next = plans.find((p) => p.slug === slug)
            if (next) setSubscribedPlan(next)
          }
        }}
      />

      <SubscriptionSuccessDialog
        plan={subscribedPlan}
        onClose={() => setSubscribedPlan(null)}
      />
    </SettingsShell>
  )
}
