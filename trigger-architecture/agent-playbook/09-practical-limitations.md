# 09 — Practical Limitations

This is the "you can't do that yet" document. Every system has boundaries, and knowing where they are is the difference between a successful rollout and a frustrating one. This doc lists the business-level limitations of the trigger system as it exists today, why each exists, what workaround is available, and (where relevant) when we expect the limitation to lift.

For technical gaps (things that would be fixed by writing more code rather than by design decisions), see [../09-known-limitations.md](../09-known-limitations.md). This doc focuses on limitations that affect how you design and deploy agents.

## 1. No cross-provider conversation threading

**What you can't do**: have an agent's Linear conversation seamlessly continue into a GitHub PR conversation. The two are independent threads even when they're about the same task.

**Why**: resource keys are provider-scoped. Linear's `linear:LIN-123` and GitHub's `octocat/repo#pr-42` are different strings, and the executor's lookup is an exact match. There's no mechanism today to say "these two different-looking strings refer to the same task."

**What to do instead**: the "two agents" pattern. Create one agent for Linear entry (`linear-coder`) and a separate GitHub-triggered agent for PR follow-ups (`linear-pr-follower`). They share the same system prompt and sandbox template. Each has its own conversation threads scoped to its provider. See [../06-multi-provider-patterns.md](../06-multi-provider-patterns.md).

**The workaround's tax**: the agent loses context between "I picked up the task" and "a reviewer commented on my PR." It has to re-orient by reading the PR body (which contains "Fixes LIN-123") and optionally fetching the Linear issue via tools. An extra Nango call at the start of each PR review run. Small cost, occasional cognitive overhead for the LLM.

**When this might lift**: when we have customer signal that the two-agents pattern is painful enough to justify building a cross-provider link mechanism (an agent tool for explicit correlation, or a schema-level link table). Not a priority today.

## 2. One provider per trigger row

**What you can't do**: write a single trigger row that pre-fetches data from multiple providers. Your Slack-triggered agent can't have a context action that fetches from Linear in the same trigger row.

**Why**: each `AgentTrigger` row binds to one connection. Every context action in that row uses the same connection's provider. There's no per-context-action connection override.

**What to do instead**: the agent fetches cross-provider data at runtime via tools. When the Slack-triggered agent needs Linear context, it uses its Linear tools mid-run instead of pre-fetching. Tools are flexible across providers; context actions are not.

**The workaround's tax**: one or two extra agent turns to fetch cross-provider data, compared to having it pre-fetched. Usually negligible.

**When this might lift**: if context-action latency becomes a real bottleneck for multi-provider agents. Unlikely to be an urgent priority because tools already cover this case.

## 3. Intercom, Jira, and most non-GitHub/Slack providers have incomplete catalogs

**What you can't do**: fully use the ref-based shorthand (`ref: conversation`, `$refs.conversation_id`) for Intercom, Jira, Linear, and most providers other than GitHub and Slack. Their trigger catalogs don't have refs defined yet; their resource definitions don't have `ref_bindings` or `resource_key_template`.

**Why**: the catalog revamp was done provider-by-provider. GitHub was first, then Slack. The other providers work mechanically — you can still write triggers and context actions — but you lose the ergonomics of resource-based shortcuts and automatic continuation.

**Slack is now complete**: the Slack catalog has been rewritten from scratch with 10 triggers, 19 actions, 3 resources (`slack_thread`, `slack_channel`, `slack_user`), and the `slack_thread` resource has a `resource_key_template` that uses coalescing refs (`event.thread_ts || event.ts`) to unify top-level messages and in-thread replies into the same thread identifier. Thread continuation works end-to-end on Slack — top-level mentions, in-thread mentions, and thread replies all route into the same agent conversation.

**What to do instead**:

1. Use explicit params in context actions instead of `ref: x` shortcuts
2. Accept that every event creates a new conversation (no continuation) until the resource gets a template
3. Define refs in the trigger catalog's JSON for your use case and regenerate — this is JSON-only work, no Go code changes

