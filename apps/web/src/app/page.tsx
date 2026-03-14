import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Nav } from "@/components/nav";
import { Footer } from "@/components/footer";
import { DotGrid } from "@/components/dot-grid";

function HeroSection() {
  return (
    <section className="relative flex flex-col items-center gap-12 px-20 pt-30 pb-20">
      <div className="pointer-events-none absolute top-10 left-1/2 h-75 w-125 -translate-x-1/2 rounded-full bg-primary opacity-[0.04] blur-[80px]" />
      <div className="absolute top-0 left-0 h-0.5 w-full bg-primary opacity-40" />
      <DotGrid rows={6} cols={6} className="absolute top-7.5 right-12.5" />

      <div className="flex max-w-225 flex-col items-center gap-6">
        <div className="flex items-center gap-2 border border-border px-3.5 py-1.5">
          <div className="size-1.5 shrink-0 bg-chart-4" />
          <span className="font-mono text-xs leading-4 tracking-[0.04em] text-muted-foreground">
            OPEN BETA — FREE TO START
          </span>
        </div>

        <h1 className="text-center font-mono text-[56px] font-semibold leading-16 tracking-[-0.03em] text-foreground">
          Your customers&apos; LLM keys deserve a vault, not a database column.
        </h1>

        <p className="max-w-170 text-center text-lg leading-7 text-muted-foreground">
          LLMVault stores LLM credentials with envelope encryption, mints scoped tokens for sandboxes, and proxies requests to any provider. Your code never sees a plaintext key.
        </p>

        <div className="flex items-center gap-4">
          <Button render={<Link href="/get-started" />} className="h-auto px-7 py-3 text-[15px] font-medium">
            Deploy in 15 minutes
          </Button>
          <Button render={<Link href="/architecture" />} variant="outline" className="h-auto px-7 py-3 text-[15px] font-medium text-muted-foreground">
            Read the architecture
          </Button>
        </div>
      </div>

      <div className="flex w-220 flex-col border border-border bg-background">
        <div className="flex items-center justify-between border-b border-border bg-card px-5 py-3">
          <div className="flex items-center gap-2">
            <div className="size-2.5 shrink-0 bg-border" />
            <div className="size-2.5 shrink-0 bg-border" />
            <div className="size-2.5 shrink-0 bg-border" />
          </div>
          <span className="font-mono text-xs leading-4 text-dim">terminal</span>
        </div>
        <div className="flex flex-col gap-1 pt-6 pr-6 pb-2 pl-6">
          <span className="font-mono text-xs leading-4 text-dim">
            # 1. Store a customer&apos;s API key (encrypted automatically)
          </span>
          <pre className="w-fit whitespace-pre font-mono text-[13px] leading-5 text-chart-2">{`curl -X POST https://api.llmvault.dev/v1/credentials \\
  -H "Authorization: Bearer $ORG_TOKEN" \\
  -d '{"provider":"anthropic","api_key":"sk-ant-..."}'`}</pre>
        </div>
        <div className="flex flex-col gap-1 pt-4 pr-6 pb-2 pl-6">
          <span className="font-mono text-xs leading-4 text-dim">
            # 2. Mint a short-lived token for the sandbox
          </span>
          <pre className="w-fit whitespace-pre font-mono text-[13px] leading-5 text-chart-2">{`curl -X POST https://api.llmvault.dev/v1/tokens \\
  -d '{"credential_id":"cred_8x7k","ttl":"1h"}'
# → {"token":"ptok_eyJhbG..."}`}</pre>
        </div>
        <div className="flex flex-col gap-1 px-6 pt-4 pb-6">
          <span className="font-mono text-xs leading-4 text-dim">
            # 3. Proxy requests — your app never sees the real key
          </span>
          <pre className="w-fit whitespace-pre font-mono text-[13px] leading-5 text-chart-2">{`curl https://api.llmvault.dev/v1/proxy/v1/messages \\
  -H "X-Proxy-Token: ptok_eyJhbG..." \\
  -d '{"model":"claude-sonnet-4-20250514","messages":[...]}'
# → streams response, key never exposed`}</pre>
        </div>
      </div>
    </section>
  );
}

