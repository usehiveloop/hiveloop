"use client"

import { useEffect } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { toast } from "sonner"
import { PageHeader } from "@/components/page-header"

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
    <>
      <PageHeader title="Home" />
      <div className="mx-auto flex w-full max-w-4xl items-center justify-center px-6 py-24">
        <span className="font-mono text-sm text-muted-foreground">Workspace</span>
      </div>
    </>
  )
}
