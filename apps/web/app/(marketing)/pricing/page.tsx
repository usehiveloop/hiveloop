"use client"

import { useMemo, useState } from "react"
import Link from "next/link"
import { motion } from "motion/react"
import { Button } from "@/components/ui/button"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Tick02Icon,
  ArrowRight01Icon,
  KeyframeIcon,
  CpuIcon,
  PlusSignIcon,
} from "@hugeicons/core-free-icons"

// ──────────────────────────────────────────────────────────────────────────
// DATA
// Prices are authored in USD. NGN is derived at a hardcoded rate.
// Credits are derived from plan $ × (1 − 0.75) / $0.00025.
// ──────────────────────────────────────────────────────────────────────────

const NGN_PER_USD = 1500

type Currency = "USD" | "NGN"
type Cycle = "monthly" | "annual"

type Plan = {
  name: string
  tagline: string
  monthlyUSD: number
  annualUSD: number // total billed upfront for the year (20% off 12×monthly)
  credits: number
  typicalRuns: number
  heavyRuns: number
  featured?: boolean
  features: string[]
}

const PLANS: Plan[] = [
  {
    name: "Starter",
    tagline: "For solo builders.",
    monthlyUSD: 9,
    annualUSD: 86,
    credits: 9_000,
    typicalRuns: 64,
    heavyRuns: 13,
    features: [
      "One team member",
      "RAG knowledge base",
      "5 concurrent agent sessions",
      "Unlimited connections",
      "Top up credits anytime",
      "Default sandbox size",
      "Community support",
    ],
  },
  {
    name: "Pro",
    tagline: "For production teams.",
    monthlyUSD: 39,
    annualUSD: 374,
    credits: 39_000,
    typicalRuns: 280,
    heavyRuns: 58,
    featured: true,
    features: [
      "Everything in Starter",
      "15 concurrent agent sessions",
      "Unlimited agents",
      "Unlimited agent triggers",
      "Priority email support",
      "Bring your own keys",
      "Large sandboxes",
    ],
  },
  {
    name: "Business",
    tagline: "For fleets of agents.",
    monthlyUSD: 99,
    annualUSD: 950,
    credits: 99_000,
    typicalRuns: 712,
    heavyRuns: 149,
    features: [
      "Everything in Pro",
      "Extra large sandboxes",
      "50 concurrent agent sessions",
      "Unlimited connections",
    ],
  },
]

const BUNDLES = [
  { priceUSD: 5, credits: 4_000, note: "25% premium" },
  { priceUSD: 20, credits: 18_000, note: "11% premium" },
  { priceUSD: 50, credits: 48_000, note: "4% premium" },
]

const FAQ: { q: string; a: string }[] = [
  {
    q: "What exactly is a credit?",
    a: "Credits are how we meter inference and sandbox time. One credit = $0.001 of underlying cost. A typical agent conversation on our default model consumes around 140 credits; a heavier, long-context analysis can run 600+.",
  },
  {
    q: "What happens when I run out of credits?",
    a: "New conversations can't start and the running one gracefully aborts on its next LLM call. You can either top up with a non-expiring credit bundle or upgrade to a plan with a higher allowance. We email you at 80% consumption so it's never a surprise.",
  },
  {
    q: "Can I use my own API keys?",
    a: "Yes — BYOK is available on every plan, toggleable per agent or per conversation. When BYOK is on, credits aren't deducted for inference. You still pay credits for sandbox time, which keeps our infrastructure cost covered.",
  },
  {
    q: "Do unused credits roll over?",
    a: "Plan credits reset at the start of each billing period. Top-up bundle credits never expire and stack on top of your plan balance. Bundles spend after plan credits, so you never lose bundle credits to a reset.",
  },
  {
    q: "Can I change plans anytime?",
    a: "Yes. Upgrading is immediate and pro-rated; downgrading takes effect at the end of your current billing period. Annual plans can switch tier; the discount carries to the new tier.",
  },
  {
    q: "What happens if I cancel?",
    a: "Your plan stays active through the end of the current period, then your account reverts to a read-only state. Bundle credits are preserved — you can resubscribe and keep using them.",
  },
]

