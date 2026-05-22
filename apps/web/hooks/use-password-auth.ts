"use client"

import { useCallback, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"

type LoginRequest = components["schemas"]["loginRequest"]
type RegisterRequest = components["schemas"]["registerRequest"]
type ConfirmEmailRequest = components["schemas"]["confirmEmailRequest"]
type ResendConfirmationRequest =
  components["schemas"]["resendConfirmationRequest"]

export type PasswordAuthInput = Required<
  Pick<LoginRequest, "email" | "password">
>
export type ConfirmEmailInput = Required<
  Pick<ConfirmEmailRequest, "email" | "code">
>

function normalizeEmail(email: string) {
  return email.trim().toLowerCase()
}

function deriveNameFromEmail(email: string) {
  const localPart = email.split("@")[0]?.trim()
  if (!localPart) return "Hivy user"

  return localPart
    .replace(/[._-]+/g, " ")
    .split(" ")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ")
}

export function usePasswordLogin() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const mutation = $api.useMutation("post", "/auth/login")

  const login = useCallback(
    ({ email, password }: PasswordAuthInput) => {
      const normalizedEmail = normalizeEmail(email)
      if (!normalizedEmail || !password) return

      const body: LoginRequest = {
        email: normalizedEmail,
        password,
      }

      mutation.mutate(
        { body },
        {
          onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
            router.replace("/w")
          },
          onError: (error) => {
            toast.error(extractErrorMessage(error, "Could not sign in"))
          },
        }
      )
    },
    [mutation, queryClient, router]
  )

  return {
    login,
    isPending: mutation.isPending,
  }
}

export function usePasswordSignup() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const [emailToConfirm, setEmailToConfirm] = useState<string | null>(null)
  const registerMutation = $api.useMutation("post", "/auth/register")
  const confirmMutation = $api.useMutation("post", "/auth/confirm-email")
  const resendMutation = $api.useMutation("post", "/auth/resend-confirmation")

  const signup = useCallback(
    ({ email, password }: PasswordAuthInput) => {
      const normalizedEmail = normalizeEmail(email)
      if (!normalizedEmail || !password) return

      const body: RegisterRequest = {
        email: normalizedEmail,
        password,
        name: deriveNameFromEmail(normalizedEmail),
      }

      registerMutation.mutate(
        { body },
        {
          onSuccess: (response) => {
            queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
            if (response?.user?.email_confirmed) {
              router.replace("/w")
              return
            }
            setEmailToConfirm(normalizedEmail)
            toast.success("Check your email for a 6-digit confirmation code")
          },
          onError: (error) => {
            toast.error(extractErrorMessage(error, "Could not create account"))
          },
        }
      )
    },
    [queryClient, registerMutation, router]
  )

  const confirmEmail = useCallback(
    ({ email, code }: ConfirmEmailInput) => {
      const normalizedEmail = normalizeEmail(email)
      const trimmedCode = code.trim()
      if (!normalizedEmail || !trimmedCode) return

      const body: ConfirmEmailRequest = {
        email: normalizedEmail,
        code: trimmedCode,
      }

      confirmMutation.mutate(
        { body },
        {
          onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
            router.replace("/w")
          },
          onError: (error) => {
            toast.error(extractErrorMessage(error, "Invalid or expired code"))
          },
        }
      )
    },
    [confirmMutation, queryClient, router]
  )

  const resendConfirmation = useCallback(() => {
    if (!emailToConfirm) return

    const body: ResendConfirmationRequest = {
      email: emailToConfirm,
    }

    resendMutation.mutate(
      { body },
      {
        onSuccess: () => {
          toast.success("A new confirmation code has been sent")
        },
        onError: (error) => {
          toast.error(
            extractErrorMessage(error, "Could not resend confirmation code")
          )
        },
      }
    )
  }, [emailToConfirm, resendMutation])

  const changeEmail = useCallback(() => {
    setEmailToConfirm(null)
  }, [])

  return {
    signup,
    confirmEmail,
    resendConfirmation,
    changeEmail,
    emailToConfirm,
    isPending: registerMutation.isPending,
    isConfirming: confirmMutation.isPending,
    isResending: resendMutation.isPending,
  }
}
