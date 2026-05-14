"use client"

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react"
import { useForm, type UseFormReturn } from "react-hook-form"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import type { components } from "@/lib/api/schema"

type Employee = components["schemas"]["employeeListItem"]

export type Channel = "slack" | "whatsapp"
export type StepKey =
  | "employee"
  | "provisioning"
  | "channel"
  | "configure"
  | "github"
  | "business"
export type OnboardingMode = "workspace" | "employee"

const STEP_ORDER: StepKey[] = [
  "employee",
  "channel",
  "configure",
  "github",
  "business",
  "provisioning",
]

const EMPLOYEE_STEP_ORDER: StepKey[] = [
  "employee",
  "channel",
  "configure",
  "github",
  "provisioning",
]

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
  bootstrapping: boolean
  bootstrapped: boolean
  mode: OnboardingMode
}

const OnboardingContext = createContext<OnboardingContextValue | null>(null)

export function useOnboarding() {
  const ctx = useContext(OnboardingContext)
  if (!ctx)
    throw new Error("useOnboarding must be used within OnboardingProvider")
  return ctx
}

export function OnboardingProvider({
  children,
  mode = "workspace",
  employeeId,
}: {
  children: React.ReactNode
  mode?: OnboardingMode
  employeeId?: string
}) {
  const form = useForm<OnboardingFormValues>({
    defaultValues: DEFAULT_VALUES,
    mode: "onChange",
  })
  const { activeOrg } = useAuth()
  const [step, setStep] = useState<StepKey>("employee")
  const stepOrder = mode === "employee" ? EMPLOYEE_STEP_ORDER : STEP_ORDER
  const stepIndex = stepOrder.indexOf(step)

  const createEmployeeMutation = $api.useMutation("post", "/v1/employees")
  const updateEmployeeMutation = $api.useMutation("put", "/v1/agents/{id}")
  const employeesQuery = $api.useQuery(
    "get",
    "/v1/employees",
    {},
    { enabled: mode === "workspace" && !employeeId }
  )
  const employeeQuery = $api.useQuery(
    "get",
    "/v1/employees/{id}",
    { params: { path: { id: employeeId ?? "" } } },
    { enabled: Boolean(employeeId), retry: false }
  )
  const bootstrappedRef = useRef(false)
  const activeOrgRef = useRef<string | null | undefined>(undefined)
  const firstEmployee = employeesQuery.data?.data?.[0]
  const resumeEmployee = employeeQuery.data
  const bootstrapEmployee = useMemo(() => {
    if (employeeId) return resumeEmployee
    if (mode === "workspace") return firstEmployee
    return undefined
  }, [employeeId, firstEmployee, mode, resumeEmployee])
  const bootstrapped = useMemo(
    () => (bootstrapEmployee?.id ? { agentId: bootstrapEmployee.id } : null),
    [bootstrapEmployee]
  )
  const shouldBootstrapExistingEmployee = mode === "workspace"

  useEffect(() => {
    const activeOrgId = activeOrg?.id ?? null
    if (activeOrgRef.current === undefined) {
      activeOrgRef.current = activeOrgId
      return
    }
    if (activeOrgRef.current === activeOrgId) return

    activeOrgRef.current = activeOrgId
    bootstrappedRef.current = false
    queueMicrotask(() => {
      form.reset(DEFAULT_VALUES)
      setStep("employee")
    })
  }, [activeOrg?.id, form])

  useEffect(() => {
    bootstrappedRef.current = false
    form.reset(DEFAULT_VALUES)
    queueMicrotask(() => setStep("employee"))
  }, [employeeId, form])

  useEffect(() => {
    if (!shouldBootstrapExistingEmployee && !employeeId) return
    if (bootstrappedRef.current) return
    if (employeeId && employeeQuery.isLoading) return
    if (!employeeId && !employeesQuery.data) return
    const first = bootstrapEmployee
    if (!first || !first.id) return
    bootstrappedRef.current = true

    hydrateEmployeeForm(form, first)
    queueMicrotask(() => setStep(nextOnboardingStep(first)))
  }, [
    bootstrapEmployee,
    employeeId,
    employeeQuery.isLoading,
    employeesQuery.data,
    form,
    shouldBootstrapExistingEmployee,
  ])

  const createEmployee: CreateEmployeeState = (() => {
    if (shouldBootstrapExistingEmployee && bootstrapped) {
      return {
        status: "success",
        agentId: bootstrapped.agentId,
      }
    }
    if (createEmployeeMutation.isPending || updateEmployeeMutation.isPending) {
      return { status: "pending" }
    }
    if (bootstrapped) {
      return {
        status: "success",
        agentId: bootstrapped.agentId,
      }
    }
    if (createEmployeeMutation.isSuccess && createEmployeeMutation.data) {
      return {
        status: "success",
        agentId: createEmployeeMutation.data.agent_id,
      }
    }
    if (updateEmployeeMutation.isError) {
      const err = updateEmployeeMutation.error as unknown as
        | { error?: string }
        | undefined
      return {
        status: "error",
        errorMessage:
          (err && typeof err === "object" && "error" in err && err.error) ||
          "Could not update your AI employee profile. Try again.",
      }
    }
    if (createEmployeeMutation.isError) {
      const err = createEmployeeMutation.error as unknown as
        | { error?: string }
        | undefined
      return {
        status: "error",
        errorMessage:
          (err && typeof err === "object" && "error" in err && err.error) ||
          "Could not save your AI employee profile. Try again.",
      }
    }
    return { status: "idle" }
  })()

  const submitEmployee = useCallback(() => {
    const v = form.getValues()
    if (bootstrapped?.agentId) {
      updateEmployeeMutation.mutate(
        {
          params: { path: { id: bootstrapped.agentId } },
          body: {
            category: v.agentCategory,
            name: v.agentName.trim(),
            description: v.agentDescription.trim(),
            avatar_url: v.agentAvatarUrl?.trim() || "",
          },
        },
        {
          onSuccess: async () => {
            if (employeeId) {
              await employeeQuery.refetch()
            }
            setStep((current) =>
              current === "employee" ? nextOnboardingStep(bootstrapEmployee) : current
            )
          },
        }
      )
      return
    }
    createEmployeeMutation.mutate(
      {
        body: {
          category: v.agentCategory,
          name: v.agentName.trim(),
          description: v.agentDescription.trim(),
          avatar_url: v.agentAvatarUrl?.trim() || "",
        },
      },
      {
        onSuccess: () => setStep("channel"),
      }
    )
  }, [
    bootstrapped,
    bootstrapEmployee,
    createEmployeeMutation,
    employeeId,
    employeeQuery,
    form,
    updateEmployeeMutation,
  ])

  const retryEmployee = useCallback(() => {
    createEmployeeMutation.reset()
    submitEmployee()
  }, [createEmployeeMutation, submitEmployee])

  const goNext = useCallback(() => {
    setStep((current) => {
      const idx = stepOrder.indexOf(current)
      return stepOrder[idx + 1] ?? current
    })
  }, [stepOrder])

  const goBack = useCallback(() => {
    setStep((current) => {
      const idx = stepOrder.indexOf(current)
      return stepOrder[idx - 1] ?? current
    })
  }, [stepOrder])

  const selectChannel = useCallback(
    (channel: Channel) => {
      form.setValue("channel", channel, { shouldDirty: true })
      setStep((current) => {
        const idx = stepOrder.indexOf(current)
        return stepOrder[idx + 1] ?? current
      })
    },
    [form, stepOrder]
  )

  return (
    <OnboardingContext.Provider
      value={{
        form,
        step,
        stepIndex,
        totalSteps: stepOrder.length,
        goNext,
        goBack,
        selectChannel,
        createEmployee,
        submitEmployee,
        retryEmployee,
        bootstrapping: employeeId ? employeeQuery.isLoading : employeesQuery.isLoading,
        bootstrapped: (shouldBootstrapExistingEmployee || Boolean(employeeId)) && Boolean(bootstrapped),
        mode,
      }}
    >
      {children}
    </OnboardingContext.Provider>
  )
}