// ──────────────────────────────────────────────────────────────────────────
// HELPERS
// ──────────────────────────────────────────────────────────────────────────

function formatCurrency(usd: number, currency: Currency, options: { compact?: boolean; decimals?: number } = {}) {
  const amount = currency === "USD" ? usd : usd * NGN_PER_USD
  const symbol = currency === "USD" ? "$" : "₦"
  const decimals = options.decimals ?? 0
  const rounded =
    decimals > 0
      ? amount.toLocaleString("en-US", { minimumFractionDigits: decimals, maximumFractionDigits: decimals })
      : Math.round(amount).toLocaleString("en-US")
  return `${symbol}${rounded}`
}

// ──────────────────────────────────────────────────────────────────────────
// PAGE
// ──────────────────────────────────────────────────────────────────────────

export default function PricingPage() {
  const [currency, setCurrency] = useState<Currency>("USD")
  const [cycle, setCycle] = useState<Cycle>("monthly")

  return (
    <main className="w-full bg-background text-foreground relative overflow-hidden">
      {/* Ambient wash — single warm primary glow anchoring the hero. */}
      <div
        aria-hidden
        className="pointer-events-none absolute inset-x-0 top-0 h-[720px] opacity-60"
        style={{
          background:
            "radial-gradient(ellipse 80% 50% at 50% 0%, color-mix(in oklch, var(--primary) 10%, transparent) 0%, transparent 70%)",
        }}
      />
      {/* Tick grid — subtle ledger motif behind the page. */}
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 opacity-[0.015] dark:opacity-[0.025]"
        style={{
          backgroundImage: "linear-gradient(to right, currentColor 1px, transparent 1px)",
          backgroundSize: "48px 100%",
        }}
      />

      <Hero />

      <Toolbar currency={currency} setCurrency={setCurrency} cycle={cycle} setCycle={setCycle} />

      <section className="relative max-w-6xl mx-auto w-full px-4 pb-20">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 lg:gap-6">
          {PLANS.map((plan, i) => (
            <PlanCard key={plan.name} plan={plan} currency={currency} cycle={cycle} index={i} />
          ))}
        </div>

        <TrialNote />
      </section>

      <BundlesSection currency={currency} />

      <ByokSection />

      <FaqSection />

      <FinalCta />
    </main>
  )
}

// ──────────────────────────────────────────────────────────────────────────
// HERO
// ──────────────────────────────────────────────────────────────────────────

function Hero() {
  return (
    <section className="relative pt-20 sm:pt-28 pb-10 px-4">
      <div className="max-w-4xl mx-auto flex flex-col items-center gap-6 text-center">
        <motion.p
          initial={{ opacity: 0, y: -8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, ease: [0.2, 0, 0, 1] }}
          className="font-mono text-[11px] font-medium uppercase tracking-[2.5px] text-primary"
        >
          Pricing · Credits-based
        </motion.p>
        <motion.h1
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.7, ease: [0.2, 0, 0, 1], delay: 0.08 }}
          className="font-heading text-[40px] sm:text-[60px] lg:text-[76px] font-bold leading-[1.02] -tracking-[1.5px]"
        >
          Pay for what your
          <br />
          <span className="italic font-medium text-primary">agents think.</span>
        </motion.h1>
        <motion.p
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6, ease: [0.2, 0, 0, 1], delay: 0.2 }}
          className="text-base sm:text-lg text-muted-foreground max-w-xl leading-relaxed"
        >
          Inference credits, unlimited runs. Platform keys by default — bring your own when you want to stretch your plan further.
        </motion.p>
      </div>
    </section>
  )
}

// ──────────────────────────────────────────────────────────────────────────
// TOOLBAR — currency + cycle toggles
// ──────────────────────────────────────────────────────────────────────────

