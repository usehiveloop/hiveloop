"use client"

import { AppSidebar } from "@/components/app-sidebar"
import { ImpersonationBanner } from "@/components/impersonation-banner"
import { Loader } from "@/components/loader"
import { OnboardingPanel } from "@/components/onboarding-panel"
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import { AuthProvider, useAuth } from "@/lib/auth/auth-context"

function AuthGate({ children }: { children: React.ReactNode }) {
  const { user, isLoading } = useAuth()
  if (isLoading || !user) {
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
