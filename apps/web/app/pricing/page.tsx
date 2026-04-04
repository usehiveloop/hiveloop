import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Logo } from "@/components/logo"
import { HugeiconsIcon } from "@hugeicons/react"
import { Tick02Icon, Cancel01Icon } from "@hugeicons/core-free-icons"

function Check() {
  return <HugeiconsIcon icon={Tick02Icon} size={16} className="text-green-500 shrink-0" />
}

function Dash() {
  return <HugeiconsIcon icon={Cancel01Icon} size={14} className="text-muted-foreground/30 shrink-0" />
}

export default function PricingPage() {
  return (
    <div className="w-full bg-background flex flex-col relative">
      {/* Nav */}
      <nav className="w-full h-16 flex items-center justify-between max-w-424 mx-auto sticky top-0 bg-background z-100 px-4 lg:px-0">
        <Link href="/">
          <Logo className="h-8" />
        </Link>
        <div className="hidden md:flex items-center gap-6 lg:gap-9">
          <Link href="/docs" className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors">Docs</Link>
          <Link href="/pricing" className="text-sm font-medium text-foreground">Pricing</Link>
          <Link href="/marketplace" className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors">Marketplace</Link>
        </div>
        <Link href="/auth">
          <Button variant="outline" size="sm">Sign in</Button>
        </Link>
      </nav>

      {/* Hero */}
      <div className="flex flex-col items-center gap-4 pt-16 sm:pt-24 pb-12 px-4">
        <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">Pricing</p>
        <h1 className="font-heading text-[28px] sm:text-[40px] lg:text-[48px] font-bold text-foreground text-center leading-[1.15] -tracking-[0.5px]">
          Simple, transparent pricing
        </h1>
        <p className="text-base sm:text-lg text-muted-foreground text-center max-w-md">
          Start free. Scale to production.
        </p>
      </div>

      {/* Plan cards */}
      <div className="max-w-4xl mx-auto w-full px-4 pb-20">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {/* Free Plan */}
          <div className="flex flex-col rounded-2xl border border-border p-8 gap-8">
            <div className="flex flex-col gap-4">
              <span className="font-mono text-[11px] font-medium uppercase tracking-[1px] text-muted-foreground">Free forever</span>
              <div className="flex items-baseline gap-1">
                <span className="font-heading text-[48px] font-bold text-foreground leading-none">$0</span>
              </div>
              <p className="text-sm text-muted-foreground">For exploring and prototyping</p>
              <Link href="/auth">
                <Button variant="outline" size="lg" className="w-full">Get started</Button>
              </Link>
            </div>

            <div className="flex flex-col gap-3">
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">What&apos;s included</span>
              <div className="flex flex-col gap-2.5">
                <div className="flex items-center gap-2.5 text-sm"><Check /> 1 agent</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Unlimited AI credentials</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Unlimited integrations & connections</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Unlimited proxy tokens</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Envelope encryption (AES-256-GCM)</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> 20+ LLM providers</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> MCP tool support</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Human-in-the-loop approvals</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Shared sandboxes</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Conversations & streaming</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> TypeScript SDK</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Community support</div>
              </div>
            </div>
          </div>

          {/* Pro Plan */}
          <div className="flex flex-col rounded-2xl border-2 border-primary/30 p-8 gap-8 relative overflow-hidden">
            <div
              className="absolute inset-0 pointer-events-none"
              style={{
                background: "radial-gradient(circle at 50% 0%, color-mix(in oklch, var(--primary) 8%, transparent) 0%, transparent 60%)",
              }}
            />
            <div className="flex flex-col gap-4 relative">
              <span className="font-mono text-[11px] font-medium uppercase tracking-[1px] text-primary">Pro</span>
              <div className="flex items-baseline gap-1">
                <span className="font-heading text-[48px] font-bold text-foreground leading-none">$4</span>
                <span className="text-sm text-muted-foreground">/month per agent</span>
              </div>
              <p className="text-sm text-muted-foreground">For teams shipping agents to production</p>
              <Link href="/auth">
                <Button size="lg" className="w-full">Start building</Button>
              </Link>
            </div>

            <div className="flex flex-col gap-3 relative">
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">Everything in Free, plus</span>
              <div className="flex flex-col gap-2.5">
                <div className="flex items-center gap-2.5 text-sm"><Check /> Unlimited agents</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Unlimited agent triggers</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Agent Forge (auto-optimization)</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Persistent agent memory</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Custom sandbox templates</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Dedicated sandboxes</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Identity scoping & isolation</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Per-identity rate limiting</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Advanced analytics & reporting</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Audit logs</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Custom domains</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> API key scopes</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Priority support</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Feature comparison table */}
      <div className="max-w-4xl mx-auto w-full px-4 pb-24">
        <div className="flex flex-col gap-8">
          <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">Compare plans</p>

          {/* Agents */}
          <div className="flex flex-col">
            <h3 className="font-heading text-sm font-semibold text-foreground pb-3 border-b border-border">Agents</h3>
            <CompareRow label="Number of agents" free="1" pro="Unlimited" />
            <CompareRow label="Agent Forge (auto-optimization)" free={false} pro={true} />
            <CompareRow label="Persistent memory (Hindsight)" free={false} pro={true} />
            <CompareRow label="Custom sandbox templates" free={false} pro={true} />
            <CompareRow label="Dedicated sandboxes" free={false} pro={true} />
            <CompareRow label="Shared sandboxes" free={true} pro={true} />
            <CompareRow label="Subagent delegation" free={false} pro={true} />
            <CompareRow label="MCP tool support" free={true} pro={true} />
            <CompareRow label="Human-in-the-loop approvals" free={true} pro={true} />
            <CompareRow label="Conversations & SSE streaming" free={true} pro={true} />
          </div>

          {/* Integrations */}
          <div className="flex flex-col">
            <h3 className="font-heading text-sm font-semibold text-foreground pb-3 border-b border-border">Integrations & Connections</h3>
            <CompareRow label="OAuth integrations" free="Unlimited" pro="Unlimited" />
            <CompareRow label="Connections" free="Unlimited" pro="Unlimited" />
            <CompareRow label="Integration action scoping" free={true} pro={true} />
            <CompareRow label="Resource-level scoping" free={false} pro={true} />
          </div>

          {/* Credentials */}
          <div className="flex flex-col">
            <h3 className="font-heading text-sm font-semibold text-foreground pb-3 border-b border-border">Credentials & Tokens</h3>
            <CompareRow label="AI credentials" free="Unlimited" pro="Unlimited" />
            <CompareRow label="Proxy tokens" free="Unlimited" pro="Unlimited" />
            <CompareRow label="Envelope encryption (AES-256-GCM)" free={true} pro={true} />
            <CompareRow label="Credential rotation" free={true} pro={true} />
            <CompareRow label="Token rate limiting" free={false} pro={true} />
            <CompareRow label="20+ LLM providers" free={true} pro={true} />
          </div>

          {/* Observability */}
          <div className="flex flex-col">
            <h3 className="font-heading text-sm font-semibold text-foreground pb-3 border-b border-border">Observability</h3>
            <CompareRow label="Generation tracking" free={true} pro={true} />
            <CompareRow label="Cost tracking & attribution" free={true} pro={true} />
            <CompareRow label="Advanced analytics & grouping" free={false} pro={true} />
            <CompareRow label="Audit logs" free={false} pro={true} />
            <CompareRow label="Usage statistics dashboard" free={false} pro={true} />
          </div>

          {/* Security */}
          <div className="flex flex-col">
            <h3 className="font-heading text-sm font-semibold text-foreground pb-3 border-b border-border">Security & Access Control</h3>
            <CompareRow label="Encryption at rest" free={true} pro={true} />
            <CompareRow label="Identity scoping & isolation" free={false} pro={true} />
            <CompareRow label="Per-identity rate limiting" free={false} pro={true} />
            <CompareRow label="API key scopes" free={false} pro={true} />
            <CompareRow label="Tool-level permissions" free={true} pro={true} />
            <CompareRow label="Custom domains" free={false} pro={true} />
          </div>

          {/* Developer */}
          <div className="flex flex-col">
            <h3 className="font-heading text-sm font-semibold text-foreground pb-3 border-b border-border">Developer Experience</h3>
            <CompareRow label="TypeScript SDK" free={true} pro={true} />
            <CompareRow label="OpenAPI specification" free={true} pro={true} />
            <CompareRow label="Webhook integrations" free={true} pro={true} />
            <CompareRow label="Self-hosting" free={true} pro={true} />
            <CompareRow label="Support" free="Community" pro="Priority" />
          </div>
        </div>
      </div>

      {/* CTA */}
      <div className="max-w-4xl mx-auto w-full px-4 pb-24">
        <div className="flex flex-col items-center gap-6 rounded-2xl border border-border p-12 text-center relative overflow-hidden">
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background: "radial-gradient(circle at 50% 100%, color-mix(in oklch, var(--primary) 6%, transparent) 0%, transparent 60%)",
            }}
          />
          <h2 className="font-heading text-2xl sm:text-3xl font-bold text-foreground relative">Ready to build?</h2>
          <p className="text-muted-foreground max-w-md relative">
            Start with the free plan and upgrade when you need production features.
          </p>
          <div className="flex gap-3 relative">
            <Link href="/auth"><Button size="lg">Get started free</Button></Link>
            <Link href="/docs"><Button variant="outline" size="lg">Read the docs</Button></Link>
          </div>
        </div>
      </div>
    </div>
  )
}

function CompareRow({
  label,
  free,
  pro,
}: {
  label: string
  free: boolean | string
  pro: boolean | string
}) {
  return (
    <div className="grid grid-cols-[1fr_100px_100px] sm:grid-cols-[1fr_140px_140px] items-center py-3 border-b border-border/50">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-sm text-center">
        {free === true ? <span className="inline-flex justify-center w-full"><Check /></span> : free === false ? <span className="inline-flex justify-center w-full"><Dash /></span> : <span className="text-foreground">{free}</span>}
      </span>
      <span className="text-sm text-center">
        {pro === true ? <span className="inline-flex justify-center w-full"><Check /></span> : pro === false ? <span className="inline-flex justify-center w-full"><Dash /></span> : <span className="text-foreground">{pro}</span>}
      </span>
    </div>
  )
}