function Toolbar({
  currency,
  setCurrency,
  cycle,
  setCycle,
}: {
  currency: Currency
  setCurrency: (c: Currency) => void
  cycle: Cycle
  setCycle: (c: Cycle) => void
}) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.5, ease: [0.2, 0, 0, 1], delay: 0.3 }}
      className="relative max-w-6xl mx-auto w-full px-4"
    >
      <div className="flex flex-col sm:flex-row items-center justify-center gap-4 sm:gap-8 py-6 mb-10 sm:mb-14 border-y border-border/60">
        <Segmented
          label="Currency"
          value={currency}
          onChange={(v) => setCurrency(v as Currency)}
          options={[
            { value: "USD", label: "USD" },
            { value: "NGN", label: "NGN" },
          ]}
        />
        <div className="hidden sm:block w-px h-6 bg-border/60" />
        <Segmented
          label="Billing"
          value={cycle}
          onChange={(v) => setCycle(v as Cycle)}
          options={[
            { value: "monthly", label: "Monthly" },
            { value: "annual", label: "Annual", trailing: "−20%" },
          ]}
        />
      </div>
    </motion.div>
  )
}

function Segmented({
  label,
  value,
  onChange,
  options,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string; trailing?: string }[]
}) {
  return (
    <div className="flex items-center gap-3">
      <span className="font-mono text-[10px] font-medium uppercase tracking-[1.8px] text-muted-foreground hidden md:inline">
        {label}
      </span>
      <div className="relative inline-flex p-1 bg-muted/60 rounded-full border border-border/80">
        {options.map((opt) => {
          const isActive = value === opt.value
          return (
            <button
              key={opt.value}
              onClick={() => onChange(opt.value)}
              className="relative z-10 px-4 py-1.5 rounded-full text-xs font-medium transition-colors flex items-center gap-1.5 cursor-pointer"
              style={{
                color: isActive ? "var(--foreground)" : "var(--muted-foreground)",
              }}
            >
              {isActive && (
                <motion.span
                  layoutId={`seg-${label}`}
                  transition={{ type: "spring", bounce: 0.2, duration: 0.45 }}
                  className="absolute inset-0 bg-background border border-border rounded-full -z-10 shadow-[0_1px_2px_rgba(0,0,0,0.04)]"
                  aria-hidden
                />
              )}
              <span>{opt.label}</span>
              {opt.trailing && (
                <span
                  className={`font-mono text-[10px] tracking-wide ${
                    isActive ? "text-primary" : "text-muted-foreground/60"
                  }`}
                >
                  {opt.trailing}
                </span>
              )}
            </button>
          )
        })}
      </div>
    </div>
  )
}

// ──────────────────────────────────────────────────────────────────────────
// PLAN CARD — credits treated as the hero currency
// ──────────────────────────────────────────────────────────────────────────

