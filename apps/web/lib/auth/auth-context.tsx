"use client"

import { createContext, useContext, useCallback, useState, useEffect } from "react"
import { useRouter } from "next/navigation"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import type { components } from "@/lib/api/schema"

type User = components["schemas"]["userResponse"]
type Org = components["schemas"]["orgMemberDTO"]

type AuthContextValue = {
  user: User | null
  orgs: Org[]
  activeOrg: Org | null
  setActiveOrg: (org: Org) => void
  logout: () => Promise<void>
  isLoading: boolean
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const router = useRouter()
  const { data, isLoading } = $api.useQuery("get", "/auth/me")

  const user = (data?.user as User) ?? null
  const orgs = (data?.orgs as Org[]) ?? []

  const [activeOrgId, setActiveOrgId] = useState<string | null>(() => {
    if (typeof window === "undefined") return null
    return localStorage.getItem("activeOrgId")
  })

  const activeOrg =
    orgs.find((o) => o.id === activeOrgId) ?? orgs[0] ?? null

  useEffect(() => {
    if (activeOrg?.id && activeOrg.id !== activeOrgId) {
      setActiveOrgId(activeOrg.id)
      localStorage.setItem("activeOrgId", activeOrg.id)
    }
  }, [activeOrg?.id, activeOrgId])

  const setActiveOrg = useCallback((org: Org) => {
    if (org.id) {
      setActiveOrgId(org.id)
      localStorage.setItem("activeOrgId", org.id)
    }
  }, [])

  const logout = useCallback(async () => {
    await api.POST("/auth/logout", { body: {} })
    router.replace("/auth")
  }, [router])

  return (
    <AuthContext.Provider
      value={{ user, orgs, activeOrg, setActiveOrg, logout, isLoading }}
    >
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error("useAuth must be used within AuthProvider")
  return ctx
}
