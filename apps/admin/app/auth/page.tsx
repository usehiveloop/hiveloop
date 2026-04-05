"use client"

import { useState, useEffect, useCallback } from "react"
import { useRouter } from "next/navigation"
import { Button } from "@/components/ui/button"
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSlot,
  InputOTPSeparator,
} from "@/components/ui/input-otp"
import { api } from "@/lib/api/client"

type AuthStep = "choose" | "email" | "code"

const RESEND_COOLDOWN = 60 // seconds

export default function AuthPage() {
  const router = useRouter()
  const [step, setStep] = useState<AuthStep>("choose")
  const [email, setEmail] = useState("")
  const [code, setCode] = useState("")
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)
  const [resendTimer, setResendTimer] = useState(0)

  // Countdown timer for resend
  useEffect(() => {
    if (resendTimer <= 0) return
    const interval = setInterval(() => {
      setResendTimer((t) => t - 1)
    }, 1000)
    return () => clearInterval(interval)
  }, [resendTimer])

  const requestOTP = useCallback(async () => {
    setLoading(true)
    setError("")
    try {
      const res = await fetch("/api/proxy/auth/otp/request", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email }),
      })
      if (!res.ok) {
        const data = await res.json().catch(() => ({}))
        setError(data.error || "Failed to send code")
        setLoading(false)
        return false
      }
      setResendTimer(RESEND_COOLDOWN)
      setLoading(false)
      return true
    } catch {
      setError("Network error")
      setLoading(false)
      return false
    }
  }, [email])

  async function handleEmailSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!email.trim()) return
    const ok = await requestOTP()
    if (ok) {
      setStep("code")
      setCode("")
    }
  }

  async function handleResend() {
    if (resendTimer > 0) return
    await requestOTP()
  }

  async function handleVerify(value: string) {
    setCode(value)
    if (value.length < 6) return

    setLoading(true)
    setError("")
    try {
      const res = await fetch("/api/proxy/auth/otp/verify", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, code: value }),
      })
      if (!res.ok) {
        const data = await res.json().catch(() => ({}))
        setError(data.error || "Invalid code")
        setCode("")
        setLoading(false)
        return
      }
      router.replace("/dashboard")
    } catch {
      setError("Network error")
      setCode("")
      setLoading(false)
    }
  }

  // -- Choose step: show "Continue with email" button --
  if (step === "choose") {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="w-full max-w-sm space-y-6 px-6">
          <div className="space-y-2 text-center">
            <h1 className="text-2xl font-semibold tracking-tight">
              Zeus Admin
            </h1>
            <p className="text-sm text-muted-foreground">
              Sign in to the platform admin panel
            </p>
          </div>

          <div className="space-y-3">
            <Button
              className="w-full"
              onClick={() => {
                setStep("email")
                setError("")
              }}
            >
              Continue with email
            </Button>
          </div>
        </div>
      </div>
    )
  }

  // -- Email step: email input --
  if (step === "email") {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="w-full max-w-sm space-y-6 px-6">
          <div className="space-y-2 text-center">
            <h1 className="text-2xl font-semibold tracking-tight">
              Zeus Admin
            </h1>
            <p className="text-sm text-muted-foreground">
              Enter your admin email to receive a sign-in code
            </p>
          </div>

          <form onSubmit={handleEmailSubmit} className="space-y-4">
            {error && (
              <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                {error}
              </div>
            )}

            <div className="space-y-2">
              <label
                htmlFor="email"
                className="text-sm font-medium leading-none"
              >
                Email
              </label>
              <input
                id="email"
                type="email"
                autoComplete="email"
                autoFocus
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="flex h-10 w-full rounded-lg border border-input bg-background px-3 py-2 text-sm outline-none transition-colors placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/30"
                placeholder="admin@ziraloop.com"
              />
            </div>

            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? "Sending code..." : "Send code"}
            </Button>
          </form>

          <p className="text-center text-sm text-muted-foreground">
            <button
              type="button"
              onClick={() => {
                setStep("choose")
                setError("")
              }}
              className="text-primary underline-offset-4 hover:underline"
            >
              Back
            </button>
          </p>
        </div>
      </div>
    )
  }

  // -- Code step: OTP input --
  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-sm space-y-6 px-6">
        <div className="space-y-2 text-center">
          <h1 className="text-2xl font-semibold tracking-tight">
            Zeus Admin
          </h1>
          <p className="text-sm text-muted-foreground">
            Enter the 6-digit code sent to{" "}
            <span className="font-medium text-foreground">{email}</span>
          </p>
        </div>

        <div className="space-y-4">
          {error && (
            <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
              {error}
            </div>
          )}

          <div className="flex justify-center">
            <InputOTP
              maxLength={6}
              value={code}
              onChange={handleVerify}
              disabled={loading}
              autoFocus
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
          </div>

          {loading && (
            <p className="text-center text-sm text-muted-foreground">
              Verifying...
            </p>
          )}

          <div className="text-center text-sm text-muted-foreground">
            {resendTimer > 0 ? (
              <span>Resend code in {resendTimer}s</span>
            ) : (
              <button
                type="button"
                onClick={handleResend}
                disabled={loading}
                className="text-primary underline-offset-4 hover:underline"
              >
                Resend code
              </button>
            )}
          </div>
        </div>

        <p className="text-center text-sm text-muted-foreground">
          <button
            type="button"
            onClick={() => {
              setStep("email")
              setCode("")
              setError("")
            }}
            className="text-primary underline-offset-4 hover:underline"
          >
            Use a different email
          </button>
        </p>
      </div>
    </div>
  )
}
