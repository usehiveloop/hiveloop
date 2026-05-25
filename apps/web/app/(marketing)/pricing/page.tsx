"use client"

import { useMemo, useState } from "react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Slider } from "@/components/ui/slider"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import {
  NavigationMenu,
  NavigationMenuContent,
  NavigationMenuItem,
  NavigationMenuLink,
  NavigationMenuList,
  NavigationMenuTrigger,
} from "@/components/ui/navigation-menu"
import { GithubIcon } from "@/components/icons"
import { MarketingFooter } from "../_components/footer"

type Plan = components["schemas"]["planDTO"]

/* ─────────────────────────── Navbar ─────────────────────────── */

function Navbar() {
  const pathname = usePathname()
  const isActive = (href: string) => pathname === href

  return (
    <NavigationMenu viewport={false} className="hidden items-center md:flex">
      <nav className="flex h-11 items-center rounded-full border border-[var(--nav-border)] bg-[var(--nav-bg)] px-2 backdrop-blur-lg">
        <NavigationMenuList>
          <NavigationMenuItem>
            <NavigationMenuLink
              asChild
              className={isActive("/") ? "bg-black/[0.03] text-foreground dark:bg-white/[0.05]" : undefined}
            >
              <Link href="/">Product</Link>
            </NavigationMenuLink>
          </NavigationMenuItem>
          <NavigationMenuItem>
            <NavigationMenuTrigger>Resources</NavigationMenuTrigger>
            <NavigationMenuContent>
              <ul className="grid w-44 gap-0.5 p-1.5">
                <li>
                  <NavigationMenuLink href="#">Blog</NavigationMenuLink>
                </li>
                <li>
                  <NavigationMenuLink href="#">Changelog</NavigationMenuLink>
                </li>
                <li>
                  <NavigationMenuLink href="#">Docs</NavigationMenuLink>
                </li>
              </ul>
            </NavigationMenuContent>
          </NavigationMenuItem>
          <NavigationMenuItem>
            <NavigationMenuTrigger>Solutions</NavigationMenuTrigger>
            <NavigationMenuContent>
              <ul className="grid w-44 gap-0.5 p-1.5">
                <li>
                  <NavigationMenuLink href="#">Use cases</NavigationMenuLink>
                </li>
                <li>
                  <NavigationMenuLink href="#">Integrations</NavigationMenuLink>
                </li>
              </ul>
            </NavigationMenuContent>
          </NavigationMenuItem>
          <NavigationMenuItem>
            <NavigationMenuLink
              asChild
              className={isActive("/pricing") ? "bg-black/[0.03] text-foreground dark:bg-white/[0.05]" : undefined}
            >
              <Link href="/pricing">Pricing</Link>
            </NavigationMenuLink>
          </NavigationMenuItem>
          <NavigationMenuItem>
            <NavigationMenuLink
              href="https://github.com/usehivy/hivy"
              target="_blank"
              rel="noopener noreferrer"
              className="ml-1 inline-flex flex-row items-center gap-1.5"
            >
              <GithubIcon size={16} />
              <span>2.4k</span>
            </NavigationMenuLink>
          </NavigationMenuItem>
        </NavigationMenuList>
      </nav>
    </NavigationMenu>
  )
}

/* ─────────────────────────── Check Icon ─────────────────────────── */

function CheckIcon({ className }: { className?: string }) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      className={className}
    >
      <path
        d="M3 8.5L6.5 12L13 5"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

/* ─────────────────────────── Feature Item ─────────────────────────── */

function FeatureItem({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-2.5">
      <div className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
        <CheckIcon className="size-3" />
      </div>
      <span className="text-sm text-muted-foreground">{children}</span>
    </div>
  )
}

/* ─────────────────────────── Pricing Header ─────────────────────────── */