function ProblemSection() {
  const questions = [
    { id: "Q1", question: '"How are our keys encrypted at rest?"', answer: "Envelope encryption. Every key gets a unique DEK (AES-256-GCM), wrapped by HashiCorp Vault Transit KMS. Plaintext is zeroed from memory immediately." },
    { id: "Q2", question: '"What if your database is compromised?"', answer: "Attackers get encrypted blobs. Decryption requires Vault access, which runs on separate infrastructure with its own auth boundary." },
    { id: "Q3", question: '"Can we revoke a key instantly?"', answer: "Sub-millisecond. Redis pub/sub propagates revocation to every proxy instance simultaneously. No cache windows. No stale credentials." },
  ];

  return (
    <section className="relative flex flex-col gap-16 overflow-clip bg-surface px-20 py-30">
      <div className="absolute top-0 left-0 h-px w-full bg-primary opacity-20" />
      <DotGrid rows={4} cols={4} className="absolute bottom-12.5 left-15" />

      <div className="flex max-w-175 flex-col gap-4">
        <span className="font-mono text-xs leading-4 tracking-wider text-primary">SECURITY BUILT IN</span>
        <h2 className="font-mono text-4xl font-semibold leading-11 tracking-tight text-foreground">
          The questions your customers will ask. We have the answers.
        </h2>
        <p className="text-base leading-6.5 text-muted-foreground">
          When your customers ask how their keys are protected, you&apos;ll have specific, technical answers — not hand-waving.
        </p>
      </div>

      <div className="flex w-7xl gap-4">
        {questions.map((q) => (
          <div key={q.id} className="flex flex-1 flex-col gap-3 border border-border bg-card p-7">
            <span className="font-mono text-[13px] leading-4 tracking-[0.02em] text-primary">{q.id}</span>
            <span className="font-mono text-[15px] font-semibold leading-5.5 text-foreground">{q.question}</span>
            <span className="text-sm leading-5.25 text-dim">{q.answer}</span>
          </div>
        ))}
      </div>
    </section>
  );
}

