"use client"

import { useSearchParams, useRouter } from "next/navigation"
import { useEffect, useRef } from "react"
import { AuthGhostLogo } from "@/components/auth-ghost-logo"
import { $api } from "@/lib/api/hooks"

export default function OAuthCallbackPage() {
  const searchParams = useSearchParams()
  const router = useRouter()
  const exchanged = useRef(false)

  const token = searchParams.get("token")
  const error = searchParams.get("error")

  const exchange = $api.useMutation("post", "/oauth/exchange", {
    onSuccess: () => {
      router.replace("/w")
    },
    onError: () => {
      router.replace("/auth/signin?error=exchange_failed")
    },
  })

  useEffect(() => {
    if (error) {
      router.replace(`/auth/signin?error=${error}`)
      return
    }

    if (!token || exchanged.current) return
    exchanged.current = true

    exchange.mutate({ body: { token } })
  }, [token, error, router, exchange])

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
