# ZiraLoop — Website Strategy & Architecture

---

## Site Goals

1. **Convert developers** to sign up / start a free trial (primary)
2. **Educate technical buyers** on the architecture and security model
3. **Enable enterprise sales** with proof points and compliance info
4. **Build SEO authority** around LLM key management, BYOK infrastructure, and sandbox credentials

---

## Site Architecture

```
ziraloop.com/
├── / (Homepage)
├── /docs (Documentation — separate subdomain or subpath)
│   ├── /docs/quickstart
│   ├── /docs/architecture
│   ├── /docs/api-reference
│   ├── /docs/sdks
│   ├── /docs/security
│   └── /docs/self-hosting
├── /pricing
├── /use-cases/
│   ├── /use-cases/bring-your-own-key
│   ├── /use-cases/sandbox-credentials
│   ├── /use-cases/connect-provider-widget
│   └── /use-cases/multi-tenant-ai-platforms
├── /blog
├── /security (Security overview + whitepaper)
├── /changelog
├── /about
└── /contact
```

---

## Page-by-Page Strategy

### Homepage (/)

**Purpose**: Convert developers to try ZiraLoop. Explain what it is, why it matters, and get them to the docs/quickstart.

**Structure**:

1. **Hero Section**
   - Headline + subheadline
   - Primary CTA: "Get Started" → /docs/quickstart
   - Secondary CTA: "View Docs" → /docs
   - Code snippet showing a 3-step integration (store key → mint token → proxy request)

2. **Problem Statement**
   - "Your customers trust you with their API keys. Are you handling them right?"
   - Visual showing the common (insecure) approach vs. the ZiraLoop approach

3. **How It Works** (3 steps)
   - Step 1: Store credentials (encrypted automatically)
   - Step 2: Mint scoped tokens (for sandboxes or sessions)
   - Step 3: Proxy requests (to any LLM provider)
   - Each step with a code snippet and a visual

4. **Key Features** (4-6 cards)
   - Envelope encryption
   - Sub-5ms proxy overhead
   - Short-lived scoped tokens
   - Any provider, one interface
   - Instant revocation
   - Multi-tenant isolation

5. **Architecture Overview**
   - Simplified diagram showing the 3-tier cache and proxy flow
   - Link to full architecture docs

6. **Use Cases** (3-4 cards linking to use case pages)
   - BYOK platforms
   - Sandbox credentials
   - Connect Provider widget
   - Multi-tenant AI platforms

7. **Social Proof** (when available)
   - Customer logos
   - Testimonial quotes
   - "Trusted by X developers" counter

8. **Open Source / Transparency Section**
   - "See exactly how your keys are protected"
   - Link to GitHub repo / architecture docs

9. **Final CTA**
   - "Start securing LLM keys in 15 minutes"
   - Get Started button

**Design notes**:
- Dark theme (conveys security, developer-focused)
- Monospace font for code, clean sans-serif for copy
- Minimal animation — focus on clarity
- Mobile-responsive but desktop-first (developer audience)

### Pricing (/pricing)

**Purpose**: Make the buy decision easy. Show clear tiers with no surprises.

**Recommended Tier Structure**:

| | Free | Pro | Enterprise |
|---|---|---|---|
| **Price** | $0 | $X/mo | Custom |
| **Credentials stored** | 10 | Unlimited | Unlimited |
| **Proxy requests/mo** | 10,000 | 500,000 | Unlimited |
| **Token mints/mo** | 100 | 10,000 | Unlimited |
| **Providers** | All | All | All |
| **Audit log retention** | 7 days | 90 days | Custom |
| **Support** | Community | Email | Dedicated + SLA |
| **Self-hosted** | No | No | Yes |
| **SSO/SAML** | No | No | Yes |

**Pricing philosophy**:
- Usage-based component (proxy requests) so price scales with value
- Generous free tier to drive PLG adoption
- Enterprise tier for self-hosted, compliance, and dedicated support

**Page elements**:
- Tier comparison table
- FAQ section (billing, overages, what counts as a request)
- "Talk to us" CTA for Enterprise
- Annual discount option