function hydrateEmployeeForm(
  form: UseFormReturn<OnboardingFormValues>,
  employee: Employee
) {
  form.setValue("agentName", employee.name ?? "")
  form.setValue("agentDescription", employee.description ?? "")
  form.setValue("agentAvatarUrl", employee.avatar_url ?? "")
  form.setValue("agentCategory", employee.category ?? "")

  const slackProfile = employee.profiles?.find(
    (p) => p.provider === "slack" && p.status === "active"
  )
  if (slackProfile) {
    form.setValue("channel", "slack")
    const identity = (slackProfile.identity ?? {}) as Record<string, unknown>
    const teamName = typeof identity.team_name === "string" ? identity.team_name : ""
    if (teamName && !form.getValues("businessName").trim()) {
      form.setValue("businessName", teamName)
    }
  }
}

function nextOnboardingStep(employee: Employee | undefined): StepKey {
  if (!employee?.id) return "employee"
  const slackProfile = employee.profiles?.find(
    (profile) => profile.provider === "slack" && profile.status === "active"
  )
  if (!slackProfile) return "channel"

  const githubProfile = employee.profiles?.find(
    (profile) => profile.provider === "github" && profile.status === "active"
  )
  const selectedRepositories = githubProfile?.config?.selected_repositories
  if (!githubProfile || !Array.isArray(selectedRepositories) || selectedRepositories.length === 0) {
    return "github"
  }

  if (employee.status === "active") return "provisioning"
  return "provisioning"
}
