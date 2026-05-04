"use client"

import { createContext, useCallback, useContext, useState } from "react"
import { useForm, type UseFormReturn } from "react-hook-form"

export type Channel = "slack" | "whatsapp"
export type StepKey = "employee" | "provisioning" | "channel" | "configure" | "business"

const STEP_ORDER: StepKey[] = ["employee", "provisioning", "channel", "configure", "business"]

export interface OnboardingFormValues {
  agentName: string
  agentDescription: string
  agentAvatarUrl: string
  agentCategory: string
  channel: Channel | null
  slackBotToken: string
  slackAppToken: string
  businessName: string
  businessWebsite: string
  businessLogoUrl: string
  businessDescription: string
}

const DEFAULT_VALUES: OnboardingFormValues = {
  agentName: "",
  agentDescription: "",
  agentAvatarUrl: "",
  agentCategory: "",
  channel: null,
  slackBotToken: "",
  slackAppToken: "",
  businessName: "",
  businessWebsite: "",
  businessLogoUrl: "",
  businessDescription: "",
}

interface OnboardingContextValue {
  form: UseFormReturn<OnboardingFormValues>
  step: StepKey
  stepIndex: number
  totalSteps: number
  goNext: () => void
  goBack: () => void
  selectChannel: (channel: Channel) => void
}

const OnboardingContext = createContext<OnboardingContextValue | null>(null)

export function useOnboarding() {
  const ctx = useContext(OnboardingContext)
  if (!ctx) throw new Error("useOnboarding must be used within OnboardingProvider")
  return ctx
}

export function OnboardingProvider({ children }: { children: React.ReactNode }) {
  const form = useForm<OnboardingFormValues>({
    defaultValues: DEFAULT_VALUES,
    mode: "onChange",
  })
  const [step, setStep] = useState<StepKey>("employee")
  const stepIndex = STEP_ORDER.indexOf(step)

  const goNext = useCallback(() => {
    setStep((current) => {
      const idx = STEP_ORDER.indexOf(current)
      return STEP_ORDER[idx + 1] ?? current
    })
  }, [])

  const goBack = useCallback(() => {
    setStep((current) => {
      const idx = STEP_ORDER.indexOf(current)
      return STEP_ORDER[idx - 1] ?? current
    })
  }, [])

  const selectChannel = useCallback(
    (channel: Channel) => {
      form.setValue("channel", channel, { shouldDirty: true })
      setStep((current) => {
        const idx = STEP_ORDER.indexOf(current)
        return STEP_ORDER[idx + 1] ?? current
      })
    },
    [form],
  )

  return (
    <OnboardingContext.Provider
      value={{
        form,
        step,
        stepIndex,
        totalSteps: STEP_ORDER.length,
        goNext,
        goBack,
        selectChannel,
      }}
    >
      {children}
    </OnboardingContext.Provider>
  )
}
