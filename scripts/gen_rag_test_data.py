#!/usr/bin/env python3
"""
Generate 10,000 realistic documents across 40 source types for RAG testing.

Each output file is a complete IngestBatch gRPC request (250 docs) ready to
pipe into grpcurl. Content is templated per source so that:
  * Semantic embeddings cluster meaningfully by topic (auth, billing, k8s...)
  * Each source has its own "voice" (Slack is chatty, Confluence is formal)
  * Metadata is shaped like the real source (Linear has ENG-1234, Jira has
    PROJ-567, GitHub has PR numbers, Slack has channels, etc.)

Output: rag-test-data/docs/docs_NN_<source>.json
"""

import json
import os
import random
from datetime import datetime, timedelta
from pathlib import Path

random.seed(42)

ROOT = Path(__file__).resolve().parent.parent
OUT_DIR = ROOT / "rag-test-data" / "docs"
OUT_DIR.mkdir(parents=True, exist_ok=True)

DATASET_NAME = "hiveloop_demo"
ORGS = ["org-acme", "org-globex", "org-initech"]
USERS = [f"user{i:02d}@example.com" for i in range(1, 51)]
TEAMS = ["backend", "frontend", "platform", "data", "mobile", "security", "sre"]
ENVIRONMENTS = ["production", "staging", "dev", "preview"]

# ---------------------------------------------------------------------------
# Topic seeds: each is a (name, sentences[]) pair. Sentences are fragments we
# sample + shuffle + stitch, so generated docs are coherent per topic.
# ---------------------------------------------------------------------------

