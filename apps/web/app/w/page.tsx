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
  BookOpen02Icon,
  PlayIcon,
} from "@hugeicons/core-free-icons"
import { Progress } from "@/components/ui/progress"
import { cn } from "@/lib/utils"

const stats = [
  {
    label: "Credits used",
    value: "6,240",
    sub: "of 10,000 this month",
    icon: ZapIcon,
    color: "text-primary",
    bg: "bg-primary/10",
  },
  {
    label: "Active connections",
    value: "4",
    sub: "Slack, GitHub, Sheets, Drive",
    icon: Plug01Icon,
    color: "text-green-600",
    bg: "bg-green-500/10",
  },
  {
    label: "Tasks completed",
    value: "128",
    sub: "Last 30 days",
    icon: CheckmarkCircle01Icon,
    color: "text-amber-600",
    bg: "bg-amber-500/10",
  },
]

const onboardingSteps = [
  {
    id: 1,
    label: "Connect Slack",
    description: "Invite hivy to your workspace",
    icon: SlackIcon,
    done: true,
  },
  {
    id: 2,
    label: "Add connections",
    description: "Link your tools and services",
    icon: Plug01Icon,
    done: true,
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
    done: false,
  },
]

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

export default function DashboardV2Page() {
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
        {stats.map((stat) => (
          <div
            key={stat.label}
            className="flex flex-col rounded-2xl border border-border bg-card p-5"
          >
            <div className="flex items-center gap-3">
              <div
                className={cn(
                  "flex h-10 w-10 items-center justify-center rounded-xl",
                  stat.bg
                )}
              >
                <HugeiconsIcon icon={stat.icon} size={20} className={stat.color} />
              </div>
              <span className="text-sm text-muted-foreground">{stat.label}</span>
            </div>
            <div className="mt-4">
              <p className="font-heading text-2xl font-medium tracking-tight text-foreground">
                {stat.value}
              </p>
              <p className="mt-0.5 text-xs text-muted-foreground">{stat.sub}</p>
            </div>
          </div>
        ))}
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