function PricingHeader() {
  return (
    <div className="mx-auto max-w-3xl text-center">
      <h1 className="font-heading text-4xl font-normal leading-[1.1] tracking-[-0.02em] text-foreground md:text-5xl">
        Pricing for AI work that actually runs
      </h1>
      <p className="mx-auto mt-4 max-w-2xl text-base leading-[1.6] text-muted-foreground">
        Start free, then choose the Business credit checkpoint that matches how
        much work your team wants Hivy to handle.
      </p>
    </div>
  )
}

/* ─────────────────────────── Best Features ─────────────────────────── */

function BestFeatures() {
  const features = [
    "Simple monthly credits",
    "Business features included",
    "Hard spend controls",
  ]

  return (
    <div className="mx-auto flex max-w-xl flex-wrap items-center justify-center gap-x-6 gap-y-3">
      {features.map((feature) => (
        <FeatureItem key={feature}>{feature}</FeatureItem>
      ))}
    </div>
  )
}

/* ─────────────────────────── Credit Slider ─────────────────────────── */

type CreditStep = {
  plan: Plan
  credits: number
  priceMinor: number
  label: string
}

function formatNGN(minor: number) {
  return new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    maximumFractionDigits: 0,
  }).format(minor / 100)
}

function formatCreditLabel(credits: number) {
  if (credits >= 1000) {
    const thousands = credits / 1000
    return `${Number.isInteger(thousands) ? thousands : thousands.toFixed(1)}K`
  }
  return credits.toLocaleString("en-NG")
}

