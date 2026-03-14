# LLMVault — Buyer Personas

---

## Persona 1: "Platform Pete" — The Platform Engineer

### Demographics
- **Title**: Senior/Staff Platform Engineer, Backend Engineer, Infrastructure Engineer
- **Company**: Series A–C AI startup or SaaS company adding AI features (20–200 employees)
- **Experience**: 5–10 years, strong backend (Go, Python, TypeScript), familiar with cloud infra
- **Reports to**: CTO or VP of Engineering

### Situation
Pete's company is building a product that lets customers connect their own LLM providers. Think: an AI-powered writing tool where enterprise customers bring their own OpenAI or Anthropic keys, or a code assistant platform where each workspace connects to a different LLM.

Pete has been tasked with building the "Connect Your LLM" feature. He needs to store customer API keys securely, proxy requests to different providers, and handle the differences between how each provider authenticates (Bearer tokens, x-api-key headers, query params).

### Pain Points
- **Security anxiety**: He knows storing API keys in plain text (or with simple AES) isn't good enough, but building proper envelope encryption + KMS integration is a rabbit hole
- **Multi-provider headaches**: OpenAI uses Bearer auth, Anthropic uses x-api-key, Gemini uses query params — each needs different handling
- **Time pressure**: He has 2 weeks to ship this, not 2 months
- **Trust burden**: Enterprise customers are asking "how do you store our keys?" and he doesn't have a great answer yet

### What Pete Searches For
- "how to securely store api keys for customers"
- "llm api key proxy"
- "bring your own api key architecture"
- "encrypt api keys at rest best practices"
- "openai proxy server"

### What Wins Pete Over
- **Clear architecture docs**: He wants to see exactly how encryption works (envelope encryption, KMS, sealed memory)
- **SDKs in his language**: TypeScript or Python SDK that makes integration trivial
- **Latency numbers**: He needs proof the proxy won't slow down streaming responses
- **Self-hosted option**: His security team may require on-prem deployment

### How Pete Evaluates
1. Reads the docs and architecture overview
2. Runs through a quickstart (< 15 minutes to first proxied request)
3. Reviews the security model with his security team
4. Load tests for latency and throughput
5. Brings to CTO for budget approval

### Messaging for Pete
**Lead with**: Technical architecture and security guarantees
**Emphasize**: "Ship in days, not months" + "Enterprise-grade encryption out of the box"
**Proof**: Architecture diagrams, latency benchmarks, open-source code

---

## Persona 2: "CTO Chris" — The Technical Co-Founder

### Demographics
- **Title**: CTO, VP of Engineering, Technical Co-Founder
- **Company**: Seed to Series B AI-native startup (5–50 employees)
- **Experience**: 10+ years, previously an engineer, now making build-vs-buy decisions
- **Reports to**: CEO / Board

### Situation
Chris's startup is building an AI-first product. They've chosen to support BYOK because enterprise customers demand it, and it also reduces their own LLM costs. Chris needs to decide whether to build credential management in-house or buy a solution. Every week of engineering time spent on infrastructure is a week not spent on the core product.

### Pain Points
- **Opportunity cost**: Every engineer building security infra is an engineer not building features
- **Enterprise readiness**: Large prospects are asking about SOC 2, key management policies, and audit trails. Chris needs answers fast.
- **Compliance pressure**: Handling customer secrets comes with liability. A breach would be existential.
- **Scaling concerns**: The quick-and-dirty approach works at 10 customers. At 1,000, it's a liability.

### What Chris Searches For
- "llm api key management service"
- "byok infrastructure for saas"
- "build vs buy api key management"
- "soc 2 compliant key storage"

### What Wins Chris Over
- **ROI math**: "3-6 months of senior eng time" vs. a monthly fee
- **Compliance story**: SOC 2, audit logs, encryption at rest — things that make enterprise sales easier
- **Case studies**: Other startups in his space using LLMVault
- **Clear pricing**: No surprises, scales with usage

### How Chris Evaluates
1. Reads the landing page and pricing
2. Has Pete (his engineer) do a technical evaluation
3. Compares cost vs. building in-house
4. Checks if it satisfies enterprise customer requirements
5. Signs off on buy decision

### Messaging for Chris
**Lead with**: Business value — time saved, risk reduced, enterprise readiness
**Emphasize**: "Ship BYOK in days" + "Pass enterprise security reviews"
**Proof**: ROI calculator, customer logos, compliance certifications

---

## Persona 3: "Sandbox Sam" — The AI Infrastructure Builder

### Demographics
- **Title**: Backend Engineer, AI Infrastructure Engineer, DevOps Lead
- **Company**: AI dev tools company, cloud IDE, coding assistant, or agent platform (10–100 employees)
- **Experience**: 5–8 years, deep in containers, sandboxing, and distributed systems
- **Reports to**: Engineering Manager or CTO

