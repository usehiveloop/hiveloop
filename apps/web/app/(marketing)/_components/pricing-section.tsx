import Link from "next/link"
import { Button } from "@/components/ui/button"

const freeFeatures = [
  "1 agent",
  "100 runs/month",
  "1 concurrent run",
  "Shared sandbox only",
  "20+ LLM providers",
  "Unlimited AI credentials",
  "MCP tool support",
  "Community support",
]

const proFeatures = [
  "Unlimited agents",
  "300 runs/agent/month included",
  "5 concurrent runs per agent",
  "Dedicated sandbox (+$2/agent/mo)",
  "Agent Forge (auto-optimization)",
  "Persistent agent memory (1 GB/agent)",
  "Advanced analytics & audit logs",
  "Priority support",
]

function CheckIcon() {
  return (
    <svg
      className="w-4 h-4 shrink-0 text-green-600 dark:text-green-400"
      viewBox="0 0 24 24"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M20 6L9 17l-5-5"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

export function PricingSection() {
  return (
    <section className="w-full px-4 sm:px-6 lg:px-0">
      <div className="w-full max-w-424 mx-auto relative">
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            backgroundImage:
              "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
            backgroundSize: "40px 40px",
            maskImage:
              "radial-gradient(ellipse at center, black, transparent 70%)",
          }}
        />
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            background:
              "radial-gradient(circle at 50% 40%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
          }}
        />

        <div className="relative flex flex-col items-center gap-10 sm:gap-14 pb-20 sm:pb-28 lg:pb-36">
          {/* Section header */}
          <div className="flex flex-col items-center gap-5 sm:gap-6 max-w-3xl text-center px-4">
            <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">
              Pricing
            </p>
            <h2 className="font-heading text-[24px] sm:text-[32px] lg:text-[44px] font-bold text-foreground leading-[1.15] -tracking-[0.5px] sm:-tracking-[1px]">
              Simple, transparent pricing.
            </h2>
            <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-2xl">
              Start free with one agent. Scale to production at $4.99/month
              per agent.
            </p>
          </div>

          {/* Plan cards */}
          <div className="w-full max-w-4xl mx-auto grid grid-cols-1 md:grid-cols-2 gap-6 px-4 lg:px-0">
            {/* Free */}
            <div className="flex flex-col rounded-2xl border border-border bg-background p-8 gap-6">
              <div className="flex flex-col gap-4">
                <span className="font-mono text-[11px] font-medium uppercase tracking-[1px] text-muted-foreground">
                  Free forever
                </span>
                <div className="flex items-baseline gap-1">
                  <span className="font-heading text-[48px] font-bold text-foreground leading-none">
                    $0
                  </span>
                </div>
                <p className="text-sm text-muted-foreground">
                  For exploring and prototyping
                </p>
              </div>
              <div className="flex flex-col gap-2.5">
                {freeFeatures.map((item) => (
                  <div
                    key={item}
                    className="flex items-center gap-2.5 text-sm"
                  >
                    <CheckIcon />
                    {item}
                  </div>
                ))}
              </div>
              <Link href="/auth" className="mt-auto">
                <Button variant="outline" size="lg" className="w-full">
                  Get started
                </Button>
              </Link>
            </div>

            {/* Pro */}
            <div className="flex flex-col rounded-2xl border-2 border-primary/30 bg-background p-8 gap-6 relative overflow-hidden">
              <div
                className="absolute inset-0 pointer-events-none"
                style={{
                  background:
                    "radial-gradient(circle at 50% 0%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
                }}
              />
              <div className="flex flex-col gap-4 relative">
                <span className="font-mono text-[11px] font-medium uppercase tracking-[1px] text-primary">
                  Pro
                </span>
                <div className="flex items-baseline gap-1">
                  <span className="font-heading text-[48px] font-bold text-foreground leading-none">
                    $4.99
                  </span>
                  <span className="text-sm text-muted-foreground">
                    /month per agent
                  </span>
                </div>
                <p className="text-sm text-muted-foreground">
                  For teams shipping agents to production
                </p>
              </div>
              <div className="flex flex-col gap-2.5 relative">
                <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground mb-1">
                  Everything in Free, plus
                </span>
                {proFeatures.map((item) => (
                  <div
                    key={item}
                    className="flex items-center gap-2.5 text-sm"
                  >
                    <CheckIcon />
                    {item}
                  </div>
                ))}
              </div>
              <Link href="/auth" className="mt-auto relative">
                <Button size="lg" className="w-full">
                  Start building
                </Button>
              </Link>
            </div>
          </div>

          <Link href="/pricing">
            <Button variant="link" className="text-muted-foreground">
              Compare all features →
            </Button>
          </Link>
        </div>
      </div>
    </section>
  )
}
