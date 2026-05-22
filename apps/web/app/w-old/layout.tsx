"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"

import { AppTopbar } from "@/components/app-topbar"
import { Loader } from "@/components/loader"
import { AuthProvider, useAuth } from "@/lib/auth/auth-context"

function AuthGate({ children }: { children: React.ReactNode }) {
  const { user, activeOrg, isLoading } = useAuth()
  const router = useRouter()

  const needsOnboarding = activeOrg !== null && !activeOrg.onboarded

  useEffect(() => {
    if (needsOnboarding) {
      router.replace("/onboarding")
    }
  }, [needsOnboarding, router])

  if (isLoading || !user || needsOnboarding) {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center">
        <Loader description="Loading workspace" />
      </div>
    )
  }
  return <>{children}</>
}

export default function WorkspaceLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <AuthProvider>
      <div className="flex min-h-screen flex-col bg-background">
        <AppTopbar />
        <main className="flex min-h-0 flex-1 flex-col">
          <AuthGate>{children}</AuthGate>
        </main>
      </div>
    </AuthProvider>
  )
}
