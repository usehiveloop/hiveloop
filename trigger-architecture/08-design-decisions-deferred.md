# 08 — Design Decisions Deferred

This document captures things we considered, designed, and consciously chose NOT to build. Each entry explains what the feature would do, what the tradeoff was, and what would make us revisit the decision. The purpose is to keep future us from re-litigating settled questions — and from forgetting that they were discussed at all.

## Cross-provider conversation continuation

**What it would do.** When an agent is triggered by a Linear issue, creates a GitHub PR, and later receives a review on that PR, the review's conversation would automatically be linked to the original Linear-issue conversation. One continuous thread spanning `linear:LIN-123 → github:owner/repo#pr-42 → merge`.

**What we considered.**

1. **Link table + agent-initiated linking**. Add an `agent_conversation_resource_links` many-to-many table. Give the agent a `zira_link_resource(connection_id, resource_type, resource_id)` tool that inserts a row tying the current conversation to a future resource key. The executor's lookup does a two-step check: primary resource key first, then fall back to the link table.
2. **Task key templates**. Add a `task_key_template` field on `AgentTrigger` that produces a cross-provider identifier. Regex-capture over payload bodies (`task_key: "linear:{{$pr.body | regex 'LIN-(\\d+)'}}"`). Brittle and hard to debug.
3. **Task abstraction layer**. Introduce a first-class `tasks` table that conversations belong to. Every agent conversation is attached to a task; tasks span providers. Much bigger schema change, forces every agent into a task model even when it doesn't need one.

**Why we deferred.** Two reasons. First, the user's explicit preference: *"we should default to doing the heavy lifting ourselves, so agents can focus only on their tasks."* The link-table approach requires the agent to call a correlation tool during its work — that's asking the agent to do plumbing, which is exactly the wrong direction. Second, the complexity was spiraling. The dispatcher PR already added `resource_key` and `terminate_on`; piling cross-provider correlation on top in the same cycle would be a lot to validate, test, and document at once. And the workaround is acceptable.

**The workaround.** Two agents with shared config, one per source. Each agent has its own scope of work and its own conversation threads. The Linear agent's work thread ends when the PR is opened; the PR-review thread is a separate, shorter thread that starts fresh. The PR body contains "Fixes LIN-123" as a backlink, which gives the agent enough context to re-orient. See [06-multi-provider-patterns.md](06-multi-provider-patterns.md) for the full pattern.

**What would make us revisit.** Real customer signal that the two-agents pattern is painful — "we really need the Linear thread and the PR thread to be one continuous conversation because [specific reason]." Until that signal arrives, YAGNI.

## Multi-provider context gathering within one trigger row

**What it would do.** A single `AgentTrigger` row could have context actions that span multiple providers. A GitHub PR opened event could fetch Linear issue details (from a linked connection) as part of its pre-flight context gathering, before the agent even starts its run.

**What we considered.** Adding a `connection_id` field to each `ContextAction` so the dispatcher routes each action to the right connection. The catalog lookup for each action would use that connection's provider. Per-action provider validation would run at save time.

**Why we deferred.** Because the agent already has tools for every provider it's connected to. Once the agent is running, if it needs to fetch a Linear issue while processing a GitHub PR event, it uses its Linear tools mid-turn. That's exactly what tools are for. Context gathering exists to pre-fetch deterministic data that the agent would otherwise waste turns on — it's not meant to replace tools, and the common case for triggers is "one provider, one fetch." If cross-provider pre-fetching becomes a real bottleneck in practice, add it then; not preemptively.

**What would make us revisit.** Customer reports of "my agent wastes N turns fetching obvious cross-provider context" where N is large enough to hurt UX or cost.

## UI bundle_id for grouping related triggers

**What it would do.** Add a nullable `bundle_id uuid` column on `agent_triggers`. Triggers that share a `bundle_id` would be presented as a single logical unit in the UI, making it easier to manage agents with multiple trigger rows (like the Linear/Slack coding agent with its four-trigger setup).

**What we considered.** Just adding the column. No dispatcher changes, no validation changes, no runtime semantics — it's a purely cosmetic grouping for the frontend. A `CREATE TABLE ... ADD COLUMN bundle_id UUID` migration plus a small API change to accept/return it.