function HowItWorksSection() {
  const steps = [
    { num: "01", title: "Store", desc: "Your customer pastes their key. You POST it to LLMVault. We generate a unique DEK, encrypt with AES-256-GCM, wrap with Vault Transit KMS, and zero the plaintext from memory. Done.", code: `POST /v1/credentials\n{\n  "provider": "anthropic",\n  "api_key": "sk-ant-..."\n}` },
    { num: "02", title: "Mint", desc: "Need sandbox access? Mint a scoped JWT with a TTL. Bound to one credential, expires automatically, dies everywhere the instant you revoke it.", code: `POST /v1/tokens\n{\n  "credential_id": "cred_8x7k",\n  "ttl": "1h"\n}` },
    { num: "03", title: "Proxy", desc: "Send LLM requests through LLMVault. We resolve the real key from encrypted storage, handle auth differences between providers, and stream the response back. Your sandbox never sees the key.", code: `POST /v1/proxy/v1/messages\nX-Proxy-Token: ptok_eyJhbG...\n{\n  "model": "claude-sonnet-4-20250514"\n}` },
  ];

  return (
    <section className="relative flex flex-col gap-16 overflow-clip border-t border-border px-20 py-30">
      <div className="absolute top-0 left-1/2 h-px w-50 -translate-x-1/2 bg-primary opacity-30" />
      <div className="absolute bottom-20 left-5 opacity-[0.08]">
        <svg width="60" height="60" viewBox="0 0 60 60" fill="none">
          <path d="M 20 5 L 45 30 L 20 55" stroke="#8B5CF6" strokeWidth="4" strokeLinecap="square" />
        </svg>
      </div>

      <div className="flex flex-col items-center gap-4">
        <span className="font-mono text-xs leading-4 tracking-wider text-primary">HOW IT WORKS</span>
        <h2 className="text-center font-mono text-4xl font-semibold leading-11 tracking-tight text-foreground">
          Three API calls. That&apos;s the whole integration.
        </h2>
      </div>

      <div className="flex w-320 gap-6">
        {steps.map((step) => (
          <div key={step.num} className="flex flex-1 flex-col gap-5 border border-border p-8">
            <div className="flex flex-col gap-3">
              <span className="font-mono text-5xl font-semibold leading-14.5 tracking-[-0.03em] text-secondary">{step.num}</span>
              <span className="font-mono text-xl font-semibold leading-6 text-foreground">{step.title}</span>
              <span className="text-sm leading-5.5 text-muted-foreground">{step.desc}</span>
            </div>
            <div className="flex flex-col border border-border bg-card p-4">
              <pre className="w-fit whitespace-pre font-mono text-xs leading-4.5 text-chart-2">{step.code}</pre>
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

function FeaturesSection() {
  const features = [
    { icon: <svg width="32" height="32" viewBox="0 0 32 32" fill="none"><rect x="2" y="2" width="28" height="28" stroke="#8B5CF6" strokeWidth="2" fill="none" /><rect x="10" y="10" width="12" height="12" fill="#8B5CF6" /></svg>, title: "Envelope Encryption", desc: "Every key gets its own DEK (AES-256-GCM), wrapped by Vault Transit KMS. Compromise your database and Redis \u2014 you still get nothing usable." },
    { icon: <svg width="32" height="32" viewBox="0 0 32 32" fill="none"><path d="M4 24 L16 8 L28 24" stroke="#8B5CF6" strokeWidth="2" fill="none" /><line x1="16" y1="24" x2="16" y2="16" stroke="#8B5CF6" strokeWidth="2" /></svg>, title: "Sub-5ms Overhead", desc: "Three-tier cache: sealed memory, Redis, Postgres. Hot path resolves in 0.01ms. Cold path: 3-8ms. Your users won\u2019t feel the extra hop." },
    { icon: <svg width="32" height="32" viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="12" stroke="#8B5CF6" strokeWidth="2" fill="none" /><line x1="16" y1="8" x2="16" y2="16" stroke="#8B5CF6" strokeWidth="2" /><line x1="16" y1="16" x2="22" y2="20" stroke="#8B5CF6" strokeWidth="2" /></svg>, title: "Scoped Tokens", desc: "Mint JWTs bound to one credential. Set a TTL from seconds to 24 hours. Hand it to a sandbox. Sleep well." },
    { icon: <svg width="32" height="32" viewBox="0 0 32 32" fill="none"><rect x="4" y="4" width="10" height="24" stroke="#8B5CF6" strokeWidth="2" fill="none" /><rect x="18" y="4" width="10" height="24" stroke="#8B5CF6" strokeWidth="2" fill="none" /><line x1="14" y1="12" x2="18" y2="12" stroke="#8B5CF6" strokeWidth="2" /><line x1="14" y1="20" x2="18" y2="20" stroke="#8B5CF6" strokeWidth="2" /></svg>, title: "Every Provider", desc: "OpenAI, Anthropic, Google, Fireworks, OpenRouter \u2014 each authenticates differently. You send one request. We handle the rest." },
    { icon: <svg width="32" height="32" viewBox="0 0 32 32" fill="none"><path d="M8 16 L16 8 L24 16 L16 24 Z" stroke="#8B5CF6" strokeWidth="2" fill="none" /><line x1="16" y1="4" x2="16" y2="8" stroke="#8B5CF6" strokeWidth="2" /><line x1="16" y1="24" x2="16" y2="28" stroke="#8B5CF6" strokeWidth="2" /><line x1="4" y1="16" x2="8" y2="16" stroke="#8B5CF6" strokeWidth="2" /><line x1="24" y1="16" x2="28" y2="16" stroke="#8B5CF6" strokeWidth="2" /></svg>, title: "Instant Revocation", desc: "Customer disconnects? Token compromised? Revocation hits every instance via Redis pub/sub. Sub-millisecond. No stale windows." },
    { icon: <svg width="32" height="32" viewBox="0 0 32 32" fill="none"><rect x="4" y="4" width="10" height="10" stroke="#8B5CF6" strokeWidth="2" fill="none" /><rect x="18" y="4" width="10" height="10" fill="#8B5CF6" /><rect x="4" y="18" width="10" height="10" stroke="#8B5CF6" strokeWidth="2" fill="none" /><rect x="18" y="18" width="10" height="10" stroke="#8B5CF6" strokeWidth="2" fill="none" /></svg>, title: "Tenant Isolation", desc: "Every DB query scoped by org_id. Every Redis key namespaced. Tenant A can never reach Tenant B \u2014 by architecture, not convention." },
  ];

  return (
    <section className="relative flex flex-col gap-16 overflow-clip border-t border-border bg-surface px-20 py-30">
      <div className="absolute top-0 left-0 h-px w-full bg-primary opacity-20" />
      <DotGrid rows={5} cols={5} className="absolute top-10 right-15" />

      <div className="flex flex-col items-center gap-4">
        <span className="font-mono text-xs leading-4 tracking-wider text-primary">FEATURES</span>
        <h2 className="text-center font-mono text-4xl font-semibold leading-11 tracking-tight text-foreground">What you get. Specifically.</h2>
      </div>

      {[features.slice(0, 3), features.slice(3, 6)].map((row, i) => (
        <div key={i} className="flex w-320 gap-4">
          {row.map((f) => (
            <div key={f.title} className="flex flex-1 flex-col gap-4 border border-border p-8">
              {f.icon}
              <span className="font-mono text-base font-semibold leading-5 text-foreground">{f.title}</span>
              <span className="text-sm leading-5.5 text-muted-foreground">{f.desc}</span>
            </div>
          ))}
        </div>
      ))}
    </section>
  );
}

function FinalCTA() {
  return (
    <section className="relative flex flex-col items-center gap-8 overflow-clip border-t border-border bg-surface px-20 py-30">
      <div className="absolute top-0 left-0 h-px w-full bg-primary opacity-20" />
      <div className="absolute top-1/2 right-25 -translate-y-1/2 opacity-[0.06]">
        <svg width="120" height="120" viewBox="0 0 120 120" fill="none">
          <rect x="48" y="0" width="24" height="120" fill="#8B5CF6" />
          <rect x="0" y="48" width="120" height="24" fill="#8B5CF6" />
        </svg>
      </div>
      <h2 className="text-center font-mono text-[40px] font-semibold leading-12 tracking-[-0.03em] text-foreground">Ship BYOK support in days, not months.</h2>
      <p className="text-center text-base leading-5 text-muted-foreground">Free tier. 10 credentials. 10,000 proxy requests. No credit card.</p>
      <div className="flex items-center gap-4">
        <Button render={<Link href="/get-started" />} className="h-auto px-8 py-3.5 text-base font-medium">Start building</Button>
        <Button render={<Link href="/contact" />} variant="outline" className="h-auto px-8 py-3.5 text-base font-medium text-muted-foreground">Talk to an engineer</Button>
      </div>
    </section>
  );
}

export default function Home() {
  return (
    <div className="mx-auto flex min-h-screen max-w-360 flex-col bg-background">
      <Nav />
      <HeroSection />
      <ProblemSection />
      <HowItWorksSection />
      <FeaturesSection />
      <FinalCTA />
      <Footer />
    </div>
  );
}
