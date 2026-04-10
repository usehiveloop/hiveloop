# 03 — Lifecycle and State Management

A conversation is born when the first event arrives, grows as more events land on the same resource, and eventually ends. How you configure that arc determines whether your agent feels coherent and helpful or fragmented and confused. This doc covers how to think about it strategically.

For the technical mechanism (resource keys, terminate rules, how the executor picks up existing conversations), see [../03-lifecycle-and-continuation.md](../03-lifecycle-and-continuation.md). This doc focuses on the business-level decisions.

## The default: continuation within a resource

By default, events on the same resource (same issue, same PR, same Linear ticket) continue the same agent conversation. This is almost always what you want. The agent remembers what it said last time, the reviewer doesn't have to re-explain context, and follow-ups are natural extensions of the thread.

The dispatcher handles continuation automatically via the resource key. You don't write any code for it — you just ensure the resource in the catalog has a `resource_key_template` defined (GitHub's `issue`, `pull_request`, `release`, and `workflow` resources all do). Every event that lands on the same resource flows into the same Bridge conversation.

**Result**: an agent reviewing a PR sees the whole arc — opened, updated, commented on, closed — as one coherent conversation. It can reference what it said in earlier turns. It can notice that a reviewer addressed the point it raised last time. This is the experience users expect.

## When continuation is wrong

There are three cases where you don't want continuation, and it's worth being explicit about each.

### 1. The event isn't about a resource

Push events aren't about a branch the way a PR is about a PR. A branch doesn't have a "conversation" — each push is its own moment. For these events, the catalog's `repository` resource has no template, so the resource key comes back empty and every push creates a new conversation. This is correct behavior — don't override it.

Same for release events: each release is a discrete moment, not a continuing story.

### 2. The conversation has been closed

When you terminate a conversation (see below), future events on the same resource don't reopen it. They create a new conversation. This is also correct — the original task wrapped up, and a new event after that is a new interaction. If a reviewer comments on a merged PR a week later, that's a fresh thread, not a resurrection of the original review.

If you genuinely need "reopen on new activity" semantics, the current system doesn't support it directly. You'd need to not use `terminate_on` at all, letting the conversation drift into inactivity and relying on sandbox sleep to reclaim resources. Most use cases don't need this — closed-means-closed is cleaner.

### 3. You need genuinely isolated runs

Some agents run in "fire and forget" mode: each event is a self-contained unit of work, and the agent shouldn't carry context forward. A notification dispatcher, a metric emitter, a logger — none of these need continuation. For these, use a resource whose template is empty, or configure a trigger whose `resource_type` points at such a resource (e.g., use `resource_type: "repository"` to avoid carrying state).

Note: most agents don't want this. If you're debating whether you want fire-and-forget or continuation, you probably want continuation.

## When to terminate

Terminate rules are how you say "when this event happens, this agent's job on this resource is done." The model is simple: the rule matches the event, the executor closes the Bridge conversation, and future events on the same resource create a new conversation instead of continuing the old one.

### The three common patterns

**Pattern 1: Terminate on the "happy path" ending.** The resource has a natural "done" state and the agent wraps up when it arrives. PR merged, issue closed, ticket resolved, release shipped.

```yaml
terminate_on:
  - trigger_keys: [pull_request.closed]
    conditions:
      match: all
      rules:
        - path: pull_request.merged
          operator: equals
          value: true
    instructions: |
      The PR was merged. Post a brief summary and thank the reviewer.
```

**Pattern 2: Terminate on the "dead path" too.** The resource also has a "not happening" ending — PR closed without merge, issue marked as duplicate, conversation abandoned. You want the agent to stop tracking these too, often silently.

```yaml
terminate_on:
  - trigger_keys: [pull_request.closed]
    conditions:
      match: all
      rules:
        - path: pull_request.merged
          operator: equals
          value: false
    silent: true
```