TOPICS = {
    "auth": [
        "Users logged in via SSO are redirected to the wrong org after the SAML assertion is consumed.",
        "Refresh token rotation broke after the move to Keycloak 24 — tokens issued before the upgrade are being rejected.",
        "WebAuthn passkey enrollment fails on Safari 17 when the user has multiple resident credentials registered.",
        "Rate-limit the /auth/otp endpoint to 5 requests per minute per email; currently it's unbounded.",
        "JWTs issued by the old service had 1h expiry; the new service defaults to 15m and nothing renewed.",
        "Audit log entries for failed MFA challenges are missing the user_agent field.",
        "Service accounts need to support client_credentials grant for the new partner integration.",
        "Session cookie SameSite setting needs to change from Lax to Strict before the Firefox 122 rollout.",
        "Password reset tokens are not being invalidated after a successful reset — they remain valid for 15 minutes.",
        "OIDC discovery URL returns 502 from behind the load balancer; the health check path is wrong.",
    ],
    "billing": [
        "Stripe webhook for customer.subscription.trial_will_end is not being dispatched to our handler.",
        "Invoice PDF generation times out for customers with > 500 line items — need to paginate.",
        "Proration is wrong when a customer downgrades mid-cycle; we're crediting too much.",
        "Usage-based metering events are being counted twice on retry when the initial request times out.",
        "EU VAT calculation is off by the rounding rule — we round per-line, Stripe rounds per-invoice.",
        "Failed payment retry cadence needs to match Dunning best practices: day 1, 3, 5, 7, then cancel.",
        "Credit notes issued via the dashboard are not syncing back to QuickBooks.",
        "The customer portal is letting users cancel subscriptions mid-term without refund logic.",
        "Coupon redemptions are not being deduplicated — same code can be applied twice in the same checkout.",
        "Annual plan renewal should create a draft invoice 7 days before renewal, not 3.",
    ],
    "kubernetes": [
        "Rolling deployments are causing a 2-3 second error spike because readiness probes lag behind container start.",
        "HPA keeps scaling up then immediately back down — the target CPU is oscillating around the threshold.",
        "Ingress controller is returning 502s intermittently; config reload timing is racing with upstream rotation.",
        "Pod disruption budget is set to 0 for the payments service — we can't drain the node.",
        "The new RBAC role for the oncall group needs get,list,watch on pods and logs across all namespaces.",
        "NetworkPolicy default-deny was applied to the staging namespace and broke the egress to the DB.",
        "CronJob for nightly ETL is overlapping because concurrencyPolicy was left at the default Allow.",
        "The cluster autoscaler can't scale from zero because the instance group has no node labels.",
        "Pod affinity rule is too strict — we want soft-anti-affinity across zones, currently set to hard.",
        "StatefulSet volume claim template was changed but existing PVCs aren't being updated — they need to be recreated.",
    ],
    "observability": [
        "Prometheus query for p99 latency returns NaN when the service has < 30 requests in the window.",
        "Grafana dashboard for the checkout funnel lost its data source binding after the Grafana 10 upgrade.",
        "OpenTelemetry span attribute `http.status_code` is missing from the ingress traces.",
        "Log volume doubled this week — Loki ingestion is near quota. Need to audit verbose logs from the new service.",
        "SLO burn-rate alert is too noisy during weekends. Multi-window multi-burn-rate policy needed.",
        "PagerDuty integration is missing the severity field on alerts coming from Alertmanager.",
        "Distributed tracing across our Go → Rust gRPC boundary is not propagating trace context.",
        "Error budget dashboard shows we've burned 40% of the monthly budget already; it's the 8th.",
        "Synthetic uptime check is failing because the TLS certificate was renewed but the CA bundle wasn't updated.",
        "Audit log retention is set to 30 days but SOC 2 requires 12 months. Need to pipe to S3 Glacier.",
    ],
    "database": [
        "Postgres migration 0042 adds a NOT NULL column without a default; backfill will take 4 hours on prod.",
        "The users_org_idx partial index is not being used by the planner on the member-list query.",
        "Vacuum is lagging on the events table — autovacuum thresholds are too conservative.",
        "Connection pool exhaustion at peak — pgbouncer is set to 100 but we're seeing 120 concurrent connections.",
        "Read replica lag hit 12s during the bulk import; we need to batch the writes smaller.",
        "JSONB query on the metadata column isn't hitting the GIN index. Query planner picks seq scan.",
        "Foreign key cascade on org_memberships is deleting audit entries — the constraint should be RESTRICT.",
        "The daily snapshot job failed because the backup role lost the SELECT grant on the new partitions.",
        "Alembic downgrade path for migration 0038 is broken — it references a column that was renamed in 0041.",
        "pgBouncer transaction pooling breaks our use of SET LOCAL statement_timeout.",
    ],
    "frontend": [
        "Next.js 15 App Router migration: the /settings page is still using the Pages Router data fetching.",
        "React Query cache invalidation after org switch isn't clearing the /api/projects query.",
        "Tailwind's new v4 engine changed how arbitrary values are parsed; our design tokens broke.",
        "Hydration mismatch warning on the dashboard — server renders timestamp in UTC, client in local.",
        "Bundle size regressed 300kb because we accidentally imported all of lodash instead of lodash/debounce.",
        "Playwright test for the invite flow is flaky — the email input doesn't receive focus reliably on webkit.",
        "Storybook stories for the Button component are out of date; the variant prop has 7 values now, stories have 3.",
        "Core Web Vitals: LCP is 4.2s on the pricing page because the hero image is eagerly loaded.",
        "Accessibility audit: 14 color-contrast failures on the dark theme, mostly in secondary text.",
        "i18n strings for the admin console are missing Spanish translations for 38 keys.",
    ],
    "rag": [
        "The reranker is returning identical scores for candidates with very different content — the model is clipping.",
        "Embedding drift: after the model update, existing chunks no longer match new queries well.",
        "Chunk overlap at 20% isn't enough for long-form legal docs; bump to 30% for that tenant.",
        "ACL filter performance degrades at 500k+ chunks per tenant — we need to partition by org.",
        "Ingest pipeline is losing mini-chunks when a section is exactly at the boundary token count.",
        "Permission sync for Confluence is 4 hours behind because the cursor-based pagination hit a rate limit.",
        "Idempotency cache is evicting entries too fast — we're seeing double-ingests on Nango webhook retries.",
        "The hybrid_alpha=0.7 default is too vector-heavy for technical content; 0.5 works better in eval.",
        "GitHub private repo docs are leaking into public search because the access_type wasn't set at ingest.",
        "Reindexing from scratch is the only way to change embedding models. Need a zero-downtime migration path.",
    ],
    "security": [
        "SOC 2 auditor flagged that our secret rotation for AWS IAM keys is quarterly, not monthly.",
        "Critical CVE in the postgres-driver version we're pinning; upgrade path breaks the prepared-statement cache.",
        "Pen test found an IDOR in the /api/users/:id endpoint — we're checking auth but not scope.",
        "A leaked .env file made it to a contributor's fork on GitHub; the key has been revoked and rotated.",
        "Our SSO IdP is Okta, and the metadata URL rotated without notice. All logins failed for 8 minutes.",
        "Vault token TTL is 30d but the auto-renew cron only runs weekly — tokens expire mid-week sometimes.",
        "SBOM generation for the release pipeline needs to include Rust crates, not just Go + npm.",
        "We need row-level encryption for the PII columns; Postgres pgcrypto is the shortest path.",
        "Supply-chain: a dependency of one of our dependencies was typosquatted. Lockfile review this Friday.",
        "Incident: an admin token was committed to a public gist. Rotated, scoped all tokens down.",
    ],
    "sre": [
        "The 2026-04-17 incident: BGP flap at the edge took us down for 9 minutes. Postmortem draft attached.",
        "Chaos day: we want to kill 30% of replicas in staging and see if the SLO holds.",
        "DR drill scheduled next week: failover postgres from us-east-1 to us-west-2, expected RTO 10m.",
        "On-call rotation is 24/7 Tue-Mon with primary and secondary. Swap requests must be 48h in advance.",
        "Blameless postmortem template: include a timeline, contributing factors, action items with owners.",
        "Capacity plan for Q3: we need 40% more headroom based on the last 90d growth curve.",
        "Runbook for the connector-pruner stuck-in-progress state: how to identify, kill, resume.",
        "SLO for IngestBatch: 99% of batches under 10s. Currently at 98.6% this month.",
        "Load test with k6: 2000 concurrent users hit the search endpoint, p99 was 450ms.",
        "Paging policy: Sev1 pages primary, Sev2 Slack-only during business hours, Sev3 ticket only.",
    ],
    "ml": [
        "Fine-tuning Qwen3-Embedding-4B on our internal ticket corpus improved retrieval P@10 by 3.2 points.",
        "Prompt engineering for the summarization agent: we're getting too many bullet points, want paragraphs.",
        "LoRA adapter size for the intent classifier is 40MB; inference on the edge is fine, cold start is not.",
        "Evaluation harness: 500 labeled queries, we A/B test embedding models nightly against this set.",
        "Cost per 1M tokens for text-embedding-3-large is $0.13; fine-tuning Qwen is $0.007 per 1M. 18x cheaper.",
        "Hallucination rate measured on the support-bot corpus: 4.7% with citations, 12% without.",
        "MMLU scores for our rerank model, measured on CS subset, dropped after the last checkpoint merge.",
        "Retrieval augmentation: we're pulling top-50 chunks but the LLM context window is 128k — room to spare.",
        "Vector DB cost projection at 10M chunks: $340/mo on LanceDB with IVF_PQ, $1200 on Pinecone.",
        "Red-team findings: 3 prompt-injection patterns that bypass our system prompt. Mitigations in review.",
    ],
    "onboarding": [
        "New engineer setup: laptop imaging takes 90 minutes; most of it is waiting on disk encryption.",
        "First-week buddy system: assign each new hire a buddy for Slack DMs, not their manager.",
        "Week 1 goal: ship a small PR. Week 2: pair with oncall. Week 4: first solo oncall shift.",
        "The internal wiki has 1400 pages; we should identify the 40 that are critical for a new joiner.",
        "HR system doesn't push into Okta automatically; IT has to manually create the account on day 1.",
        "Expense reimbursement process: use Brex card for anything under $500, Concur for anything over.",
        "Laptop preferences: MBP M-series, 32GB or 64GB RAM depending on role, external keyboard/mouse on request.",
        "Remote-first onboarding: 2 hours daily with a rotating engineer for the first week, then pair programming.",
        "Access requests go through Opal; standing access to prod is audited monthly.",
        "Welcome lunch: team-dependent; remote folks get a DoorDash gift card.",
    ],
    "hiring": [
        "Tech screen is 60min: 2 behavioral, 2 technical. Interviewer guide updated 2026-04-01.",
        "Onsite loop: 5 rounds, 45min each. System design, coding, coding, domain, behavioral.",
        "Debrief happens within 2 hours. Use the rubric, not vibes. Flag any bias in the feedback.",
        "Compensation bands for L4/L5/L6 engineers in SF vs NYC vs remote; updated quarterly.",
        "Offer letters include equity vest schedule (4y, 1y cliff) and a 2y post-termination exercise window.",
        "Diversity sourcing: 40% of pipeline from URM channels; measured weekly.",
        "Candidate NPS survey after every loop, won or lost, to improve the experience.",
        "Internal transfer policy: must be in-role 18 months unless manager approves a shorter tenure.",
        "Referral bonus: $5000 at start, $5000 at 6 months. Doubled for senior and staff hires.",
        "Greenhouse ATS integration with Hiveloop for candidate tracking.",
    ],
    "legal": [
        "Master Services Agreement redline with enterprise customer: they want data-residency guarantees in EU.",
        "Data Processing Addendum updated for the new subprocessors list — add SiliconFlow and OpenAI.",
        "Privacy policy needs updating to reflect the new sub-processor for rerank (SiliconFlow).",
        "NDA templates: mutual vs one-way. Default is mutual. Legal must review any redlines.",
        "Open-source license audit: we're using 3 AGPL deps that need to be replaced before enterprise launch.",
        "Trademark filing for the Hiveloop name in the EU is in examiner review, 6 months expected.",
        "Contract review SLA: 5 business days for standard, 10 for redlined.",
        "SOC 2 Type II audit window is July 1 through June 30; Vanta is our automation platform.",
        "GDPR data subject access request: must respond within 30 days. Tooling under internal/privacy/.",
        "Vendor risk assessment for new SaaS tools: security, privacy, cost, contract review.",
    ],
    "product": [
        "PRD for the self-service org invites feature: requirements, wireframes, metrics to track.",
        "User research: 12 customer interviews about the search experience. Top ask: save and share queries.",
        "Launch plan for the new dashboard: beta for 2 weeks with 5 customers, then GA.",
        "Feature flag policy: use flags for any change that could break existing users; remove within 30 days.",
        "A/B test results for the new signup flow: +12% activation on the redesigned form.",
        "Product roadmap Q2: RAG ingestion, search API, connector marketplace, rerank controls.",
        "NPS dropped 3 points this quarter; top complaint is ingestion latency on Confluence.",
        "Customer advisory board meeting notes: 8 customers, 2h, topics were SSO, SAML, SCIM.",
        "OKR: 'RAG search is used by 60% of paying orgs weekly' — currently at 34%.",
        "Churn analysis: 3 enterprise customers churned this quarter, all cited lack of SCIM provisioning.",
    ],
    "marketing": [
        "Blog post draft: 'How we cut RAG ingestion time by 10x with LanceDB'.",
        "Launch campaign for the search API: blog, Twitter thread, HN launch post, ProductHunt.",
        "Ad spend for this quarter: $40k across Google and LinkedIn, measured by signups not clicks.",
        "Webinar schedule: 3 per quarter, 45min each, demo + Q&A. Topics lined up through Q3.",
        "Brand guidelines updated: new logo, new typefaces, new color palette. Asset library in Figma.",
        "Customer case study: Acme Corp scaled from 10k to 100M docs on Hiveloop in 6 months.",
        "Podcast interview with the CTO scheduled for May 12; prep deck in progress.",
        "Conference sponsorships Q2: KubeCon, SRECon, AI Engineer Summit. Total spend $120k.",
        "SEO: we rank #3 for 'RAG ingestion pipeline' on Google; aim for top spot by end of Q3.",
        "Competitor analysis: Onyx and Glean are the two closest; we differentiate on LanceDB + Rust.",
    ],
    "finance": [
        "Q1 actuals vs plan: revenue +8% ahead of plan, OpEx +3% ahead. Net burn within target.",
        "Q2 board deck in draft; finance owns slides 1-15, product owns 16-30.",
        "Annual plan for 2026: headcount grows from 45 to 72, OpEx from $8M to $14M.",
        "ARR forecast model updated with new customer cohort data; confidence interval tightened.",
        "Bank account for the UK entity is open; move GBP-denominated customers to it by July.",
        "SAFE conversion mechanics for the Series A: valuation cap of $50M, 20% discount.",
        "Stock option plan: 4000 shares reserved for Q2 hires, 2500 remaining after current offers.",
        "Vendor spend analysis: top 5 are AWS, GCP, Stripe, Datadog, Figma. Total $180k/mo.",
        "Revenue recognition policy updated for annual contracts with milestone deliverables.",
        "Audit engagement letter signed with Ernst & Young; fieldwork starts October.",
    ],
}

