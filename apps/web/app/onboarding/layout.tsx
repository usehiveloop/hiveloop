"use client"

import { FullPageLoader } from "@/components/full-page-loader"
import { AuthProvider, useAuth } from "@/lib/auth/auth-context"

function OnboardingGate({ children }: { children: React.ReactNode }) {
  const { user, isLoading } = useAuth()

  if (isLoading || !user) {
    return <FullPageLoader description="Loading workspace" />
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
