"use client"

import { createContext, useContext, useCallback } from "react"
import { useRouter } from "next/navigation"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import type { components } from "@/lib/api/schema"

type User = components["schemas"]["userResponse"]

type AuthContextValue = {
  user: User | null
  logout: () => Promise<void>
  isLoading: boolean
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const router = useRouter()
  const { data, isLoading } = $api.useQuery("get", "/auth/me")

  const user = (data?.user as User) ?? null

  const logout = useCallback(async () => {
    await api.POST("/auth/logout", { body: {} as never })
    router.replace("/auth")
  }, [router])

  return (
    <AuthContext.Provider value={{ user, logout, isLoading }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error("useAuth must be used within AuthProvider")
  return ctx
}
