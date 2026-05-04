"use client"

import { AuthProvider } from "@/lib/auth/auth-context"

export default function OnboardingLayout({ children }: { children: React.ReactNode }) {
  return <AuthProvider>{children}</AuthProvider>
}
