"use client"

import { useState, useEffect, useCallback } from "react"
import { useRouter } from "next/navigation"
import Link from "next/link"
import { AnimatePresence, motion } from "motion/react"
import { Button } from "@/components/ui/button"
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSlot,
  InputOTPSeparator,
} from "@/components/ui/input-otp"
import { HugeiconsIcon } from "@hugeicons/react"
import { Loading03Icon } from "@hugeicons/core-free-icons"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Logo } from "@/components/logo"
import { apiUrl } from "@/lib/api/client"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"

type AuthStep = "buttons" | "email" | "code"

const RESEND_COOLDOWN = 60

const stepVariants = {
  initial: { opacity: 0, y: 8 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -8 },
}

const stepTransition = { duration: 0.2, ease: "easeInOut" as const }

export default function AuthPage() {
  const router = useRouter()
  const [step, setStep] = useState<AuthStep>("buttons")
  const [email, setEmail] = useState("")
  const [code, setCode] = useState("")
  const [resendTimer, setResendTimer] = useState(0)

  const otpRequest = $api.useMutation("post", "/auth/otp/request")
  const otpVerify = $api.useMutation("post", "/auth/otp/verify")

  useEffect(() => {
    if (resendTimer <= 0) return
    const interval = setInterval(() => {
      setResendTimer((t) => t - 1)
    }, 1000)
    return () => clearInterval(interval)
  }, [resendTimer])

  const requestOTP = useCallback(async () => {
    otpRequest.mutate(
      { body: { email } as never },
      {
        onSuccess: () => {
          setResendTimer(RESEND_COOLDOWN)
          if (step === "email") {
            setStep("code")
            setCode("")
          }
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to send code"))
        },
      },
    )
  }, [email, otpRequest, step])

  async function handleEmailSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!email.trim()) return
    requestOTP()
  }

  function handleVerify(value: string) {
    setCode(value)
    if (value.length < 6) return

    otpVerify.mutate(
      { body: { email, code: value } as never },
      {
        onSuccess: () => {
          router.replace("/w")
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Invalid or expired code"))
          setCode("")
        },
      },
    )
  }

  return (
    <div className="flex min-h-screen bg-background">
      <div className="flex flex-col justify-center w-full lg:w-1/2 px-6 sm:px-12 lg:px-24">
        <div className="w-full max-w-md mx-auto flex flex-col gap-6">
          <div className="w-full flex justify-center">
            <Link href="/">
              <Logo className="h-8" />
            </Link>
          </div>

          <div className="flex flex-col gap-3">
            <h1 className="font-heading text-[28px] text-center font-bold text-foreground leading-tight -tracking-micro">
              Build, run and monitor <br />your agents
            </h1>
            <p className="text-base text-muted-foreground leading-relaxed text-center">
              Connect your apps, provide controlled access to your ai agents, and get full visibility into their work.
            </p>
          </div>

          <AnimatePresence mode="wait">
            {step === "buttons" && (
              <motion.div
                key="buttons"
                variants={stepVariants}
                initial="initial"
                animate="animate"
                exit="exit"
                transition={stepTransition}
                className="flex flex-col gap-3 mt-4 max-w-sm w-full mx-auto"
              >
                <Button variant="outline" size="default" className="w-full h-12 cursor-pointer" render={<a href={apiUrl("/oauth/github")} />}>
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" className="mr-2.5 opacity-70">
                    <title>github</title>
                    <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/>
                  </svg>
                  Continue with GitHub
                </Button>

                <Button variant="outline" size="default" className="w-full h-12 cursor-pointer" render={<a href={apiUrl("/oauth/google")} />}>
                  <svg width="18" height="18" viewBox="0 0 24 24" className="mr-2.5">
                    <title>google</title>
                    <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 01-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z" fill="#4285F4"/>
                    <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
                    <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18A11.96 11.96 0 001 12c0 1.94.46 3.77 1.18 5.27l3.66-2.84z" fill="#FBBC05"/>
                    <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
                  </svg>
                  Continue with Google
                </Button>

                <Button variant="outline" size="default" className="w-full h-12 cursor-pointer" render={<a href={apiUrl("/oauth/x")} />}>
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" className="mr-2.5 opacity-70">
                    <title>x</title>
                    <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/>
                  </svg>
                  Continue with X
                </Button>

                <Button variant="outline" size="default" className="w-full h-12 cursor-pointer" onClick={() => { setStep("email");  }}>
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="mr-2.5 opacity-70">
                    <title>email</title>
                    <rect width="20" height="16" x="2" y="4" rx="2"/>
                    <path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7"/>
                  </svg>
                  Continue with email
                </Button>
              </motion.div>
            )}

            {step === "email" && (
              <motion.div
                key="email"
                variants={stepVariants}
                initial="initial"
                animate="animate"
                exit="exit"
                transition={stepTransition}
                className="flex flex-col gap-3 mt-4 max-w-sm w-full mx-auto"
              >
                <form onSubmit={handleEmailSubmit} className="flex flex-col gap-3">
                  <div className="space-y-2">
                    <Label htmlFor="email">Email</Label>
                    <Input
                      id="email"
                      type="email"
                      autoComplete="email"
                      autoFocus
                      required
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      className="h-12"
                      placeholder="you@company.com"
                    />
                  </div>

                  <Button type="submit" className="w-full h-12" loading={otpRequest.isPending}>
                    Send code
                  </Button>
                </form>

                <button
                  type="button"
                  onClick={() => { setStep("buttons");  }}
                  className="cursor-pointer text-sm text-muted-foreground text-center hover:text-foreground transition-colors"
                >
                  All sign-in options
                </button>
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
                className="flex flex-col gap-4 mt-4 max-w-sm w-full mx-auto"
              >
                <p className="text-sm text-muted-foreground text-center">
                  Enter the 6-digit code sent to{" "}
                  <span className="font-medium text-foreground">{email}</span>
                </p>

                <div className="flex justify-center my-3">
                  <InputOTP
                    maxLength={6}
                    value={code}
                    onChange={handleVerify}
                    disabled={otpVerify.isPending}
                    autoFocus
                  >
                    <InputOTPGroup>
                      <InputOTPSlot index={0} className="size-12 text-lg" />
                      <InputOTPSlot index={1} className="size-12 text-lg" />
                      <InputOTPSlot index={2} className="size-12 text-lg" />
                    </InputOTPGroup>
                    <InputOTPSeparator className="mx-3" />
                    <InputOTPGroup>
                      <InputOTPSlot index={3} className="size-12 text-lg" />
                      <InputOTPSlot index={4} className="size-12 text-lg" />
                      <InputOTPSlot index={5} className="size-12 text-lg" />
                    </InputOTPGroup>
                  </InputOTP>
                </div>

                {otpVerify.isPending && (
                  <p className="text-center text-sm text-muted-foreground">Verifying...</p>
                )}

                <div className="text-center text-sm text-muted-foreground">
                  {resendTimer > 0 ? (
                    <span>Resend code in {resendTimer}s</span>
                  ) : (
                    <button
                      type="button"
                      onClick={() => requestOTP()}
                      disabled={otpRequest.isPending}
                      className="cursor-pointer inline-flex items-center gap-1.5 underline underline-offset-4 hover:text-foreground transition-colors disabled:opacity-50"
                    >
                      {otpRequest.isPending && (
                        <HugeiconsIcon icon={Loading03Icon} className="size-3.5 animate-spin" />
                      )}
                      Resend code
                    </button>
                  )}
                </div>

                <button
                  type="button"
                  onClick={() => { setStep("email"); setCode("");  }}
                  className="cursor-pointer text-sm text-muted-foreground text-center hover:text-foreground transition-colors"
                >
                  Use a different email
                </button>
              </motion.div>
            )}
          </AnimatePresence>

          <div className="flex items-center justify-center">
            <p className="text-xs text-muted-foreground/60 leading-relaxed">
              By continuing, you agree to our{" "}
              <a href="https://hiveloop.com" className="text-muted-foreground hover:text-foreground underline underline-offset-2 transition-colors">
                Terms of Service
              </a>{" "}
              and{" "}
              <a href="https://hiveloop.com" className="text-muted-foreground hover:text-foreground underline underline-offset-2 transition-colors">
                Privacy Policy
              </a>
              .
            </p>
          </div>
        </div>
      </div>

      <div className="hidden lg:flex w-1/2 p-4">
        <div className="w-full h-full rounded-2xl border border-border bg-muted flex items-center justify-center">
          <span className="font-mono text-sm text-muted-foreground">Illustration</span>
        </div>
      </div>
    </div>
  )
}
