# ZiraLoop — Content Strategy

---

## Content Goals

1. **SEO**: Rank for high-intent developer keywords around LLM key management, BYOK, and API proxying
2. **Authority**: Establish ZiraLoop as the definitive voice on LLM credential security
3. **Conversion**: Drive developers from content → docs → signup → first proxy request
4. **Education**: Create the category of "LLM credential management" (it doesn't formally exist yet)

---

## Content Pillars

### Pillar 1: LLM Key Security
Everything about securely storing, managing, and proxying LLM API keys. This is our core category.

### Pillar 2: BYOK Architecture
Patterns, architectures, and best practices for platforms that let customers bring their own LLM keys.

### Pillar 3: Sandbox & Agent Security
How to give AI agents and sandboxed environments safe access to LLM APIs.

### Pillar 4: Multi-Provider LLM Infrastructure
Working with multiple LLM providers — auth schemes, streaming differences, failover patterns.

---

## Launch Content (Publish Before/At Launch)

### Blog Posts

1. **"Why Your LLM API Key Storage Is Probably Insecure"**
   - Type: Problem-awareness
   - Audience: Platform Pete, CTO Chris
   - Keywords: "store api keys securely", "api key encryption best practices"
   - Content: Walk through common mistakes (plaintext in env vars, simple AES without KMS, keys in logs, cached plaintext), explain what proper key management looks like
   - CTA: "ZiraLoop handles this → Get Started"

2. **"Envelope Encryption for API Keys: A Practical Guide"**
   - Type: Technical education
   - Audience: Platform Pete, Sandbox Sam
   - Keywords: "envelope encryption", "api key encryption kms"
   - Content: Deep technical walkthrough of envelope encryption pattern — DEK generation, AES-GCM encryption, KMS wrapping, key rotation. Show how ZiraLoop implements it.
   - CTA: "See how ZiraLoop implements this → Architecture Docs"

3. **"Building BYOK (Bring Your Own Key) for Your AI Product"**
   - Type: Architecture guide
   - Audience: Platform Pete, CTO Chris
   - Keywords: "bring your own key", "byok saas", "customer api key management"
   - Content: Full architecture guide for BYOK — key ingestion, storage, proxying, revocation, audit trails. Show the build-it-yourself approach, then show the ZiraLoop approach.
   - CTA: "Ship BYOK in days → Quick Start"

4. **"Scoped Credentials for AI Agents: Why Short-Lived Tokens Matter"**
   - Type: Problem-solution
   - Audience: Sandbox Sam
   - Keywords: "ai agent credentials", "sandbox api key", "short lived tokens"
   - Content: Why real API keys don't belong in sandboxes. Blast radius analysis. How scoped, short-lived tokens limit damage. ZiraLoop's token minting and revocation.
   - CTA: "Give agents tokens, not keys → Get Started"

5. **"Introducing ZiraLoop: The Missing Layer Between Your App and LLM Providers"** (Launch Announcement)
   - Type: Product launch
   - Audience: All personas
   - Keywords: "ziraloop", "llm proxy", "llm key management"
   - Content: What we built, why we built it, who it's for, how to get started
   - Distribution: Hacker News, Twitter, Reddit, dev newsletters

---

## Post-Launch Content Calendar (First 3 Months)

### Month 1: Foundation

| Week | Title | Type | Pillar |
|------|-------|------|--------|
| 1 | "How OpenAI, Anthropic, and Google Handle API Auth Differently (And Why It Matters)" | Technical guide | Multi-Provider |
| 2 | "3-Tier Caching for Sub-5ms API Key Resolution" | Engineering deep-dive | Key Security |
| 3 | "What Enterprise Customers Ask About Your Key Management (And How to Answer)" | Business content | BYOK Architecture |
| 4 | "Building a 'Connect Your LLM Provider' Widget: A Step-by-Step Guide" | Tutorial | BYOK Architecture |

### Month 2: Depth

| Week | Title | Type | Pillar |
|------|-------|------|--------|
| 1 | "Redis Pub/Sub for Real-Time Credential Revocation" | Engineering deep-dive | Key Security |
| 2 | "Securing AI Agent Sandboxes: A Complete Guide" | Technical guide | Sandbox Security |
| 3 | "API Key Rotation Without Downtime" | Tutorial | Key Security |
| 4 | "Multi-Tenant Isolation Patterns for AI Platforms" | Architecture guide | BYOK Architecture |

### Month 3: Scale

| Week | Title | Type | Pillar |
|------|-------|------|--------|
| 1 | "Comparing LLM Proxy Architectures: Gateway vs. Sidecar vs. Library" | Comparison | Multi-Provider |
| 2 | "Building SOC 2 Compliance for Your AI Product (The Key Management Part)" | Business content | Key Security |
| 3 | "Load Testing Your LLM Proxy: Tools and Methodology" | Engineering deep-dive | Multi-Provider |
| 4 | Case study: [First Customer] | Social proof | BYOK Architecture |

---

## SEO Keyword Targets

### High-Intent (Bottom of Funnel)
- "llm api key proxy" — no dominant result, greenfield
- "llm api key management" — no dominant result
- "bring your own key llm" — low competition
- "byok api key storage" — low competition
- "secure api key proxy" — moderate competition
- "sandbox api credentials" — low competition

### Mid-Intent (Middle of Funnel)
- "how to store api keys securely" — moderate competition, high volume
- "envelope encryption api keys" — low competition
- "short lived api tokens" — moderate competition
- "multi-tenant api key management" — low competition
- "api key rotation best practices" — moderate competition

### Educational (Top of Funnel)
- "api key security best practices" — high competition, high volume
- "byok saas architecture" — low competition
- "ai agent security" — growing volume
- "llm proxy architecture" — low competition

---

## Distribution Channels

### Owned
- Blog (ziraloop.com/blog)
- Documentation (ziraloop.com/docs)
- Changelog (ziraloop.com/changelog)
- Email newsletter (for updates and new content)

### Earned
- Hacker News (launch post, major technical blog posts)
- Reddit: r/programming, r/devops, r/MachineLearning, r/SideProject
- Twitter/X: Developer community, AI builder community
- Dev.to and Hashnode cross-posts
- Developer newsletters: TLDR, Bytes, Changelog

### Community
- Discord/Slack community for users
- GitHub Discussions (if open-source)
- Stack Overflow answers (answer questions about API key security, link to guides)

---

## Content Production Notes

- Every blog post should include working code examples
- Architecture posts should include diagrams (Mermaid or Excalidraw style)
- Every post should have a clear, non-pushy CTA to docs or signup
- Technical depth > breadth. One deep post beats three shallow ones.
- Write for developers who can sniff out marketing BS instantly
- No stock photos. Use diagrams, code, and terminal screenshots.