TOPIC_NAMES = list(TOPICS.keys())


def pick_topic():
    return random.choice(TOPIC_NAMES)


def body_from_topic(topic: str, n_sentences: int) -> str:
    pool = TOPICS[topic]
    picked = random.sample(pool, k=min(n_sentences, len(pool)))
    return " ".join(picked)


def iso_ts(days_ago: int) -> str:
    dt = datetime(2026, 4, 22) - timedelta(days=days_ago)
    dt = dt.replace(
        hour=random.randint(8, 18),
        minute=random.randint(0, 59),
        second=random.randint(0, 59),
    )
    return dt.isoformat() + "Z"


def random_acl(is_public: bool, include_group: bool = True) -> list:
    if is_public:
        return []
    acl = [f"user_email:{random.choice(USERS)}"]
    if include_group and random.random() < 0.5:
        team = random.choice(TEAMS)
        acl.append(f"external_group:team_{team}")
    return acl


# ---------------------------------------------------------------------------
# 40 source-specific generators. Each takes (idx, org) and returns a Document.
# ---------------------------------------------------------------------------


def linear_ticket(idx, org):
    topic = pick_topic()
    num = 1000 + idx
    title = f"[{topic.upper()}] " + body_from_topic(topic, 1).rstrip(".")
    body = body_from_topic(topic, random.randint(2, 4))
    reproduction = body_from_topic(topic, random.randint(2, 3))
    status = random.choice(["Triage", "Todo", "In Progress", "In Review", "Done", "Cancelled"])
    priority = random.choice(["No priority", "Low", "Medium", "High", "Urgent"])
    assignee = random.choice(USERS)
    labels = random.sample(["bug", "feature", "tech-debt", "customer-report", "p0", "p1", "security"], k=random.randint(1, 3))
    sections = [
        {"text": f"## Summary\n{body}", "title": "Summary", "link": ""},
        {"text": f"## Steps to reproduce\n{reproduction}", "title": "Reproduction", "link": ""},
        {"text": f"## Expected vs actual\n{body_from_topic(topic, 2)}", "title": "Expected / Actual", "link": ""},
    ]
    is_public = random.random() < 0.2
    return {
        "doc_id": f"linear-ENG-{num}",
        "semantic_id": f"ENG-{num}: {title}",
        "link": f"https://linear.app/hiveloop/issue/ENG-{num}",
        "doc_updated_at": iso_ts(random.randint(0, 90)),
        "acl": random_acl(is_public),
        "is_public": is_public,
        "sections": sections,
        "metadata": {
            "source": "linear", "team": random.choice(TEAMS), "status": status,
            "priority": priority, "labels": ",".join(labels), "topic": topic,
        },
        "primary_owners": [assignee],
        "secondary_owners": [],
    }


def jira_issue(idx, org):
    topic = pick_topic()
    project = random.choice(["PROJ", "PLAT", "ENG", "OPS", "SEC"])
    num = 500 + idx
    title = body_from_topic(topic, 1).rstrip(".")
    body = body_from_topic(topic, random.randint(3, 5))
    sprint = f"Sprint {random.randint(20, 45)}"
    issue_type = random.choice(["Bug", "Story", "Task", "Epic", "Spike"])
    is_public = random.random() < 0.15
    return {
        "doc_id": f"jira-{project}-{num}",
        "semantic_id": f"{project}-{num}: {title}",
        "link": f"https://hiveloop.atlassian.net/browse/{project}-{num}",
        "doc_updated_at": iso_ts(random.randint(0, 120)),
        "acl": random_acl(is_public),
        "is_public": is_public,
        "sections": [
            {"text": body, "title": "Description", "link": ""},
            {"text": f"## Acceptance criteria\n- {body_from_topic(topic, 1)}\n- {body_from_topic(topic, 1)}",
             "title": "Acceptance", "link": ""},
        ],
        "metadata": {
            "source": "jira", "project": project, "type": issue_type,
            "sprint": sprint, "topic": topic,
        },
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [random.choice(USERS)],
    }


