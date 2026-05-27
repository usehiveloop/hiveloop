"use client"

import Link from "next/link"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ZapIcon,
  Plug01Icon,
  CheckmarkCircle01Icon,
  ArrowRight01Icon,
  SlackIcon,
  UserAdd01Icon,
  AwardIcon,
  CommandIcon,
  PlayIcon,
} from "@hugeicons/core-free-icons"
import { Progress } from "@/components/ui/progress"
import { cn } from "@/lib/utils"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"

function formatK(n: number) {
  if (n >= 10_000) return `${(n / 1000).toFixed(1).replace(/\.0$/, "")}k`
  return n.toLocaleString("en-US")
}

function CheckIcon({ className }: { className?: string }) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      className={className}
    >
      <path
        d="M3 8.5L6.5 12L13 5"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

function StatCardSkeleton() {
  return (
    <div className="flex flex-col rounded-2xl border border-border bg-card p-5">
      <div className="flex items-center gap-3">
        <div className="h-10 w-10 animate-pulse rounded-xl bg-muted" />
        <div className="h-4 w-24 animate-pulse rounded bg-muted" />
      </div>
      <div className="mt-4">
        <div className="h-8 w-20 animate-pulse rounded bg-muted" />
        <div className="mt-2 h-3 w-32 animate-pulse rounded bg-muted" />
      </div>
    </div>
  )
}

