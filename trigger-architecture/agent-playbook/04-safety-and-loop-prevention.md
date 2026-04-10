# 04 — Safety and Loop Prevention

The single most dangerous failure mode in a trigger-driven agent system is **the infinite loop**: an agent responds to an event, its response generates a new event, that new event triggers another response, and the cycle repeats until something external stops it. Loops have been the first production incident for every trigger-based agent system I've seen. This document covers how loops happen, how to prevent them, and a few other safety concerns that matter almost as much.

Read this doc before writing your first trigger config. Every mistake in here is one that looks fine on paper and only bites in production.

## The basic loop pattern

Agent A posts a comment on PR #42. The comment triggers the webhook `issue_comment.created`. Agent A is listening for that event. Agent A fires, sees the new comment, responds to it with another comment. That comment fires another event. Repeat until someone notices and pulls the plug.

The simplest version is pure self-reference: agent A triggers on its own comments. More subtle versions involve multiple agents or multiple event types. All of them share the same shape: **the agent's output causes an event that re-triggers the agent** (or a related agent).

### Why this is so common

Three reasons:

1. **Agents naturally act on the surfaces they observe.** A code review agent comments on PRs; a triage agent labels issues; a support agent writes messages. Every one of those actions is an event on the same surface the agent is watching.
2. **Conditions are the only safeguard.** Without explicit filters, the default behavior is "respond to every event that matches the trigger key." The agent's own output matches just fine.
3. **It takes one missed condition to start a loop.** You filter out Dependabot, Renovate, and Mergify — and forget to filter out yourself. That's all it takes.

The fix is simple: **always exclude the agent itself from its own triggers**. Every trigger config that posts back to the surface it's watching needs this filter.

## The self-exclusion pattern

Every trigger that has the agent acting on the same surface it's listening to should exclude its own activity. On GitHub, the filter is usually on `sender.login` or `sender.type`:

```yaml
conditions:
  match: all
  rules:
    # ... your other conditions ...
    - path: sender.login
      operator: not_equals
      value: zira-review-bot[bot]
```

If you use a personal access token or user account, the filter uses that username:

```yaml
- path: sender.login
  operator: not_equals
  value: your-bot-username
```

If you don't know the agent's sender identity at config time, use the bot-type filter as a fallback:

```yaml
- path: sender.type
  operator: not_equals
  value: Bot
```

This excludes all bots, not just yours — a sledgehammer, but it prevents loops when you can't identify your own bot specifically.

### Don't rely on race conditions or cooldowns

Some people try to fix loops with rate limiting or cooldown windows ("don't respond to the same PR within 60 seconds"). This doesn't work:

- **It doesn't actually prevent the loop** — it just slows it down to one cycle per minute.
- **It makes debugging harder** — the loop is still happening, just slowly.
- **It breaks legitimate use cases** — a reviewer makes three quick comments in 30 seconds, and you ignore two of them.

The correct fix is to exclude the agent's own identity entirely. Loops should be impossible, not slow.

## Multi-agent loops

Two agents can loop with each other even if neither loops with itself. Agent A comments on a PR. Agent B is listening for comments. Agent B fires and comments in response. Agent A is listening for comments. Agent A fires and responds. Neither is directly self-referential, but together they loop.

The fix is symmetric: each agent excludes the OTHER agents' activity too.

```yaml
# In Agent A's config
- path: sender.login
  operator: not_one_of
  value: [zira-review-bot[bot], zira-triage-bot[bot], zira-summary-bot[bot]]
```

This gets tedious as you add agents. Two strategies that scale better:

**Strategy 1: Exclude all bots with a single filter.**

```yaml
- path: sender.type
  operator: not_equals
  value: Bot
```

Loses the ability to respond to useful bots (e.g., GitHub Actions bots leaving deployment status), but it's bulletproof against all-bot loops. Use this when your agent doesn't care about bot-originated events.

**Strategy 2: Maintain a shared "zira-agents" list.**

Keep a list of all your bot identities in a shared place (documentation, config). Every agent's `not_one_of` filter uses the full list. When you add a new agent, update every existing agent's filter. Annoying, but reliable.

## Multi-event loops

Agent A triggers on `issue_comment.created`. Agent A posts a comment. That fires `issue_comment.created`. Standard loop — caught by self-exclusion.

But what about this: agent A triggers on `pull_request_review.submitted`. It posts a review. That fires `pull_request_review.submitted`. Same class of loop — still caught by self-exclusion.

