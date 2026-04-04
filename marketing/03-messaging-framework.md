# ZiraLoop — Messaging Framework

---

## Brand Voice

**Tone**: Technically credible, direct, no-BS. Like a senior engineer explaining architecture at a whiteboard — confident but not arrogant. Respects the reader's intelligence.

**We sound like**: Stripe's docs meet Cloudflare's blog.
**We don't sound like**: Enterprise marketing fluff or overhyped AI startup copy.

**Voice principles**:
- Lead with the problem, not the product
- Be specific — numbers, not adjectives
- Show the architecture, don't just claim security
- Use developer language, not marketing language
- Acknowledge trade-offs honestly

---

## Headline Options (Ranked)

### Primary (Recommended)
**"Your customers' LLM keys deserve a vault, not a database column."**

### Alternatives
- "The secure proxy layer for LLM API keys."
- "Store keys. Mint tokens. Proxy requests. Ship faster."
- "Stop storing LLM API keys in your database."
- "BYOK infrastructure for AI platforms."
- "Give agents credentials, not keys."
- "The API key layer your AI platform is missing."

---

## Tagline Options

- **"Secure LLM key management for platforms."** (descriptive, clear)
- **"Keys in. Tokens out. Requests proxied."** (punchy, action-oriented)
- **"The vault between your app and every LLM."** (plays on the brand name)

---

## Core Messages by Audience

### For Platform Engineers (Pete)

**Primary message**: Ship BYOK in days, not months. ZiraLoop handles encryption, proxying, and multi-provider auth schemes so you don't have to.

**Supporting messages**:
1. Envelope encryption (AES-256-GCM + Vault Transit) — your customers' keys never touch your code in plaintext
2. Sub-5ms proxy overhead with 3-tier caching — your users won't feel the extra hop
3. One API for all providers — Bearer, x-api-key, query params, whatever. ZiraLoop abstracts it away.
4. Integrate in an afternoon with our SDK. First proxied request in under 15 minutes.

### For CTOs (Chris)

**Primary message**: Every week your engineers spend building key management is a week they're not building your product. ZiraLoop gives you enterprise-grade credential security without the infrastructure investment.

**Supporting messages**:
1. 3-6 months of senior engineering time, replaced by a single integration
2. Pass enterprise security reviews with built-in audit trails, encryption at rest, and tenant isolation
3. SOC 2 readiness without building compliance infrastructure yourself
4. Predictable pricing that scales with your usage, not your headcount

### For Sandbox Builders (Sam)

**Primary message**: Give your sandboxed agents scoped, short-lived credentials that auto-expire and can be revoked instantly — instead of real API keys that live forever.

**Supporting messages**:
1. Mint a JWT scoped to one credential with a configurable TTL (seconds to 24 hours)
2. Revocation propagates across all proxy instances in sub-millisecond time via Redis pub/sub
3. If a sandbox is compromised, only that sandbox's token is affected — blast radius is minimal
4. Full streaming support — SSE chunks flow through with no buffering, no corruption

### For Widget Builders (Wendy)

**Primary message**: Add a "Connect Your LLM Provider" flow to your app as easily as adding a Stripe payment widget. We handle the security, you handle the experience.

**Supporting messages**:
1. Drop-in React component with provider presets (OpenAI, Anthropic, Google, and more)
2. Customizable theming to match your brand
3. Webhooks for connect/disconnect events
4. Your customers see a polished flow. Your backend sees encrypted credentials.

---

## Feature-Benefit Mapping

