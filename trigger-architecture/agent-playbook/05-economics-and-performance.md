# 05 — Economics and Performance

Every trigger-driven agent run has a cost: API calls to the provider (via Nango), LLM tokens, sandbox compute, database writes. At small volume these are negligible. At scale — dozens of agents, thousands of events per day — they start to matter. This document covers the cost structure, the variables you can tune, and how to think about economics when planning an agent rollout.

The goal isn't to save every penny. It's to make cost-aware decisions rather than cost-surprising ones, and to know which knobs to turn if a specific agent is more expensive than it should be.

## What a single agent run costs

A typical agent run — one webhook fires, one agent picks it up, runs context actions, talks to the LLM, takes an action — has four cost buckets:

### 1. Nango proxy calls

Every context action is one HTTP call to the provider's API, proxied through Nango. Providers usually have rate limits on their APIs (GitHub allows a few thousand requests per hour per token, Linear and Slack have similar caps). The direct money cost is usually small (Nango pricing is per-connection or per-month, not per-call), but the rate limit is a hard ceiling.

**Rule of thumb**: a typical trigger has 1–4 context actions. A busy agent processing 100 events per hour makes 100–400 Nango calls. GitHub's primary rate limit is 5,000 requests/hour for authenticated apps, so you have headroom for ~12 busy agents per connection before you hit it — more if you use fewer context actions per trigger.

**How to reduce**: use fewer context actions. Let the agent fetch data mid-run via tools only when it actually needs the data. Pre-fetching everything "just in case" wastes calls.

### 2. LLM tokens

The biggest variable cost. Every run puts the opening prompt (system prompt + context + instructions) into the LLM, which generates a response, potentially calls tools (each of which is another LLM turn), and eventually emits a final answer. Token usage depends on:

- **System prompt size** — a verbose system prompt costs tokens on every run. Keep it tight.
- **Context size** — the bigger your context actions' responses, the more tokens per run. A `pulls_list_files` on a 500-file PR is expensive.
- **Conversation history** — continuation events include the prior conversation in context. Long-running conversations get expensive over time.
- **Turn count** — tool-heavy runs take multiple turns. Each turn is a full LLM call with the current message history.

**Rule of thumb**: a simple PR review run (opened event, 3 context actions, 500-line diff, one review comment response) uses roughly 5K–20K tokens. A complex debugging run (continuation event, 20+ conversation messages, many tool calls) can use 100K+ tokens.

**How to reduce**:

- Keep system prompts focused and short
- Avoid pre-fetching large responses unless the agent definitely needs them
- Cap continuation conversations at a reasonable length (e.g., summarize-and-reset after 30 messages)
- Use cheaper models for simpler tasks when accuracy allows

### 3. Sandbox compute

Each run executes in a sandbox. For shared agents, this is the pool sandbox (one sandbox serving many conversations). For dedicated agents, it's a per-agent sandbox that's provisioned when needed and sleeps when idle.

Sandboxes sleep cheaply. An idle sandbox costs almost nothing. An active sandbox costs real money for the duration it's running. Typical runs are seconds to minutes; most of the time a sandbox is sleeping.

**Shared vs. dedicated tradeoff**:

- **Shared**: one sandbox handles many agents. Cost per run is low (amortized over many runs). Cold starts don't exist (the sandbox is warm). But agents share process-level state — careful with filesystem changes, environment variables, etc.
- **Dedicated**: one sandbox per agent. Each dedicated agent has its own sandbox, which provisions on first run and sleeps when idle. Cost per run includes provisioning overhead the first time, then wake-up cost on subsequent runs. Agents get isolated filesystems, environments, and tools.

**Rule of thumb**: use shared for most agents. Use dedicated only when you need isolation (agents writing to filesystem, running background processes, or requiring specific installed tools).

### 4. Database and infrastructure

Every dispatch is a DB query (the trigger lookup) and a few writes (the prepared run metadata, the conversation record, the event log). These are cheap individually but add up at scale.

**Rule of thumb**: ignore this bucket until you're processing tens of thousands of events per day. Before then, the other three buckets dominate.

## Cost per agent class

Different agent types have different cost profiles:

### Read-only observer agents

(Log-writers, metric-emitters, simple notification relays.)

- **Per run**: 1–2 Nango calls, small LLM usage, shared sandbox
- **Cost profile**: cheapest class. Can run on every event without breaking the bank.
- **Example**: a "log every new PR" agent that just records new PRs to a spreadsheet

### Reviewer agents

(Code review, PR triage, documentation review.)

- **Per run**: 3–5 Nango calls, moderate LLM usage (diff is expensive), shared sandbox
- **Cost profile**: moderate. A single repo's worth of PRs is fine; 50 repos is still okay; hundreds starts to matter.
- **Example**: the code review agent from [07-worked-examples.md](07-worked-examples.md)

### Autonomous worker agents

(Agents that take multi-step actions, like the "coding agent" that creates and updates PRs.)

- **Per run**: many tool calls, large LLM usage, often dedicated sandbox
- **Cost profile**: highest class. Each run might cost $0.10–$1.00 in LLM tokens alone. Plan accordingly.
- **Example**: the Linear-to-PR coding agent from [07-worked-examples.md](07-worked-examples.md)

### Support and customer-facing agents

(Drafting replies, triaging support tickets.)

- **Per run**: moderate Nango calls, moderate-to-large LLM usage (context + tone), shared sandbox usually fine
- **Cost profile**: moderate. Volume is usually the issue — a busy support channel can fire many events per hour.
- **Example**: the Intercom support drafter from [07-worked-examples.md](07-worked-examples.md)

## Scaling considerations

When you go from "a few agents" to "many agents," a few dynamics change:

### API rate limits become the bottleneck

At single-agent scale, the LLM token cost dominates. At multi-agent scale, provider rate limits hit you first. GitHub's 5,000 requests/hour limit means a single app can't support more than ~50 active agents each making 100 requests/hour via context actions. Linear is similar. Slack has per-method limits.

**Plan**: monitor Nango rate limit headers. If you're approaching a ceiling, either reduce context actions per trigger or use multiple app installations.

### Shared sandbox saturation

At high run rates, the shared sandbox pool might not keep up. Each sandbox can only run a certain number of concurrent conversations. If you push past that, runs queue up and latency rises.

**Plan**: the orchestrator auto-provisions additional pool sandboxes under load, but there's a warm-up cost. Monitor run latency; if p99 is climbing, you may need to tune pool size.

### Database query performance

The dispatcher query (`trigger_keys && ? OR terminate_event_keys && ?`) uses array overlap, which is indexed. At tens of thousands of agent_triggers rows, this still returns in low milliseconds. At hundreds of thousands, re-check. You're probably fine, but worth knowing.

### Log volume

Every dispatch logs multiple structured events. At 100 webhooks/hour and 3 matches each, that's 300+ log lines per hour from just one provider. At 1000 webhooks/hour you're generating visible log volume.

**Plan**: have a log retention policy. Keep recent logs for debugging (hours to days), aggregate older logs into metrics and drop the raw lines.

## The economics of context actions

Context actions are the single biggest knob you can turn on per-run cost. Here's how to reason about them:

### Every context action has a cost-value ratio

- **Cost** = one Nango call (rate limit impact) + tokens for the response in the prompt
- **Value** = how often the agent actually uses the fetched data

If the agent uses the data in 90% of runs, it's worth pre-fetching. If it uses it in 20% of runs, you're wasting 80% of the fetches — let the agent decide via tools instead.

### The "just in case" trap

The most common over-spending pattern: pre-fetching data "just in case the agent wants it." A PR review agent might fetch:

- The PR itself
- All files changed
- All existing comments
- The author's profile
- The repo's CONTRIBUTING.md
- The repo's README.md
- The list of CODEOWNERS
- The linked issue (if any)
- Recent CI runs
- Recent reviews by this reviewer

That's 10 context actions per event. Most runs will use 2–3 of them. The other 7–8 are pure waste.

The correct strategy: pre-fetch only the stuff the agent uses in >80% of runs (probably: the PR, the files, CONTRIBUTING.md). Leave the rest as tools the agent can call if needed.

### Optional context actions