More subtle: agent A triggers on `pull_request_review.submitted`. It pushes new commits. That fires `pull_request.synchronize`. Agent A also listens for `pull_request.synchronize` and handles it by fetching the latest files. Not a loop yet — the sync handler doesn't push commits. Safe.

But if the sync handler ALSO pushes commits (e.g., auto-formatting), you have a two-step loop: review → push → sync → push → sync → push... Each step is triggered by the previous step's output. Self-exclusion doesn't help because the commits are triggering `synchronize`, not `review`, and the sender is GitHub itself, not the bot.

**The fix for multi-event loops**: make sure no chain of "event → agent action → new event" forms a cycle. If agent A handles event X by causing event Y, and agent A also handles event Y, make sure handling event Y doesn't cause event X.

In the auto-formatting example, the fix is to make the sync handler idempotent: it only pushes commits if the files aren't already formatted. First run formats, second run is a no-op.

## Write-action discipline

The trigger system enforces that **context actions must be read-only**. The handler validates this at save time and rejects any configuration where a `context_actions` entry references a catalog action with `Access: "write"`.

Why this matters: context gathering runs before the agent even sees the event. If context actions could write, a single misconfigured trigger could accidentally mutate production state across thousands of events without anyone reviewing the changes. By forcing context to be read-only, we guarantee that nothing destructive happens before the LLM has a chance to reason about it.

### Writes happen through tools

If the agent needs to write, it does so via **tools** during the run. Tools are the set of actions the LLM can call during its conversation turns. Each tool call is a deliberate LLM decision, not an automatic pre-fetch. This means:

- The LLM's reasoning is in the loop for every write
- Writes are logged per-run as tool calls, with full arguments
- A write-action misconfiguration affects one run's output, not thousands of runs' side effects