| Feature | Benefit | Proof Point |
|---------|---------|-------------|
| Envelope encryption (AES-256-GCM + Vault Transit KMS) | Customer keys are never stored in plaintext — anywhere | Architecture diagram showing 3-layer encryption |
| 3-tier cache (Memory → Redis → Postgres/Vault) | Sub-5ms overhead on every LLM request | Published latency benchmarks (p50/p95/p99) |
| Scoped JWT tokens with TTL | Sandboxes get just enough access for just long enough | Token lifecycle demo |
| Redis pub/sub invalidation | Credential revocation takes effect everywhere instantly | Revocation latency measurement |
| Multi-provider auth scheme abstraction | One integration handles OpenAI, Anthropic, Google, and any provider | Supported providers page |
| Multi-tenant isolation (org-scoped queries, namespaced Redis keys) | Tenant A can never access Tenant B's credentials — by architecture, not just by code | Security architecture doc |
| Audit log | Full trail of every credential created, revoked, and proxied | Audit log screenshot / API docs |
| memguard sealed memory | Even if the process is dumped, keys can't be extracted from memory | Security whitepaper |
| Distroless container, nonroot | Minimal attack surface in production | Dockerfile walkthrough |
| Streaming pass-through (FlushInterval: -1) | SSE responses stream identically through the proxy | Streaming fidelity test results |

---

## Objection Handling

### "We can build this ourselves."
You absolutely can. The question is whether you should. Envelope encryption, KMS integration, multi-tier caching with pub/sub invalidation, auth scheme abstraction, sealed memory, and multi-tenant isolation is a 3-6 month project for a senior engineer. ZiraLoop ships all of that, battle-tested, so your team can focus on what makes your product unique.

### "Why wouldn't we just use AWS Secrets Manager / HashiCorp Vault directly?"
Those are excellent tools for storing secrets. But they're not proxy layers. You'd still need to build the streaming reverse proxy, the auth scheme abstraction, the short-lived token system, the multi-tenant isolation, and the caching layer on top. ZiraLoop uses Vault under the hood — and adds everything else you need.

### "What about latency? Another hop means slower responses."
The proxy adds < 5ms on the hot path (L1 cache hit: 0.01ms). For context, a typical LLM API call takes 500ms-30s. The proxy overhead is invisible.

### "What if ZiraLoop goes down?"
ZiraLoop is designed for horizontal scaling and graceful degradation. If Redis goes down, L1 memory cache and L3 (Postgres/Vault) still serve requests. If Vault is temporarily unavailable, cached credentials continue working within their hard expiry. For maximum control, you can self-host the entire stack.

### "How do we know our customers' keys are safe?"
We publish our full security architecture. Envelope encryption with Vault Transit KMS. Sealed memory (memguard) — keys can't be extracted even from memory dumps. Encrypted values in Redis — even a Redis compromise yields nothing usable. Multi-tenant isolation enforced at the database query level. We also offer self-hosted deployment for teams that want full control.

### "We only support OpenAI. Do we need this?"
Today, yes. But your customers will ask for Anthropic, Google, and others. ZiraLoop's auth scheme abstraction means adding a new provider is zero engineering work — just a different base_url and auth_scheme at credential creation time.

---

## Competitive Positioning Statements

### vs. Building In-House
"Building secure LLM key management in-house takes 3-6 months of senior engineering time — and then you need to maintain it. ZiraLoop gives you the same enterprise-grade security, pre-built and production-ready."

### vs. Generic API Gateways (Kong, Apigee)
"API gateways handle routing and rate limiting, but they weren't built for LLM credential custodianship. ZiraLoop adds envelope encryption, scoped sandbox tokens, multi-provider auth scheme abstraction, and instant credential revocation — things a generic gateway doesn't do."

### vs. LLM Gateways (LiteLLM, Portkey)
"LLM gateways focus on observability, model routing, and cost management — using your keys. ZiraLoop focuses on securely managing your customers' keys. They solve different problems and can work together: route through ZiraLoop for key management, then through your LLM gateway for observability."

### vs. Generic Secret Managers (Vault, AWS SM)
"Secret managers store secrets. ZiraLoop stores secrets AND proxies requests, mints scoped tokens, handles multi-provider auth schemes, caches with sub-5ms latency, and propagates revocations instantly. It's the full stack for LLM credential management, not just storage."
