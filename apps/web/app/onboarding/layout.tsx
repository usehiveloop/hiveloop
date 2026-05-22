"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"
import { FullPageLoader } from "@/components/full-page-loader"
import { AuthProvider, useAuth } from "@/lib/auth/auth-context"

function OnboardingGate({ children }: { children: React.ReactNode }) {
  const { user, activeOrg, isLoading } = useAuth()
  const router = useRouter()

  useEffect(() => {
    if (activeOrg?.onboarded) {
      router.replace("/w")
    }
  }, [activeOrg?.onboarded, router])

  if (isLoading || !user || activeOrg?.onboarded) {
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
