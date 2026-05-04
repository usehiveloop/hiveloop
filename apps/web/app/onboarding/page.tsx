"use client"

import { AnimatePresence, motion } from "motion/react"
import { OnboardingShell } from "./_components/onboarding-shell"
import { StepIndicator } from "./_components/step-indicator"
import { EmployeeStep } from "./_components/employee-step"
import { ProvisioningStep } from "./_components/provisioning-step"
import { ChannelStep } from "./_components/channel-step"
import { ConfigureSlackStep } from "./_components/configure-slack-step"
import { ConfigureWhatsappStep } from "./_components/configure-whatsapp-step"
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
  const { step, stepIndex, totalSteps, form } = useOnboarding()
  const channel = form.watch("channel")

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
            {step === "business" && <BusinessStep />}
          </motion.div>
        </AnimatePresence>
      </div>
    </>
  )
}
