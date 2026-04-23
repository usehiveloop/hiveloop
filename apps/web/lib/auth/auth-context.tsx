"use client"

import { createContext, useContext, useCallback, useState, useEffect, useRef, useMemo } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { FullPageLoader } from "@/components/full-page-loader"
import type { components } from "@/lib/api/schema"

type User = components["schemas"]["userResponse"]
type Org = components["schemas"]["orgMemberDTO"]

const ACTIVE_ORG_COOKIE = "hiveloop_active_org"

function getOrgIdFromCookie(): string | null {
  if (typeof document === "undefined") return null
  const match = document.cookie.match(new RegExp(`(?:^|; )${ACTIVE_ORG_COOKIE}=([^;]+)`))
  return match ? decodeURIComponent(match[1]) : null
}

function setOrgIdCookie(orgId: string) {
  document.cookie = `${ACTIVE_ORG_COOKIE}=${encodeURIComponent(orgId)}; path=/; max-age=${60 * 60 * 24 * 365}; samesite=lax`
}

type ImpersonatingInfo = {
  userId: string
  email: string
  name: string
}

function getImpersonatingInfo(): ImpersonatingInfo | null {
  if (typeof document === "undefined") return null
  const match = document.cookie.match(/(?:^|; )__impersonating=([^;]+)/)
  if (!match) return null
  try {
    return JSON.parse(decodeURIComponent(match[1])) as ImpersonatingInfo
  } catch {
    return null
  }
}

interface AuthContextValue {
  user: User | null
  orgs: Org[]
  activeOrg: Org | null
  setActiveOrg: (org: Org) => void
  addOrg: (org: Org) => void
  logout: () => Promise<void>
  isLoading: boolean
  isPlatformAdmin: boolean
  isImpersonating: boolean
  impersonatedUser: ImpersonatingInfo | null
  impersonate: (userId: string) => Promise<void>
  stopImpersonating: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const { data, isLoading, isError } = $api.useQuery("get", "/auth/me", {}, { retry: false })
  const hasRedirected = useRef(false)

  const user = (data?.user as User) ?? null
  const orgs = (data?.orgs as Org[]) ?? []
  const isPlatformAdmin = (data as Record<string, unknown>)?.is_platform_admin === true

  const [activeOrgId, setActiveOrgId] = useState<string | null>(() => getOrgIdFromCookie())

  const impersonatingInfo = useMemo(() => getImpersonatingInfo(), [])
  const isImpersonating = impersonatingInfo !== null

  const activeOrg =
    orgs.find((org) => org.id === activeOrgId) ?? orgs[0] ?? null

  useEffect(() => {
    if (isError && !hasRedirected.current) {
      hasRedirected.current = true
      router.replace("/auth")
    }
  }, [isError, router])

  useEffect(() => {
    if (activeOrg?.id && activeOrg.id !== activeOrgId) {
      setActiveOrgId(activeOrg.id)
      setOrgIdCookie(activeOrg.id)
    }
  }, [activeOrg?.id, activeOrgId])

  const setActiveOrg = useCallback((org: Org) => {
    if (org.id) {
      setActiveOrgId(org.id)
      setOrgIdCookie(org.id)
      queryClient.invalidateQueries()
    }
  }, [queryClient])

  const addOrg = useCallback((org: Org) => {
    queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
    if (org.id) {
      setActiveOrgId(org.id)
      setOrgIdCookie(org.id)
    }
  }, [queryClient])

  const logout = useCallback(async () => {
    await api.POST("/auth/logout", { body: {} })
    router.replace("/auth")
  }, [router])

  // `/api/auth/impersonate` and `/api/auth/stop-impersonate` are local Next.js
  // route handlers (see app/api/auth/…/route.ts). They mutate session cookies
  // before/after proxying to the backend, so they live outside the OpenAPI
  // schema and cannot go through the typed `$api` client. Raw fetch is the
  // correct path here.
  const impersonate = useCallback(async (userId: string) => {
    const response = await fetch("/api/auth/impersonate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ userId }),
    })
    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: "impersonation failed" }))
      throw new Error(error.error ?? "impersonation failed")
    }
    window.location.reload()
  }, [])

  const stopImpersonating = useCallback(async () => {
    const response = await fetch("/api/auth/stop-impersonate", {
      method: "POST",
    })
    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: "failed to stop impersonating" }))
      throw new Error(error.error ?? "failed to stop impersonating")
    }
    window.location.reload()
  }, [])

  if (isLoading || isError) {
    return <FullPageLoader />
  }

  return (
    <AuthContext.Provider
      value={{
        user,
        orgs,
        activeOrg,
        setActiveOrg,
        addOrg,
        logout,
        isLoading,
        isPlatformAdmin,
        isImpersonating,
        impersonatedUser: impersonatingInfo,
        impersonate,
        stopImpersonating,
      }}
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
