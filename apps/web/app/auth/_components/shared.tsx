"use client"

import Link from "next/link"
import { AnimatePresence, motion } from "motion/react"
import { AuthGhostLogo } from "@/components/auth-ghost-logo"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { apiUrl } from "@/lib/api/client"

/* ─────────────────────────── Shared Auth Card ─────────────────────────── */

interface AuthCardProps {
  children: React.ReactNode
}

export function AuthCard({ children }: AuthCardProps) {
  return (
    <div className="relative flex min-h-screen items-center justify-center bg-background px-4 py-12 font-display">
      {/* Background glow */}
      <div className="pointer-events-none fixed inset-0 overflow-hidden">
        <div className="absolute -top-52 -left-28 h-112 w-md rounded-full bg-(--glow-left) opacity-40 blur-[140px]" />
        <div className="absolute -top-40 left-1/2 h-112 w-md -translate-x-1/2 rounded-full bg-(--glow-center) opacity-35 blur-[140px]" />
        <div className="absolute -top-52 -right-28 h-112 w-md rounded-full bg-(--glow-right) opacity-35 blur-[140px]" />
      </div>

      {/* Card */}
      <div className="relative z-10 w-full max-w-105 rounded-2xl border border-border bg-secondary/80 p-8 shadow-xl backdrop-blur-xl sm:p-10">
        {children}
      </div>
    </div>
  )
}

export { AuthGhostLogo }

/* ─────────────────────────── Step Animation ─────────────────────────── */

const stepVariants = {
  initial: { opacity: 0, y: 8 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -8 },
}

const stepTransition = { duration: 0.2, ease: "easeInOut" as const }

export { stepVariants, stepTransition }

/* ─────────────────────────── OAuth Buttons ─────────────────────────── */

export function OAuthButtons({ nextPath = "/w" }: { nextPath?: string }) {
  const withNext = (path: string) => {
    if (nextPath === "/w") return apiUrl(path)
    return apiUrl(`${path}?next=${encodeURIComponent(nextPath)}`)
  }

  return (
    <div className="flex flex-col gap-3">
      <Button
        variant="outline"
        size="lg"
        className="w-full justify-start gap-3"
        render={<a href={withNext("/oauth/github")} />}
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/>
        </svg>
        Continue with GitHub
      </Button>
      <Button
        variant="outline"
        size="lg"
        className="w-full justify-start gap-3"
        render={<a href={withNext("/oauth/google")} />}
      >
        <svg width="18" height="18" viewBox="0 0 24 24">
          <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 01-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z" fill="#4285F4"/>
          <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
          <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18A11.96 11.96 0 001 12c0 1.94.46 3.77 1.18 5.27l3.66-2.84z" fill="#FBBC05"/>
          <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
        </svg>
        Continue with Google
      </Button>
      <Button
        variant="outline"
        size="lg"
        className="w-full justify-start gap-3"
        render={<a href={withNext("/oauth/x")} />}
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
          <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"/>
        </svg>
        Continue with X
      </Button>
    </div>
  )
}

/* ─────────────────────────── Divider ─────────────────────────── */

export function AuthDivider() {
  return (
    <div className="relative flex items-center gap-4">
      <div className="h-px flex-1 bg-border" />
      <span className="text-xs font-medium text-muted-foreground">or</span>
      <div className="h-px flex-1 bg-border" />
    </div>
  )
}

/* ─────────────────────────── Auth Footer ─────────────────────────── */

export function AuthFooter() {
  return (
    <div className="text-center">
      <p className="text-xs text-muted-foreground/60 leading-relaxed">
        By continuing, you agree to our{" "}
        <a href="/terms" className="text-muted-foreground hover:text-foreground underline underline-offset-2 transition-colors">
          Terms
        </a>{" "}
        and{" "}
        <a href="/legal" className="text-muted-foreground hover:text-foreground underline underline-offset-2 transition-colors">
          Privacy Policy
        </a>
        .
      </p>
    </div>
  )
}
