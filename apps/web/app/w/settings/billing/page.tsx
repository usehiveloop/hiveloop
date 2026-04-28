"use client"

import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Progress } from "@/components/ui/progress"
import { Skeleton } from "@/components/ui/skeleton"
import { SettingsShell } from "@/components/settings-shell"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowUpRight03Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useAuth } from "@/lib/auth/auth-context"
import type { components } from "@/lib/api/schema"

type Plan = components["schemas"]["planDTO"]

const DEFAULT_PROVIDER = "paystack"

const PLAN_FEATURES: Record<string, string[]> = {
  free: [
    "1,000 welcome credits (one-time)",
    "Up to 3 agents",
    "Community support",
  ],
  starter: [
    "9,000 credits every month",
    "Unlimited agents",
    "Webhook & cron triggers",
    "Email support",
  ],
  pro: [
    "39,000 credits every month",
    "Everything in Starter",
    "Custom sandbox templates",
    "Custom preview domains",
    "Priority support",
  ],
  business: [
    "99,000 credits every month",
    "Everything in Pro",
    "SSO (SAML)",
    "99.9% uptime SLA",
    "Dedicated support",
  ],
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

  const subscription = subscriptionQuery.data
  const plans = (plansQuery.data ?? []) as Plan[]

  const currentSlug = subscription?.plan_slug || fallbackSlug
  const currentPlan = plans.find((p) => p.slug === currentSlug) ?? null

  const credits = subscription?.credits_balance ?? activeOrg?.credits ?? 0
  const total = currentPlan
    ? (currentPlan.monthly_credits ?? 0) + (currentPlan.welcome_credits ?? 0)
    : 0
  const pctRemaining =
    total > 0 ? Math.min(1, Math.max(0, credits / total)) : 0

  const portal = $api.useMutation("post", "/v1/billing/portal")

  function handleManage() {
    portal.mutate(
      {
        body: {
          provider: subscription?.provider ?? DEFAULT_PROVIDER,
        } as never,
      },
      {
        onSuccess: (data) => {
          if (data.portal_url) window.location.href = data.portal_url
        },
        onError: (err) => {
          toast.error(extractErrorMessage(err, "Failed to open billing portal"))
        },
      },
    )
  }

  const hasProvider = Boolean(subscription?.provider)

  return (
    <SettingsShell
      title="Billing"
      description="Plan, credits, and your provider's portal."
      action={
        hasProvider ? (
          <Button
            variant="outline"
            size="sm"
            onClick={handleManage}
            loading={portal.isPending}
          >
            Manage billing
            <HugeiconsIcon icon={ArrowUpRight03Icon} size={13} />
          </Button>
        ) : null
      }
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
                  (subscription?.status === "active" ||
                  (currentPlan.price_cents ?? 0) === 0
                    ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                    : "bg-muted text-muted-foreground")
                }
              >
                {subscription?.status ?? "active"}
              </span>
            </div>
            {subscription?.current_period_end ? (
              <p className="mt-1.5 text-[12px] text-muted-foreground">
                Renews{" "}
                <span className="text-foreground">
                  {formatDate(subscription.current_period_end)}
                </span>
                .
              </p>
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
          <Label className="text-[13px] font-medium">
            Credits this period
          </Label>
          <span className="font-mono text-[12px] tabular-nums text-muted-foreground">
            <span className="text-foreground">
              {credits.toLocaleString("en-US")}
            </span>
            {total > 0 ? (
              <>
                <span className="px-1 text-muted-foreground/50">/</span>
                {total.toLocaleString("en-US")}
              </>
            ) : null}
          </span>
        </div>
        <Progress
          value={Math.round(pctRemaining * 100)}
          max={100}
          className="mt-2"
          aria-label="Credits remaining"
        />
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
          <p className="text-[13px] text-muted-foreground">
            No plans available.
          </p>
        ) : (
          <ul className="flex flex-col gap-2">
            {plans.map((plan) => {
              const active = plan.slug === currentSlug
              const isFree = (plan.price_cents ?? 0) === 0
              const features = (plan.slug && PLAN_FEATURES[plan.slug]) || []
              return (
                <li
                  key={plan.slug}
                  className={
                    "flex flex-col gap-3 rounded-lg border px-3.5 py-3 " +
                    (active
                      ? "border-primary/40 bg-primary/5"
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
                        {(plan.monthly_credits ?? 0).toLocaleString("en-US")}{" "}
                        credits / month
                        {(plan.welcome_credits ?? 0) > 0
                          ? ` · ${(plan.welcome_credits ?? 0).toLocaleString(
                              "en-US",
                            )} welcome`
                          : ""}
                      </p>
                    </div>
                    <span className="shrink-0 font-mono text-[13px] tabular-nums">
                      {isFree
                        ? "Free"
                        : formatMoney(
                            plan.price_cents ?? 0,
                            plan.currency ?? "USD",
                          )}
                    </span>
                  </div>

                  {features.length > 0 ? (
                    <ul className="flex flex-col gap-1.5">
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
                          <span>{feature}</span>
                        </li>
                      ))}
                    </ul>
                  ) : null}
                </li>
              )
            })}
          </ul>
        )}
      </section>
    </SettingsShell>
  )
}
