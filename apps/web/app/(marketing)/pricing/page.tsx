import Link from "next/link"
import { Button } from "@/components/ui/button"
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
      {/* Hero */}
      <div className="flex flex-col items-center gap-4 pt-16 sm:pt-24 pb-12 px-4">
        <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">Pricing</p>
        <h1 className="font-heading text-[28px] sm:text-[40px] lg:text-[48px] font-bold text-foreground text-center leading-[1.15] -tracking-[0.5px]">
          Simple, transparent pricing
        </h1>
        <p className="text-base sm:text-lg text-muted-foreground text-center max-w-lg">
          Start free with one agent. Pay per agent as you scale. Bring your own LLM keys — you only pay us for the platform.
        </p>
      </div>

      {/* Plan cards */}
      <div className="max-w-4xl mx-auto w-full px-4 pb-8">
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
                <div className="flex items-center gap-2.5 text-sm"><Check /> 100 runs/month</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> 1 concurrent run</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Shared sandbox only</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Unlimited AI credentials</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Unlimited integrations</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> 20+ LLM providers</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> MCP tool support</div>
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
                <span className="font-heading text-[48px] font-bold text-foreground leading-none">$3.99</span>
                <span className="text-sm text-muted-foreground">/mo per agent</span>
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
                <div className="flex items-center gap-2.5 text-sm"><Check /> 500 runs/agent/month</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> 5 concurrent runs per agent</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Shared sandbox included</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> $0.01/run overage</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Agent Forge</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Persistent memory (1 GB/agent)</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Advanced analytics & audit logs</div>
                <div className="flex items-center gap-2.5 text-sm"><Check /> Priority support</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Dedicated sandbox add-on */}
      <div className="max-w-4xl mx-auto w-full px-4 pb-20">
        <div className="rounded-2xl border border-border p-8 sm:p-10 relative overflow-hidden">
          <div className="flex flex-col md:flex-row md:items-start gap-8 md:gap-12">
            <div className="flex flex-col gap-4 md:flex-1">
              <span className="font-mono text-[11px] font-medium uppercase tracking-[1px] text-muted-foreground">Add-on for Pro agents</span>
              <div className="flex items-baseline gap-2">
                <span className="font-heading text-[36px] sm:text-[48px] font-bold text-foreground leading-none">+$3</span>
                <span className="text-sm text-muted-foreground">/mo per agent</span>
              </div>
              <h3 className="font-heading text-lg font-semibold text-foreground">Dedicated sandbox</h3>
              <p className="text-sm text-muted-foreground max-w-md">
                For agents that need full system access — code review, security scanning, builds, or anything requiring shell, filesystem, and repo cloning. Each run gets an isolated sandbox that spins up in ~2 seconds.
              </p>
              <Link href="/auth">
                <Button variant="outline" size="lg">Add to any Pro agent</Button>
              </Link>
            </div>
            <div className="flex flex-col gap-2.5 md:flex-1">
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground mb-1">Everything in Pro, plus</span>
              <div className="flex items-center gap-2.5 text-sm"><Check /> Isolated sandbox per run</div>
              <div className="flex items-center gap-2.5 text-sm"><Check /> Full shell & filesystem access</div>
              <div className="flex items-center gap-2.5 text-sm"><Check /> Git clone, build tools, linters</div>
              <div className="flex items-center gap-2.5 text-sm"><Check /> Code execution & interpreters</div>
              <div className="flex items-center gap-2.5 text-sm"><Check /> 10 GB disk per sandbox</div>
              <div className="flex items-center gap-2.5 text-sm"><Check /> $0.05/run overage (instead of $0.01)</div>
              <div className="flex items-center gap-2.5 text-sm"><Check /> ~2s sandbox cold start</div>
              <div className="flex items-center gap-2.5 text-sm"><Check /> Custom sandbox templates</div>
            </div>
          </div>
        </div>
      </div>

      {/* Usage & overage breakdown */}
      <div className="max-w-5xl mx-auto w-full px-4 pb-20">
        <div className="flex flex-col gap-8">
          <div className="flex flex-col gap-2">
            <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">Usage-based pricing</p>
            <h2 className="font-heading text-xl sm:text-2xl font-bold text-foreground">Runs, overages, and what they cost</h2>
            <p className="text-sm text-muted-foreground max-w-2xl">
              A run is a single agent execution — a PR review, a support ticket response, a deploy check. Each plan includes runs. If you go over, you pay per extra run.
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            <div className="rounded-2xl border border-border p-6 flex flex-col gap-4">
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">Free</span>
              <div className="flex flex-col gap-2">
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Included runs</span>
                  <span className="text-sm font-medium text-foreground">100/month</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Overage</span>
                  <span className="text-sm font-medium text-foreground">Not available</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Concurrent runs</span>
                  <span className="text-sm font-medium text-foreground">1</span>
                </div>
              </div>
              <p className="text-xs text-muted-foreground">Agent pauses at 100 runs. Upgrade to Pro for more.</p>
            </div>

            <div className="rounded-2xl border-2 border-primary/30 p-6 flex flex-col gap-4 relative overflow-hidden">
              <div
                className="absolute inset-0 pointer-events-none"
                style={{
                  background: "radial-gradient(circle at 50% 0%, color-mix(in oklch, var(--primary) 6%, transparent) 0%, transparent 60%)",
                }}
              />
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-primary relative">Pro (shared sandbox)</span>
              <div className="flex flex-col gap-2 relative">
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Included runs</span>
                  <span className="text-sm font-medium text-foreground">500/agent/month</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Overage rate</span>
                  <span className="text-sm font-medium text-foreground">$0.01/run</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Concurrent runs</span>
                  <span className="text-sm font-medium text-foreground">5/agent</span>
                </div>
              </div>
              <p className="text-xs text-muted-foreground relative">Example: 700 runs = $3.99 + (200 x $0.01) = $5.99/month</p>
            </div>

            <div className="rounded-2xl border border-border p-6 flex flex-col gap-4">
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">Pro + Dedicated sandbox</span>
              <div className="flex flex-col gap-2">
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Included runs</span>
                  <span className="text-sm font-medium text-foreground">500/agent/month</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Overage rate</span>
                  <span className="text-sm font-medium text-foreground">$0.05/run</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Concurrent runs</span>
                  <span className="text-sm font-medium text-foreground">5/agent</span>
                </div>
              </div>
              <p className="text-xs text-muted-foreground">Example: 750 runs = $6.99 + (250 x $0.05) = $19.49/month</p>
            </div>
          </div>
        </div>
      </div>

      {/* Example scenarios */}
      <div className="max-w-5xl mx-auto w-full px-4 pb-20">
        <div className="flex flex-col gap-8">
          <div className="flex flex-col gap-2">
            <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">Real-world examples</p>
            <h2 className="font-heading text-xl sm:text-2xl font-bold text-foreground">What will you actually pay?</h2>
          </div>

          <div className="flex flex-col">
            <div className="grid grid-cols-[1fr_80px_80px_100px] sm:grid-cols-[1fr_120px_120px_120px] items-center py-3 border-b border-border">
              <span className="text-xs font-mono uppercase tracking-wider text-muted-foreground">Agent</span>
              <span className="text-xs font-mono uppercase tracking-wider text-muted-foreground text-center">Sandbox</span>
              <span className="text-xs font-mono uppercase tracking-wider text-muted-foreground text-center">Runs/mo</span>
              <span className="text-xs font-mono uppercase tracking-wider text-muted-foreground text-center">Monthly cost</span>
            </div>
            {[
              { agent: "Support bot", sandbox: "Shared", runs: "~200", cost: "$3.99" },
              { agent: "Content writer", sandbox: "Shared", runs: "~100", cost: "$3.99" },
              { agent: "Deploy monitor", sandbox: "Shared", runs: "~150", cost: "$3.99" },
              { agent: "Code review agent", sandbox: "Dedicated", runs: "~300", cost: "$6.99" },
              { agent: "Code review agent", sandbox: "Dedicated", runs: "~750", cost: "$19.49" },
              { agent: "Security scanner", sandbox: "Dedicated", runs: "~60", cost: "$6.99" },
              { agent: "Bug triage agent", sandbox: "Dedicated", runs: "~500", cost: "$6.99" },
            ].map((row, index) => (
              <div key={index} className="grid grid-cols-[1fr_80px_80px_100px] sm:grid-cols-[1fr_120px_120px_120px] items-center py-3 border-b border-border/50">
                <span className="text-sm text-foreground">{row.agent}</span>
                <span className="text-sm text-muted-foreground text-center">{row.sandbox}</span>
                <span className="text-sm text-muted-foreground text-center">{row.runs}</span>
                <span className="text-sm font-medium text-foreground text-center">{row.cost}</span>
              </div>
            ))}
          </div>
          <p className="text-xs text-muted-foreground">
            All agents include 500 runs/month. Shared agents overage at $0.01/run. Dedicated agents overage at $0.05/run. You bring your own LLM API keys — inference costs are not included.
          </p>
        </div>
      </div>

      {/* Feature comparison table */}
      <div className="max-w-5xl mx-auto w-full px-4 pb-24">
        <div className="flex flex-col gap-8">
          <p className="font-mono text-[11px] font-medium uppercase tracking-[1.5px] text-primary">Compare plans</p>

          {/* Agents & Limits */}
          <div className="flex flex-col">
            <h3 className="font-heading text-sm font-semibold text-foreground pb-3 border-b border-border">Agents & Limits</h3>
            <CompareRow label="Number of agents" free="1" pro="Unlimited" />
            <CompareRow label="Runs per month" free="100" pro="500/agent" />
            <CompareRow label="Overage (shared sandbox)" free={false} pro="$0.01/run" />
            <CompareRow label="Overage (dedicated sandbox)" free={false} pro="$0.05/run" />
            <CompareRow label="Concurrent runs" free="1" pro="5/agent" />
            <CompareRow label="Agent memory storage" free={false} pro="1 GB/agent" />
            <CompareRow label="Agent Forge (auto-optimization)" free={false} pro={true} />
          </div>

          {/* Sandboxes */}
          <div className="flex flex-col">
            <h3 className="font-heading text-sm font-semibold text-foreground pb-3 border-b border-border">Sandboxes</h3>
            <CompareRow label="Shared sandbox" free={true} pro={true} />
            <CompareRow label="Dedicated sandbox" free={false} pro="+$3/agent/mo" />
            <CompareRow label="Custom sandbox templates" free={false} pro={true} />
            <CompareRow label="Shell & filesystem access" free={false} pro="Dedicated only" />
            <CompareRow label="Code execution & interpreters" free={false} pro="Dedicated only" />
            <CompareRow label="Disk per sandbox" free={false} pro="10 GB" />
            <CompareRow label="Sandbox cold start" free={false} pro="~2 seconds" />
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
      <div className="max-w-5xl mx-auto w-full px-4 pb-24">
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
