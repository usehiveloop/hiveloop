"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"

import { AppSidebar } from "@/components/app-sidebar"
import { ImpersonationBanner } from "@/components/impersonation-banner"
import { Loader } from "@/components/loader"
import { OnboardingPanel } from "@/components/onboarding-panel"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
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
      <div className="flex flex-1 items-center justify-center">
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
      <ImpersonationBanner />
      <SidebarProvider>
        <AppSidebar />
        <SidebarInset>
          <AuthGate>{children}</AuthGate>
        </SidebarInset>
        <OnboardingPanel />
      </SidebarProvider>
    </AuthProvider>
  )
}
