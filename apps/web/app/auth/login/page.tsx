"use client"

import { useState } from "react"
import Link from "next/link"
import { AnimatePresence, motion } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon } from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSlot,
  InputOTPSeparator,
} from "@/components/ui/input-otp"
import {
  AuthCard,
  AuthGhostLogo,
  AuthDivider,
  AuthFooter,
  OAuthButtons,
  stepVariants,
  stepTransition,
} from "../_components/shared"

type Step = "buttons" | "email" | "code" | "success"

export default function LoginPage() {
  const [step, setStep] = useState<Step>("buttons")
  const [email, setEmail] = useState("")
  const [code, setCode] = useState("")
  const [resendTimer, setResendTimer] = useState(0)

  const handleEmailSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!email.trim()) return
    setStep("code")
    setResendTimer(60)
  }

  const handleCodeComplete = (value: string) => {
    setCode(value)
    if (value.length === 6) {
      setTimeout(() => setStep("success"), 800)
    }
  }

  const handleResend = () => {
    setResendTimer(60)
  }

  return (
    <>
      <div className="fixed top-5 right-0 left-0 z-50 mx-auto flex max-w-5xl items-center px-4 md:px-0">
        <Button variant="ghost" size="sm" asChild>
          <Link href="/" className="font-display flex items-center">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={14} />
            Home
          </Link>
        </Button>
      </div>

      <AuthCard>
        <div className="flex flex-col gap-8">
        {/* Header */}
        <div className="flex flex-col items-center gap-4">
          <AuthGhostLogo />
          <div className="text-center">
            <h1 className="font-heading text-2xl font-normal tracking-[-0.02em] text-foreground">
              Sign in to hivy
            </h1>
            <p className="mt-1.5 text-sm text-muted-foreground">
              Welcome back. Sign in to manage your AI coworkers.
            </p>
          </div>
        </div>

        {/* Content */}
        <AnimatePresence mode="wait">
          {step === "buttons" && (
            <motion.div
              key="buttons"
              variants={stepVariants}
              initial="initial"
              animate="animate"
              exit="exit"
              transition={stepTransition}
              className="flex flex-col gap-6"
            >
              <OAuthButtons />
              <AuthDivider />
              <form
                onSubmit={handleEmailSubmit}
                className="flex flex-col gap-3"
              >
                <div className="space-y-2">
                  <Label htmlFor="email">Work email</Label>
                  <Input
                    id="email"
                    type="email"
                    autoComplete="email"
                    required
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="you@company.com"
                  />
                </div>
                <Button type="submit" size="lg" className="w-full">
                  Continue with email
                </Button>
              </form>
              <p className="text-center text-sm text-muted-foreground">
                Don't have an account?{" "}
                <Link
                  href="/auth/signup"
                  className="font-medium text-foreground hover:underline underline-offset-4 transition-colors"
                >
                  Sign up
                </Link>
              </p>
            </motion.div>
          )}

          {step === "code" && (
            <motion.div
              key="code"
              variants={stepVariants}
              initial="initial"
              animate="animate"
              exit="exit"
              transition={stepTransition}
              className="flex flex-col gap-6"
            >
              <div className="text-center">
                <p className="text-sm text-muted-foreground">
                  Enter the 6-digit code sent to{" "}
                  <span className="font-medium text-foreground">{email}</span>
                </p>
              </div>

              <InputOTP
                maxLength={6}
                value={code}
                onChange={handleCodeComplete}
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

              <div className="text-center text-sm text-muted-foreground">
                {resendTimer > 0 ? (
                  <span>Resend code in {resendTimer}s</span>
                ) : (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={handleResend}
                    className="h-auto px-0 py-0 underline underline-offset-4 hover:text-foreground"
                  >
                    Resend code
                  </Button>
                )}
              </div>

              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setStep("buttons")
                  setCode("")
                }}
                className="mx-auto h-auto px-0 py-0 text-sm text-muted-foreground hover:text-foreground"
              >
                Use a different email
              </Button>
            </motion.div>
          )}

          {step === "success" && (
            <motion.div
              key="success"
              variants={stepVariants}
              initial="initial"
              animate="animate"
              exit="exit"
              transition={stepTransition}
              className="flex flex-col items-center gap-4 py-4"
            >
              <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 text-primary">
                <svg
                  width="24"
                  height="24"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <polyline points="20 6 9 17 4 12" />
                </svg>
              </div>
              <div className="text-center">
                <h2 className="font-heading text-lg font-medium text-foreground">
                  Welcome back
                </h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  Redirecting to your workspace...
                </p>
              </div>
            </motion.div>
          )}
        </AnimatePresence>

        {/* Footer */}
        <AuthFooter />
      </div>
    </AuthCard>
    </>
  )
}
