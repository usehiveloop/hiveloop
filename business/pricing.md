# Hiveloop Pricing Model

**Last updated:** 2026-04-24
**Status:** Active (replaces the per-agent / per-run Polar-era model)

## Summary

Credit-based inference billing with unlimited agent runs. Three subscription tiers plus non-expiring top-up bundles. Platform keys by default; BYOK opt-in per agent or conversation.

| Plan | Monthly (USD) | Annual (USD) | Credits/month |
|---|---|---|---|
| Starter | $9 | $86 (20% off) | 9,000 |
| Pro | $39 | $374 (20% off) | 39,000 |
| Business | $99 | $950 (20% off) | 99,000 |

| Bundle | Price (USD) | Credits | $/credit |
|---|---|---|---|
| Small | $5 | 4,000 | $0.00125 (25% premium) |
| Medium | $20 | 18,000 | $0.00111 (11% premium) |
| Large | $50 | 48,000 | $0.00104 (4% premium) |

New accounts receive **150 credits** on sign-up as a trial. No "Free" plan is shown on the pricing page — the trial lives behind a "Get started for free" CTA.

## Credit economics

### Unit

**1 credit = $0.001** of inference + sandbox cost at list price.

At a 75% gross-margin target, each credit reserves $0.00025 of COGS budget. The charge for any conversation is:

```
credits = COGS × 4,000
```

Meaning the user pays 4× what the underlying work cost us.

### Plan allotment derivation

```
Plan price × (1 − 0.75) / $0.00025 = plan credits

Starter:   $9  × 0.25 / $0.00025 =  9,000 credits
Pro:       $39 × 0.25 / $0.00025 = 39,000 credits
Business:  $99 × 0.25 / $0.00025 = 99,000 credits
```

### What COGS includes

- **Inference**: tokens consumed on the active model (GLM 5.1 by default — $0.437/Mtok in, $4.40/Mtok out).
- **Sandbox**: bare-metal amortised cost per minute of active sandbox time.
- **Not included**: platform infra (servers, DB, egress), dev, support. Those live inside the plan margin.

### Sandbox size multiplier

Default sandbox time is embedded in the inference credit charge. Larger sandboxes are charged additional per-minute credits only while they're running:

| Sandbox tier | Resources | Extra credits/minute |
|---|---|---|
| Default | ≤1 vCPU, ≤2 GB RAM | 0 (included) |
| Large | 2 vCPU, 4 GB RAM | +0.2 |
| XL | 4 vCPU, 8 GB RAM | +0.5 |
| XXL | 8 vCPU, 16 GB RAM | +1.2 |

Assumes default-sandbox COGS ~$0.0008/min at moderate bare-metal utilisation. Tune after real infrastructure-cost data stabilises.

### Per-conversation cost (GLM 5.1, no caching, default sandbox)

| Workload | Input | Output | Inference | Sandbox | Total COGS | Credits |
|---|---|---|---|---|---|---|
| Light | 10k | 1k | $0.0088 | $0.0016 | $0.0104 | 42 |
| Typical | 30k | 4k | $0.0307 | $0.0040 | $0.0347 | 139 |
| Heavy | 200k | 15k | $0.1534 | $0.0120 | $0.1654 | 662 |
| Extreme | 500k | 30k | $0.3505 | $0.0240 | $0.3745 | 1,498 |

### Runs per plan (typical workload)

| Plan | Typical conversations | Heavy conversations |
|---|---|---|
| Starter | ~64 | ~13 |
| Pro | ~280 | ~58 |
| Business | ~712 | ~149 |

## BYOK (bring your own keys)

Every plan supports BYOK, toggleable per agent or per conversation.

- **Inference cost**: skipped — zero credits consumed for tokens.
- **Sandbox cost**: still charged at the active sandbox tier's credit rate.
- **Plan price**: unchanged.
- **No tier gating**: BYOK is available on every plan.

A BYOK-heavy Pro subscriber's plan stretches roughly **6–10× further** than a platform-keys user, depending on workload weight.

## Free trial (hidden)

- 150 credits on sign-up (~6 typical conversations).
- Credits expire **30 days after sign-up** if unclaimed.
- No card required.
- Marketing message: *"Get started for free. 150 credits on sign-up."*
- No "Free" column on the pricing page.
- When credits exhaust, the user can't start new conversations until they subscribe or buy a bundle.

## Annual discount

