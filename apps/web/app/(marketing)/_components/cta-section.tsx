import Link from "next/link"
import { Button } from "@/components/ui/button"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowRight01Icon, CpuIcon } from "@hugeicons/core-free-icons"

export function CtaSection() {
  return (
    <section className="w-full px-4 sm:px-6 lg:px-0">
      <div className="w-full max-w-424 mx-auto relative pb-20 sm:pb-28 lg:pb-36">
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
              "radial-gradient(ellipse 50% 60% at 50% 40%, color-mix(in oklch, var(--primary) 12%, transparent) 0%, transparent 70%)",
          }}
        />

        <div className="relative flex flex-col items-center gap-6 sm:gap-7 pt-20 sm:pt-28 text-center">
          <HugeiconsIcon
            icon={CpuIcon}
            size={22}
            className="text-primary"
          />
          <p className="font-mono text-[11px] font-medium uppercase tracking-[2px] text-primary">
            Pricing · Credits-based
          </p>
          <h2 className="font-heading text-[36px] sm:text-[52px] lg:text-[64px] font-bold text-foreground leading-[1.03] -tracking-[1px] sm:-tracking-[1.4px] max-w-3xl">
            Start with{" "}
            <span className="italic font-medium text-primary">
              150 free credits.
            </span>
          </h2>
          <p className="text-base sm:text-lg text-muted-foreground leading-relaxed max-w-xl">
            Enough to build your first agent, watch it run, and decide
            whether this is the platform for you — before you spend a dollar.
          </p>
          <div className="flex flex-col items-center gap-3">
            <Link href="/auth">
              <Button size="lg" className="group cursor-pointer">
                Get started free
                <HugeiconsIcon
                  icon={ArrowRight01Icon}
                  size={15}
                  className="ml-1.5 opacity-80 group-hover:translate-x-0.5 transition-transform"
                />
              </Button>
            </Link>
            <p className="font-mono text-[10px] uppercase tracking-[1.8px] text-muted-foreground/70">
              No card required · Free trial expires after 30 days
            </p>
          </div>
          <Link
            href="/pricing"
            className="font-mono text-[11px] uppercase tracking-[1.5px] text-muted-foreground hover:text-foreground transition-colors mt-4"
          >
            See full pricing →
          </Link>
        </div>
      </div>
    </section>
  )
}
