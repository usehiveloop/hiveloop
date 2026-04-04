"use client"

import { useSearchParams, useRouter } from "next/navigation"
import { useEffect, useRef } from "react"
import { $api } from "@/lib/api/hooks"
import { LogoMark } from "@/components/logo"

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
      router.replace("/auth?error=exchange_failed")
    },
  })

  useEffect(() => {
    if (error) {
      router.replace(`/auth?error=${error}`)
      return
    }

    if (!token || exchanged.current) return
    exchanged.current = true

    exchange.mutate({ body: { token } })
  }, [token, error, router])

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="flex flex-col items-center gap-4">
        <LogoMark className="h-10 w-10 animate-pulse" />
        <p className="text-sm text-muted-foreground">
          {error || exchange.isError
            ? `Something went wrong (${error ?? "exchange_failed"})`
            : "Signing you in..."}
        </p>
      </div>
    </div>
  )
}