**Why we deferred.** We haven't seen any evidence that users actually find the multiple-row approach painful. Users rarely have more than 4 trigger rows per agent; most will have 1 or 2. Adding a grouping concept before there's pain is premature generalization. The right time to add this is when a real customer says "I have 10 triggers on this agent and the UI is unreadable."

**What would make us revisit.** UI feedback from real users, or a design session that identifies a specific ergonomic problem the current structure causes.

## Raw payload templating (`{{$payload.x.y}}`)

**What it would do.** Let trigger configs reference fields in the raw webhook payload directly, without routing through the catalog's `refs` map. Useful for providers whose trigger catalogs don't yet define refs (e.g., Intercom's current state).

**What we considered.** Adding a third template resolution mode to `template.go`:

- `$refs.x` — resolved from the ref map (dispatcher-time)
- `{{$refs.x}}` — same, mustache syntax
- `{{$step.x}}` — deferred to executor (context action result)
- **New**: `{{$payload.x.y}}` — resolved from the raw webhook body (dispatcher-time)

The implementation is ~20 lines in `template.go` and `context_builder.go`.

**Why we deferred.** The cleaner fix is to add refs to the catalog for providers that lack them. Refs have better documentation, discoverability in the UI, and autocomplete support. Raw payload templating is an escape hatch that would cause users to write harder-to-maintain configs. It's better to spend 5 minutes adding refs to a provider's trigger catalog than to let configs reach into raw payloads.

**What would make us revisit.** A case where the catalog can't possibly expose a ref that the user needs — e.g., a deeply nested field that varies per sub-event and can't be statically pathed. Unlikely but possible.

## Dispatch handled inline in the HTTP handler

**What it would do.** Skip the `TypeTriggerDispatch` asynq task entirely. Run the dispatcher directly inside the Nango webhook handler and enqueue only the per-agent run tasks.

**What we considered.** This was tempting because the dispatch step is microseconds of CPU + one DB query — small enough to run synchronously without hurting webhook response times. Asynq feels like overhead for work that fast.

**Why we chose asynq.** Three reasons:

1. **The user's explicit ask.** From the original runtime design conversation: *"once we receive a webhook dispatch, we send the entire payload to asynq job that handles all of this logic in the background with extremely detailed logging."* Keeping dispatch in asynq matches this ask, gives visible logs in worker output, and means a slow DB query never delays the webhook ACK.
2. **Retries.** If the DB hiccups during the trigger query, asynq retries the dispatch. Inlined, the webhook would fail and Nango/GitHub would redeliver later — which works but slows things down and gives us less control.
3. **Symmetry across providers.** Every provider's HTTP handler does the same thing: parse, resolve connection, enqueue dispatch task. The dispatch logic lives in one place and never has to know about HTTP. The Intercom custom endpoint (when it exists) uses the same path.

**What would make us revisit.** If asynq becomes a bottleneck in practice, or if we need to return dispatch outcomes synchronously to the webhook caller for some reason. Neither is likely.

## Monolithic job: dispatch + execution in one asynq task

**What it would do.** Instead of splitting dispatch and execution into separate task types, run both in a single `TypeTriggerDispatch` task. Load matching triggers, build `PreparedRun`s, fire the context actions against Nango, start Bridge conversations — all in one asynq handler.

**What we considered.** Simpler structure, one asynq task type, one delivery_id trace from webhook to conversation.

**Why we rejected it.** The pain shows up under load:

- If agent A's first context action takes 30 seconds, agents B and C wait. The whole job is at the mercy of the slowest agent.
- One retry retries everyone. If A succeeded but B failed, retrying re-runs A — duplicate conversations and duplicate Nango calls. Per-agent idempotency keys become mandatory.
- Asynq's `MaxRetry`/`Timeout`/`Queue` settings apply to the whole job. Can't say "agent A is critical, agent B is bulk."
- A single panic in agent C kills the whole batch — A and B never run.

**What we chose instead.** Dispatch is its own asynq task. It produces `[]PreparedRun` and (in the next PR) enqueues one child task per non-skipped run. Execution tasks have their own retry budgets, own timeouts, own queues. Per-agent isolation, per-agent idempotency keys like `dispatch:{delivery_id}:agent:{agent_trigger_id}`.

**What would make us revisit.** Nothing specific. This is a settled design.

## GitHub direct webhook endpoint

**What it would do.** Add `POST /internal/webhooks/github` as an HTTP handler separate from the Nango webhook path. Verify `X-Hub-Signature-256` against a per-integration secret stored on the `Integration` row. Parse the GitHub body directly, enqueue `TypeTriggerDispatch`.

**What we considered.** This lets us bypass Nango for GitHub webhook delivery, reducing latency (no extra hop), giving us full control over signature verification, and removing a dependency on Nango's webhook forwarding behavior.

**Why we deferred.** Nango already handles GitHub App installation management, signature verification, and webhook delivery. Routing through Nango is the path of least resistance and keeps a single webhook ingress code path in the handler. Adding a second path doubles the maintenance surface (two handlers to keep in sync) for a latency win that's not currently painful.

The dispatcher is already designed to be called from multiple webhook sources — the `dispatchTrigger` function in `nango_webhooks_dispatch.go` could be called from any HTTP handler once the caller resolves the connection. So adding a GitHub-direct endpoint later is additive, not a rewrite.

**What would make us revisit.** Nango becoming a bottleneck for GitHub event volume, or a feature we need (e.g., GitHub App-specific event types) that Nango doesn't forward cleanly.

## Per-trigger conditions auto-inheritance into context actions

**What it would do.** Let context actions reference the parent trigger's condition results or ref extractions in a more structured way than raw string substitution. For example, a context action could say "only fire if the trigger's conditions all passed at strength X."

**What we considered.** Briefly. The use case was "skip expensive context fetches when the conditions are about to fail anyway." A condition pre-flight could short-circuit the context build.

**Why we didn't.** Conditions already short-circuit — the dispatcher evaluates conditions before building context requests for runs that fail the check, and builds context requests afterwards only for observability. Adding a more structured conditional context step would complicate the config for a marginal gain.

**What would make us revisit.** If context action volume per trigger grows to the point where wasted builds hurt dispatcher latency. Current tests have 1–4 context actions per trigger; none take appreciable time to build.

## A `require_existing: true` flag on terminate rules

**What it would do.** When a graceful terminate rule fires but no existing conversation is found for the resource key, skip the run instead of creating a brand-new conversation just to close it.

**What we considered.** This is the current edge case for graceful close: if a `pull_request.closed` terminate event arrives and the agent never saw the `pull_request.opened` event (historical replay, webhook redelivery, ordering weirdness), the executor creates a fresh conversation, sends the final message, closes. The agent gets one short-lived run to say goodbye to a PR it never knew existed, which is usually fine but sometimes annoying.

**Why we deferred.** The default behavior is acceptable. Adding a flag adds configuration surface, and the improvement is marginal. If customers complain that graceful terminates are creating ghost conversations, add the flag then.

**What would make us revisit.** Customer reports of "my agent keeps getting woken up for PRs it already forgot about."

## A `conversation.reopen` operator for terminate rules

**What it would do.** When an event arrives on a previously-closed conversation's resource, instead of always creating a new conversation (current behavior), optionally reopen the old one based on a configurable rule.

**What we considered.** The scenario: a PR is merged and the conversation is closed; later, a comment arrives on the merged PR. Currently the executor creates a new conversation because the old one is `status = 'closed'`. But for some use cases, reopening the original might be cleaner.

**Why we didn't.** "New conversation for comments on closed PRs" matches human intuition — the original work is done, and a later comment is a fresh thread. Reopening would create confusion about "is this the same conversation or a new one?" and the current behavior is what most users would expect without thinking about it.

**What would make us revisit.** A use case where reopening is semantically important (e.g., a long-running support ticket that gets closed and reopened multiple times).

## Summary

Nothing on this list is a "never." Every deferred feature has a revisit condition. The goal of writing them down is to make sure we're making conscious tradeoffs rather than forgetting options exist. When a customer or a bug report surfaces one of these, we can come back and see the original reasoning, then decide whether the tradeoff has changed.

## Where to go from here

- The current limitations that these deferrals leave in place: [09-known-limitations.md](09-known-limitations.md)
- The multi-provider pattern that compensates for the biggest deferral (cross-provider continuation): [06-multi-provider-patterns.md](06-multi-provider-patterns.md)
