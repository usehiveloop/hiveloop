"use client"

import Link from "next/link"
import { Logo } from "@/components/logo"
import { Button } from "@/components/ui/button"
import { HeaderWorkspaceSwitcher } from "@/components/workspace-switcher"

export function OnboardingShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="fixed inset-0 flex h-screen w-screen flex-col bg-background">
      <header className="flex w-full items-center justify-between gap-4 px-6 py-5 sm:px-10">
        <div className="flex min-w-0 items-center gap-3">
          <Link href="/" aria-label="Hiveloop home" className="shrink-0">
            <Logo className="h-8" />
          </Link>
          <HeaderWorkspaceSwitcher className="hidden sm:flex" />
        </div>

        <div className="flex shrink-0 items-center gap-2">
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

      <div className="px-6 pb-2 sm:hidden">
        <HeaderWorkspaceSwitcher className="w-full max-w-full" />
      </div>

      <main className="flex flex-1 items-start justify-center overflow-y-auto px-6 pb-16 pt-8 sm:px-10">
        {children}
      </main>
    </div>
  )
}
