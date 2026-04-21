# HiveLoop Pricing Model

## Summary

| Plan | Price | Target |
|------|-------|--------|
| Free | $0/month | Exploring & prototyping |
| Pro | $4.99/agent/month | Teams shipping to production |
| Dedicated sandbox add-on | +$2/agent/month | Agents needing shell, filesystem, repo cloning |

## Plan details

### Free

- 1 agent
- 100 runs/month
- 1 concurrent run
- Shared sandbox only
- Unlimited AI credentials
- Unlimited integrations & connections
- 20+ LLM providers
- MCP tool support
- Human-in-the-loop approvals
- Conversations & streaming
- TypeScript SDK
- Community support

### Pro ($4.99/agent/month)

Everything in Free, plus:

- Unlimited agents
- 300 runs/agent/month included
- $0.01 per extra run (shared sandbox)
- $0.05 per extra run (dedicated sandbox)
- 5 concurrent runs per agent
- Dedicated sandbox (+$2/agent/month add-on)
- Agent Forge (auto-optimization)
- Persistent agent memory (1 GB/agent)
- Custom sandbox templates
- Identity scoping & isolation
- Per-identity rate limiting
- Advanced analytics & audit logs
- Custom domains
- API key scopes
- Priority support

### Dedicated sandbox add-on (+$2/agent/month)

- Ephemeral sandboxes (spin up per run, tear down after)
- Runs drawn from the Pro plan's 300/agent/month included allocation
- Additional runs at $0.05/run overage
- Full system access: shell, filesystem, code execution
- Repo cloning, build tools, linters
- 10 GB disk per sandbox instance

---

## Infrastructure cost model

### Server: Hetzner AX101 (~$110/month)

- AMD Ryzen 9 5950X: 16 cores / 32 threads
- 128 GB RAM
- 2x 1.92 TB NVMe SSD

### Shared agents

