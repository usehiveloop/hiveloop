"use client"

import { useEffect } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { toast } from "sonner"

export default function WorkspacePage() {
  const router = useRouter()
  const searchParams = useSearchParams()

  useEffect(() => {
    if (searchParams.get("checkout") === "success") {
      toast.success("Subscription activated! You're on the Pro plan.")
      router.replace("/w")
    }
  }, [searchParams, router])

  return (
    <div className="flex items-center justify-center min-h-[calc(100vh-54px)]">
      <span className="font-mono text-sm text-muted-foreground">Workspace</span>
    </div>
  )
}
