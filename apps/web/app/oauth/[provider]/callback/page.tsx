"use client"

import { useSearchParams, useRouter } from "next/navigation"
import { Suspense, useEffect, useRef } from "react"
import { AuthGhostLogo } from "@/components/auth-ghost-logo"
import { $api } from "@/lib/api/hooks"
import { safeAuthRedirect } from "@/hooks/use-password-auth"

function OAuthCallbackContents() {
  const searchParams = useSearchParams()
  const router = useRouter()
  const exchanged = useRef(false)

  const token = searchParams.get("token")
  const error = searchParams.get("error")
  const nextPath = safeAuthRedirect(searchParams.get("next"))
  const signinHref = `/auth/signin?error=exchange_failed${nextPath === "/w" ? "" : `&next=${encodeURIComponent(nextPath)}`}`

  const exchange = $api.useMutation("post", "/oauth/exchange", {
    onSuccess: () => {
      router.replace(nextPath)
    },
    onError: () => {
      router.replace(signinHref)
    },
  })

  useEffect(() => {
    if (error) {
      const nextQuery = nextPath === "/w" ? "" : `&next=${encodeURIComponent(nextPath)}`
      router.replace(`/auth/signin?error=${encodeURIComponent(error)}${nextQuery}`)
      return
    }

    if (!token || exchanged.current) return
    exchanged.current = true

    exchange.mutate({ body: { token } })
  }, [token, error, router, exchange, nextPath])

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <AuthGhostLogo
        logoClassName="h-16 w-16"
        description={
          error || exchange.isError
            ? `Something went wrong (${error ?? "exchange_failed"})`
            : "Signing you in..."
        }
      />
    </div>
  )
}

export default function OAuthCallbackPage() {
  return (
    <Suspense
      fallback={
        <div className="flex min-h-screen items-center justify-center bg-background">
          <AuthGhostLogo logoClassName="h-16 w-16" description="Signing you in..." />
        </div>
      }
    >
      <OAuthCallbackContents />
    </Suspense>
  )
}
