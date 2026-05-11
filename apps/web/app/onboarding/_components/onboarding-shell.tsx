"use client"

import { AppTopbar } from "@/components/app-topbar"

export function OnboardingShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="fixed inset-0 flex h-screen w-screen flex-col bg-background">
      <AppTopbar showPrimaryNav={false} onboarding logoHref="/" />

      <main className="flex flex-1 items-start justify-center overflow-y-auto px-6 pb-16 pt-8 sm:px-10">
        {children}
      </main>
    </div>
  )
}