**Pattern 3: Terminate on a hand-off.** The agent's job ends when a human takes over. A support draft-writer terminates when an admin sends a reply. A triage bot terminates when the ticket gets assigned to a human.

```yaml
terminate_on:
  - trigger_keys: [conversation.admin.replied]
    silent: true
```

### Graceful vs. silent close

By default, termination is **graceful**: the agent runs one more time on the terminating event, with fresh context and the rule's instructions, before the conversation closes. This lets the agent post a goodbye, a summary, or a final status update.

Set `silent: true` to skip the final run entirely. The executor just closes the conversation without running the agent at all. Use silent for:

- Events where there's nothing meaningful to say (PR closed without merge)
- Events where the agent might interfere (human admin took over the conversation)
- Cost-sensitive flows where the final run isn't worth the tokens
- Scenarios where the event is already a hand-off and posting more would be noise

Graceful close is the default for a reason: most agents have something useful to say on close. Silent is opt-in.

### First-match-wins ordering

You can stack multiple terminate rules with different conditions. The dispatcher evaluates them in order and picks the first whose conditions pass. Use this for branching behavior at termination:

```yaml
terminate_on:
  # Merged → graceful close with summary
  - trigger_keys: [pull_request.closed]
    conditions:
      match: all
      rules:
        - path: pull_request.merged
          operator: equals
          value: true
    instructions: "PR merged — post summary"

  # Not merged → silent close
  - trigger_keys: [pull_request.closed]
    silent: true
```

The first rule only fires if `merged: true`. When a PR closes without merging, it falls through to the second rule, which silently closes.

Order matters. Put the more-specific rules first; use catch-all rules last.

### Inheritance

Terminate rules inherit the parent trigger's conditions by default. If the parent says "skip drafts," the terminate rule also skips drafts — you never close a conversation you were never tracking. This is almost always what you want. Opt out with `ignore_parent_conditions: true` only when you have a specific reason (e.g., the terminate rule's scope is genuinely different from the parent's).

### When NOT to write a terminate rule

Three cases where you should leave `terminate_on` empty:

1. **The resource has no natural ending.** Push events, releases, one-shot actions — nothing to terminate.
2. **You want the conversation to persist indefinitely.** Sandboxes sleep when idle, so there's no cost penalty to leaving a conversation open forever. If the agent might still be useful later, don't close it.
3. **You're not sure what "done" means yet.** Terminate rules are additive — you can add them later. Don't guess at closing conditions during initial setup.

The system is designed so that "no terminate rule" means "conversation stays open as long as events keep arriving." That's a reasonable default.

## Long-running conversations

Some agent jobs genuinely have no natural end point. A support agent watching an Intercom conversation might go weeks without a clear terminal event. A feature-flag governance agent might react to dozens of events over a release cycle. For these, the conversation lifecycle is measured in weeks or months, not minutes.

What to keep in mind:

- **The Bridge conversation grows over time.** Every event adds messages. Very long conversations can hit context limits or get unwieldy for the LLM to process.
- **Sandboxes sleep between events.** No cost penalty for long idle periods.
- **Memory matters more.** Long-running agents benefit from explicit memory updates. "Last time I reviewed this PR, the author agreed to change X" is worth remembering if the PR sits for weeks.
- **Consider periodic summaries.** If the conversation crosses a certain size threshold, have the agent summarize its state into memory and start a fresh "chapter." The trigger system doesn't do this automatically — it's an agent-level pattern you build into the system prompt.

For most agents, this isn't an issue. Most conversations live for hours or days, not weeks. Just know that if you design a conversation to live longer, you need to plan for it.

## The three state layers

Agents look stateful to the user, but the state lives in three different places, each with different properties. Knowing which layer to use is the key to designing agents that behave predictably.

### Layer 1: The webhook payload

The webhook contains the current snapshot of the resource. When a `pull_request.synchronize` event arrives, the payload has the current PR state — title, description, files, reviewers, mergeable state, everything. This is always fresh, always authoritative for what's happening right now.

