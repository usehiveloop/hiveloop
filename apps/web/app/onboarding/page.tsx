"use client"

import { AnimatePresence, motion } from "motion/react"
import { Skeleton } from "@/components/ui/skeleton"
import { OnboardingShell } from "./_components/onboarding-shell"
import { StepIndicator } from "./_components/step-indicator"
import { EmployeeStep } from "./_components/employee-step"
import { ProvisioningStep } from "./_components/provisioning-step"
import { ChannelStep } from "./_components/channel-step"
import { ConfigureSlackStep } from "./_components/configure-slack-step"
import { ConfigureWhatsappStep } from "./_components/configure-whatsapp-step"
import { GithubStep } from "./_components/github-step"
import { BusinessStep } from "./_components/business-step"
import { OnboardingProvider, useOnboarding } from "./_components/context"

const stepVariants = {
  initial: { opacity: 0, y: 12 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -12 },
}

export default function OnboardingPage() {
  return (
    <OnboardingProvider>
      <OnboardingShell>
        <div className="mx-auto flex w-full max-w-xl flex-col items-center gap-10">
          <Wizard />
        </div>
      </OnboardingShell>
    </OnboardingProvider>
  )
}

function Wizard() {
  const { step, stepIndex, totalSteps, form, bootstrapping } = useOnboarding()
  const channel = form.watch("channel")

  if (bootstrapping) {
    return <OnboardingSkeleton />
  }

  return (
    <>
      <StepIndicator total={totalSteps} currentIndex={stepIndex} />

      <div className="w-full">
        <AnimatePresence mode="wait">
          <motion.div
            key={step}
            variants={stepVariants}
            initial="initial"
            animate="animate"
            exit="exit"
            transition={{ duration: 0.18 }}
            className="w-full"
          >
            {step === "employee" && <EmployeeStep />}
            {step === "provisioning" && <ProvisioningStep />}
            {step === "channel" && <ChannelStep />}
            {step === "configure" && channel === "slack" && <ConfigureSlackStep />}
            {step === "configure" && channel === "whatsapp" && <ConfigureWhatsappStep />}
            {step === "github" && <GithubStep />}
            {step === "business" && <BusinessStep />}
          </motion.div>
        </AnimatePresence>
      </div>
    </>
  )
}

function OnboardingSkeleton() {
  return (
    <div
      role="status"
      aria-live="polite"
      aria-busy="true"
      className="mx-auto flex w-full max-w-xl flex-col items-center gap-10"
    >
      <span className="sr-only">Loading…</span>
      <div className="flex items-center gap-1.5">
        <Skeleton className="size-1.5 rounded-full" />
        <Skeleton className="size-1.5 rounded-full" />
        <Skeleton className="size-1.5 rounded-full" />
        <Skeleton className="size-1.5 rounded-full" />
        <Skeleton className="size-1.5 rounded-full" />
        <Skeleton className="size-1.5 rounded-full" />
      </div>
      <div className="flex w-full flex-col items-center gap-6">
        <Skeleton className="size-24 rounded-md" />
        <div className="flex w-full flex-col items-center gap-2">
          <Skeleton className="h-8 w-48 rounded-md" />
          <Skeleton className="h-4 w-72 rounded-md" />
          <Skeleton className="h-4 w-56 rounded-md" />
        </div>
        <Skeleton className="mt-4 h-11 w-64 rounded-md" />
      </div>
    </div>
  )
}
