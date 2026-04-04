import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Logo } from "@/components/logo"

export default function Home() {
  return (
    <div className="w-full bg-background flex flex-col relative">
      <nav className="w-full h-16 flex items-center justify-between max-w-424 mx-auto sticky top-0 bg-background z-100 px-4 lg:px-0">
        <Link href="/"><Logo className="h-8" /></Link>
        <div className="hidden md:flex items-center gap-6 lg:gap-9">
          <Link href="/docs" className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors">Docs</Link>
          <Link href="/pricing" className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors">Pricing</Link>
          <Link href="/marketplace" className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors">Marketplace</Link>
        </div>
        <Link href="/auth">
          <Button variant="outline" size="sm">
            Sign in
          </Button>
        </Link>
      </nav>

      <div className="flex flex-1 px-4 sm:px-6 lg:px-0">
        <div className="w-full max-w-424 lg:min-h-425 mx-auto relative overflow-hidden">
          {/* Grid background */}
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              backgroundImage:
                "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
              backgroundSize: "40px 40px",
              maskImage:
                "radial-gradient(ellipse at center, black, transparent 80%)",
            }}
          />
          {/* Hero glow */}
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background:
                "radial-gradient(circle at 50% 40%, color-mix(in oklch, var(--primary) 12%, transparent) 0%, transparent 70%)",
            }}
          />
          <div className="relative flex flex-col items-center gap-6 sm:gap-8 pt-12 sm:pt-16 lg:pt-25 px-4 sm:px-8 lg:px-0">
            <div className="flex items-center gap-2 px-4 py-2 bg-muted border border-border rounded-full">
              <span className="w-1.5 h-1.5 rounded-full bg-green-500" />
              <span className="font-mono text-[11px] font-medium uppercase tracking-[0.5px] text-muted-foreground">
                introducing the marketplace with revenue sharing
              </span>
            </div>
            <h1 className="font-heading text-[28px] sm:text-[40px] lg:text-[56px] font-bold text-foreground text-center leading-[1.15] -tracking-[0.5px] sm:-tracking-[1px]">
              Run production-grade <br className="hidden sm:block" /> agents at
              scale
            </h1>
            <p className="text-base sm:text-lg lg:text-xl text-muted-foreground text-center leading-relaxed max-w-160">
              The complete platform for building, running and monitoring real world agents
              with memory, observability and access control.
            </p>
            <div className="flex flex-col sm:flex-row gap-3 sm:gap-3.5 pt-2 w-full sm:w-auto">
              <Button size="default" className="sm:hidden">
                Join the waitlist
              </Button>
              <Button variant="outline" size="default" className="sm:hidden">
                View Docs
              </Button>
              <Button size="lg" className="hidden sm:inline-flex">
                Join the waitlist
              </Button>
              <Button variant="outline" size="lg" className="hidden sm:inline-flex">
                View Docs
              </Button>
            </div>
          </div>

          <div className="px-4 lg:px-0">
            <div className="relative z-10 w-full max-w-5xl bg-black dark:bg-card min-h-60 sm:min-h-80 lg:min-h-180 mt-8 sm:mt-12 lg:mt-16 lg:mx-auto border border-border rounded-4xl shadow-[0_0_60px_-20px_color-mix(in_oklch,var(--primary)_12%,transparent),0_0_20px_-10px_color-mix(in_oklch,var(--primary)_8%,transparent)] flex items-center justify-center">
              <div className="relative flex items-center justify-center">
                {/* Pulse rings */}
                <span className="absolute w-12 h-12 lg:w-32 lg:h-32 rounded-full border border-foreground/20 lg:border-2 animate-[ping_2.5s_ease-out_infinite]" />
                <span className="absolute w-12 h-12 lg:w-32 lg:h-32 rounded-full border border-foreground/12 lg:border-2 animate-[ping_2.5s_ease-out_0.8s_infinite]" />
                <span className="absolute w-12 h-12 lg:w-32 lg:h-32 rounded-full bg-foreground/5 animate-[ping_2.5s_ease-out_0.4s_infinite]" />
                <svg
                  className="relative w-8 h-8 lg:w-20 lg:h-20 text-muted-foreground"
                  viewBox="0 0 24 24"
                  fill="none"
                  xmlns="http://www.w3.org/2000/svg"
                >
                  <title>play</title>
                  <path
                    fillRule="evenodd"
                    clipRule="evenodd"
                    d="M7.23832 3.04445C5.65196 2.1818 3.75 3.31957 3.75 5.03299L3.75 18.9672C3.75 20.6806 5.65196 21.8184 7.23832 20.9557L20.0503 13.9886C21.6499 13.1188 21.6499 10.8814 20.0503 10.0116L7.23832 3.04445ZM2.25 5.03299C2.25 2.12798 5.41674 0.346438 7.95491 1.72669L20.7669 8.6938C23.411 10.1317 23.411 13.8685 20.7669 15.3064L7.95491 22.2735C5.41674 23.6537 2.25 21.8722 2.25 18.9672L2.25 5.03299Z"
                    fill="currentColor"
                  />
                </svg>
              </div>
            </div>
          </div>

          {/* Trusted by */}
          <div className="relative z-10 flex flex-col items-center gap-8 py-20 sm:py-28 px-4">
            <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
              Trusted by companies of all sizes
            </p>
            <div className="flex flex-wrap items-center justify-center gap-x-12 gap-y-6 sm:gap-x-16 opacity-40">
              <span className="text-lg sm:text-xl font-semibold text-muted-foreground tracking-tight">
                Vercel
              </span>
              <span className="text-lg sm:text-xl font-semibold text-muted-foreground tracking-tight">
                Linear
              </span>
              <span className="text-lg sm:text-xl font-semibold text-muted-foreground tracking-tight">
                Stripe
              </span>
              <span className="text-lg sm:text-xl font-semibold text-muted-foreground tracking-tight">
                Notion
              </span>
              <span className="text-lg sm:text-xl font-semibold text-muted-foreground tracking-tight">
                Raycast
              </span>
              <span className="text-lg sm:text-xl font-semibold text-muted-foreground tracking-tight">
                Supabase
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
