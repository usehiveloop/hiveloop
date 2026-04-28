"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import { SparklesIcon, Tick02Icon } from "@hugeicons/core-free-icons"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import type { components } from "@/lib/api/schema"

type Plan = components["schemas"]["planDTO"]

interface SubscriptionSuccessDialogProps {
  plan: Plan | null
  onClose: () => void
}

export function SubscriptionSuccessDialog({
  plan,
  onClose,
}: SubscriptionSuccessDialogProps) {
  const open = plan !== null

  const monthly = plan?.monthly_credits ?? 0
  const welcome = plan?.welcome_credits ?? 0
  const totalCredits = monthly + welcome
  const features = plan?.features ?? []

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose()
      }}
    >
      <DialogContent showCloseButton className="sm:max-w-md">
        <div className="flex flex-col items-center gap-3 pt-2 text-center">
          <span className="flex h-11 w-11 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
            <HugeiconsIcon icon={SparklesIcon} size={20} />
          </span>
          <DialogTitle className="text-[16px]">
            Welcome to {plan?.name ?? "your new plan"}
          </DialogTitle>
          <DialogDescription className="text-[13px]">
            Your subscription is active. Thanks for upgrading.
          </DialogDescription>
        </div>

        {totalCredits > 0 ? (
          <div className="mt-2 rounded-lg border border-border/60 bg-muted/30 px-4 py-3">
            <p className="text-[12px] text-muted-foreground">Credits applied</p>
            <p className="mt-0.5 font-mono text-[18px] tabular-nums">
              {totalCredits.toLocaleString("en-US")}
            </p>
            {welcome > 0 ? (
              <p className="mt-1 text-[11px] text-muted-foreground">
                {monthly.toLocaleString("en-US")} monthly
                {" + "}
                {welcome.toLocaleString("en-US")} welcome bonus
              </p>
            ) : (
              <p className="mt-1 text-[11px] text-muted-foreground">
                Refills every month.
              </p>
            )}
          </div>
        ) : null}

        {features.length > 0 ? (
          <div className="mt-1">
            <p className="mb-2 text-[12px] font-medium text-muted-foreground">
              What's included
            </p>
            <ul className="flex flex-col gap-1.5">
              {features.map((feature) => (
                <li
                  key={feature}
                  className="flex items-start gap-2 text-[13px]"
                >
                  <HugeiconsIcon
                    icon={Tick02Icon}
                    size={14}
                    className="mt-0.5 shrink-0 text-emerald-500"
                  />
                  <span>{feature}</span>
                </li>
              ))}
            </ul>
          </div>
        ) : null}

        <DialogFooter>
          <Button onClick={onClose} className="w-full sm:w-auto">
            Get started
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