### Use Case Pages (/use-cases/*)

**Purpose**: SEO landing pages + conversion pages for specific buyer intents.

**Each page follows this structure**:
1. Problem statement specific to the use case
2. How ZiraLoop solves it
3. Architecture diagram for this use case
4. Code example specific to this use case
5. Relevant features highlighted
6. CTA: Get Started / Read Docs

#### /use-cases/bring-your-own-key
**Target keywords**: "bring your own api key", "byok llm", "customer api key storage"
**Hook**: "Your customers want to use their own LLM keys. You need to store them securely. Here's how."

#### /use-cases/sandbox-credentials
**Target keywords**: "sandbox api key", "temporary llm credentials", "short lived api tokens"
**Hook**: "Your agents run in sandboxes. Real API keys don't belong there."

#### /use-cases/connect-provider-widget
**Target keywords**: "connect llm provider", "api key widget", "byok widget react"
**Hook**: "Add a 'Connect Your LLM' flow to your app in 30 minutes."

#### /use-cases/multi-tenant-ai-platforms
**Target keywords**: "multi-tenant llm", "multi-tenant api key management"
**Hook**: "Every tenant gets isolated LLM access. By architecture, not by accident."

### Security Page (/security)

**Purpose**: Build trust with technical buyers and satisfy enterprise security reviews.

**Content**:
- Encryption architecture (envelope encryption diagram)
- Key management (Vault Transit KMS)
- Data at rest / in transit / in cache encryption
- Memory protection (memguard, sealed memory)
- Multi-tenant isolation guarantees
- Audit logging capabilities
- Container security (distroless, nonroot)
- Compliance status (SOC 2 roadmap)
- Responsible disclosure policy
- Link to download security whitepaper (gated for lead gen)

### Documentation (/docs)

**Purpose**: Primary conversion and retention tool. Developers evaluate by reading docs.

**Key sections**:
- **Quickstart**: < 15 minutes to first proxied request (highest priority page)
- **Architecture**: Full technical deep-dive (caching, encryption, proxy flow)
- **API Reference**: OpenAPI spec, every endpoint documented
- **SDKs**: TypeScript, Python, Go clients
- **Security Model**: Detailed security architecture
- **Self-Hosting Guide**: Docker Compose and Kubernetes deployment
- **Guides**: Provider-specific setup (OpenAI, Anthropic, Google, etc.)

**Docs tooling**: Mintlify, Nextra, or custom — should feel fast, searchable, and developer-friendly.

### Blog (/blog)

**Purpose**: SEO, thought leadership, and developer education.

**Content strategy**: See `06-content-strategy.md`

---

## Technical Implementation Recommendations

### Stack
- **Framework**: Next.js (App Router) or Astro — fast, SEO-friendly, developer-familiar
- **Styling**: Tailwind CSS — dark theme, utility-first
- **Docs**: Mintlify or Fumadocs — developer-focused docs platform
- **Analytics**: Plausible or PostHog — privacy-friendly, developer-friendly
- **CMS for blog**: MDX files in the repo (developers prefer this to a CMS)

### SEO Foundations
- Server-rendered pages (SSR or SSG)
- Proper meta tags, Open Graph, and Twitter cards on every page
- Sitemap.xml and robots.txt
- Structured data (Organization, SoftwareApplication, FAQ)
- Fast Core Web Vitals (target all green)

### Conversion Tracking
- Sign-up events
- Docs page views (which pages correlate with conversion?)
- Quickstart completion
- First API call
- Time-to-first-proxy-request

---

## Launch Priority

**Phase 1 (Pre-launch / Landing page)**:
1. Homepage with email capture
2. Quickstart docs page
3. Architecture docs page

**Phase 2 (Launch)**:
4. Full docs site
5. Pricing page
6. Security page
7. Blog with 3-5 launch posts

**Phase 3 (Growth)**:
8. Use case pages
9. Widget demo page
10. Customer case studies
11. Comparison pages (vs. building in-house, vs. alternatives)