function PlanCard({
  plan,
  currency,
  cycle,
  index,
}: {
  plan: Plan
  currency: Currency
  cycle: Cycle
  index: number
}) {
  const perMonthUSD = useMemo(
    () => (cycle === "annual" ? plan.annualUSD / 12 : plan.monthlyUSD),
    [cycle, plan],
  )
  const priceDisplay = useMemo(
    () => formatCurrency(perMonthUSD, currency, { decimals: cycle === "annual" ? 2 : 0 }),
    [perMonthUSD, currency, cycle],
  )
  const billedDisplay = useMemo(() => {
    if (cycle === "monthly") return "Billed monthly. Cancel anytime."
    return `${formatCurrency(plan.annualUSD, currency)} billed once per year`
  }, [cycle, plan, currency])

  return (
    <motion.div
      initial={{ opacity: 0, y: 24 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.55, ease: [0.2, 0, 0, 1], delay: 0.35 + index * 0.09 }}
      className={`
        relative flex flex-col rounded-2xl p-7 gap-6
        ${plan.featured
          ? "border border-primary/50 bg-card shadow-[0_30px_60px_-30px_color-mix(in_oklch,var(--primary)_40%,transparent)]"
          : "border border-border/70 bg-card/70"
        }
      `}
    >
      {plan.featured && (
        <>
          <div
            aria-hidden
            className="absolute inset-0 rounded-2xl pointer-events-none opacity-70"
            style={{
              background:
                "radial-gradient(ellipse 90% 50% at 50% 0%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
            }}
          />
          <div className="absolute -top-2.5 left-1/2 -translate-x-1/2 px-3 py-1 rounded-full bg-primary text-primary-foreground text-[10px] font-mono font-medium uppercase tracking-[1.8px]">
            Most popular
          </div>
        </>
      )}

      <div className="relative flex flex-col gap-1.5">
        <p
          className={`font-mono text-[11px] font-medium uppercase tracking-[2px] ${
            plan.featured ? "text-primary" : "text-muted-foreground"
          }`}
        >
          {plan.name}
        </p>
        <p className="text-sm text-muted-foreground leading-snug whitespace-nowrap">{plan.tagline}</p>
      </div>

      {/* PRICE */}
      <div className="relative flex flex-col gap-1.5">
        <div className="flex items-baseline gap-2 flex-wrap">
          <motion.span
            key={`${currency}-${cycle}-${plan.name}`}
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.3 }}
            className="font-heading font-bold text-[52px] leading-none -tracking-[1.5px] text-foreground"
          >
            {priceDisplay}
          </motion.span>
          <span className="text-sm text-muted-foreground">/month</span>
        </div>
        <p className="font-mono text-[11px] text-muted-foreground tracking-wide">{billedDisplay}</p>
      </div>

      {/* CREDITS — the hero number */}
      <CreditsPanel plan={plan} />

      {/* CTA */}
      <Link href="/auth" className="block">
        <Button
          size="lg"
          variant={plan.featured ? "default" : "outline"}
          className="w-full group cursor-pointer"
        >
          Start with {plan.name}
          <HugeiconsIcon
            icon={ArrowRight01Icon}
            size={15}
            className="ml-1.5 opacity-80 group-hover:translate-x-0.5 transition-transform"
          />
        </Button>
      </Link>

      {/* FEATURES */}
      <ul className="relative flex flex-col gap-2.5 pt-1">
        {plan.features.map((feature) => (
          <li key={feature} className="flex items-start gap-2.5 text-sm text-foreground leading-snug">
            <HugeiconsIcon
              icon={Tick02Icon}
              size={14}
              className={`mt-1 shrink-0 ${plan.featured ? "text-primary" : "text-foreground/60"}`}
            />
            <span>{feature}</span>
          </li>
        ))}
      </ul>
    </motion.div>
  )
}

function CreditsPanel({ plan }: { plan: Plan }) {
  return (
    <div
      className={`
        relative -mx-7 px-7 py-5 border-y flex flex-col gap-1.5
        ${plan.featured ? "border-primary/15" : "border-border/60"}
      `}
    >
      <span className="font-mono text-[10px] font-medium uppercase tracking-[1.8px] text-muted-foreground">
        Monthly credits
      </span>
      <div className="flex items-baseline gap-2">
        <span className="font-mono font-medium text-[36px] leading-none text-foreground tabular-nums">
          {plan.credits.toLocaleString("en-US")}
        </span>
      </div>
      <p className="font-mono text-[11px] text-muted-foreground tracking-wide">
        ≈ <span className="text-foreground">{plan.typicalRuns}</span> typical runs
        {" · "}
        <span className="text-foreground">{plan.heavyRuns}</span> heavy runs
      </p>
    </div>
  )
}

// ──────────────────────────────────────────────────────────────────────────
// TRIAL NOTE
// ──────────────────────────────────────────────────────────────────────────

function TrialNote() {
  return (
    <motion.div
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ duration: 0.5, delay: 0.75 }}
      className="mt-10 flex flex-col items-center gap-2 text-center"
    >
      <p className="font-mono text-[11px] uppercase tracking-[2px] text-primary">Every new account</p>
      <p className="text-sm sm:text-base text-muted-foreground max-w-md">
        Get{" "}
        <span className="font-mono font-medium text-foreground">1,000 credits</span>{" "}
        free on sign-up. No card required.
      </p>
    </motion.div>
  )
}

