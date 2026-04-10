# 09 — Known Limitations

Every system has gaps. This document lists the ones we know about in the trigger system as it stands today, what each gap means in practice, and what would close it. If you're debugging behavior that doesn't match your expectations, start here — there's a good chance the answer is already documented.

Limitations are organized by layer: dispatcher, catalog, executor contract, and operational.

## Dispatcher limitations

### No cross-provider conversation continuation

**Symptom.** An agent triggered by a Linear issue and later responding to a GitHub PR review doesn't continue the same conversation. The two threads are independent, each with its own context.

**Why.** Resource keys are provider-scoped by construction. Linear's `linear:LIN-123` and GitHub's `octocat/repo#pr-42` are different strings, and the lookup is an exact match. The dispatcher has no mechanism for correlating them.

**Workaround.** Two agents with shared config (same system prompt, same tools), one per source. Each agent has its own conversation threads scoped to its source. See [06-multi-provider-patterns.md](06-multi-provider-patterns.md). The `Fixes LIN-123` backlink in the PR body gives the agent enough context to re-orient.

**What would close it.** A link table + agent-initiated correlation, explicitly deferred. See [08-design-decisions-deferred.md](08-design-decisions-deferred.md).

### Raw payload templating not supported

**Symptom.** You can't write a context action that references a field directly in the webhook body without going through `$refs`. `{{$payload.data.item.id}}` is not a recognized template form.

**Why.** The template layer only resolves `$refs.x`, `{{$refs.x}}`, and `{{$step.x}}`. Raw payload access would require a fourth resolution mode.

**Workaround.** Add the field you need to the catalog's `TriggerDef.Refs` map. This is a JSON-only change in `internal/mcp/catalog/providers/<provider>.triggers.json`, no code required. Once the ref exists, `$refs.x` resolves it.

**What would close it.** ~20 lines of new template-resolution code in `template.go` and `context_builder.go`. Explicitly deferred — the ref-catalog approach has better discoverability and autocomplete.

### `create` and `delete` events are header-dependent

**Symptom.** The dispatcher can't distinguish `create` from `delete` events by payload shape alone. When the `X-GitHub-Event` header isn't forwarded, both events look identical and the inference returns empty.

**Why.** `create` and `delete` have structurally identical bodies (`ref`, `ref_type`, `repository`, `sender` — nothing else distinguishes them). The type lives only in the header.

**Workaround.** Ensure Nango is configured to forward the `X-GitHub-Event` header through to our webhook. This is a Nango configuration question, not a code change on our side.

**What would close it.** Nothing in our code. If Nango stops forwarding the header, we'd need to add a custom GitHub webhook endpoint that reads the header directly.

### Check run events have no continuation key

**Symptom.** Multiple `check_run.completed` events on the same check run don't continue a conversation. Each fires as a new thread.

**Why.** The `workflow` resource's template is `$refs.owner/$refs.repo#run-$refs.run_id`, which expects the ref `run_id`. `check_run.completed` doesn't expose `run_id` (it exposes `check_run_id` and `check_run_name` instead), so the template substitution fails closed and produces an empty key.

**Workaround.** For most use cases this is actually fine — check runs are usually one-shot status updates, not conversational. If you need continuation for check runs specifically, you'd want a dedicated `check_run` resource in the catalog with its own template referencing `check_run_id`.

**What would close it.** Add a new resource `check_run` in `cmd/fetchactions-oas3/config.go` with the appropriate template and path prefixes, then regenerate. Move the `check_run.completed` trigger's `resource_type` to point at the new resource.

### No ordering guarantees across webhooks

**Symptom.** If GitHub fires `pull_request.synchronize` and then `pull_request_review.submitted` in close succession, there's no guarantee the dispatcher processes them in that order. They might enqueue or execute in the opposite order.

**Why.** Asynq queues are concurrent by design. Each webhook produces one asynq task; the worker pool processes them in parallel. Within a single webhook's dispatch, runs are sorted by trigger ID for deterministic order, but there's no cross-webhook sequencing.

**Workaround.** Agents should be designed to handle out-of-order events — a review arriving before the agent knows about the commit that was just pushed is a normal state, not an error. The agent re-reads the PR state on each event anyway (via context actions), so it recovers.

**What would close it.** Per-resource ordered queues (e.g., hash the resource key to a fixed worker). Not built, not currently needed.

### Skipped runs still run context-request building

**Symptom.** When a run is skipped due to a failing condition, the dispatcher still builds the context requests for observability. This is wasted work if conditions nearly always fail for this trigger.

**Why.** Debugging visibility is better than CPU savings for runs this cheap. "Why did my trigger not fire?" is much easier to answer when you can see the requests that would have been fired.

**Workaround.** None needed — the overhead is microseconds per run.

**What would close it.** A config flag on `AgentTrigger` that says "skip context building on skipped runs." Not worth the complexity.

### No per-webhook deduplication

**Symptom.** If the same webhook is delivered twice (network glitch, Nango retry, manual redelivery), the dispatcher processes both deliveries and potentially creates duplicate runs.