Tools are configured at the agent level (via the agent's `Integrations` JSON and tool allow-list), not at the trigger level. The trigger decides when the agent runs; the agent's tool configuration decides what the agent can do once it's running.

### The read-only context guarantee is load-bearing

Don't try to bypass it. If your trigger needs to write as part of context gathering, you're designing the trigger wrong. Write operations should be tool calls, decided by the LLM at runtime based on the specific event. Pre-fetched writes are a footgun with no useful use case.

## Condition fail-closed by default

A subtle safety property: when conditions can't be evaluated (missing path, malformed value), they fail rather than pass. This is "fail-closed" — safer than fail-open, which would let ambiguous events through.

Example: you have a condition `pull_request.draft not_equals true`. If the webhook arrives on an event where the payload doesn't have a `pull_request` field at all (e.g., an `issue_comment.created` on an issue, not a PR), the path can't be resolved. The condition returns false, the run is skipped. Your agent safely ignores the event.

The alternative — "if we can't evaluate it, assume it passes" — would cause agents to fire on events they weren't designed for. The current behavior is correct.

What this means in practice: you don't need to write defensive conditions like "if this field exists and it's not X." Just write the positive condition, and the dispatcher handles the missing-field case safely.

## The "runaway agent" scenario

Even without a loop, an agent can cause trouble by doing the right thing too often. Common runaway scenarios:

### Scenario 1: Over-commenting

The agent is configured to comment on every PR. A user opens 50 PRs in a short period (e.g., splitting a large feature into small commits). The agent comments on every one within seconds. The user is now flooded with bot comments.

**Fix**: conditions that scope down what counts as "interesting." Only comment on PRs with specific labels, only on PRs targeting specific branches, only on PRs with nontrivial diffs.

### Scenario 2: Over-responding

The agent responds to every comment on a PR. A reviewer leaves 20 inline comments as a line-by-line review. The agent tries to respond to each of them individually, generating 20 of its own comments, each inviting another round of review.

**Fix**: instructions that explicitly say "respond with one summary comment to the full review, not to individual inline comments." Or conditions that only trigger on `pull_request_review.submitted` (which fires once per review) rather than `pull_request_review_comment.created` (which fires once per inline comment).

### Scenario 3: Chain reactions

The agent responds to CI failures by pushing fixes. A push triggers more CI. CI fails again (or fails differently). The agent pushes another fix. Repeat until the PR is a mess of auto-commits.

**Fix**: rate limiting via conditions. "Only respond to CI failures if the failing run is the most recent one and the head SHA hasn't changed in 5 minutes" — harder to express in YAML, so usually done by having the agent self-limit in its instructions: "if you've already pushed a fix for this build, wait for a human before pushing another."

Better yet: scope the agent to specific failure types (e.g., only lint failures, not test failures) and let humans handle the rest.

## Permissions and scope minimization

Every agent should have the smallest set of tools and connections it needs. Don't give an agent write access to production if it only needs to read. Don't add a Slack connection to an agent that only handles GitHub events.

Why: a misconfigured or compromised agent is bounded by its permissions. A reviewer agent with write access to merge PRs can, in a worst-case bug, merge PRs it shouldn't. The same agent with read-only GitHub access and comment-only permissions can only leave bad comments. Both are bad, but one is catastrophic and the other is embarrassing.

Scope minimization is a platform-level concern (which connections the agent has, which tools are enabled), not a trigger-level concern. But it interacts with trigger design: when you extend an agent's triggers to a new event type, make sure the agent's existing permission scope still covers the new job. If it doesn't, create a new agent with the narrower scope.

## Testing for loops before production

Before enabling a new trigger in production, do two things:

### Test 1: Manual trace

Walk through the trigger by hand. Ask: "If this agent does what I'm telling it to do, what events will that produce? Do any of those events match this agent's triggers?"

If the answer is "yes," you have a loop candidate. Either:
- Add a self-exclusion filter
- Narrow the conditions to exclude the agent's own activity by another signal
- Change the agent's action to not produce the triggering event

### Test 2: Staging webhooks

Set up the agent in a staging environment. Fire a few test webhooks (using tools like GitHub's "Redeliver" button or a simple cURL). Watch the dispatch logs (see [06-observability-and-debugging.md](06-observability-and-debugging.md)) for each event. Make sure:

- The agent fires exactly once per test event
- The agent doesn't fire on its own output
- Skipped runs show the expected skip reasons

Only after both tests pass should you roll to production. Even then, watch the production logs for the first few hours to catch any loops that only manifest at scale.

## Circuit breakers

For high-stakes agents (ones with write access, customer-visible output, or the ability to spend real money), consider implementing a **circuit breaker** at the agent level. The pattern:

1. The agent's system prompt instructs it to check a counter in memory before taking destructive action
2. If the counter exceeds a threshold in a short window (e.g., "more than 5 PR merges in the last hour"), the agent refuses to act and asks for human intervention
3. The counter resets after a cool-down period

The trigger system doesn't provide this natively, but you can implement it with the agent's memory feature and some careful prompting. It's not foolproof (agents can forget to check), but it catches the most common runaway scenarios — loops, cascades, over-enthusiastic batch processing.

Use circuit breakers for:

- Agents that merge PRs
- Agents that close issues
- Agents that send customer-visible messages
- Agents that spend money (e.g., trigger builds, provision infrastructure)

Don't bother for:

- Read-only agents
- Agents whose output is only visible to the requesting user
- Agents that only post non-destructive comments

## Emergency stop

Every agent should have an emergency-stop mechanism. The simplest: a flag in the agent's system prompt that says "if this string appears in your memory, refuse to act." When something goes wrong in production, an operator adds the flag to the agent's memory, and the agent stops behaving until the flag is cleared.

More robust: flip `Enabled: false` on the agent's triggers. This stops the dispatcher from even loading them — no runs get enqueued. Recovery is instant (set `Enabled: true` again).

Most robust: revoke the connection the agent depends on. Nango-level revocation is the nuclear option — it stops all events from that source, affecting every agent that uses it. Use only when scoped revocation isn't enough.

Having a written runbook for "the agent is behaving badly" is worth more than any amount of defensive configuration. Know what to do when something goes wrong before it goes wrong.

## A note on trust

Trigger-driven agents operate semi-autonomously on real customer data. Every safety measure in this document exists because the consequences of a misbehaving agent are higher than for a misbehaving function. An agent that posts a bad comment is embarrassing; an agent that merges the wrong PR or closes 500 active tickets is a customer incident.

The right default is low trust: start with read-only permissions, narrow scope, specific conditions, self-exclusion filters. Expand the agent's capabilities only after it has demonstrated good behavior over a meaningful period. "Earn more autonomy by behaving well" is the right progression for agents.

This is the opposite of how we normally think about software features, where "more powerful" is "better." For agents, "less powerful but predictable" beats "more powerful but surprising" every time.

## Where to go from here

- Economics and rate-limiting considerations: [05-economics-and-performance.md](05-economics-and-performance.md)
- Observability: how to catch loops before they escalate: [06-observability-and-debugging.md](06-observability-and-debugging.md)
- Common mistakes that lead to safety issues: [08-anti-patterns.md](08-anti-patterns.md)