def github_pr(idx, org):
    topic = pick_topic()
    repo = random.choice(["hiveloop/hiveloop", "hiveloop/rag-engine", "hiveloop/web", "hiveloop/infra"])
    num = 2000 + idx
    title = body_from_topic(topic, 1).rstrip(".")
    body = body_from_topic(topic, random.randint(3, 5))
    checks = random.choice(["all passing", "1 failing (flaky)", "all passing, needs review"])
    reviewers = random.sample(USERS, k=random.randint(1, 3))
    is_public = "hiveloop/hiveloop" in repo and random.random() < 0.1
    return {
        "doc_id": f"gh-pr-{repo.replace('/', '_')}-{num}",
        "semantic_id": f"{repo}#{num}: {title}",
        "link": f"https://github.com/{repo}/pull/{num}",
        "doc_updated_at": iso_ts(random.randint(0, 60)),
        "acl": random_acl(is_public),
        "is_public": is_public,
        "sections": [
            {"text": f"## What\n{body}", "title": "What", "link": ""},
            {"text": f"## Why\n{body_from_topic(topic, 2)}", "title": "Why", "link": ""},
            {"text": f"## Tests\n{body_from_topic(topic, 1)}. CI: {checks}.",
             "title": "Tests", "link": ""},
        ],
        "metadata": {"source": "github", "kind": "pull_request", "repo": repo, "topic": topic,
                     "state": random.choice(["open", "merged", "closed"])},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": reviewers,
    }