**The workaround's tax**: more verbose YAML, no automatic continuation. Workable, but not polished.

**When this might lift**: on a provider-by-provider basis as customers need each provider. Each provider is a ~30-minute catalog edit. Not blocked on anything but prioritization.

## 4. Check run events don't continue conversations

**What you can't do**: have multiple `check_run.completed` events on the same check run continue the same agent conversation. Each one creates a new thread.

**Why**: the `workflow` resource's template references `run_id`, but check run events expose `check_run_id` instead. The template fails to resolve cleanly and produces an empty key, which means "new conversation every time."

**What to do instead**: accept fire-and-forget semantics for check run events. Most check run use cases are one-shot status updates, so this is usually fine. If you need continuation for check runs specifically, you'd need a dedicated `check_run` resource in the catalog — doable, but not built.

**When this might lift**: when someone needs it. Low priority because most check_run use cases don't benefit from continuation.

## 5. `create` and `delete` events depend on webhook headers

**What you can't do**: distinguish GitHub `create` from `delete` events purely from the payload. They have identical body shapes; only the `X-GitHub-Event` header tells them apart.

**Why**: GitHub's webhook design. Both events have `ref`, `ref_type`, `repository`, `sender` and nothing else. The only difference is the event header.

**What to do instead**: rely on Nango to forward the `X-GitHub-Event` header through. If Nango is configured correctly, this works. If the header isn't forwarded, the dispatcher's payload-shape inference will fail for these two events specifically and log an error.

**When this might lift**: if Nango's header forwarding becomes a problem, we'd add a custom GitHub webhook endpoint that reads the header directly. Not currently needed.

## 6. No raw payload templating

**What you can't do**: reference arbitrary payload fields in trigger configs without going through the `$refs` system. For example, `{{$payload.pull_request.commits_url}}` doesn't work.

**Why**: the template resolution layer only understands `$refs.x` and `{{$step.x}}`. Raw payload access would require a new template mode and isn't currently supported.

**What to do instead**: add the field you need to the trigger catalog's `refs` map. It's a JSON-only change in `internal/mcp/catalog/providers/<provider>.triggers.json`. Once the ref exists, use `$refs.x` normally. Better than raw access anyway — refs are named, documented, and work with autocomplete.

**When this might lift**: unlikely to be built, because the ref-first approach is strictly better. The only case where raw access would matter is deeply-nested per-instance fields that can't be statically named — rare.

## 7. No conversation re-opening

**What you can't do**: reopen a closed conversation when a new event arrives on the same resource. Once a conversation is closed (via `terminate_on` or manual action), future events create a new conversation instead.

**Why**: the executor's lookup explicitly filters out closed conversations (`status != 'closed'`). This matches "the original work is done; a new comment is a fresh thread" semantics, which is almost always what you want.

**What to do instead**: if you need sticky continuation across long-term resource lifecycles (e.g., a support ticket that gets closed and reopened many times), either:

1. Don't use `terminate_on` at all — let the conversation drift into inactivity. Sandboxes sleep cheaply.
2. Have the agent summarize state into memory before terminating, so the next conversation can read the summary.

**The workaround's tax**: option 1 means conversations grow forever (but sleep cheaply). Option 2 means the agent has to do extra work at termination. Neither is free.

**When this might lift**: no current plans. The closed-means-closed semantics are a deliberate choice, not a bug.

## 8. No agent-initiated conversation linking

**What you can't do**: have the agent explicitly say "this conversation I'm in should also receive events that match resource key X from now on." There's no tool like `zira_link_resource`.

**Why**: we discussed this extensively and chose not to build it. The reason: agents shouldn't be responsible for plumbing. Their job is the task, not the orchestration layer. See [../08-design-decisions-deferred.md](../08-design-decisions-deferred.md).

**What to do instead**: the two-agents pattern for cross-provider, or structure your agent's scope so it operates on a single resource type with natural continuation.

**When this might lift**: when the "two agents" workaround becomes painful in ways that outweigh the simplicity cost. Unclear when that threshold is.