**Why.** Asynq has a `Unique()` option for task dedup, but we don't use it on `TypeTriggerDispatch`. Adding it requires a stable unique key per webhook, which should be the provider-assigned delivery ID (`X-GitHub-Delivery` for GitHub).

**Workaround.** The executor's `ensureConversation` lookup handles the common case: a redelivered `pull_request.synchronize` finds the existing conversation and sends the same message twice. The second send is harmless — the agent just processes the same event twice. For idempotency-sensitive cases, the executor would need explicit dedup.

**What would close it.** `asynq.Unique(24*time.Hour, asynq.WithUniqueKey(deliveryID))` on the dispatch task. One line in `tasks.NewTriggerDispatchTask`. Should probably add this soon.

## Catalog limitations

### Intercom triggers lack refs

**Symptom.** The Intercom provider has trigger definitions, but most of them have empty `refs` maps. Users can't reference `$refs.conversation_id` in their YAML because the ref isn't extracted from the payload.

**Why.** The trigger catalog is hand-maintained for each provider. GitHub got full refs as part of this design work; Intercom didn't. It's a one-time catalog edit per provider, not a code gap.

**Workaround.** Short-term: use raw payload access — not supported, so actually there's no current workaround beyond adding the refs. This is one of the gaps that prompted the "two agents with shared config" pattern discussion.

**What would close it.** Edit `internal/mcp/catalog/providers/intercom.triggers.json` and add appropriate `refs` entries for each trigger. Example:

```json
"conversation.user.replied": {
  "refs": {
    "conversation_id": "data.item.id",
    "contact_id": "data.item.contacts.contacts.0.id",
    "state": "data.item.state"
  }
}
```

One-time edit, no code changes, no regeneration needed (triggers files aren't generated — they're hand-maintained).

### Intercom resources lack `ref_bindings` and templates

**Symptom.** Intercom context actions can't use `ref: conversation` — the `conversation` resource has no `ref_bindings`, so there's nothing to auto-fill from. Intercom conversations also have no `resource_key_template`, so every event creates a new conversation.

**Why.** Same reason as above — the resource definitions haven't been filled out. The `intercom.actions.json` file lacks `resources` with these fields populated.

**Workaround.** Use explicit `params` in context actions until the resource is filled out. Continuation doesn't work at all for Intercom until a template is added.

**What would close it.** Add resource definitions in the generator config (if Intercom is OpenAPI-driven) or directly in the hand-maintained actions file, then regenerate or edit. Same pattern as the GitHub resources.

### `repos_upload_release_asset` is missing

**Symptom.** No way to upload a release asset through the catalog.

**Why.** The action's endpoint is on `uploads.github.com`, not `api.github.com`. The current catalog assumes one host per provider, and `ExecutionConfig.Path` is just a path — no way to specify a different host.

**Workaround.** None in the catalog. An agent that needs to upload release assets has to use raw HTTP via its own tools, not the catalog-routed Nango proxy.

**What would close it.** Add a `Host` field to `ExecutionConfig` that overrides the default. Plumb it through the executor when firing the request. Small change but requires touching the executor path, so deferred.

### Orphaned schemas

**Symptom.** 15 schemas in `github.actions.json` are defined but never referenced by any action's `response_schema` or another schema's `schema_ref`.

**Why.** Generator drift over time. Some are superseded by newer schemas; some are naming inconsistencies (`branch-short` vs `short-branch`). The audit's orphan detection was conservative and didn't follow `items.$ref` references, so the real orphan count is probably lower than 15.

**Workaround.** None needed — orphans cost nothing at runtime.

**What would close it.** A catalog-cleanup pass that drops unused schemas during generation. Low priority.

### Discussions don't have a dedicated resource

**Symptom.** `discussion.created` and `discussion_comment.created` point at `resource_type: "repository"`, which has no template. Every discussion event creates a new conversation even for events on the same discussion.

**Why.** Discussions didn't get a dedicated resource during the catalog revamp. They work structurally like issues but are in a separate GitHub API surface.

**Workaround.** For most use cases, "new conversation per discussion event" is fine. Discussions aren't usually as thread-heavy as issues.

**What would close it.** Add a `discussion` resource to `cmd/fetchactions-oas3/config.go` with its own template `$refs.owner/$refs.repo#discussion-$refs.discussion_number`, move the discussion triggers to point at it, regenerate.

## Executor contract limitations

The executor doesn't exist yet. These are limitations the executor PR will inherit and need to address.

### Schema changes required for continuation

**What the executor needs.** Two new columns on `agent_conversations`:

```sql
ALTER TABLE agent_conversations
  ADD COLUMN connection_id UUID,
  ADD COLUMN resource_key TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_agent_conversations_resource
  ON agent_conversations (agent_id, connection_id, resource_key)
  WHERE resource_key != '';
```

The partial index excludes user-initiated conversations (empty resource key) so the lookup stays fast.

**What the dispatcher already provides.** `PreparedRun.ConnectionID` and `PreparedRun.ResourceKey` are populated for every run. The executor just reads them.

