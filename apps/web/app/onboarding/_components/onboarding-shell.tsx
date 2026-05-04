"use client"

import Link from "next/link"
import { Logo } from "@/components/logo"
import { Button } from "@/components/ui/button"

export function OnboardingShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="fixed inset-0 flex h-screen w-screen flex-col bg-background">
      <header className="flex w-full items-center justify-between px-6 py-5 sm:px-10">
        <Link href="/" aria-label="Hiveloop home" className="shrink-0">
          <Logo className="h-8" />
        </Link>

        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            render={<a href="mailto:support@usehiveloop.com" />}
          >
            Support
          </Button>
          <Button
            variant="ghost"
            size="sm"
            render={<Link href="/auth?logout=1" />}
          >
            Log out
          </Button>
        </div>
      </header>

      <main className="flex flex-1 items-start justify-center overflow-y-auto px-6 pb-16 pt-8 sm:px-10">
        {children}
      </main>
    </div>
  )
}
