"use client"

import { createContext, useCallback, useContext, useEffect, useState } from "react"
import { useForm, type UseFormReturn } from "react-hook-form"
import { $api } from "@/lib/api/hooks"

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

interface CreateEmployeeState {
  status: "idle" | "pending" | "success" | "error"
  agentId?: string
  sandboxId?: string
  errorMessage?: string
}

interface OnboardingContextValue {
  form: UseFormReturn<OnboardingFormValues>
  step: StepKey
  stepIndex: number
  totalSteps: number
  goNext: () => void
  goBack: () => void
  selectChannel: (channel: Channel) => void
  createEmployee: CreateEmployeeState
  submitEmployee: () => void
  retryEmployee: () => void
  /** True while we're still resolving whether the user already has an employee. */
  bootstrapping: boolean
  /** True once we've hydrated state from an existing employee. */
  bootstrapped: boolean
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

  const createEmployeeMutation = $api.useMutation("post", "/v1/employees")

  // Boot-time check: if the user already has an employee, hydrate state and
  // jump them past the create form. We pick the first one returned.
  const employeesQuery = $api.useQuery("get", "/v1/employees")
  const [bootstrapped, setBootstrapped] = useState<{
    agentId: string
    sandboxId: string
  } | null>(null)

  useEffect(() => {
    if (bootstrapped) return
    if (!employeesQuery.data) return
    const first = employeesQuery.data.data?.[0]
    if (!first || !first.id) return

    form.setValue("agentName", first.name ?? "")
    form.setValue("agentDescription", first.description ?? "")
    form.setValue("agentAvatarUrl", first.avatar_url ?? "")
    form.setValue("agentCategory", first.category ?? "")

    setBootstrapped({
      agentId: first.id,
      sandboxId: first.sandbox?.id ?? "",
    })
    setStep("provisioning")
  }, [employeesQuery.data, bootstrapped, form])

  const createEmployee: CreateEmployeeState = (() => {
    if (bootstrapped) {
      return {
        status: "success",
        agentId: bootstrapped.agentId,
        sandboxId: bootstrapped.sandboxId,
      }
    }
    if (createEmployeeMutation.isPending) return { status: "pending" }
    if (createEmployeeMutation.isSuccess && createEmployeeMutation.data) {
      return {
        status: "success",
        agentId: createEmployeeMutation.data.agent_id,
        sandboxId: createEmployeeMutation.data.sandbox_id,
      }
    }
    if (createEmployeeMutation.isError) {
      const err = createEmployeeMutation.error as unknown as { error?: string } | undefined
      return {
        status: "error",
        errorMessage:
          (err && typeof err === "object" && "error" in err && err.error) ||
          "Could not provision your AI employee. Try again.",
      }
    }
    return { status: "idle" }
  })()

  const submitEmployee = useCallback(() => {
    const v = form.getValues()
    createEmployeeMutation.mutate({
      body: {
        category: v.agentCategory,
        name: v.agentName.trim(),
        description: v.agentDescription.trim(),
        avatar_url: v.agentAvatarUrl?.trim() || "",
      },
    })
  }, [form, createEmployeeMutation])

  const retryEmployee = useCallback(() => {
    createEmployeeMutation.reset()
    submitEmployee()
  }, [createEmployeeMutation, submitEmployee])

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
        createEmployee,
        submitEmployee,
        retryEmployee,
        bootstrapping: employeesQuery.isLoading,
        bootstrapped: Boolean(bootstrapped),
      }}
    >
      {children}
    </OnboardingContext.Provider>
  )
}
