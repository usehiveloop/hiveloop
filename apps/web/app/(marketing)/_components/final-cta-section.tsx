import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

export function FinalCtaSection() {
  return (
    <section className="w-full px-4 sm:px-6 lg:px-0">
      <div className="w-full max-w-424 mx-auto relative pb-20 sm:pb-28 lg:pb-36">
        <div className="relative flex flex-col items-center gap-8 rounded-4xl border border-border ring-1 ring-foreground/5 p-10 sm:p-16 lg:p-20 text-center overflow-hidden max-w-5xl mx-auto shadow-[0_0_60px_-20px_color-mix(in_oklch,var(--primary)_12%,transparent),0_0_20px_-10px_color-mix(in_oklch,var(--primary)_8%,transparent)]">
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background:
                "radial-gradient(circle at 50% 100%, color-mix(in oklch, var(--primary) 10%, transparent) 0%, transparent 60%)",
            }}
          />
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              backgroundImage:
                "linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)",
              backgroundSize: "40px 40px",
              maskImage:
                "radial-gradient(ellipse at center bottom, black 20%, transparent 70%)",
            }}
          />
          <h2 className="relative font-heading text-[24px] sm:text-[32px] lg:text-[44px] font-bold text-foreground leading-[1.15] -tracking-[0.5px] sm:-tracking-[1px]">
            Stop subscribing.{" "}
            <br className="hidden sm:block" />
            Start building.
          </h2>
          <p className="relative text-base sm:text-lg text-muted-foreground leading-relaxed max-w-lg">
            Join the waitlist and be the first to run your own agents on
            Hiveloop.
          </p>
          <div className="relative flex flex-col sm:flex-row gap-2.5 w-full sm:w-auto">
            <Input
              type="email"
              placeholder="Enter your email"
              className="h-10 sm:h-12 sm:w-72 rounded-full text-sm sm:text-base px-5"
            />
            <Button size="default" className="sm:hidden rounded-full h-10">
              Join the waitlist
            </Button>
            <Button size="lg" className="hidden sm:inline-flex rounded-full h-12">
              Join the waitlist
            </Button>
          </div>
        </div>
      </div>
    </section>
  )
}
