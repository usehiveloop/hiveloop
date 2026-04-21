# 06 — Observability and Debugging

You'll spend more time debugging a trigger config than writing it. This doc is about how to figure out what happened when an agent doesn't behave the way you expected — which is most of the time during initial development and occasionally in production. The goal is to cut debugging time from "hours of confusion" to "minutes of log reading."

The dispatcher logs every decision it makes. Learning to read those logs is the single highest-leverage debugging skill in this system.

## The structured logging contract

Every dispatch decision produces a log line with consistent fields. The most important are:

- `delivery_id` — the provider-assigned unique ID for the webhook (e.g., GitHub's `X-GitHub-Delivery` header). Searching by this gives you the complete trace for one webhook from arrival to final disposition.
- `provider` — which provider the event came from
- `trigger_key` — the composite event key (e.g., `issues.opened`, `push`)
- `org_id` — the tenant
- `connection_id` — which connection the webhook arrived on
- `agent_trigger_id` — per-match field, identifies the specific `AgentTrigger` row
- `agent_id` — which agent owns the matched trigger
- `intent` — `normal` or `terminate`
- `resource_key` — the resolved resource key (empty if no continuation)
- `skip_reason` — why a run was filtered out (present only on skipped runs)
- `context_requests` — count of context actions that would fire
- `sandbox_strategy` — `reuse_pool` or `create_dedicated`

Every trigger evaluation generates at least 3–4 log lines: arrival, per-match decisions, final summary. You can reconstruct the full flow for any webhook from these.

## The "does my trigger fire?" workflow

The most common question: "I set up a trigger but the agent isn't firing. Why?"

The debugging sequence:

### Step 1: Did the webhook arrive?

Search logs for `delivery_id=<the-id>`. If you find NO entries, the webhook didn't reach the dispatcher. Either:

- Nango didn't forward it (check Nango's own delivery logs)
- The signature verification failed (look for `nango webhook: invalid signature`)
- The webhook handler crashed before logging anything (check for process-level errors)

If you find the initial arrival log but no subsequent lines, the dispatcher hit an error before finishing. Look for ERROR-level lines around the same timestamp.

### Step 2: Did the event type get recognized?

Look for `dispatch: webhook received` followed by one of:

- `dispatch: trigger key not in catalog, ignoring` — the event type isn't in the catalog. Either it's a new event type we haven't added, or the trigger key is computed wrong (check the event type vs. event action extraction in `nango_webhooks_dispatch.go`).
- `dispatch: provider has no triggers in catalog, ignoring` — the provider doesn't have a triggers file. Add one, or this provider doesn't support webhook-driven agents.

If neither of those appears and the dispatch seems to proceed, continue to Step 3.

### Step 3: Did agent triggers match?

Look for `dispatch: agent triggers matched count=N`. The count tells you how many `AgentTrigger` rows the store query returned.

- **count=0** — no trigger is listening for this event on this connection. Check your config: is `connection_id` correct? Is the event key in `trigger_keys`? Is `enabled` true?
- **count>0** — triggers matched; check the next step.

If you expected a specific agent to match and it didn't, check the DB directly:

```sql
SELECT id, trigger_keys, enabled, connection_id
FROM agent_triggers
WHERE agent_id = '<your-agent-id>';
```

Common issues:

- `enabled = false` (GORM's zero-value bug can produce this accidentally)
- `connection_id` points at the wrong connection
- The event key isn't in `trigger_keys`
- The trigger was deleted and the UI is showing a cached version

### Step 4: Did conditions pass?

For each matched trigger, look for one of:

- `dispatch: normal run built` (with `skip_reason` empty) — the run fired
- `dispatch: skipped, conditions did not match` (with `reason` populated) — conditions failed
- `dispatch: prepared run built` (summary) — the build succeeded even if skipped

The `reason` field points directly at the failing condition: `condition 2 (pull_request.draft not_equals) failed`. This tells you exactly which rule rejected the event.

If the skip reason says `parent condition N ... failed`, the parent trigger's conditions rejected the terminate rule's inherited check. If it says `terminate rule condition N ... failed`, the rule's own condition failed.

### Step 5: Did the context build succeed?

If conditions passed, the next step is building context requests. Look for `dispatch: context request build error` lines. Common errors:

- `action "X" not in catalog` — typo in the action name, or the action was removed
- `ref "Y" is not a resource of provider` — you specified `ref: something` but `something` isn't a resource
- `references step "Z" that does not appear earlier` — template references a `$step.x` that doesn't exist

The final `dispatch: normal run built` line confirms the build succeeded. If it's not there, something above failed.

## Reading skip reasons

Skipped runs are your best friend. They tell you exactly why a trigger didn't fire, without you having to reconstruct it from code.

Skip reasons have consistent structure:

| Pattern | Meaning |
|---|---|
| `condition N (path op) failed` | The parent trigger's Nth condition failed. `op` tells you which operator. |
| `parent condition N (...) failed` | A terminate rule's inherited parent condition failed. |
| `terminate rule condition N (...) failed` | A terminate rule's own condition failed. |
| `no conditions matched (any mode); condition N failed` | `match: any` and all conditions failed (this is the LAST failing one). |
| `ambiguous: event key is in both trigger_keys and terminate_on` | Config error caught at dispatch. Fix the config. |
| `invalid conditions JSON: ...` | The `Conditions` column has malformed JSON. DB corruption or manual edit. |
| `context_actions build error: ...` | A context action couldn't be resolved. Usually a typo or removed catalog entry. |
| `terminate requires a resource key, but none resolved` | The resource's template is empty but `terminate_on` is configured. Fix the catalog. |

When you see a skip reason you don't recognize, search the dispatcher code for the literal string — every skip reason is hardcoded at one site, and reading the context tells you why.

## Testing a trigger config before production

Three layers of testing, in order of trust:

### Layer 1: Validate at save time

Every `POST /v1/agents/{id}/triggers` goes through `validateTriggerRequest`. This catches:

- Bad trigger keys
- Missing context actions
- Write actions in context
- Bad conditions
- Ambiguous terminate rules

If the save succeeds, you know the configuration is at least structurally valid. This is the first gate and the cheapest to run.

### Layer 2: Send a test webhook

Most providers let you replay webhook deliveries. GitHub has a "Redeliver" button in the webhook settings; Linear has a test-send option; Slack has event API testing. Fire a test webhook at your staging environment and watch the logs.

For providers without built-in redelivery, you can cURL against `/internal/webhooks/nango` with a captured payload. Not officially supported, but it works.

What to look for in the logs:

- The `delivery_id` shows up in the arrival log
- The trigger is matched (count ≥ 1)
- Conditions pass OR the skip reason is the one you expected
- Context actions build without errors
- The final summary line shows the expected run intent

If all of these are present, the dispatch logic is working. Whether the executor (and the actual Nango calls) work is a separate question tested at the next layer.

### Layer 3: Production smoke test

For high-stakes agents, deploy to production with a narrow scope first (e.g., a condition like `repository.name equals test-repo`), trigger a few real events, and verify the behavior. Then widen the scope by removing the condition.

This is the slowest and most cautious option, but it's the only way to test the full pipeline including Nango calls, Bridge conversation creation, and LLM behavior. Budget time for this on any production rollout.

## Common debugging scenarios

### "The agent is responding to its own comments"

You have a loop. The agent's comments are firing an `issue_comment.created` event that matches its own trigger. Fix: add self-exclusion (`sender.login not_equals <bot>`) to the conditions.

See [04-safety-and-loop-prevention.md](04-safety-and-loop-prevention.md).

### "The agent responds to every PR but I only wanted main-branch PRs"

Your conditions are too loose. Check:

- Do you have a `pull_request.base.ref equals main` condition?
- Does the trigger key `pull_request.opened` also include `synchronize` events that might bypass the condition? (No — conditions apply to every event.)
- Is the draft filter present? Draft PRs might be slipping through.

Expected fix: add or tighten the conditions.

### "The agent runs once and then never again on the same PR"

Your conversation is closing somewhere. Check:

- Is there a `terminate_on` rule that's firing on an unexpected event? Logs for `dispatch: terminate run built` will show it.
- Did the resource key change between events? If refs drifted, the executor would create a new conversation each time instead of continuing.

Usually the first one. Walk through the PR's event history: opened → synchronize → commented → closed. Which of those fired the terminator?

### "Context is missing in the agent's prompt"

The context action isn't resolving. Check:

- Dispatcher logs for `dispatch: context request build error`
- The agent's conversation log (the actual opening message sent by the executor) to see what was in `{{$variable}}` positions
- Whether the action is marked `optional: true` and returning empty silently

If it's silent, the optional fetch returned nothing. If it's missing entirely, the build step failed.

### "Conditions worked in staging but not production"

Check:

- The connection in production is different from staging — your `connection_id` in the trigger config should be the production connection's UUID
- Provider differences: if you tested on `github` but production uses `github-app`, the dispatcher does variant fallback but some refs or actions might behave slightly differently
- Payload differences: staging events might have different shapes than production (e.g., test webhooks lack some fields)

Reproduce by firing a real webhook in production with a narrow condition first.

### "Two agents both fire on the same event but one is a dup"

You have two `AgentTrigger` rows with overlapping configuration. The dispatcher fans out correctly, but the agents are doing the same job. Expected: create one agent with the combined config; delete the other trigger. Or scope each agent to a more specific condition so they don't overlap.

### "The terminate rule doesn't fire"

Check:

- Is the event key actually in the terminate rule's `trigger_keys`?
- Does the parent have conditions that are excluding the terminate event? (The `parent condition failed` skip reason is the giveaway.)
- Is the resource key resolving? Check for `dispatch: terminate skipped, no resource key`.

### "The run fires but the agent does the wrong thing"

This isn't a trigger issue — it's an agent issue. The dispatcher did its job (it produced the right `PreparedRun`); the agent's reasoning or execution is wrong. Debug via:

- The agent's conversation log in Bridge (what was the opening prompt? what did the LLM reply?)
- The system prompt (does it give the agent the scope and constraints it needs?)
- The instructions block (is it specific enough about what to do and when to stop?)

This is out of scope for trigger debugging. See the agent's own observability tools.

## Keeping logs useful at scale

At production volumes, raw dispatch logs fill up fast. Two practices to keep them usable:

### Structured fields, not free-form messages

Every dispatcher log line should have the core fields (`delivery_id`, `trigger_key`, etc.) so you can filter and aggregate. The dispatcher code already does this — don't add `slog.Info(fmt.Sprintf(...))` calls that produce unstructured messages.

### Log-to-metric for high-volume signals

The count of skipped runs, broken down by reason, is the kind of thing you want as a dashboard rather than a log grep. Ditto for dispatcher latency, per-connection rate, and top-10 busiest triggers. Build these as Prometheus (or your metrics system's) counters on top of the log stream.

The trigger system doesn't emit metrics natively today (see [09-practical-limitations.md](09-practical-limitations.md)), but the logs contain everything needed to derive them.

## The "three log searches" technique

When something weird is happening in production and you don't know where to start, run these three searches:

1. **`delivery_id=<specific webhook>`** — the full trace for one event. Use this when a user reports "event X didn't work."
2. **`agent_id=<suspect agent> AND skip_reason NOT EMPTY`** — all skipped runs for one agent. Use this when "the agent isn't firing as often as expected."
3. **`trigger_key=<problem event> AND skip_reason EMPTY`** — all successful runs for a specific event type. Use this when "too many runs are firing" or "I want to see what events are getting through."

These three cover about 90% of production debugging needs.

## When the logs aren't enough

Sometimes you need to step into the code. The pattern:

1. Identify the specific webhook that misbehaved (get the `delivery_id` from user reports or from logs)
2. Retrieve the raw webhook body from Nango's delivery logs (or reconstruct it from GitHub/Slack/etc.)
3. Write a test case in `internal/trigger/dispatch/dispatcher_test.go` using the captured payload as a fixture
4. Run the test locally with verbose logging to see what the dispatcher actually does with that specific payload
5. Fix the logic, verify the test passes, commit

This turns "mysterious production bug" into "reproducible test case" in about 20 minutes per incident. It's worth the effort because fixed-with-test bugs rarely come back; fixed-without-test bugs often do.

## The debugging mindset

Two mental models that save time:

### "Explain the dispatch one line at a time"

When a user says "my trigger didn't work," resist the urge to guess. Instead, pull the log lines for that event and narrate what happened, line by line, out loud (or to a rubber duck).

"The webhook arrived at 14:32. It was a `pull_request.opened` event on connection X. The dispatcher found 2 matching triggers. Trigger A was skipped because condition 0 (sender.login not_equals) failed — the sender was hiveloop-bot[bot] which matches the exclusion filter. Trigger B fired normally. So the user's trigger is correctly firing only for non-bot users. The issue is that the user expected Trigger A to fire, but Trigger A is configured to exclude bots..."

Narrating the logs forces you to read them carefully rather than skim, and 80% of the time the bug becomes obvious in the process.

### "Assume the dispatcher is right; find the config bug"

The dispatcher is heavily tested (see [../05-testing.md](../05-testing.md)) and rarely wrong. When something is misbehaving, the first hypothesis should be "the trigger config is wrong" or "the expectation is wrong," not "the dispatcher is wrong." Check the config and the user's mental model before digging into the code.

This is the opposite of normal software debugging where you assume everything is a bug. For this system, treat the dispatcher as a trusted black box and focus on the inputs.

## Where to go from here

- The safety patterns you're often debugging violations of: [04-safety-and-loop-prevention.md](04-safety-and-loop-prevention.md)
- Full trigger configuration examples for reference: [07-worked-examples.md](07-worked-examples.md)
- The test suite that validates dispatcher behavior: [../05-testing.md](../05-testing.md)