### `{{$step.x}}` resolution is deferred

**What the executor needs.** After each context request returns, the executor resolves `{{$step.x}}` placeholders in the next request's params (and in the instructions) using the response data. The dispatcher already records which steps are referenced via `DeferredVars` on `PreparedRun` and `ContextRequest`, so the executor knows which results to thread through.

**What the dispatcher already provides.** The full list of deferred steps, validated at dispatch time (a reference to a step that doesn't appear earlier is a build error). The executor can trust that every step name in `DeferredVars` corresponds to a real `ContextRequest.As` earlier in the slice.

### No retry isolation per agent yet

**What the executor needs.** The design calls for fan-out — dispatch produces `[]PreparedRun`, the executor enqueues one child task per non-skipped run, each child task runs independently with its own retry budget. This is the executor's job, not the dispatcher's.

**What the dispatcher already provides.** Skipped runs are filtered by the executor (check `run.Skipped()`). Non-skipped runs go into child tasks. The dispatcher's contract is "here are the runs to execute"; how they're scheduled is executor-land.

### Silent close has no conversation-to-close in edge cases

**Symptom (future).** A terminate-silent rule fires but the lookup finds no existing conversation for the resource key. Nothing to close.

**Expected behavior.** The executor logs and exits. No side effects, no ghost conversation created.

**What the dispatcher already provides.** A `PreparedRun` with `RunIntent = terminate`, `SilentClose = true`, and a valid `ResourceKey`. The executor's job is to handle the no-match case gracefully.

## Operational limitations

### Postgres test setup is required for the real-DB test

**Symptom.** `TestGormAgentTriggerStore_FindMatching` skips unless Postgres is reachable on `127.0.0.1:5433`.

**Why.** The test uses `gorm.Open(postgres.Open(dsn))` and pings the connection. No Postgres, no test run.

**Workaround.** Run `make test-setup` to bring up the docker-compose services, then run tests. Or set `DATABASE_URL` to point at any compatible Postgres instance.

**What would close it.** Nothing — this is by design. Skipping gracefully is the right behavior when the infrastructure isn't available; forcing tests to pass without a real DB would undermine the whole point of the real-DB test.

### Stale handler tests from pre-revamp

**Symptom.** `TestResourceDef`, `TestListResourceTypes`, `TestValidateResourcesWithConnectionResources`, `TestRequestConfig` in `internal/mcp/catalog/catalog_test.go` expect the old single-resource GitHub catalog (one resource called `"repo"`). They fail against the current multi-resource catalog.

**Why.** The tests weren't updated when the catalog was restructured. They're pre-existing, not from the dispatcher work.

**Workaround.** Ignore them when running dispatcher tests (`go test ./internal/trigger/dispatch/`). They'll need updating at some point but don't block trigger work.

**What would close it.** Update the test expectations to match the current catalog shape (issue, pull_request, release, workflow, etc.). Half a day of work, not urgent.

### No metrics or observability integration

**Symptom.** The dispatcher logs every decision but doesn't emit metrics. There's no counter for "runs dispatched per minute," no histogram for "dispatch latency," no gauge for "skipped runs by reason."

**Why.** No metrics were requested yet, and the existing codebase doesn't have a standardized metrics integration pattern the dispatcher can copy.

**Workaround.** Log-based dashboards work fine at current volume. Search logs by `delivery_id` for tracing; aggregate by `skip_reason` for skip-rate alerts.

**What would close it.** Add Prometheus counters and histograms to `Dispatcher.Run` and the asynq handler. Straightforward but not currently needed.

### No rate limiting on the dispatch path

**Symptom.** A misconfigured webhook source could flood the trigger dispatch queue.

**Why.** No rate limiting is enforced at the webhook handler, asynq task, or dispatcher layers.

**Workaround.** Asynq's queue-level concurrency limits act as a soft cap. The `QueueCritical` config in `cmd/server/worker.go` sets worker concurrency; dispatches beyond that queue up.

**What would close it.** Per-connection rate limits at the webhook handler. Not currently needed.

## Summary

Most of these limitations are either intentional tradeoffs (see [08-design-decisions-deferred.md](08-design-decisions-deferred.md) for the rationale) or one-line catalog fixes that just haven't been done for providers other than GitHub. Nothing on this list is a runtime correctness bug or a security issue in the current code.

The biggest real gap is **cross-provider continuation**, which is handled by the "two agents with shared config" pattern until enough customer signal accumulates to justify building the link table. The next biggest is **Intercom's missing refs**, which is a catalog-only edit that can happen any time (Slack already has a full catalog; Intercom, Jira, Linear still don't).

## Where to go from here

- The design decisions behind these tradeoffs: [08-design-decisions-deferred.md](08-design-decisions-deferred.md)
- The multi-provider workaround: [06-multi-provider-patterns.md](06-multi-provider-patterns.md)
- The catalog audit that surfaced several of these limitations: [07-catalog-validation-report.md](07-catalog-validation-report.md)