Shared agents are stateless API relays. They receive triggers, call LLMs (using the user's own API key), call tools via API, and respond. Minimal compute.

```
Per agent: ~50 MB RAM, ~0.01 CPU idle
Capacity per box: ~2,000 agents
Cost per agent: $110 / 2,000 = $0.055/month
```

At $4.99/agent/month, shared agents are ~99% margin before revenue share and payment processing.

### Dedicated agents (ephemeral sandboxes)

Dedicated sandboxes are ephemeral — they spin up for each run and tear down after. This is dramatically more efficient than persistent always-on sandboxes.

```
Per run lifecycle:
  Sandbox spin-up (Daytona): ~2s
  Git clone (layer cached):  ~5-10s
  Agent does work + LLM:     ~60-120s
  Teardown:                  instant
  Average run duration:      ~2 minutes
  Resources per run:         2 vCPU, 2 GB RAM

Per box capacity:
  Max concurrent runs:       16 (32 threads / 2 vCPU)
  Runs per hour:             480 (16 concurrent x 30 runs/hour)
  Monthly at 50% util:       ~172,800 runs/month
  Cost per run:              $110 / 172,800 = ~$0.0006

  With overhead (disk I/O, network, orchestration): ~$0.003/run
```

### Why ephemeral, not persistent

A persistent always-on sandbox reserves 2 vCPU + 2 GB RAM 24/7 for a single agent. That yields only ~48 agents per box at $6.88/agent/month cost — making a $2 add-on impossible.

Ephemeral sandboxes share resources across thousands of agents. A box serving 172,800 runs/month can handle thousands of agents averaging 300 runs each. The cost per run drops to fractions of a cent.

### Cost per run comparison

| Model | Agents per box | Cost per agent/month | Notes |
|-------|---------------|---------------------|-------|
| Persistent (always-on) | ~48 | $6.88 | Wastes resources when idle |
| Ephemeral (per-run) | ~576 at 300 runs each | $0.90 | Only uses resources during runs |

---

## Payment processing

### Polar (4% + $0.40 per transaction)

All billing runs through [Polar](https://polar.sh), a payment processor built for developer tools and open-source monetization.

```
Fee structure:
  Percentage fee:  4% of total charge
  Flat fee:        $0.40 per transaction (per user per billing cycle)

Example fees:
  Single shared agent ($4.99):      4% × $4.99 + $0.40 = $0.60
  Single dedicated agent ($6.99):   4% × $6.99 + $0.40 = $0.68
  5 agents mixed ($27.96):          4% × $27.96 + $0.40 = $1.52
```

Polar handles:
- Subscription billing and invoicing
- Marketplace payouts to creators (native split support)
- Usage-based overage charges
- Tax compliance and receipts

The flat $0.40/transaction fee is per user per billing cycle, not per agent. This means the fee impact decreases as users add more agents — the percentage component scales linearly but the flat fee is amortized.

---

## Revenue share model

### Structure

Revenue share applies to **marketplace agents only** (agents built by third-party creators and installed by users). Self-built agents have no revenue share.

- **Creator gets**: 50% of the base agent fee ($4.99) = $2.50/install/month
- **Platform keeps**: 50% of the base agent fee ($4.99) = $2.50/install/month
- **Infrastructure add-ons** (dedicated sandbox, overages, extra memory): 100% to platform

Revenue share does NOT apply to:
- Dedicated sandbox add-on ($2/agent/month)
- Overage run fees ($0.01/run)
- Extra memory ($1/GB/month)

### Per-agent economics

Worst-case scenario: single-agent user (maximum Polar flat fee impact).

| | User pays | Creator gets | Polar fee | Infra cost | Profit |
|---|---|---|---|---|---|
| Shared (marketplace) | $4.99 | $2.50 | $0.60 | $0.06 | $1.84 |
| Shared (self-built) | $4.99 | $0 | $0.60 | $0.06 | $4.34 |
| Dedicated (marketplace) | $6.99 | $2.50 | $0.68 | $0.90 | $2.91 |
| Dedicated (self-built) | $6.99 | $0 | $0.68 | $0.90 | $5.41 |

### Why 50/50

Unlike app stores where the platform simply distributes software, HiveLoop does the heavy lifting:

- Platform provides infrastructure, sandboxes, observability, memory, connections
- Platform handles billing, auth, scaling, uptime
- Platform built the marketplace, SDK, and orchestration layer
- Creator's work is front-loaded: build the agent once, configure tools, write the prompt

A 50/50 split reflects this equal partnership:

- Creator brings the value (the agent itself + promotion)
- Platform brings the infrastructure and distribution
- At 1,000 installs, a creator earns $2,500/month — meaningful income with zero ops burden
- "You build it, we split it" is a clean, easy-to-communicate story

Comparison to other marketplaces:

- Apple App Store: 70/30 (creator/platform) — but Apple doesn't run the apps
- Shopify App Store: 80/20 — but merchants run their own stores
- HiveLoop: 50/50 — platform runs everything, creator just builds and promotes

The key difference: on HiveLoop, the creator has zero ongoing operational cost. No servers, no billing, no support infrastructure. The platform bears 100% of the operational burden. 50/50 is fair for that level of service.

---

## Scaling projections

Assumptions:
- Average 2.5 agents per paying user
- 80% shared / 20% dedicated agent mix
- 70% marketplace / 30% self-built
- 300 included runs/agent/month
- 40% of agents exceed included runs, averaging 175 extra runs
- Polar processing: (4% × total revenue) + ($0.40 × number of users)

### Revenue model

Revenue share is 50/50 on base fee ($4.99) for marketplace agents only. Infrastructure add-ons are 100% platform.

| Paying users | Total agents | Shared | Dedicated | Gross revenue | To creators | Polar fees | Infra cost | Net profit |
|---|---|---|---|---|---|---|---|---|
| 1,500 | 3,750 | 3,000 | 750 | $24,938 | $6,563 | $1,598 | $330 | $16,447 |
| 5,000 | 12,500 | 10,000 | 2,500 | $83,125 | $21,875 | $5,325 | $1,100 | $54,825 |
| 15,000 | 37,500 | 30,000 | 7,500 | $249,375 | $65,625 | $15,975 | $3,080 | $164,695 |
| 50,000 | 125,000 | 100,000 | 25,000 | $831,250 | $218,750 | $53,250 | $10,200 | $549,050 |

### Infrastructure scaling

| Paying users | Shared agents | Shared boxes | Dedicated agents | Dedicated runs/mo | Dedicated boxes | Total boxes | Monthly infra |
|---|---|---|---|---|---|---|---|
| 1,500 | 3,000 | 2 | 750 | 225,000 | 1 | 3 | $330 |
| 5,000 | 10,000 | 5 | 2,500 | 750,000 | 5 | 10 | $1,100 |
| 15,000 | 30,000 | 15 | 7,500 | 2,250,000 | 13 | 28 | $3,080 |
| 50,000 | 100,000 | 50 | 25,000 | 7,500,000 | 43 | 93 | $10,200 |

Infra stays below 3-4% of revenue at every scale. Polar fees are the larger cost center at ~6.4% of gross revenue, but this is competitive for a processor that handles marketplace payouts natively.

### Growth milestones

| Milestone | Paying users needed | Monthly net |
|---|---|---|
| $10k/month | ~910 | $10,000 |
| $50k/month | ~4,560 | $50,000 |
| $100k/month | ~9,120 | $100,000 |
| $1M/month | ~91,200 | $1,000,000 |

---

## Competitive positioning

The $4.99/agent/month price point makes the comparison with individual SaaS tools dramatic:

| Tool | Monthly cost | HiveLoop equivalent |
|------|-------------|-------------------|
| CodeRabbit (code review) | $30/month | $4.99/month (shared) or $6.99 (dedicated) |
| Cursor (AI coding) | $20/month | $4.99/month |
| Lovable (UI generation) | $25/month | $4.99/month |
| Devin (autonomous dev) | $500/month | $6.99/month (dedicated) |
| Jasper (content writing) | $49/month | $4.99/month |
| Intercom Fin (support) | $99/month | $4.99/month |
| **Total** | **$723/month** | **$29.94 - $41.94/month** |

Users bring their own LLM API keys, so the biggest variable cost (inference) is not borne by the platform. This is the core of the business model — Hiveloop is an orchestration and infrastructure layer, not a model provider.

---

## Key pricing decisions and rationale

### Why $4.99 and not $5?

Psychological pricing. $4.99 feels like "under $5" and anchors in the $4 range. The $0.01 difference has outsized impact on perception. It's also the sweet spot for per-agent pricing — cheap enough that adding a second or third agent feels effortless, expensive enough to signal quality.

### Why $2 for dedicated and not $5?

At $2, a dedicated code review agent costs $6.99/month total. Compared to CodeRabbit at $30/month, that's a 4x savings that's immediately compelling. The "wow" factor drives conversion.

At $5, the total would be $9.99 — still cheaper than competitors, but the gap narrows and the story weakens. Growth > margin at this stage.

### Why 300 runs instead of 500?

Most agents use 100-200 runs/month. Including 300 covers the vast majority of users without hitting overages, keeping the pricing simple and predictable. Users who exceed are power users running heavy workloads who understand usage-based billing.

The lower allocation also means tighter unit economics and fewer resources consumed by the included tier. At 300 runs, the math works cleanly — the $0.05/run overage kicks in at a point where users are genuinely heavy users, not just slightly above average.

### Why Polar?

Polar is built specifically for developer tools and open-source monetization. Unlike Stripe or Paddle, Polar handles marketplace payouts natively — meaning creator revenue share can be split automatically without building custom payout infrastructure.

At 4% + $0.40, Polar's fees are higher than raw Stripe (2.9% + $0.30), but the native marketplace split support, developer-focused billing UI, and built-in tax compliance eliminate the need for custom billing infrastructure that would cost far more to build and maintain.

### Why ephemeral sandboxes?

Persistent sandboxes (always-on) can only fit ~48 per server, costing $6.88/agent/month. That makes any add-on below $7 unprofitable.

Ephemeral sandboxes (spin up per run, tear down after) share resources across thousands of agents. Cost per run drops to ~$0.003. This unlocks the $2 price point with healthy margins.

Sandbox cold start is ~2 seconds using self-hosted Daytona, making ephemeral sandboxes viable even for latency-sensitive workflows. No trade-off on user experience.

### Why revenue share on base fee only?

Creators built the agent — they deserve a cut of the agent fee. They didn't build the infrastructure — sandbox compute, storage, and overages are platform costs that the platform should price and retain.

This keeps the creator incentive simple ($2.50/install/month) while giving the platform full control over infrastructure economics.

### Why 50/50 split and not 70/30?

Traditional app store splits (70/30 creator/platform) don't apply here because those platforms only distribute software — the developer runs their own servers, handles their own billing, manages their own uptime.

On HiveLoop, the platform does all of that:
- Runs the agent infrastructure 24/7
- Provides sandboxes, memory, observability, connections
- Handles billing, auth, scaling
- Built the SDK, marketplace, and orchestration layer

The creator's contribution is front-loaded: build the agent, configure it, promote it. The platform's cost is ongoing every month. 50/50 reflects this reality.

At 1,000 installs, a creator still earns $2,500/month with zero operational burden — that's compelling enough to attract quality builders.

---

## Risks and mitigations

### Risk: Heavy users on dedicated sandbox abuse the $2 add-on

Mitigation: The $0.05/run overage kicks in after 300 runs. An extreme user doing 2,000 runs/month pays $6.99 + (1,700 × $0.05) = $91.99/month. Infra cost: ~$6. Highly profitable. The $0.05/run rate naturally discourages abuse while remaining fair for moderate overages.

### Risk: Revenue share makes marketplace agents unprofitable

Mitigation: At 50/50, the platform keeps $2.50/agent on shared marketplace agents — 45x the infra cost. Even after Polar fees ($0.60 worst case), the margin is $1.84/agent. Infra add-ons (100% kept) cover dedicated sandbox costs. Self-built agents (30% of total, no rev share) provide additional margin. Even in a worst-case all-marketplace scenario, margins remain healthy.

### Risk: Polar fees eat into margins at scale

Mitigation: At 6.4% of gross revenue, Polar fees are the largest cost center after creator payouts — but they replace the entire billing, payout, and tax compliance stack. Building this in-house would cost engineering time worth far more than the fee delta vs. raw Stripe. If Polar fees become problematic at very high scale (50K+ users), the option to negotiate volume pricing or migrate to a custom billing stack remains open.

### Risk: Ephemeral sandbox cold start is too slow

Mitigation: Self-hosted Daytona achieves ~2 second sandbox spin-up. Combined with layer-cached git clones, most runs are fully operational within 5-10 seconds. This is fast enough for event-driven workflows (PR reviews, ticket handling, deploys) which are the primary dedicated sandbox use case.

### Risk: Users bypass platform by self-hosting

Mitigation: Self-hosting is supported and encouraged (listed as a feature). It builds trust and community. Revenue comes from the managed platform's convenience — marketplace, observability, one-click deploys. Users who self-host often convert to managed when they scale.