Flat **20% off** when billed annually. Revenue is recognised monthly; credits are granted monthly on the anniversary of each billing cycle (not front-loaded for the year) to prevent end-of-year credit dumps.

## Credit rollover

- **Subscription credits** (`reason = plan_grant`): expire at end of billing period. Unused credits are forfeited.
- **Bundle credits** (`reason = topup`): never expire. Persist in ledger until spent.

Balance computation sums both pools. The billing-period renewal process zeroes unconsumed `plan_grant` rows and inserts a fresh grant entry.

## Mid-conversation exhaustion

**Hard abort.** The LLM proxy returns 402 Payment Required on the next call when balance would go below zero. The conversation is marked `status=credits_exhausted`. No soft overdraft.

Users get an automated email at 80% of monthly plan consumption as an early warning.

## Currency

Pricing page displays USD and NGN side by side with a currency toggle. Conversion rate: **1 USD = 1,500 NGN** (hardcoded at this version).

- Internal accounting is USD.
- NGN rate is reviewed monthly; plan NGN prices update when the rate diverges ≥10% from the current peg.

## Gross margin

### By construction at 100% plan consumption: **75% GM**

### Blended by utilisation

| Plan utilisation | Effective GM |
|---|---|
| 20% | 95% |
| 40% | 90% |
| 60% | 85% |
| 80% | 80% |
| 100% | 75% |

Realistic blended GM for paid subscribers: **80–87%** based on SaaS utilisation norms.

### Bundle margins (always above floor)

| Bundle | Margin at 100% consumption |
|---|---|
| Small ($5) | 80% |
| Medium ($20) | 77.5% |
| Large ($50) | 76% |

### Post-launch margin lever: prompt caching

With 80% input cache-hit rate (0.1× read cost, 1.25× one-time write, amortised over ~5 LLM calls/conversation):

- Typical COGS drops from $0.0347 → ~$0.0255 (−27%)
- If user charge is held at 139 credits: margin on that run rises from 75% → ~**82%** (+7 GM points)

Not currently enabled. Tracked as a margin lever for rollout after GA.

## Sensitivity

### Model price shift

Margins hold at 75% because credit charge scales linearly with COGS. What shifts is user-perceived conversations-per-plan:

| Input price change (vs $0.437/Mtok) | Typical runs on Pro |
|---|---|
| −50% | ~400 |
| Baseline | ~280 |
| +50% | ~215 |

### Plan-utilisation drift

If real subscriber workloads skew heavier than the "typical" profile (30k in / 4k out), users hit the paywall sooner than modelled. Metrics to monitor:

- Median credits consumed per paying subscriber per billing cycle
- % of subscribers paywalled before period end
- Top-up bundle conversion rate
- Upgrade rate from Starter → Pro

### Currency risk (NGN)

NGN rate is pegged at 1,500/USD. If the naira weakens to 2,000/USD and we don't re-price, Nigerian subscribers effectively pay ~25% less in USD terms. Review monthly.

## Revenue model (back-of-envelope)

Assumes 65% blended utilisation, 70% of paying customers on Pro, 15% each on Starter and Business:

| | Starter | Pro | Business | Blended |
|---|---|---|---|---|
| Price/mo | $9 | $39 | $99 | — |
| Subscriber share | 15% | 70% | 15% | — |
| Weighted ARPU | $1.35 | $27.30 | $14.85 | **$43.50/mo** |
| GM per subscriber | 85% | 85% | 85% | **~$37/mo** |

## Open product decisions

These are **not yet decided** and shape follow-up pricing work:

- **Plan feature differentiation** — do Starter/Pro/Business differ only by credit allotment, or do we stratify BYOK access, team seats, sandbox-size access, SSO, audit-log retention, priority support?
- **Team / seat billing** — per-seat add-on vs. included count per plan.
- **Multi-year enterprise contracts** — custom pricing, committed-spend discounts.
- **Overdraft / grace period** — soft buffer for legacy enterprise accounts.
- **LLM model mix beyond GLM 5.1** — cost pass-through vs. single credit rate across models.

## Changelog

- **2026-04-24** — Replaced per-agent/per-run Polar model with credit-based inference billing. Three tiers (Starter $9 / Pro $39 / Business $99), unlimited runs, 20% annual discount, NGN + USD currency support, hidden 150-credit trial.
- **Historical** — See git history for the Polar-era model (per-agent monthly, per-run overage).
