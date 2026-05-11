"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"
import { Loader } from "@/components/loader"
import { AuthProvider } from "@/lib/auth/auth-context"
import { useAuth } from "@/lib/auth/auth-context"

function OnboardingGate({ children }: { children: React.ReactNode }) {
  const { user, activeOrg, isLoading } = useAuth()
  const router = useRouter()
  const isOnboarded = activeOrg?.onboarded === true

  useEffect(() => {
    if (user && isOnboarded) {
      router.replace("/w")
    }
  }, [isOnboarded, router, user])

  if (isLoading || isOnboarded) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader description="Loading workspace" />
      </div>
    )
  }

  return <>{children}</>
}

export default function OnboardingLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <AuthProvider>
      <OnboardingGate>{children}</OnboardingGate>
    </AuthProvider>
  )
}
