"use client"

import { useAuth } from "@/lib/auth/auth-context"

export function ImpersonationBanner() {
  const { isImpersonating, impersonatedUser, stopImpersonating } = useAuth()

  if (!isImpersonating || !impersonatedUser) return null

  return (
    <div className="sticky top-0 z-[100] flex items-center justify-center gap-4 bg-amber-500 px-4 py-2 text-sm font-medium text-black">
      <span>
        Viewing as {impersonatedUser.name} ({impersonatedUser.email})
      </span>
      <button
        type="button"
        onClick={stopImpersonating}
        className="rounded bg-black/20 px-3 py-1 text-xs font-semibold transition-colors hover:bg-black/30"
      >
        Stop Impersonating
      </button>
    </div>
  )
}
