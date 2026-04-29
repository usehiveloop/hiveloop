"use client"

import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowRight01Icon,
  Cancel01Icon,
  CodeSquareIcon,
  PlayCircleIcon,
  Plug01Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import { cn } from "@/lib/utils"

interface OnboardingStep {
  id: string
  title: string
  description: string
  icon: typeof Plug01Icon
  completed: boolean
}

const ONBOARDING_STEPS: OnboardingStep[] = [
  {
    id: "connection",
    title: "Add your first connection",
    description: "Bring in the tools your agents will reach for.",
    icon: Plug01Icon,
    completed: false,
  },
  {
    id: "agent",
    title: "Create your first agent",
    description: "Pick a starter or build one for your workflow.",
    icon: CodeSquareIcon,
    completed: false,
  },
  {
    id: "session",
    title: "Run your first session",
    description: "Give the agent a task and watch it work.",
    icon: PlayCircleIcon,
    completed: false,
  },
]

interface OnboardingPanelProps {
  dismissed?: boolean
  currentStep?: string
  onCompleteStep?: (stepId: string) => void
  onDismiss?: () => void
}

export function OnboardingPanel({
  dismissed = false,
  currentStep = "connection",
  onCompleteStep,
  onDismiss,
}: OnboardingPanelProps) {
  const [steps, setSteps] = useState<OnboardingStep[]>(ONBOARDING_STEPS)
  const [isDismissed, setIsDismissed] = useState(dismissed)

  if (isDismissed) return null

  const completed = steps.filter((s) => s.completed).length
  const progress = completed / steps.length

  const handleDismiss = () => {
    setIsDismissed(true)
    onDismiss?.()
  }

  const handleStepClick = (stepId: string) => {
    setSteps((prev) =>
      prev.map((step) =>
        step.id === stepId ? { ...step, completed: !step.completed } : step
      )
    )
    onCompleteStep?.(stepId)
  }

  return (
    <div
      role="region"
      aria-label="Onboarding"
      className="fixed right-5 bottom-5 z-40 w-[min(380px,calc(100vw-40px))] overflow-hidden rounded-2xl border border-border/70 bg-background shadow-[0_1px_0_oklch(0_0_0/0.03),0_28px_56px_-24px_oklch(0_0_0/0.18)] dark:shadow-[0_1px_0_oklch(1_0_0/0.04),0_28px_56px_-24px_oklch(0_0_0/0.55)]"
    >
      <div aria-hidden className="absolute inset-x-0 top-0 h-px bg-border/60" />
      <div
        aria-hidden
        className="absolute left-0 top-0 h-px bg-primary transition-[width] duration-500 ease-out"
        style={{ width: `${progress * 100}%` }}
      />

      <div className="flex items-start justify-between gap-4 px-5 pt-5 pb-2">
        <div className="min-w-0">
          <h3 className="font-heading text-[15px] font-medium leading-tight text-foreground">
            Set up your workspace
          </h3>
          <p className="mt-1.5 text-[12px] leading-tight text-muted-foreground">
            <span className="tabular-nums text-foreground">{completed}</span>
            <span className="mx-1 text-muted-foreground/50">/</span>
            <span className="tabular-nums">{steps.length}</span>
            <span className="ml-1.5">complete</span>
          </p>
        </div>
        <button
          type="button"
          onClick={handleDismiss}
          aria-label="Dismiss onboarding"
          className="-mt-1 flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
        >
          <HugeiconsIcon icon={Cancel01Icon} size={14} />
        </button>
      </div>

      <ol className="flex flex-col px-2 pb-3">
        {steps.map((step, index) => {
          const isInProgress = currentStep === step.id && !step.completed
          return (
            <li key={step.id}>
              <button
                type="button"
                onClick={() => handleStepClick(step.id)}
                aria-current={isInProgress ? "step" : undefined}
                className="group flex w-full items-center gap-4 rounded-xl px-3 py-5 text-left transition-colors hover:bg-muted/40"
              >
                <span
                  className={cn(
                    "flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-[12px] font-medium tabular-nums transition-colors",
                    step.completed && "bg-primary text-primary-foreground",
                    isInProgress && "border-2 border-primary bg-transparent",
                    !step.completed && !isInProgress && "bg-muted text-muted-foreground"
                  )}
                >
                  {step.completed ? (
                    <HugeiconsIcon icon={Tick02Icon} size={14} />
                  ) : isInProgress ? null : (
                    String(index + 1).padStart(2, "0")
                  )}
                </span>

                <span className="min-w-0 flex-1">
                  <span
                    className={cn(
                      "block text-[14px] font-medium leading-tight",
                      step.completed
                        ? "text-muted-foreground line-through decoration-muted-foreground/40"
                        : "text-foreground"
                    )}
                  >
                    {step.title}
                  </span>
                  <span
                    className={cn(
                      "mt-1.5 block text-[12.5px] leading-relaxed",
                      step.completed
                        ? "text-muted-foreground/70"
                        : "text-muted-foreground"
                    )}
                  >
                    {step.description}
                  </span>
                </span>

                <HugeiconsIcon
                  icon={ArrowRight01Icon}
                  size={14}
                  className="shrink-0 text-muted-foreground/0 transition-all group-hover:translate-x-0.5 group-hover:text-muted-foreground"
                />
              </button>
            </li>
          )
        })}
      </ol>
    </div>
  )
}
