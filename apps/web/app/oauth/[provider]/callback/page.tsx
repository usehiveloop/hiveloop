"use client"

import { useSearchParams, useRouter } from "next/navigation"
import { useEffect, useRef } from "react"
import { motion } from "motion/react"
import { $api } from "@/lib/api/hooks"
import { AuthGhostLogo } from "@/app/auth/_components/shared"

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
      <div className="flex flex-col items-center gap-4">
        <motion.div
          animate={{ scale: [1, 1.18, 1] }}
          transition={{ duration: 1.2, repeat: Infinity, ease: "easeInOut" }}
        >
          <AuthGhostLogo className="[&_svg]:h-16 [&_svg]:w-16" />
        </motion.div>
        <p className="text-sm text-muted-foreground">
          {error || exchange.isError
            ? `Something went wrong (${error ?? "exchange_failed"})`
            : "Signing you in..."}
        </p>
      </div>
    </div>
  )
}