def github_issue(idx, org):
    topic = pick_topic()
    repo = random.choice(["hiveloop/hiveloop", "hiveloop/rag-engine", "hiveloop/web"])
    num = 3000 + idx
    title = body_from_topic(topic, 1).rstrip(".")
    body = body_from_topic(topic, random.randint(2, 4))
    reproduction = body_from_topic(topic, 2)
    is_public = "hiveloop/hiveloop" in repo and random.random() < 0.2
    return {
        "doc_id": f"gh-issue-{repo.replace('/', '_')}-{num}",
        "semantic_id": f"{repo}#{num}: {title}",
        "link": f"https://github.com/{repo}/issues/{num}",
        "doc_updated_at": iso_ts(random.randint(0, 180)),
        "acl": random_acl(is_public),
        "is_public": is_public,
        "sections": [
            {"text": f"## Bug report\n{body}\n\n### Reproduction\n{reproduction}",
             "title": "Report", "link": ""},
        ],
        "metadata": {"source": "github", "kind": "issue", "repo": repo, "topic": topic,
                     "state": random.choice(["open", "closed"])},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def github_discussion(idx, org):
    topic = pick_topic()
    repo = "hiveloop/hiveloop"
    num = 4000 + idx
    title = f"RFC: " + body_from_topic(topic, 1).rstrip(".")
    body = body_from_topic(topic, random.randint(4, 6))
    return {
        "doc_id": f"gh-discussion-{num}",
        "semantic_id": f"{repo} Discussion #{num}: {title}",
        "link": f"https://github.com/{repo}/discussions/{num}",
        "doc_updated_at": iso_ts(random.randint(0, 120)),
        "acl": random_acl(True),
        "is_public": True,
        "sections": [{"text": body, "title": "Discussion", "link": ""}],
        "metadata": {"source": "github", "kind": "discussion", "repo": repo, "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def gitlab_mr(idx, org):
    topic = pick_topic()
    num = 5000 + idx
    title = body_from_topic(topic, 1).rstrip(".")
    body = body_from_topic(topic, 3)
    is_public = False
    return {
        "doc_id": f"gl-mr-{num}",
        "semantic_id": f"infra!{num}: {title}",
        "link": f"https://gitlab.com/hiveloop/infra/-/merge_requests/{num}",
        "doc_updated_at": iso_ts(random.randint(0, 90)),
        "acl": random_acl(is_public),
        "is_public": is_public,
        "sections": [{"text": body, "title": "MR description", "link": ""}],
        "metadata": {"source": "gitlab", "kind": "merge_request", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def gitlab_issue(idx, org):
    topic = pick_topic()
    num = 6000 + idx
    title = body_from_topic(topic, 1).rstrip(".")
    body = body_from_topic(topic, random.randint(2, 3))
    return {
        "doc_id": f"gl-issue-{num}",
        "semantic_id": f"infra#{num}: {title}",
        "link": f"https://gitlab.com/hiveloop/infra/-/issues/{num}",
        "doc_updated_at": iso_ts(random.randint(0, 120)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Issue", "link": ""}],
        "metadata": {"source": "gitlab", "kind": "issue", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def slack_engineering(idx, org):
    topic = pick_topic()
    author = random.choice(USERS)
    ts = iso_ts(random.randint(0, 30))
    thread_len = random.randint(1, 5)
    msgs = []
    for i in range(thread_len):
        speaker = random.choice(USERS)
        text = body_from_topic(topic, 1)
        msgs.append(f"**{speaker}**: {text}")
    body = "\n".join(msgs)
    return {
        "doc_id": f"slack-eng-{idx}",
        "semantic_id": f"#engineering thread {ts[:10]}",
        "link": f"https://hiveloop.slack.com/archives/C01ABCDEF/p{int(random.random()*1e13)}",
        "doc_updated_at": ts,
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Thread", "link": ""}],
        "metadata": {"source": "slack", "channel": "#engineering", "topic": topic,
                     "thread_len": str(thread_len)},
        "primary_owners": [author],
        "secondary_owners": [],
    }


def slack_incidents(idx, org):
    topic = pick_topic()
    author = random.choice(USERS)
    ts = iso_ts(random.randint(0, 60))
    severity = random.choice(["Sev1", "Sev2", "Sev3"])
    body = body_from_topic(topic, 4)
    return {
        "doc_id": f"slack-incident-{idx}",
        "semantic_id": f"#incidents {severity} {ts[:10]}",
        "link": f"https://hiveloop.slack.com/archives/C02INCIDENT/p{int(random.random()*1e13)}",
        "doc_updated_at": ts,
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": f"{severity} incident log:\n{body}", "title": "Incident", "link": ""}],
        "metadata": {"source": "slack", "channel": "#incidents", "severity": severity, "topic": topic},
        "primary_owners": [author],
        "secondary_owners": random.sample(USERS, k=3),
    }


def slack_support(idx, org):
    topic = pick_topic()
    customer = f"acme-{random.randint(1,50)}"
    body = body_from_topic(topic, 3)
    return {
        "doc_id": f"slack-support-{idx}",
        "semantic_id": f"#support customer={customer}",
        "link": f"https://hiveloop.slack.com/archives/C03SUPPORT/p{int(random.random()*1e13)}",
        "doc_updated_at": iso_ts(random.randint(0, 30)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": f"Customer {customer}: {body}", "title": "Support thread", "link": ""}],
        "metadata": {"source": "slack", "channel": "#support", "customer": customer, "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def slack_random(idx, org):
    topic = pick_topic()
    body = body_from_topic(topic, 2)
    return {
        "doc_id": f"slack-random-{idx}",
        "semantic_id": f"#random",
        "link": f"https://hiveloop.slack.com/archives/C04RANDOM/p{int(random.random()*1e13)}",
        "doc_updated_at": iso_ts(random.randint(0, 10)),
        "acl": random_acl(True),
        "is_public": True,
        "sections": [{"text": body, "title": "Message", "link": ""}],
        "metadata": {"source": "slack", "channel": "#random", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def teams_messages(idx, org):
    topic = pick_topic()
    body = body_from_topic(topic, random.randint(2, 3))
    return {
        "doc_id": f"teams-{idx}",
        "semantic_id": f"Teams message {iso_ts(random.randint(0,30))[:10]}",
        "link": f"https://teams.microsoft.com/l/message/...",
        "doc_updated_at": iso_ts(random.randint(0, 30)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Message", "link": ""}],
        "metadata": {"source": "teams", "channel": "General", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def zoom_transcript(idx, org):
    topic = pick_topic()
    body = body_from_topic(topic, 6)
    duration = random.randint(25, 60)
    return {
        "doc_id": f"zoom-{idx}",
        "semantic_id": f"Zoom meeting transcript ({duration}m)",
        "link": f"https://hiveloop.zoom.us/rec/share/{random.getrandbits(40):x}",
        "doc_updated_at": iso_ts(random.randint(0, 60)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": f"Transcript ({duration}m):\n{body}", "title": "Transcript", "link": ""}],
        "metadata": {"source": "zoom", "duration_min": str(duration), "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": random.sample(USERS, k=random.randint(2, 6)),
    }


def fireflies_transcript(idx, org):
    topic = pick_topic()
    body = body_from_topic(topic, 7)
    return {
        "doc_id": f"fireflies-{idx}",
        "semantic_id": f"Fireflies meeting notes",
        "link": f"https://app.fireflies.ai/view/{random.getrandbits(60):x}",
        "doc_updated_at": iso_ts(random.randint(0, 45)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [
            {"text": f"## Summary\n{body_from_topic(topic, 3)}", "title": "Summary", "link": ""},
            {"text": f"## Action items\n{body_from_topic(topic, 2)}", "title": "Actions", "link": ""},
        ],
        "metadata": {"source": "fireflies", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": random.sample(USERS, k=random.randint(2, 5)),
    }


def gmail_thread(idx, org):
    topic = pick_topic()
    body = body_from_topic(topic, 3)
    subj = body_from_topic(topic, 1).rstrip(".")
    return {
        "doc_id": f"gmail-{idx}",
        "semantic_id": f"Email: {subj}",
        "link": f"https://mail.google.com/mail/u/0/#inbox/{random.getrandbits(48):x}",
        "doc_updated_at": iso_ts(random.randint(0, 90)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Email body", "link": ""}],
        "metadata": {"source": "gmail", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [random.choice(USERS)],
    }


def confluence_page(idx, org):
    topic = pick_topic()
    space = random.choice(["ENG", "OPS", "PRODUCT", "HR"])
    title = body_from_topic(topic, 1).rstrip(".").title()
    body = body_from_topic(topic, 6)
    sections = [
        {"text": f"# {title}\n{body_from_topic(topic, 2)}", "title": "Overview", "link": ""},
        {"text": f"## Details\n{body}", "title": "Details", "link": ""},
        {"text": f"## References\n{body_from_topic(topic, 1)}", "title": "References", "link": ""},
    ]
    is_public = space == "ENG" and random.random() < 0.15
    return {
        "doc_id": f"confluence-{space}-{1000+idx}",
        "semantic_id": f"[{space}] {title}",
        "link": f"https://hiveloop.atlassian.net/wiki/spaces/{space}/pages/{1000+idx}",
        "doc_updated_at": iso_ts(random.randint(0, 365)),
        "acl": random_acl(is_public),
        "is_public": is_public,
        "sections": sections,
        "metadata": {"source": "confluence", "space": space, "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def notion_engineering(idx, org):
    topic = pick_topic()
    title = body_from_topic(topic, 1).rstrip(".").title()
    body = body_from_topic(topic, 5)
    return {
        "doc_id": f"notion-eng-{idx}",
        "semantic_id": f"Engineering wiki: {title}",
        "link": f"https://www.notion.so/hiveloop/{random.getrandbits(60):x}",
        "doc_updated_at": iso_ts(random.randint(0, 180)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [
            {"text": f"# {title}\n{body}", "title": "Wiki", "link": ""},
        ],
        "metadata": {"source": "notion", "database": "engineering-wiki", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def notion_handbook(idx, org):
    topic = pick_topic()
    title = body_from_topic(topic, 1).rstrip(".").title()
    body = body_from_topic(topic, 4)
    is_public = random.random() < 0.4
    return {
        "doc_id": f"notion-handbook-{idx}",
        "semantic_id": f"Handbook: {title}",
        "link": f"https://www.notion.so/hiveloop/handbook/{random.getrandbits(60):x}",
        "doc_updated_at": iso_ts(random.randint(0, 365)),
        "acl": random_acl(is_public),
        "is_public": is_public,
        "sections": [{"text": f"# {title}\n{body}", "title": "Handbook", "link": ""}],
        "metadata": {"source": "notion", "database": "handbook", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def gdoc(idx, org):
    topic = pick_topic()
    title = body_from_topic(topic, 1).rstrip(".").title()
    body = body_from_topic(topic, 5)
    return {
        "doc_id": f"gdoc-{idx}",
        "semantic_id": f"Google Doc: {title}",
        "link": f"https://docs.google.com/document/d/{random.getrandbits(120):x}",
        "doc_updated_at": iso_ts(random.randint(0, 180)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Document", "link": ""}],
        "metadata": {"source": "google_drive", "kind": "doc", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def api_doc(idx, org):
    methods = ["GET", "POST", "PUT", "DELETE", "PATCH"]
    resources = ["/v1/users", "/v1/orgs", "/v1/connections", "/v1/rag/search", "/v1/rag/ingest",
                 "/v1/billing/invoices", "/v1/oauth/token", "/v1/webhooks", "/v1/api-keys", "/v1/datasets"]
    method = random.choice(methods)
    resource = random.choice(resources)
    body = (f"### {method} {resource}\n\n"
            f"{body_from_topic(pick_topic(), 2)}\n\n"
            f"**Parameters**:\n- `org_id` (string, required)\n- `limit` (int, optional, default 50)\n\n"
            f"**Response 200**:\n```json\n{{\"data\": [...], \"next_cursor\": \"...\"}}\n```\n\n"
            f"**Errors**: 401 unauthenticated, 403 forbidden, 404 not found, 429 rate limited.")
    return {
        "doc_id": f"apidoc-{method}-{resource.replace('/', '_')}-{idx}",
        "semantic_id": f"API: {method} {resource}",
        "link": f"https://docs.hiveloop.com/api{resource}",
        "doc_updated_at": iso_ts(random.randint(0, 120)),
        "acl": random_acl(True),
        "is_public": True,
        "sections": [{"text": body, "title": "Endpoint", "link": ""}],
        "metadata": {"source": "api_docs", "method": method, "resource": resource},
        "primary_owners": [],
        "secondary_owners": [],
    }


def gitbook(idx, org):
    topic = pick_topic()
    title = body_from_topic(topic, 1).rstrip(".").title()
    body = body_from_topic(topic, 5)
    return {
        "doc_id": f"gitbook-{idx}",
        "semantic_id": f"Docs: {title}",
        "link": f"https://docs.hiveloop.com/{idx}",
        "doc_updated_at": iso_ts(random.randint(0, 180)),
        "acl": random_acl(True),
        "is_public": True,
        "sections": [{"text": f"# {title}\n{body}", "title": "Guide", "link": ""}],
        "metadata": {"source": "gitbook", "topic": topic},
        "primary_owners": [],
        "secondary_owners": [],
    }


def readme_docs(idx, org):
    topic = pick_topic()
    title = body_from_topic(topic, 1).rstrip(".").title()
    body = body_from_topic(topic, 4)
    return {
        "doc_id": f"readme-{idx}",
        "semantic_id": f"Readme.io: {title}",
        "link": f"https://hiveloop.readme.io/docs/{idx}",
        "doc_updated_at": iso_ts(random.randint(0, 180)),
        "acl": random_acl(True),
        "is_public": True,
        "sections": [{"text": body, "title": "Article", "link": ""}],
        "metadata": {"source": "readme", "topic": topic},
        "primary_owners": [],
        "secondary_owners": [],
    }


def zendesk_ticket(idx, org):
    topic = pick_topic()
    num = 7000 + idx
    customer_email = f"customer{random.randint(1,200)}@acmecorp.com"
    title = body_from_topic(topic, 1).rstrip(".")
    body = body_from_topic(topic, 3)
    status = random.choice(["open", "pending", "solved", "closed"])
    priority = random.choice(["low", "normal", "high", "urgent"])
    return {
        "doc_id": f"zd-{num}",
        "semantic_id": f"Zendesk #{num}: {title}",
        "link": f"https://hiveloop.zendesk.com/agent/tickets/{num}",
        "doc_updated_at": iso_ts(random.randint(0, 180)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [
            {"text": f"Customer {customer_email} says: {body}", "title": "Ticket", "link": ""},
            {"text": f"Agent response: {body_from_topic(topic, 2)}", "title": "Response", "link": ""},
        ],
        "metadata": {"source": "zendesk", "status": status, "priority": priority,
                     "customer": customer_email, "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def intercom_conversation(idx, org):
    topic = pick_topic()
    body = body_from_topic(topic, 4)
    return {
        "doc_id": f"intercom-{idx}",
        "semantic_id": f"Intercom conversation {iso_ts(random.randint(0,30))[:10]}",
        "link": f"https://app.intercom.com/a/apps/hiveloop/conversations/{random.getrandbits(40):x}",
        "doc_updated_at": iso_ts(random.randint(0, 90)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Conversation", "link": ""}],
        "metadata": {"source": "intercom", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def freshdesk_ticket(idx, org):
    topic = pick_topic()
    num = 8000 + idx
    body = body_from_topic(topic, 3)
    return {
        "doc_id": f"fd-{num}",
        "semantic_id": f"Freshdesk #{num}",
        "link": f"https://hiveloop.freshdesk.com/a/tickets/{num}",
        "doc_updated_at": iso_ts(random.randint(0, 180)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Ticket body", "link": ""}],
        "metadata": {"source": "freshdesk", "topic": topic,
                     "status": random.choice(["open", "pending", "resolved"])},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def salesforce_opportunity(idx, org):
    topic = pick_topic()
    num = 9000 + idx
    amount = random.randint(10, 500) * 1000
    stage = random.choice(["Prospecting", "Qualification", "Proposal", "Negotiation", "Closed Won", "Closed Lost"])
    account = random.choice(["Acme Corp", "Globex Inc", "Initech", "Umbrella", "Wayne Enterprises"])
    body = (f"Opportunity with {account}: ${amount}. Stage: {stage}.\n\n"
            f"Notes: {body_from_topic(topic, 2)}")
    return {
        "doc_id": f"sfdc-opp-{num}",
        "semantic_id": f"Opportunity: {account} ${amount}",
        "link": f"https://hiveloop.lightning.force.com/lightning/r/Opportunity/{random.getrandbits(60):x}/view",
        "doc_updated_at": iso_ts(random.randint(0, 120)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Opportunity", "link": ""}],
        "metadata": {"source": "salesforce", "kind": "opportunity",
                     "stage": stage, "amount": str(amount), "account": account},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def salesforce_account(idx, org):
    num = 10000 + idx
    account_name = f"Account-{num}"
    industry = random.choice(["Tech", "Finance", "Healthcare", "Retail", "Education"])
    body = (f"Account: {account_name}, industry {industry}.\n"
            f"Annual revenue: ${random.randint(1, 500)}M.\n"
            f"Employees: {random.randint(50, 10000)}.\n"
            f"Notes: {body_from_topic(pick_topic(), 2)}")
    return {
        "doc_id": f"sfdc-acct-{num}",
        "semantic_id": f"Account: {account_name}",
        "link": f"https://hiveloop.lightning.force.com/lightning/r/Account/{random.getrandbits(60):x}/view",
        "doc_updated_at": iso_ts(random.randint(0, 365)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Account", "link": ""}],
        "metadata": {"source": "salesforce", "kind": "account", "industry": industry},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def hubspot_contact(idx, org):
    name = f"Contact-{idx}"
    body = (f"Contact {name}, title {random.choice(['CTO', 'VP Eng', 'Director', 'Manager', 'IC'])}\n"
            f"Last interaction: {body_from_topic(pick_topic(), 2)}")
    return {
        "doc_id": f"hs-contact-{idx}",
        "semantic_id": f"Contact: {name}",
        "link": f"https://app.hubspot.com/contacts/{random.getrandbits(32):x}/contact/{idx}",
        "doc_updated_at": iso_ts(random.randint(0, 180)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Contact", "link": ""}],
        "metadata": {"source": "hubspot", "kind": "contact"},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def hubspot_deal(idx, org):
    num = 11000 + idx
    amount = random.randint(5, 200) * 1000
    stage = random.choice(["appointment", "qualified", "presentation", "decisionmaker", "contract", "closedwon", "closedlost"])
    body = f"Deal #{num}: ${amount}, stage {stage}.\n{body_from_topic(pick_topic(), 2)}"
    return {
        "doc_id": f"hs-deal-{num}",
        "semantic_id": f"Deal #{num} ${amount}",
        "link": f"https://app.hubspot.com/contacts/{random.getrandbits(32):x}/deal/{num}",
        "doc_updated_at": iso_ts(random.randint(0, 120)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Deal", "link": ""}],
        "metadata": {"source": "hubspot", "kind": "deal", "stage": stage, "amount": str(amount)},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def runbook(idx, org):
    topic = pick_topic()
    title = f"Runbook: {body_from_topic(topic, 1).rstrip('.')}"
    body = (f"## Symptoms\n{body_from_topic(topic, 1)}\n\n"
            f"## Diagnosis\n{body_from_topic(topic, 2)}\n\n"
            f"## Remediation\n{body_from_topic(topic, 2)}\n\n"
            f"## Verify\n{body_from_topic(topic, 1)}")
    return {
        "doc_id": f"runbook-{idx}",
        "semantic_id": title,
        "link": f"https://hiveloop.atlassian.net/wiki/spaces/OPS/pages/runbook-{idx}",
        "doc_updated_at": iso_ts(random.randint(0, 365)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Runbook", "link": ""}],
        "metadata": {"source": "runbook", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": random.sample(USERS, k=2),
    }


def postmortem(idx, org):
    topic = pick_topic()
    date = iso_ts(random.randint(0, 180))
    sev = random.choice(["SEV1", "SEV2"])
    title = f"Postmortem {date[:10]}: {body_from_topic(topic, 1).rstrip('.')}"
    body = (f"## Impact\n{body_from_topic(topic, 1)}\n\n"
            f"## Timeline\n{body_from_topic(topic, 3)}\n\n"
            f"## Root cause\n{body_from_topic(topic, 2)}\n\n"
            f"## Action items\n{body_from_topic(topic, 2)}")
    return {
        "doc_id": f"postmortem-{idx}",
        "semantic_id": title,
        "link": f"https://hiveloop.atlassian.net/wiki/spaces/OPS/pages/postmortem-{idx}",
        "doc_updated_at": date,
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Postmortem", "link": ""}],
        "metadata": {"source": "postmortem", "severity": sev, "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": random.sample(USERS, k=3),
    }


def adr(idx, org):
    topic = pick_topic()
    num = 50 + idx
    title = f"ADR-{num:04d}: {body_from_topic(topic, 1).rstrip('.').title()}"
    body = (f"## Status\nAccepted\n\n"
            f"## Context\n{body_from_topic(topic, 2)}\n\n"
            f"## Decision\n{body_from_topic(topic, 2)}\n\n"
            f"## Consequences\n{body_from_topic(topic, 1)}")
    return {
        "doc_id": f"adr-{num:04d}",
        "semantic_id": title,
        "link": f"https://github.com/hiveloop/adr/blob/main/{num:04d}.md",
        "doc_updated_at": iso_ts(random.randint(0, 365)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "ADR", "link": ""}],
        "metadata": {"source": "adr", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def rfc(idx, org):
    topic = pick_topic()
    num = 100 + idx
    title = f"RFC-{num:04d}: {body_from_topic(topic, 1).rstrip('.').title()}"
    body = (f"## Summary\n{body_from_topic(topic, 1)}\n\n"
            f"## Motivation\n{body_from_topic(topic, 2)}\n\n"
            f"## Design\n{body_from_topic(topic, 4)}\n\n"
            f"## Alternatives considered\n{body_from_topic(topic, 2)}")
    return {
        "doc_id": f"rfc-{num:04d}",
        "semantic_id": title,
        "link": f"https://github.com/hiveloop/rfcs/pull/{num}",
        "doc_updated_at": iso_ts(random.randint(0, 365)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "RFC", "link": ""}],
        "metadata": {"source": "rfc", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": random.sample(USERS, k=3),
    }


def sprint_retro(idx, org):
    team = random.choice(TEAMS)
    sprint_num = random.randint(30, 50)
    body = (f"## Went well\n{body_from_topic(pick_topic(), 2)}\n\n"
            f"## Didn't go well\n{body_from_topic(pick_topic(), 2)}\n\n"
            f"## Action items\n{body_from_topic(pick_topic(), 1)}")
    return {
        "doc_id": f"retro-{team}-{sprint_num}-{idx}",
        "semantic_id": f"{team} retro — Sprint {sprint_num}",
        "link": f"https://www.notion.so/retros/{team}-sprint-{sprint_num}",
        "doc_updated_at": iso_ts(random.randint(0, 180)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Retro", "link": ""}],
        "metadata": {"source": "retro", "team": team, "sprint": str(sprint_num)},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": random.sample(USERS, k=random.randint(3, 6)),
    }


def standup_note(idx, org):
    user = random.choice(USERS)
    body = (f"## Yesterday\n{body_from_topic(pick_topic(), 1)}\n"
            f"## Today\n{body_from_topic(pick_topic(), 1)}\n"
            f"## Blockers\n{body_from_topic(pick_topic(), 1) if random.random() < 0.3 else 'None.'}")
    return {
        "doc_id": f"standup-{idx}",
        "semantic_id": f"Standup {user} {iso_ts(random.randint(0,30))[:10]}",
        "link": f"https://www.notion.so/standups/{random.getrandbits(60):x}",
        "doc_updated_at": iso_ts(random.randint(0, 30)),
        "acl": [f"user_email:{user}"] + [f"external_group:team_{random.choice(TEAMS)}"],
        "is_public": False,
        "sections": [{"text": body, "title": "Standup", "link": ""}],
        "metadata": {"source": "standup"},
        "primary_owners": [user],
        "secondary_owners": [],
    }


def one_on_one(idx, org):
    manager = random.choice(USERS)
    report = random.choice(USERS)
    body = (f"## Discussion\n{body_from_topic(pick_topic(), 2)}\n\n"
            f"## Career\n{body_from_topic(pick_topic(), 1)}\n\n"
            f"## Action items\n{body_from_topic(pick_topic(), 1)}")
    return {
        "doc_id": f"1on1-{idx}",
        "semantic_id": f"1:1 {manager}/{report}",
        "link": f"https://www.notion.so/1on1s/{random.getrandbits(60):x}",
        "doc_updated_at": iso_ts(random.randint(0, 90)),
        # 1:1 notes are the strictest ACL — only manager + report
        "acl": [f"user_email:{manager}", f"user_email:{report}"],
        "is_public": False,
        "sections": [{"text": body, "title": "1:1", "link": ""}],
        "metadata": {"source": "one_on_one"},
        "primary_owners": [manager],
        "secondary_owners": [report],
    }


def okr_doc(idx, org):
    quarter = random.choice(["Q1 2026", "Q2 2026", "Q3 2026", "Q4 2025"])
    team = random.choice(TEAMS)
    body = (f"## Objective\n{body_from_topic(pick_topic(), 1)}\n\n"
            f"## Key results\n- KR1: {body_from_topic(pick_topic(), 1)}\n"
            f"- KR2: {body_from_topic(pick_topic(), 1)}\n"
            f"- KR3: {body_from_topic(pick_topic(), 1)}")
    return {
        "doc_id": f"okr-{team}-{quarter.replace(' ', '-')}-{idx}",
        "semantic_id": f"OKR {team} {quarter}",
        "link": f"https://www.notion.so/okrs/{team}-{quarter.replace(' ', '-')}-{idx}",
        "doc_updated_at": iso_ts(random.randint(0, 120)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "OKR", "link": ""}],
        "metadata": {"source": "okr", "team": team, "quarter": quarter},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


def prd(idx, org):
    topic = pick_topic()
    title = f"PRD: {body_from_topic(topic, 1).rstrip('.').title()}"
    body = (f"## Problem\n{body_from_topic(topic, 2)}\n\n"
            f"## Goals & non-goals\n{body_from_topic(topic, 2)}\n\n"
            f"## User stories\n{body_from_topic(topic, 3)}\n\n"
            f"## Success metrics\n{body_from_topic(topic, 1)}")
    return {
        "doc_id": f"prd-{idx}",
        "semantic_id": title,
        "link": f"https://www.notion.so/prds/{random.getrandbits(60):x}",
        "doc_updated_at": iso_ts(random.randint(0, 180)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "PRD", "link": ""}],
        "metadata": {"source": "prd", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": random.sample(USERS, k=random.randint(2, 5)),
    }


def design_doc(idx, org):
    topic = pick_topic()
    title = f"Design: {body_from_topic(topic, 1).rstrip('.').title()}"
    body = (f"## Overview\n{body_from_topic(topic, 2)}\n\n"
            f"## Architecture\n{body_from_topic(topic, 3)}\n\n"
            f"## Data model\n{body_from_topic(topic, 2)}\n\n"
            f"## Rollout plan\n{body_from_topic(topic, 1)}")
    return {
        "doc_id": f"design-{idx}",
        "semantic_id": title,
        "link": f"https://www.notion.so/design/{random.getrandbits(60):x}",
        "doc_updated_at": iso_ts(random.randint(0, 180)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Design doc", "link": ""}],
        "metadata": {"source": "design_doc", "topic": topic},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": random.sample(USERS, k=random.randint(2, 4)),
    }


def user_research(idx, org):
    customer = f"customer-{random.randint(1,100)}"
    body = (f"Interviewed {customer}.\n\n"
            f"## Context\n{body_from_topic(pick_topic(), 2)}\n\n"
            f"## Key findings\n{body_from_topic(pick_topic(), 2)}\n\n"
            f"## Recommendations\n{body_from_topic(pick_topic(), 1)}")
    return {
        "doc_id": f"research-{idx}",
        "semantic_id": f"User research: {customer}",
        "link": f"https://dovetailapp.com/interviews/{random.getrandbits(60):x}",
        "doc_updated_at": iso_ts(random.randint(0, 120)),
        "acl": random_acl(False),
        "is_public": False,
        "sections": [{"text": body, "title": "Interview notes", "link": ""}],
        "metadata": {"source": "user_research", "customer": customer},
        "primary_owners": [random.choice(USERS)],
        "secondary_owners": [],
    }


# ---------------------------------------------------------------------------
# Registry: 40 sources × 250 docs each = 10,000 total.
# ---------------------------------------------------------------------------

SOURCES = [
    ("linear_tickets", linear_ticket),
    ("jira_issues", jira_issue),
    ("github_prs", github_pr),
    ("github_issues", github_issue),
    ("github_discussions", github_discussion),
    ("gitlab_mrs", gitlab_mr),
    ("gitlab_issues", gitlab_issue),
    ("slack_engineering", slack_engineering),
    ("slack_incidents", slack_incidents),
    ("slack_support", slack_support),
    ("slack_random", slack_random),
    ("teams_messages", teams_messages),
    ("zoom_transcripts", zoom_transcript),
    ("fireflies_transcripts", fireflies_transcript),
    ("gmail_threads", gmail_thread),
    ("confluence_pages", confluence_page),
    ("notion_engineering", notion_engineering),
    ("notion_handbook", notion_handbook),
    ("gdocs", gdoc),
    ("api_docs", api_doc),
    ("gitbook", gitbook),
    ("readme_docs", readme_docs),
    ("zendesk_tickets", zendesk_ticket),
    ("intercom_conversations", intercom_conversation),
    ("freshdesk_tickets", freshdesk_ticket),
    ("salesforce_opportunities", salesforce_opportunity),
    ("salesforce_accounts", salesforce_account),
    ("hubspot_contacts", hubspot_contact),
    ("hubspot_deals", hubspot_deal),
    ("runbooks", runbook),
    ("postmortems", postmortem),
    ("adrs", adr),
    ("rfcs", rfc),
    ("sprint_retros", sprint_retro),
    ("standups", standup_note),
    ("one_on_ones", one_on_one),
    ("okrs", okr_doc),
    ("prds", prd),
    ("design_docs", design_doc),
    ("user_research", user_research),
]

assert len(SOURCES) == 40, f"expected 40 sources, got {len(SOURCES)}"

DOCS_PER_FILE = 250
VECTOR_DIM = int(os.environ.get("RAG_VECTOR_DIM", "3072"))
MODE = "INGESTION_MODE_UPSERT"

manifest = []
for i, (name, gen) in enumerate(SOURCES):
    docs = []
    for j in range(DOCS_PER_FILE):
        org = random.choice(ORGS)
        doc = gen(j, org)
        docs.append(doc)
    # IngestBatch uses one org_id per call; spread sources across orgs so
    # test queries can exercise org isolation. Each file picks one org.
    file_org = random.choice(ORGS)
    batch = {
        "dataset_name": DATASET_NAME,
        "org_id": file_org,
        "mode": MODE,
        "idempotency_key": f"bulk-{name}-v1",
        "declared_vector_dim": VECTOR_DIM,
        "documents": docs,
    }
    out_path = OUT_DIR / f"docs_{i+1:02d}_{name}.json"
    out_path.write_text(json.dumps(batch, indent=None))
    manifest.append({
        "file": out_path.name,
        "source": name,
        "docs": len(docs),
        "org": file_org,
        "size_bytes": out_path.stat().st_size,
    })
    print(f"[{i+1:>2}/40] {name:<26} {len(docs)} docs → {out_path.name} ({out_path.stat().st_size:>7} B, org={file_org})")

total_docs = sum(m["docs"] for m in manifest)
total_bytes = sum(m["size_bytes"] for m in manifest)
summary = {
    "dataset_name": DATASET_NAME,
    "vector_dim": VECTOR_DIM,
    "total_files": len(manifest),
    "total_docs": total_docs,
    "total_bytes": total_bytes,
    "orgs": sorted(set(m["org"] for m in manifest)),
    "files": manifest,
}
summary_path = OUT_DIR.parent / "manifest.json"
summary_path.write_text(json.dumps(summary, indent=2))
print(f"\nTotal: {total_docs} docs, {total_bytes/1024/1024:.1f} MiB across {len(manifest)} files.")
print(f"Manifest: {summary_path}")