export default function DashboardV2Page() {
  const { activeOrg } = useAuth()
  const dashboardQuery = $api.useQuery("get", "/v1/dashboard", {})

  const dashboard = dashboardQuery.data
  const isLoading = dashboardQuery.isLoading

  const credits = dashboard?.credits
  const connections = dashboard?.connections
  const onboarding = dashboard?.onboarding
  const schedules = dashboard?.schedules

  const totalPlanCredits =
    (activeOrg?.plan?.monthly_credits ?? 0) +
    (activeOrg?.plan?.welcome_credits ?? 0)

  const onboardingSteps = [
    {
      id: 1,
      label: "Connect Slack",
      description: "Invite hivy to your workspace",
      icon: SlackIcon,
      done: !!(onboarding?.plan_selected),
    },
    {
      id: 2,
      label: "Add connections",
      description: "Link your tools and services",
      icon: Plug01Icon,
      done: !!(onboarding && (onboarding.extra_tools_connected ?? 0) >= (onboarding.extra_tools_required ?? 0)),
    },
    {
      id: 3,
      label: "Invite your team",
      description: "Collaborate with coworkers",
      icon: UserAdd01Icon,
      done: false,
    },
    {
      id: 4,
      label: "Claim free credits",
      description: "Get more usage on us",
      icon: AwardIcon,
      done: false,
    },
    {
      id: 5,
      label: "Create a skill",
      description: "Teach hivy something new",
      icon: CommandIcon,
      done: !!(schedules && schedules.total && schedules.total > 0),
    },
  ]

  const completedSteps = onboardingSteps.filter((s) => s.done).length
  const progress = (completedSteps / onboardingSteps.length) * 100

  return (
    <div className="mx-auto w-full max-w-5xl space-y-8">
      {/* Header */}
      <div>
        <h1 className="font-heading text-3xl font-normal tracking-[-0.02em] text-foreground md:text-4xl">
          Dashboard
        </h1>

      </div>

      {/* Stats cards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {isLoading ? (
          <>
            <StatCardSkeleton />
            <StatCardSkeleton />
            <StatCardSkeleton />
          </>
        ) : (
          <>
            <div className="flex flex-col rounded-2xl border border-border bg-card p-5">
              <div className="flex items-center gap-3">
                <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10">
                  <HugeiconsIcon icon={ZapIcon} size={20} className="text-primary" />
                </div>
                <span className="text-sm text-muted-foreground">Credits used</span>
              </div>
              <div className="mt-4">
                <p className="font-heading text-2xl font-medium tracking-tight text-foreground">
                  {formatK(credits?.spent_this_period ?? 0)}
                </p>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  of {formatK(totalPlanCredits)} this month
                </p>
              </div>
            </div>

            <div className="flex flex-col rounded-2xl border border-border bg-card p-5">
              <div className="flex items-center gap-3">
                <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-green-500/10">
                  <HugeiconsIcon icon={Plug01Icon} size={20} className="text-green-600" />
                </div>
                <span className="text-sm text-muted-foreground">Active connections</span>
              </div>
              <div className="mt-4">
                <p className="font-heading text-2xl font-medium tracking-tight text-foreground">
                  {connections?.total ?? 0}
                </p>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {connections?.slack_connected ? "Slack" : ""}
                  {connections?.non_slack_connected && connections.non_slack_connected > 0 ? `${connections.slack_connected ? ", " : ""}${connections.non_slack_connected} other${connections.non_slack_connected !== 1 ? "s" : ""}` : ""}
                </p>
              </div>
            </div>

            <div className="flex flex-col rounded-2xl border border-border bg-card p-5">
              <div className="flex items-center gap-3">
                <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-amber-500/10">
                  <HugeiconsIcon icon={CheckmarkCircle01Icon} size={20} className="text-amber-600" />
                </div>
                <span className="text-sm text-muted-foreground">Schedules</span>
              </div>
              <div className="mt-4">
                <p className="font-heading text-2xl font-medium tracking-tight text-foreground">
                  {schedules?.total ?? 0}
                </p>
                <p className="mt-0.5 text-xs text-muted-foreground">Active schedules</p>
              </div>
            </div>
          </>
        )}
      </div>

      {/* Video prompt card */}
      <Link
        href="#"
        className="group flex items-center gap-4 rounded-2xl border border-border bg-card p-5 transition-all hover:border-muted-foreground/20 hover:bg-muted/30"
      >
        <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
          <HugeiconsIcon icon={PlayIcon} size={24} />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-foreground">
            Watch the 3-minute hivy walkthrough
          </p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            Learn how to get the most out of your AI coworker.
          </p>
        </div>
        <HugeiconsIcon
          icon={ArrowRight01Icon}
          size={16}
          className="shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5"
        />
      </Link>

      {/* Onboarding card */}
      <div className="rounded-2xl border border-border bg-card p-6 sm:p-8">
        <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h2 className="font-heading text-xl font-medium tracking-[-0.02em] text-foreground">
              Getting started
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Complete these steps to get the most out of hivy.
            </p>
          </div>
          <div className="flex items-center gap-3">
            <span className="text-sm font-medium text-foreground">
              {completedSteps} of {onboardingSteps.length}
            </span>
            <div className="w-32">
              <Progress value={progress} />
            </div>
          </div>
        </div>

        <div className="flex flex-col gap-3">
          {onboardingSteps.map((step, index) => (
            <Link
              key={step.id}
              href="#"
              className={cn(
                "group relative flex flex-1 items-center gap-4 rounded-xl border p-4 transition-all",
                step.done
                  ? "border-primary/20 bg-primary/[0.03]"
                  : "border-border bg-background hover:border-muted-foreground/20 hover:bg-muted/30"
              )}
            >
              {/* Icon */}
              <div
                className={cn(
                  "flex h-10 w-10 shrink-0 items-center justify-center rounded-xl",
                  step.done ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground"
                )}
              >
                <HugeiconsIcon icon={step.icon} size={20} />
              </div>

              {/* Text */}
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <p className="text-sm font-semibold text-foreground">
                    {step.label}
                  </p>
                  {step.done && (
                    <span className="rounded-full bg-primary/10 px-1.5 py-0 text-[10px] font-medium text-primary">
                      Done
                    </span>
                  )}
                </div>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {step.description}
                </p>
              </div>

              {/* Step number or arrow */}
              <div className="shrink-0">
                {step.done ? (
                  <span className="flex h-6 w-6 items-center justify-center rounded-full bg-primary text-primary-foreground">
                    <CheckIcon className="size-3" />
                  </span>
                ) : (
                  <span className="flex h-6 w-6 items-center justify-center rounded-full bg-muted text-[11px] font-semibold text-muted-foreground">
                    {index + 1}
                  </span>
                )}
              </div>
            </Link>
          ))}
        </div>
      </div>
    </div>
  )
}