## 9. No built-in rate limiting per agent

**What you can't do**: set a rule like "this agent can only run 5 times per hour per repository." Rate limiting isn't enforced by the trigger system.

**Why**: no one has needed it yet at a scale that justifies building it. Asynq's per-queue concurrency provides a crude fleet-wide cap, but per-agent or per-resource rate limiting would need to be added.

**What to do instead**:

1. Use trigger conditions to reduce the event volume (narrow scope = fewer runs)
2. Use instructions with self-limiting language ("if you've acted 3 times on this PR in the last hour, stop and ask for help")
3. Use circuit breakers in the system prompt for high-stakes agents

These are all prompt-layer solutions, which means they're soft — the LLM can disobey. For hard rate limiting, you'd need to add it at the dispatcher or executor layer.

**When this might lift**: when a customer hits a rate-runaway in production and we need to add it reactively. Not yet.

## 10. No circuit breakers at the platform layer

**What you can't do**: tell the platform "if any agent produces more than X runs in Y minutes, pause it automatically." Circuit breakers are ad-hoc, implemented per-agent via prompt engineering.

**Why**: no platform-level kill-switch exists beyond `Enabled: false` on individual trigger rows. Disabling agents is manual, which means a runaway needs human intervention to stop.

**What to do instead**:

1. For each high-stakes agent, document the emergency stop procedure (toggle `Enabled` on all its triggers)
2. Have monitoring that alerts on unusual agent run rates
3. Test the stop procedure before you need it

**When this might lift**: when we build better operational tooling. Plan calls for this, no timeline.

## 11. No multi-tenant filtering on shared agents

**What you can't do**: have a single agent serve multiple customers with tenant-specific behavior. The agent's config is fixed per agent row, not per customer.

**Why**: the trigger system is per-org already — each organization has its own agents, its own connections, its own triggers. Within an org, all agents share the same reasoning. There's no "per-repo custom prompt" or "per-team custom behavior" built in.

**What to do instead**: create one agent per tenant. If you have many tenants, this becomes tedious, but the alternative is worse — one agent trying to behave differently per tenant is unpredictable.

For platform builders targeting multi-tenant SaaS use cases, this is a known gap. Most customers don't hit it because they have one team and one agent config.

**When this might lift**: when a multi-tenant platform use case becomes a clear priority. Not yet.

## 12. Webhook redelivery causes duplicate processing

**What you can't do**: assume every webhook is processed exactly once. Webhook redelivery (network glitches, Nango retries, manual redelivery during debugging) can cause the same event to be processed multiple times.

**Why**: asynq task deduplication isn't enabled on the trigger dispatch task. Adding `asynq.Unique(duration, asynq.WithUniqueKey(deliveryID))` would fix it but isn't done.

**What to do instead**: design agents to be idempotent. The continuation mechanism helps — a redelivered event on an open conversation just appends another message, and the agent sees "the same event already discussed" in the history.

For agents that take destructive actions (e.g., merging PRs), add explicit idempotency: check whether the action was already done before doing it again.

**When this might lift**: low-effort fix (a few lines in `tasks.NewTriggerDispatchTask`). Should probably be added soon as defensive hygiene.

## 13. No multi-webhook batching

**What you can't do**: have the dispatcher batch related webhooks into a single agent run. If a user opens a PR, immediately labels it, and immediately assigns a reviewer, that's three separate agent runs, not one.

**Why**: webhooks are processed independently by design. Each dispatch is a separate decision, each with its own `PreparedRun`. Batching would require a debounce window and cross-event correlation logic that doesn't exist.

**What to do instead**: pick trigger events carefully so you don't fire on noise. If you don't care about label changes, don't listen for `issues.labeled` — then the label event won't cause a separate run.

For truly close-together events that should be handled together, consider terminating on the first one and letting continuation handle the follow-ups.

**When this might lift**: unlikely to be built. Batching across webhook events is a significant design change with unclear value.