Use `optional: true` for context actions that might fail. An optional fetch that returns nothing is cheaper than a failed run — no wasted execution, just an empty variable in the prompt. Examples:

```yaml
- as: guidelines
  action: repos_get_content
  ref: repository
  params:
    path: ".github/CONTRIBUTING.md"
  optional: true
```

Some repos don't have a CONTRIBUTING.md. Without `optional: true`, the missing file would fail the entire run. With it, the agent just sees an empty `guidelines` variable and proceeds.

## Continuation is a cost optimization

Continuing an existing conversation is usually cheaper than starting a new one, because:

1. The prior conversation history is already loaded, so the agent has context "for free" (no new context actions needed for already-known data)
2. No new sandbox cold-start cost (for dedicated agents)
3. The Bridge conversation is already warm

But it's not free — the token cost of the prior conversation is included in every continuation event. A very long conversation can become MORE expensive than starting fresh because of context bloat.

**Rule of thumb**: continuation is cheaper up to about 20–30 messages. Past that, the prior-context token cost starts outweighing the savings, and fresh conversations might be better. The trigger system doesn't do this automatically — it's a consideration when designing the agent's workflow.

## Monitoring cost in practice

You can't optimize what you don't measure. At minimum, log:

- **Dispatches per hour** (trigger volume)
- **Matched runs per dispatch** (agent count)
- **Skipped runs and reasons** (condition efficiency)
- **Context action counts per run** (waste detector)
- **Run latency** (performance health)

Aggregate by agent ID and by trigger row. If one agent is 10x the cost of the others, investigate. Usually it's either:

- Too many context actions
- Too many tool calls per run (LLM going off on tangents)
- Continuation on very long-lived conversations
- A loop you didn't catch

The third bullet is the silent killer. An agent that started cheap and has been running for weeks can slowly accumulate multi-megabyte conversations that cost more per run than the agent cost in its first week combined.

## When to split a high-cost agent

Sometimes an agent is expensive because it's doing too much. Symptoms:

- The system prompt is 2+ pages
- The context actions list has 6+ entries
- Most runs use more than 10 tools in the conversation
- Token usage per run is consistently above 50K

The fix is usually to split the agent into 2–3 smaller ones, each with a tighter scope. A smaller agent with fewer context actions and a shorter prompt is faster, cheaper, and more reliable. See [01-agent-design-principles.md](01-agent-design-principles.md) for the design-level case.

## Cost-conscious design checklist

Before rolling out a new agent at scale:

- [ ] System prompt is under 500 words
- [ ] Context actions list is 5 or fewer entries
- [ ] Every context action has a justification ("the agent uses this in >80% of runs")
- [ ] Non-essential context actions are marked `optional: true`
- [ ] Conditions exclude known-noisy sources (bots, drafts, closed states)
- [ ] Instructions include an explicit terminal action
- [ ] A terminate rule exists for the natural "done" state
- [ ] Sandbox strategy matches the agent's actual needs (shared by default; dedicated only with justification)
- [ ] Expected run rate is estimated (events/hour × match rate × average run cost)

None of this is hard. Most of it is just making the cost think happen *before* production, not after.

## A note on LLM provider choice

Different LLMs have different cost profiles. A run on a large frontier model costs more than the same run on a smaller model, sometimes by 10x. For cost-sensitive workflows, consider:

- **Cheap models for simple tasks** — triage, labeling, summarization
- **Frontier models for complex reasoning** — debugging, code generation, judgment calls
- **Model switching within an agent** — not directly supported by the trigger system, but you can have two agents with identical triggers and different models, enabling one and disabling the other based on cost vs. quality needs

This is a per-agent decision, not a trigger decision. But it interacts with trigger cost thinking — an agent configured to use an expensive model better have good reasons for the events it fires on.

## Where to go from here

- The loop prevention patterns that stop cost runaways: [04-safety-and-loop-prevention.md](04-safety-and-loop-prevention.md)
- How to monitor an agent in production: [06-observability-and-debugging.md](06-observability-and-debugging.md)
- Worked examples showing cost-conscious agent design: [07-worked-examples.md](07-worked-examples.md)
