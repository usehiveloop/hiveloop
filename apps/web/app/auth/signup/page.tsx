"use client"

import Link from "next/link"
import { motion } from "motion/react"
import { Controller, useForm } from "react-hook-form"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon } from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSlot,
  InputOTPSeparator,
} from "@/components/ui/input-otp"
import { Label } from "@/components/ui/label"
import {
  usePasswordSignup,
  type PasswordAuthInput,
  type ConfirmEmailInput,
} from "@/hooks/use-password-auth"
import {
  AuthCard,
  AuthGhostLogo,
  AuthDivider,
  AuthFooter,
  OAuthButtons,
  stepVariants,
  stepTransition,
} from "../_components/shared"

type ConfirmationFormValues = Required<Pick<ConfirmEmailInput, "code">>

export default function SignupPage() {
  const {
    signup,
    confirmEmail,
    resendConfirmation,
    changeEmail,
    emailToConfirm,
    isPending,
    isConfirming,
    isResending,
  } = usePasswordSignup()
  const signupForm = useForm<PasswordAuthInput>({
    defaultValues: {
      email: "",
      password: "",
    },
  })
  const confirmationForm = useForm<ConfirmationFormValues>({
    defaultValues: {
      code: "",
    },
  })

  const onSignupSubmit = signupForm.handleSubmit((values) => signup(values))
  const onConfirmSubmit = confirmationForm.handleSubmit(({ code }) => {
    if (!emailToConfirm) return
    confirmEmail({ email: emailToConfirm, code })
  })

  return (
    <>
      <div className="fixed top-5 right-0 left-0 z-50 mx-auto flex max-w-5xl items-center px-4 md:px-0">
        <Button
          variant="ghost"
          size="sm"
          render={<Link href="/" className="flex items-center font-display" />}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} size={14} />
          Home
        </Button>
      </div>

      <AuthCard>
        <div className="flex flex-col gap-8">
          {/* Header */}
          <div className="flex flex-col items-center gap-4">
            <AuthGhostLogo />
            <div className="text-center">
              <h1 className="font-heading text-2xl font-normal tracking-[-0.02em] text-foreground">
                Create your account
              </h1>
              <p className="mt-1.5 text-sm text-muted-foreground">
                Start free with 1,000 credits — no card required.
              </p>
            </div>
          </div>

          <motion.div
            key={emailToConfirm ? "confirm-email" : "password-signup"}
            variants={stepVariants}
            initial="initial"
            animate="animate"
            exit="exit"
            transition={stepTransition}
            className="flex flex-col gap-6"
          >
            {emailToConfirm ? (
              <form
                onSubmit={onConfirmSubmit}
                className="flex flex-col items-center gap-6"
              >
                <div className="text-center">
                  <p className="text-sm text-muted-foreground">
                    Enter the 6-digit code sent to{" "}
                    <span className="font-medium text-foreground">
                      {emailToConfirm}
                    </span>
                  </p>
                </div>

                <Controller
                  name="code"
                  control={confirmationForm.control}
                  rules={{ required: true, minLength: 6 }}
                  render={({ field }) => (
                    <InputOTP
                      maxLength={6}
                      value={field.value}
                      onChange={(value) => {
                        field.onChange(value)
                        if (value.length === 6 && emailToConfirm) {
                          confirmEmail({ email: emailToConfirm, code: value })
                        }
                      }}
                      disabled={isConfirming}
                    >
                      <InputOTPGroup>
                        <InputOTPSlot index={0} />
                        <InputOTPSlot index={1} />
                        <InputOTPSlot index={2} />
                      </InputOTPGroup>
                      <InputOTPSeparator />
                      <InputOTPGroup>
                        <InputOTPSlot index={3} />
                        <InputOTPSlot index={4} />
                        <InputOTPSlot index={5} />
                      </InputOTPGroup>
                    </InputOTP>
                  )}
                />

                <Button
                  type="submit"
                  size="lg"
                  className="w-full"
                  loading={isConfirming}
                >
                  Confirm email
                </Button>

                <div className="flex items-center gap-4 text-sm">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={resendConfirmation}
                    loading={isResending}
                    className="h-auto px-0 py-0"
                  >
                    Resend code
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      confirmationForm.reset()
                      changeEmail()
                    }}
                    className="h-auto px-0 py-0"
                  >
                    Use a different email
                  </Button>
                </div>
              </form>
            ) : (
              <>
                <OAuthButtons />
                <AuthDivider />
                <form onSubmit={onSignupSubmit} className="flex flex-col gap-3">
                  <div className="space-y-2">
                    <Label htmlFor="email">Work email</Label>
                    <Input
                      id="email"
                      type="email"
                      autoComplete="email"
                      required
                      className="h-12 rounded-lg"
                      placeholder="you@company.com"
                      {...signupForm.register("email", { required: true })}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="password">Password</Label>
                    <Input
                      id="password"
                      type="password"
                      autoComplete="new-password"
                      required
                      className="h-12 rounded-lg"
                      placeholder="Create a password"
                      {...signupForm.register("password", { required: true })}
                    />
                  </div>
                  <Button
                    type="submit"
                    size="lg"
                    className="w-full"
                    loading={isPending}
                  >
                    Create account
                  </Button>
                </form>
                <p className="text-center text-sm text-muted-foreground">
                  Already have an account?{" "}
                  <Link
                    href="/auth/login"
                    className="font-medium text-foreground underline-offset-4 transition-colors hover:underline"
                  >
                    Sign in
                  </Link>
                </p>
              </>
            )}
          </motion.div>

          {/* Footer */}
          <AuthFooter />
        </div>
      </AuthCard>
    </>
  )
}
