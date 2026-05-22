"use client"

import { Loader } from "@/components/loader"
import { AuthProvider, useAuth } from "@/lib/auth/auth-context"

function OnboardingGate({ children }: { children: React.ReactNode }) {
  const { user, isLoading } = useAuth()

  if (isLoading || !user) {
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