### Situation
Sam is building an environment where AI agents run in sandboxed containers. Each agent needs access to an LLM, but exposing real API keys to the sandbox is a security risk — if the sandbox is compromised, the key is compromised. Sam needs a way to give each sandbox a short-lived, scoped credential that can be revoked instantly.

### Pain Points
- **Sandbox security**: Can't put real API keys in environment variables inside sandboxes
- **Token lifecycle**: Needs tokens that expire automatically and can be revoked mid-session
- **Blast radius**: If one sandbox is compromised, only that sandbox's token should be affected
- **Multi-tenant isolation**: Different customers' agents must never access each other's credentials

### What Sam Searches For
- "short lived api tokens for sandboxes"
- "llm proxy for sandboxed agents"
- "temporary api key for ai agents"
- "scoped credential management"
- "sandbox api key security"

### What Wins Sam Over
- **Token minting API**: Simple call to mint a scoped token with TTL
- **Instant revocation**: Token dies everywhere within milliseconds
- **Tenant isolation guarantees**: Architecture-level isolation, not just application-level
- **Streaming support**: The proxy must handle SSE streaming without buffering

### How Sam Evaluates
1. Reads docs, specifically token minting and revocation
2. Tests token TTL and revocation in a sandbox environment
3. Benchmarks streaming latency through the proxy
4. Reviews multi-tenant isolation guarantees
5. Integrates into their container orchestration

### Messaging for Sam
**Lead with**: Scoped, short-lived tokens + instant revocation
**Emphasize**: "Give agents credentials, not keys" + "Sub-5ms proxy overhead"
**Proof**: Token lifecycle demo, latency benchmarks, architecture docs

---

## Persona 4: "Widget Wendy" — The Developer Experience Builder

### Demographics
- **Title**: Full-Stack Engineer, Frontend Engineer, DX Engineer
- **Company**: SaaS product adding AI features, developer tools company (20–200 employees)
- **Experience**: 4–8 years, frontend-heavy (React/Next.js), building customer-facing features
- **Reports to**: Product Manager or Engineering Manager

### Situation
Wendy is building the "Connect Your LLM Provider" flow in her company's app. She needs a way for customers to paste their API keys, select a provider, and start using AI features. She wants a polished UX — ideally something embeddable like a Stripe widget — that handles the security side so she can focus on the interface.

### Pain Points
- **Frontend security**: She knows she shouldn't store API keys in localStorage, but the alternatives are complex
- **Provider fragmentation**: Each LLM provider has different setup steps, and she needs to guide customers through each
- **UX polish**: She wants a "Connect Provider" flow as smooth as connecting a payment method
- **Backend complexity**: She doesn't want to build the encryption/proxy backend herself

### What Wendy Searches For
- "connect llm provider widget"
- "api key input component react"
- "bring your own key ui component"
- "stripe-like widget for api keys"

### What Wins Wendy Over
- **Drop-in widget**: React component or embeddable iframe for the "Connect Provider" flow
- **Provider presets**: Pre-configured settings for major providers (OpenAI, Anthropic, Google, etc.)
- **Good docs with frontend examples**: Copy-paste code that works
- **Webhooks**: Events when a customer connects/disconnects a provider

### How Wendy Evaluates
1. Looks at the widget demo
2. Checks if there's a React/Next.js SDK
3. Follows the quickstart to embed the widget
4. Checks customization options (theming, provider list)

### Messaging for Wendy
**Lead with**: "Connect Provider" widget + drop-in SDKs
**Emphasize**: "Stripe-like experience for LLM key management"
**Proof**: Live widget demo, code examples, provider presets

---

## Persona Priority & Buying Center

| Persona | Role in Purchase | Priority | Volume |
|---------|-----------------|----------|--------|
| Platform Pete | Evaluator & Champion | P0 | High — every AI platform has this person |
| CTO Chris | Decision Maker & Budget Holder | P0 | Medium — involved in buy decisions |
| Sandbox Sam | Evaluator & Power User | P1 | Medium — growing as agent platforms emerge |
| Widget Wendy | User & Influencer | P2 | Medium — drives adoption once purchased |

### Typical Buying Flow
1. **Pete** discovers LLMVault (search, HN, Twitter, dev community)
2. **Pete** evaluates technically, builds a proof-of-concept
3. **Pete** champions it internally, presents to **Chris**
4. **Chris** evaluates ROI, security posture, and pricing
5. **Chris** approves purchase
6. **Pete** + **Sam** integrate into production
7. **Wendy** builds the customer-facing connection flow

### Implication for Marketing
- **Top-of-funnel content** should target Pete and Sam (technical content, tutorials, architecture posts)
- **Mid-funnel content** should target Chris (ROI, case studies, security reviews)
- **Product-led growth** should target Pete and Sam (free tier, quick start, sandbox experience)
- **Widget/SDK marketing** is a secondary but powerful acquisition channel targeting Wendy