**Use the payload for**: the current facts of the resource. Don't ask the agent to remember what the PR looked like last time; the new payload has the current view, and the old view is in the conversation history if needed.

### Layer 2: The Bridge conversation

The conversation history holds every message the agent has sent or received for this resource. On continuation events, the agent picks up with the full history in context. This is where short-term memory lives — "I said X three messages ago, the user responded with Y."

**Use the conversation for**: threading context across events on the same resource. This is the default and covers most agent use cases.

### Layer 3: Agent memory (`SharedMemory: true`)

When an agent has `SharedMemory: true` set on the `Agent` model, it can read and write long-lived facts that persist across conversations. "This repo's team prefers snake_case in tests" is the kind of fact that belongs in memory — it applies to every PR review, not just one.

**Use memory for**: cross-conversation facts. Team conventions, long-lived preferences, lessons learned from past interactions.

**Don't use memory for**: facts that should be fresh per event (PR state), facts that can be re-derived from the payload (author, files, labels), facts that change frequently (review comments on the current PR).

### Choosing the right layer

| What | Where |
|---|---|
| Current resource state | Payload |
| Current file contents | Payload (via context action) |
| What the agent said last time | Conversation history |
| What the user said earlier in the thread | Conversation history |
| Team conventions | Memory |
| Long-term preferences the user expressed | Memory |
| Which PRs the agent has reviewed this week | Memory (rarely; usually the conversation is enough) |

The default for a new agent is **payload + conversation**. Add memory only when you have a concrete use case that benefits from it. Memory is powerful, but it's also a source of correctness bugs — agents can misremember, act on stale facts, or contradict the current payload if they trust memory over the event.

## Handling out-of-order events

The dispatcher makes no guarantees about ordering across webhooks. If GitHub fires `pull_request.synchronize` and `pull_request_review.submitted` within the same second, they might be processed in either order (asynq tasks run concurrently on the worker pool).

This means your agent might see:

- A review arrive before it knows about the commit that triggered the review
- A CI status land before the agent has processed the push that kicked off the run
- A label change and a comment interleaved in an unexpected order

**Design for this.** The agent should re-read the current state on each event rather than assuming continuity from the last event. Context actions are your friend here — they always return the current state, not the state from last time.

In practice, this is fine because:

- Continuation means the agent has the prior messages as context
- Fresh context actions give it the current resource state
- The agent's reasoning is robust to "oh, I also see X happened recently"

It's a problem only when agents assume strict ordering and are brittle to violations. Don't write instructions that say "respond only if the last thing that happened was X" — assume events can arrive out of order.

## State management anti-patterns

**Writing per-conversation state into payload-shaped fields**: the agent can't modify the webhook payload. If you need cross-event state, use memory or the conversation history, not the payload.

**Relying on `pull_request.updated_at` for ordering**: timestamps are a guide, not a guarantee. Use them to inform the agent ("this PR was last updated 2h ago") but don't use them to enforce order ("only respond if this is newer than what I saw last time").

**Trying to "undo" previous agent actions in a new run**: if the agent posted a bad comment in an earlier turn, the new run can acknowledge it ("I see my earlier comment was unhelpful") but can't delete it without a tool call. Design around this — prefer writing nothing to writing wrong, since wrong is permanent.

**Treating continuation as infinite memory**: the conversation history has a size limit (context window). For very long-running conversations, the oldest messages will be truncated. Don't rely on precisely remembering what happened 50 messages ago; the LLM might not see it anymore.

## Where to go from here

- The technical mechanism for continuation and termination: [../03-lifecycle-and-continuation.md](../03-lifecycle-and-continuation.md)
- The failure mode that most often bites agents with long-lived conversations: [04-safety-and-loop-prevention.md](04-safety-and-loop-prevention.md)
- Worked examples with explicit lifecycle choices: [07-worked-examples.md](07-worked-examples.md)