function CreditSlider({
  steps,
  value,
  onChange,
}: {
  steps: CreditStep[]
  value: number
  onChange: (value: number) => void
}) {
  return (
    <div className="mx-auto w-full max-w-xl">
      <div className="mb-4 flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">
          Business credits per month
        </span>
        <span className="font-heading text-lg font-medium text-foreground">
          {steps[value].credits.toLocaleString()}
        </span>
      </div>
      <Slider
        min={0}
        max={steps.length - 1}
        step={1}
        value={[value]}
        onValueChange={(v) => {
          const arr = Array.isArray(v) ? v : [v]
          if (typeof arr[0] === "number") onChange(arr[0])
        }}
      />
      <div className="mt-3 flex justify-between">
        {steps.map((step, i) => (
          <button
            key={step.label}
            type="button"
            onClick={() => onChange(i)}
            className={`text-xs font-medium transition-colors ${
              i === value
                ? "text-foreground"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            {step.label}
          </button>
        ))}
      </div>
    </div>
  )
}

/* ─────────────────────────── Free Plan Card ─────────────────────────── */

function FreePlanCard({ plan }: { plan: Plan | undefined }) {
  const credits = plan?.welcome_credits ?? 0
  const features = plan?.features ?? []

  return (
    <div className="flex flex-col rounded-lg border border-border bg-secondary p-6 sm:p-8">
      <div className="mb-6">
        <div className="mb-2 inline-flex rounded-full bg-muted px-3 py-1 text-xs font-medium text-muted-foreground">
          Free
        </div>
        <div className="mt-4 flex items-baseline gap-1">
          <span className="font-heading text-4xl font-medium text-foreground">
            ₦0
          </span>
          <span className="text-sm text-muted-foreground">/month</span>
        </div>
        <p className="mt-2 text-sm text-muted-foreground">
          Try hivy with {credits.toLocaleString()} credits.
        </p>
      </div>

      <Button variant="secondary" size="lg" className="w-full">
        Get started for free
      </Button>

      <div className="my-6 h-px bg-border" />

      <div className="flex flex-col gap-3">
        {features.map((feature) => (
          <FeatureItem key={feature}>{feature}</FeatureItem>
        ))}
      </div>
    </div>
  )
}

/* ─────────────────────────── Business Plan Card ─────────────────────────── */

function BusinessPlanCard({ tier }: { tier: CreditStep }) {
  const features = tier.plan.features ?? []

  return (
    <div className="relative flex flex-col rounded-lg border border-primary/20 bg-secondary p-6 sm:p-8">
      <div className="absolute -top-3 left-1/2 -translate-x-1/2">
        <div className="rounded-full bg-primary px-3 py-1 text-xs font-semibold text-primary-foreground">
          Business
        </div>
      </div>

      <div className="mb-6">
        <div className="mb-2 inline-flex rounded-full bg-primary/10 px-3 py-1 text-xs font-medium text-primary">
          {tier.label} checkpoint
        </div>
        <div className="mt-4 flex items-baseline gap-1">
          <span className="font-heading text-4xl font-medium text-foreground">
            {formatNGN(tier.priceMinor)}
          </span>
          <span className="text-sm text-muted-foreground">/month</span>
        </div>
        <p className="mt-2 text-sm text-muted-foreground">
          Includes {tier.credits.toLocaleString()} monthly credits.
        </p>
      </div>

      <Button size="lg" className="w-full">
        Upgrade to Business
      </Button>

      <div className="my-6 h-px bg-border" />

      <div className="flex flex-col gap-3">
        {features.map((feature) => (
          <FeatureItem key={feature}>{feature}</FeatureItem>
        ))}
      </div>
    </div>
  )
}

/* ─────────────────────────── FAQ ─────────────────────────── */

function FAQ() {
  const items = [
    {
      question: "What are credits?",
      answer:
        "Credits are consumed by AI model calls and sandbox runtime. Business checkpoints include a monthly credit balance, and Free includes 500 credits to try Hivy.",
    },
    {
      question: "Do you mark up AI usage?",
      answer:
        "No. Hivy has zero markup on AI usage. Your credits are used against the underlying model cost.",
    },
    {
      question: "What does the plan price include?",
      answer:
        "Each Business checkpoint includes monthly usage credits, workflow orchestration, integrations, memory, security, and support.",
    },
    {
      question: "What happens if I run out of credits?",
      answer:
        "You can move to a larger Business checkpoint or add more usage. If spend caps are enabled, new AI calls and sandbox starts pause before unexpected overage.",
    },
    {
      question: "Can I change my plan later?",
      answer:
        "Yes. You can move between Business checkpoints as your workload changes.",
    },
    {
      question: "What is included in Free?",
      answer:
        "Free includes 500 credits so you can test a real workflow before upgrading.",
    },
    {
      question: "Are credits charged for every feature?",
      answer:
        "No. Credits are charged only for AI tokens and sandbox runtime. Business features are included in your checkpoint.",
    },
    {
      question: "Do you offer refunds?",
      answer:
        "Yes. If you're not satisfied, contact us within 14 days of your first paid invoice for a full refund — no questions asked.",
    },
  ]

  return (
    <div className="mx-auto max-w-2xl">
      <h2 className="mb-8 text-center font-heading text-3xl font-normal leading-[1.1] tracking-[-0.02em] text-foreground">
        Frequently asked questions
      </h2>
      <div className="flex flex-col gap-3">
        {items.map((item) => (
          <div
            key={item.question}
            className="rounded-lg border border-border bg-secondary p-5"
          >
            <h3 className="font-heading text-sm font-medium text-foreground">
              {item.question}
            </h3>
            <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
              {item.answer}
            </p>
          </div>
        ))}
      </div>
    </div>
  )
}

/* ─────────────────────────── Custom Plan CTA ─────────────────────────── */

function CustomPlanCTA() {
  const features = [
    "Above 100K monthly credits",
    "Premium model routing",
    "Custom sandbox profiles",
    "Security and deployment reviews",
  ]

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6 rounded-lg border border-border bg-secondary p-6 sm:flex-row sm:items-start sm:justify-between sm:p-8">
      <div className="flex-1">
        <h3 className="font-heading text-xl font-medium text-foreground">
          Need a larger checkpoint?
        </h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Talk to our team for committed usage, custom runtime limits, and
          enterprise rollout support.
        </p>
        <div className="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2">
          {features.map((feature) => (
            <FeatureItem key={feature}>{feature}</FeatureItem>
          ))}
        </div>
      </div>
      <div className="shrink-0 sm:self-center">
        <Button variant="secondary" size="lg" render={<a href="#" />}>
          Talk to sales
        </Button>
      </div>
    </div>
  )
}

/* ─────────────────────────── Page ─────────────────────────── */

export default function PricingPage() {
  const plansQuery = $api.useQuery("get", "/v1/plans")
  const [sliderValue, setSliderValue] = useState(1)
  const plans = (plansQuery.data ?? []) as Plan[]
  const freePlan = plans.find((plan) => plan.slug === "free")
  const creditSteps = useMemo(
    () =>
      plans
        .filter(
          (plan) =>
            plan.slug?.startsWith("business-") &&
            (plan.monthly_credits ?? 0) > 0,
        )
        .sort((a, b) => (a.monthly_credits ?? 0) - (b.monthly_credits ?? 0))
        .map((plan) => ({
          plan,
          credits: plan.monthly_credits ?? 0,
          priceMinor: plan.price_cents ?? 0,
          label: formatCreditLabel(plan.monthly_credits ?? 0),
        })),
    [plans],
  )
  const selectedSliderValue =
    creditSteps.length === 0
      ? 0
      : Math.min(sliderValue, creditSteps.length - 1)
  const selectedTier = creditSteps[selectedSliderValue]

  return (
    <main className="relative flex min-h-screen flex-col items-center bg-background font-display text-foreground">
      {/* Background glow */}
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="absolute -top-52 -left-28 h-[28rem] w-[28rem] rounded-full bg-[var(--glow-left)] opacity-55 blur-[140px]" />
        <div className="absolute -top-40 left-1/2 h-[28rem] w-[28rem] -translate-x-1/2 rounded-full bg-[var(--glow-center)] opacity-50 blur-[140px]" />
        <div className="absolute -top-52 -right-28 h-[28rem] w-[28rem] rounded-full bg-[var(--glow-right)] opacity-50 blur-[140px]" />
      </div>

      {/* Floating header */}
      <div className="fixed top-5 right-0 left-0 z-50 mx-auto flex max-w-5xl items-center justify-between px-4 md:px-0">
        <Link
          href="/"
          className="font-heading text-xl font-bold tracking-tight text-foreground"
        >
          hivy
        </Link>
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2">
          <Navbar />
        </div>
        <div className="flex items-center gap-2 sm:gap-3">
          <div className="hidden sm:block">
            <Button variant="ghost" size="sm" render={<a href="#" />}>
              Talk to sales
            </Button>
          </div>
          <Button size="sm" render={<a href="/auth/signup" />}>
            Hire hivy
          </Button>
        </div>
      </div>

      {/* Content */}
      <div className="relative z-10 w-full max-w-5xl px-4 pt-36 sm:pt-44 lg:pt-52">
        <PricingHeader />

        <div className="mx-auto mt-10 max-w-xl">
          <BestFeatures />
        </div>

        {selectedTier ? (
          <div className="mx-auto mt-14 max-w-xl">
            <CreditSlider
              steps={creditSteps}
              value={selectedSliderValue}
              onChange={setSliderValue}
            />
          </div>
        ) : null}

        {plansQuery.isLoading ? (
          <div className="mx-auto mt-12 max-w-4xl rounded-lg border border-border bg-secondary p-8 text-center text-sm text-muted-foreground">
            Loading pricing plans.
          </div>
        ) : selectedTier ? (
          <div className="mx-auto mt-12 grid max-w-4xl grid-cols-1 gap-5 md:grid-cols-2">
            <FreePlanCard plan={freePlan} />
            <BusinessPlanCard tier={selectedTier} />
          </div>
        ) : (
          <div className="mx-auto mt-12 max-w-4xl rounded-lg border border-border bg-secondary p-8 text-center text-sm text-muted-foreground">
            Pricing plans are temporarily unavailable.
          </div>
        )}

        <div className="mx-auto mt-8 max-w-4xl">
          <CustomPlanCTA />
        </div>

        <div className="mx-auto mt-20 max-w-4xl">
          <FAQ />
        </div>

      </div>

      <MarketingFooter />
    </main>
  )
}
