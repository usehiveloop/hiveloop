"use client"

import { useState } from "react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Slider } from "@/components/ui/slider"
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
    <div className="mx-auto max-w-2xl text-center">
      <h1 className="font-heading text-4xl font-normal leading-[1.1] tracking-[-0.02em] text-foreground md:text-5xl">
        Simple, credits-based pricing
      </h1>
      <p className="mx-auto mt-4 max-w-lg text-base leading-[1.6] text-muted-foreground">
        Start free, scale as you grow. Only pay for what you use.
      </p>
    </div>
  )
}

/* ─────────────────────────── Best Features ─────────────────────────── */

function BestFeatures() {
  const features = [
    "No setup fees",
    "Cancel anytime",
    "Credits never expire",
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

const creditSteps = [
  { credits: 5000, price: 49000, label: "5K" },
  { credits: 10000, price: 79000, label: "10K" },
  { credits: 25000, price: 129000, label: "25K" },
  { credits: 50000, price: 199000, label: "50K" },
  { credits: 100000, price: 289000, label: "100K" },
]

function CreditSlider({
  value,
  onChange,
}: {
  value: number
  onChange: (value: number) => void
}) {
  return (
    <div className="mx-auto w-full max-w-xl">
      <div className="mb-4 flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">
          Credits per month
        </span>
        <span className="font-heading text-lg font-medium text-foreground">
          {creditSteps[value].credits.toLocaleString()}
        </span>
      </div>
      <Slider
        min={0}
        max={creditSteps.length - 1}
        step={1}
        value={[value]}
        onValueChange={(v) => {
          const arr = Array.isArray(v) ? v : [v]
          if (typeof arr[0] === "number") onChange(arr[0])
        }}
      />
      <div className="mt-3 flex justify-between">
        {creditSteps.map((step, i) => (
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

function FreePlanCard() {
  const features = [
    "1,000 credits per month",
    "Up to 3 team members",
    "Core integrations (Slack, GitHub, Sheets)",
    "Basic task automation",
    "Shared team memory",
    "Community support",
  ]

  return (
    <div className="flex flex-col rounded-2xl border border-border bg-secondary p-6 sm:p-8">
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
          Perfect for trying hivy out with your team.
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

function BusinessPlanCard({ sliderValue }: { sliderValue: number }) {
  const tier = creditSteps[sliderValue]

  const features = [
    `${tier.credits.toLocaleString()} credits per month`,
    "Unlimited team members",
    "All integrations",
    "AI-powered task automation",
    "Recurring tasks & scheduling",
    "Shared team memory",
    "Granular permissions",
    "API & webhook access",
    "Standard support (24h response)",
  ]

  return (
    <div className="relative flex flex-col rounded-2xl border border-primary/20 bg-secondary p-6 sm:p-8">
      <div className="absolute -top-3 left-1/2 -translate-x-1/2">
        <div className="rounded-full bg-primary px-3 py-1 text-xs font-semibold text-primary-foreground">
          Most popular
        </div>
      </div>

      <div className="mb-6">
        <div className="mb-2 inline-flex rounded-full bg-primary/10 px-3 py-1 text-xs font-medium text-primary">
          Business
        </div>
        <div className="mt-4 flex items-baseline gap-1">
          <span className="font-heading text-4xl font-medium text-foreground">
            ₦{tier.price.toLocaleString()}
          </span>
          <span className="text-sm text-muted-foreground">/month</span>
        </div>
        <p className="mt-2 text-sm text-muted-foreground">
          For teams that want to automate their workflow.
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
        "Credits are the currency hivy uses to perform tasks. Each action — sending a message, reading a document, or executing a workflow — consumes a small number of credits. You only pay for what you use.",
    },
    {
      question: "Do credits expire?",
      answer:
        "No. Your credits never expire as long as your account is active. Unused credits roll over to the next month.",
    },
    {
      question: "What happens if I run out of credits?",
      answer:
        "Your AI coworkers will pause until your credits are replenished. You can upgrade your plan anytime, or wait for your next monthly refill.",
    },
    {
      question: "Can I change my plan later?",
      answer:
        "Yes. You can upgrade or downgrade your plan at any time from your workspace settings. Changes take effect immediately.",
    },
    {
      question: "Is there a free trial?",
      answer:
        "Yes. Every new workspace starts on the Free plan with 1,000 credits — no credit card required.",
    },
    {
      question: "How do I estimate how many credits I need?",
      answer:
        "A typical team of 5 uses around 10,000 credits per month. Start with the Free plan and monitor your usage in the dashboard. You can always adjust.",
    },
    {
      question: "What's included in the free plan?",
      answer:
        "The Free plan includes 1,000 credits per month, up to 3 team members, core integrations, basic automation, shared memory, and community support.",
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
            className="rounded-2xl border border-border bg-secondary p-5"
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
    "Bring your own LLM keys",
    "Custom preview domains",
    "Multiple AI coworkers",
    "On-premise deployment option",
  ]

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6 rounded-2xl border border-border bg-secondary p-6 sm:flex-row sm:items-start sm:justify-between sm:p-8">
      <div className="flex-1">
        <h3 className="font-heading text-xl font-medium text-foreground">
          Need a custom plan?
        </h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Talk to our team for tailored pricing, unlimited credits, and
          enterprise features.
        </p>
        <div className="mt-6 grid grid-cols-1 gap-3 sm:grid-cols-2">
          {features.map((feature) => (
            <FeatureItem key={feature}>{feature}</FeatureItem>
          ))}
        </div>
      </div>
      <div className="shrink-0 sm:self-center">
        <Button variant="secondary" size="lg" asChild>
          <a href="#">Talk to sales</a>
        </Button>
      </div>
    </div>
  )
}

/* ─────────────────────────── Page ─────────────────────────── */

export default function PricingPage() {
  const [sliderValue, setSliderValue] = useState(2)

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
            <Button variant="ghost" size="sm" asChild>
              <a href="#">Talk to Sales</a>
            </Button>
          </div>
          <Button size="sm" asChild>
            <a href="#">Hire hivy</a>
          </Button>
        </div>
      </div>

      {/* Content */}
      <div className="relative z-10 w-full max-w-5xl px-4 pt-36 sm:pt-44 lg:pt-52">
        <PricingHeader />

        <div className="mx-auto mt-10 max-w-xl">
          <BestFeatures />
        </div>

        <div className="mx-auto mt-14 max-w-xl">
          <CreditSlider value={sliderValue} onChange={setSliderValue} />
        </div>

        <div className="mx-auto mt-12 grid max-w-4xl grid-cols-1 gap-5 md:grid-cols-2">
          <FreePlanCard />
          <BusinessPlanCard sliderValue={sliderValue} />
        </div>

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
