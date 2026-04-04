# ZiraLoop — Product Positioning

## One-Liner

ZiraLoop is the secure proxy layer that lets platforms store, manage, and proxy LLM API keys — so customer credentials never touch your code.

## Elevator Pitch (30 seconds)

Every AI-powered platform that supports "Bring Your Own Key" faces the same problem: how do you securely store your customers' LLM API keys, proxy requests without exposing credentials, and give sandboxed agents scoped access — without building months of security infrastructure yourself?

ZiraLoop handles all of it. Store keys with envelope encryption, mint short-lived tokens for sandboxes, and proxy requests to any LLM provider with sub-5ms overhead. Your customers' keys never touch your application code.

## Category

**Developer Infrastructure / AI Security**

ZiraLoop creates a new sub-category: **LLM Credential Management & Proxy**.

It sits at the intersection of:
- Secret management (like Vault, but purpose-built for LLM keys)
- API gateway (like Kong, but LLM-aware with auth scheme abstraction)
- Token management (short-lived scoped tokens for sandboxed AI environments)

## Positioning Statement

**For** platform engineers building AI products with bring-your-own-key or sandboxed LLM access,
**who** need to securely handle customer API keys without building custom infrastructure,
**ZiraLoop is** a secure proxy layer
**that** stores LLM credentials with enterprise-grade encryption, mints scoped short-lived tokens, and proxies requests to any provider with sub-5ms latency.
**Unlike** building in-house or using generic secret managers,
**ZiraLoop** is purpose-built for LLM workflows — handling auth scheme differences across providers, multi-tenant isolation, and instant credential revocation out of the box.

## Core Value Propositions

### 1. Security Without the Engineering Cost
Envelope encryption (AES-256-GCM + Vault Transit KMS), sealed memory (memguard), and multi-tenant isolation — all pre-built. Your customers' API keys are encrypted at rest, in transit, and in cache. Even if Redis is compromised, the attacker gets nothing usable.

### 2. Sub-5ms Proxy Overhead
Three-tier cache architecture (in-memory → Redis → Postgres/Vault) means the hot path adds < 5ms to LLM requests. Your users won't notice it's there.

### 3. Scoped, Short-Lived Tokens for Sandboxes
Mint JWT tokens scoped to specific credentials with configurable TTLs (up to 24h). Perfect for giving AI agents in sandboxes just enough access, for just long enough. Revoke instantly — propagates across all instances in milliseconds.

### 4. Any Provider, One Interface
OpenAI (Bearer), Anthropic (x-api-key), Google (query param), and any other provider — ZiraLoop abstracts away auth scheme differences. Store the credential once, proxy everywhere.

### 5. Instant Revocation
Redis pub/sub propagates credential and token revocations across all proxy instances in sub-millisecond time. When a customer disconnects their key, it's dead everywhere immediately.

## Strategic Narrative

### The Problem (Status Quo)

The AI platform market is exploding. Every SaaS product is adding AI features, and many let customers bring their own LLM API keys (BYOK). This creates a critical trust problem:

**Customers are handing you the keys to their AI spend.** A leaked OpenAI key can rack up thousands of dollars. An exposed Anthropic key can access sensitive data. Customers need to trust that your platform handles their credentials with the same care a bank handles card numbers.

Most teams solve this with a quick hack: encrypt the key, store it in the database, decrypt it when needed. This works until:
- An engineer accidentally logs the plaintext key
- A cache layer stores it unencrypted
- A sandbox environment gets compromised and the key leaks
- A customer revokes access but cached credentials keep working for minutes
- You need to support a new provider with a different auth scheme
- You need to prove to enterprise customers that you handle keys securely

Building this properly requires envelope encryption, KMS integration, sealed memory, multi-tier caching with invalidation, and auth scheme abstraction. That's 3-6 months of senior engineering time — time better spent on your core product.

### The Shift

ZiraLoop exists because **LLM credential management is infrastructure, not a feature**. Just like you don't build your own payment processing (you use Stripe) or your own auth system (you use Auth0/Clerk), you shouldn't build your own LLM key management.

### The New World

With ZiraLoop, a platform engineer can:
1. Store a customer's API key with a single API call (encrypted automatically)
2. Mint a scoped sandbox token in one more call
3. Proxy LLM requests through ZiraLoop — it handles auth, streaming, and cleanup
4. Revoke access instantly when needed

The developer never touches plaintext keys. The sandbox never sees real credentials. The customer sees audit logs of exactly how their key was used.

## Proof Points (To Build)

- **Latency benchmarks**: Publish p50/p95/p99 proxy latency numbers
- **Security audit**: Third-party security review of the encryption architecture
- **Open source core**: Build trust through transparency (consider open-sourcing the proxy)
- **SOC 2 / compliance**: Enterprise readiness signal
- **Case studies**: First 3-5 customers using BYOK + sandbox patterns