// ──────────────────────────────────────────────────────────────────────────
// BUNDLES
// ──────────────────────────────────────────────────────────────────────────

function BundlesSection({ currency }: { currency: Currency }) {
  return (
    <section className="relative border-t border-border/60">
      <div className="max-w-6xl mx-auto w-full px-4 py-20 sm:py-24 grid md:grid-cols-[1fr_auto] gap-12 md:gap-20 items-start">
        <div className="flex flex-col gap-4 max-w-xl">
          <p className="font-mono text-[11px] font-medium uppercase tracking-[2px] text-primary">
            Top-ups
          </p>
          <h2 className="font-heading text-[32px] sm:text-[44px] font-bold leading-[1.05] -tracking-[1px]">
            Run out mid-month? Keep going.
          </h2>
          <p className="text-muted-foreground text-base leading-relaxed">
            Top-up bundles never expire and stack on top of your plan balance. Buy any time,
            from the dashboard. Cheaper per credit the more you buy.
          </p>
        </div>

        <div className="w-full md:w-auto md:min-w-[420px] flex flex-col gap-2">
          {BUNDLES.map((bundle, i) => (
            <motion.div
              key={bundle.priceUSD}
              initial={{ opacity: 0, x: 12 }}
              whileInView={{ opacity: 1, x: 0 }}
              viewport={{ once: true, amount: 0.4 }}
              transition={{ duration: 0.5, ease: [0.2, 0, 0, 1], delay: i * 0.08 }}
              className="group relative flex items-center justify-between gap-4 px-5 py-4 rounded-xl border border-border/70 bg-card/70 hover:border-primary/50 hover:bg-card transition-colors"
            >
              <div className="flex items-baseline gap-3">
                <span className="font-heading text-[28px] font-bold text-foreground leading-none -tracking-[0.5px]">
                  {formatCurrency(bundle.priceUSD, currency)}
                </span>
                <HugeiconsIcon icon={PlusSignIcon} size={12} className="text-muted-foreground/40" />
                <span className="font-mono text-[15px] text-foreground tabular-nums">
                  {bundle.credits.toLocaleString()} <span className="text-muted-foreground">credits</span>
                </span>
              </div>
              <span className="font-mono text-[10px] uppercase tracking-[1.5px] text-muted-foreground/80">
                {bundle.note}
              </span>
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  )
}

// ──────────────────────────────────────────────────────────────────────────
// BYOK EXPLAINER
// ──────────────────────────────────────────────────────────────────────────

function ByokSection() {
  return (
    <section className="relative border-t border-border/60 bg-muted/30">
      <div className="max-w-6xl mx-auto w-full px-4 py-20 sm:py-24">
        <div className="grid md:grid-cols-[auto_1fr] gap-10 md:gap-16 items-start">
          <div className="flex md:flex-col items-start gap-4">
            <div className="w-14 h-14 rounded-xl border border-primary/40 bg-primary/5 flex items-center justify-center">
              <HugeiconsIcon icon={KeyframeIcon} size={22} className="text-primary" />
            </div>
            <p className="font-mono text-[11px] font-medium uppercase tracking-[2px] text-primary">
              BYOK
            </p>
          </div>

          <div className="flex flex-col gap-6 max-w-3xl">
            <h2 className="font-heading text-[32px] sm:text-[44px] font-bold leading-[1.05] -tracking-[1px]">
              Your own keys.{" "}
              <span className="italic font-medium text-primary">Up to 8× more runs</span>{" "}
              on the same plan.
            </h2>
            <p className="text-muted-foreground text-base leading-relaxed">
              Toggle BYOK per agent or per conversation. When it&apos;s on, we don&apos;t charge credits
              for inference — only for sandbox time. Every plan supports BYOK; nothing extra to unlock.
            </p>

            <div className="grid sm:grid-cols-3 gap-4 pt-3">
              <ByokRow
                label="Platform keys"
                runs="~280"
                subtext="typical runs on Pro"
              />
              <ByokRow
                label="BYOK · default sandbox"
                runs="~2,400"
                subtext="typical runs on Pro"
                highlight
              />
              <ByokRow
                label="BYOK · XL sandbox"
                runs="~600"
                subtext="typical runs on Pro"
              />
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}

function ByokRow({
  label,
  runs,
  subtext,
  highlight,
}: {
  label: string
  runs: string
  subtext: string
  highlight?: boolean
}) {
  return (
    <div
      className={`
        flex flex-col gap-1.5 p-4 rounded-lg border
        ${highlight ? "border-primary/40 bg-primary/[0.03]" : "border-border/60 bg-background/70"}
      `}
    >
      <span className="font-mono text-[10px] uppercase tracking-[1.6px] text-muted-foreground">
        {label}
      </span>
      <span
        className={`font-mono font-medium text-2xl leading-tight tabular-nums ${
          highlight ? "text-primary" : "text-foreground"
        }`}
      >
        {runs}
      </span>
      <span className="text-xs text-muted-foreground">{subtext}</span>
    </div>
  )
}

// ──────────────────────────────────────────────────────────────────────────
// FAQ
// ──────────────────────────────────────────────────────────────────────────

function FaqSection() {
  return (
    <section className="relative border-t border-border/60">
      <div className="max-w-4xl mx-auto w-full px-4 py-20 sm:py-24">
        <div className="flex flex-col gap-2 mb-10">
          <p className="font-mono text-[11px] font-medium uppercase tracking-[2px] text-primary">
            Questions
          </p>
          <h2 className="font-heading text-[32px] sm:text-[44px] font-bold leading-[1.05] -tracking-[1px]">
            Everything you&apos;ll ask in the first five minutes.
          </h2>
        </div>

        <div className="flex flex-col divide-y divide-border/70 border-y border-border/70">
          {FAQ.map((item, i) => (
            <FaqItem key={i} q={item.q} a={item.a} />
          ))}
        </div>
      </div>
    </section>
  )
}

function FaqItem({ q, a }: { q: string; a: string }) {
  return (
    <details className="group py-5">
      <summary className="flex items-center justify-between gap-4 cursor-pointer list-none">
        <span className="font-heading text-base sm:text-lg font-medium text-foreground -tracking-[0.3px]">
          {q}
        </span>
        <span
          className="w-6 h-6 rounded-full border border-border/80 flex items-center justify-center text-muted-foreground group-open:border-primary group-open:text-primary group-open:rotate-45 transition-all"
          aria-hidden
        >
          <HugeiconsIcon icon={PlusSignIcon} size={12} />
        </span>
      </summary>
      <p className="mt-3 text-sm sm:text-[15px] text-muted-foreground leading-relaxed max-w-3xl pr-10">
        {a}
      </p>
    </details>
  )
}

// ──────────────────────────────────────────────────────────────────────────
// FINAL CTA
// ──────────────────────────────────────────────────────────────────────────

function FinalCta() {
  return (
    <section className="relative border-t border-border/60">
      <div
        aria-hidden
        className="pointer-events-none absolute inset-x-0 bottom-0 h-[400px] opacity-60"
        style={{
          background:
            "radial-gradient(ellipse 50% 80% at 50% 100%, color-mix(in oklch, var(--primary) 10%, transparent) 0%, transparent 70%)",
        }}
      />
      <div className="relative max-w-4xl mx-auto w-full px-4 py-24 sm:py-32 flex flex-col items-center gap-6 text-center">
        <HugeiconsIcon icon={CpuIcon} size={22} className="text-primary" />
        <h2 className="font-heading text-[40px] sm:text-[56px] font-bold leading-[1.03] -tracking-[1.2px]">
          Start with{" "}
          <span className="italic font-medium text-primary">1,000 free credits.</span>
        </h2>
        <p className="text-muted-foreground max-w-lg leading-relaxed">
          Enough to build your first agent, watch it run, and decide whether this is the
          platform for you — before you spend a dollar.
        </p>
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
    </section>
  )
}