## 14. Per-trigger instructions don't support advanced templating

**What you can't do**: write instructions with loops, conditionals, or complex transformations. No `{{#each files}}`, no `{{#if condition}}`, no computed values.

**Why**: the template layer supports only `$refs.x`, `{{$refs.x}}`, and `{{$step.x}}` substitution. Full-featured templating (Handlebars, Mustache, Go templates) isn't supported.

**What to do instead**: keep templating to simple variable substitution. For anything more complex, let the LLM handle it. LLMs are good at formatting lists of items, conditional responses, and summaries — give them the raw data and let them format it.

**When this might lift**: no plans. The minimalist approach keeps the dispatcher simple, and LLMs fill the gap naturally.

## 15. No scheduled triggers

**What you can't do**: configure a trigger that fires on a schedule (e.g., "every day at 9 AM") instead of on a webhook event.

**Why**: the trigger system is webhook-driven by design. There's no cron-style scheduler integrated with it.

**What to do instead**: for scheduled work, use asynq's periodic task scheduler directly. You'd write a separate task handler that runs on schedule and manually invokes the agent (creating a conversation and sending a message). This is more code than a simple trigger, but it's straightforward.

**When this might lift**: if enough customers want scheduled triggers and it becomes a priority. Low urgency because webhooks cover most real use cases.

## 16. No A/B testing or gradual rollout tools

**What you can't do**: enable a new trigger for 10% of events, measure behavior, then roll out to 100%. There's no built-in rollout framework.

**Why**: not built. The system assumes triggers are enabled all-or-nothing.

**What to do instead**: use conditions to manually scope the rollout. Start with `repository.name equals single-test-repo`; after verifying, broaden to a few repos; eventually remove the scope entirely. It's manual but it works.

**When this might lift**: if rollout automation becomes a priority. Not yet.

## 17. No metrics or dashboards out of the box

**What you can't do**: open a dashboard and see "runs per hour by agent," "skip rate by reason," "average latency per trigger." These metrics aren't emitted natively.

**Why**: the dispatcher logs structured events but doesn't push to a metrics system. Building the metrics layer is scoped for later.

**What to do instead**: roll your own metrics from the logs. If you have a log aggregation system (Datadog, Loki, etc.), you can build dashboards by counting log lines matched to specific patterns. It's work, but the data is there.

**When this might lift**: when someone prioritizes observability tooling. Small-to-medium project.

## 18. No agent sandboxing across orgs

**What you can't do**: share an agent definition across orgs. Each org has its own agents, its own triggers, its own connections. A "canonical code reviewer" that multiple orgs share isn't supported.

**Why**: org isolation is a core tenet. Cross-org sharing would require careful thinking about permissions and data boundaries.

**What to do instead**: for platform builders, provide agent templates (copy-paste configurations) that each org can instantiate for themselves. This preserves the isolation while making setup easier.

**When this might lift**: uncertain. Depends on product direction and whether a marketplace-style agent store becomes a priority.

## The meta-point: these limits exist for reasons

Every limitation in this doc has a reason — most are intentional design choices, a few are "we didn't get to it yet." None are bugs in the traditional sense; the system does what it's supposed to do, just not what you might wish it did in edge cases.

When planning an agent deployment, use this list as a checklist: which of these limitations apply to your use case? For the ones that do, is the workaround acceptable? If not, is the limit worth building around?

Most successful agent deployments never hit any of these limits. The limits show up when you're stretching the system — trying to build something more ambitious than the design anticipated. That's fine, and often the right move, as long as you know what you're working around.

## Where to go from here

- The technical limitations that pair with these: [../09-known-limitations.md](../09-known-limitations.md)
- The design decisions behind the biggest limits: [../08-design-decisions-deferred.md](../08-design-decisions-deferred.md)
- Multi-provider workaround details: [../06-multi-provider-patterns.md](../06-multi-provider-patterns.md)
- Anti-patterns that often stem from trying to work around limits the wrong way: [08-anti-patterns.md](08-anti-patterns.md)
