# 04 — Validation and Safety

The trigger system enforces invariants at three layers: save-time (handler validation before the DB accepts a config), dispatch-time (safety-net checks when a webhook actually arrives), and catalog-time (the generator that produces the data the dispatcher reads from). Each layer catches a different class of bug. This document describes what each layer checks and why.

## Save-time: the handler

`internal/handler/agent_triggers.go` is the HTTP layer for creating and updating agent triggers. It's also the primary line of defense against bad configuration. Every CRUD request goes through `validateTriggerRequest` (for creates) or scoped per-field validators (for updates).

### What `validateTriggerRequest` checks

Required fields:

- `connection_id` is present and is a valid UUID
- `trigger_keys` is non-empty
- The connection exists, belongs to the org, and has `revoked_at IS NULL`
- The connection is in the agent's `integrations` allow-list (you can't wire a trigger on a connection the agent has no access to)

Provider resolution:

```go
provider, err := resolveProviderFromConnection(db, connectionID, orgID)
```

This loads the `Connection.Integration.Provider` field. Every subsequent check is scoped to this provider — trigger keys and context actions must exist in the catalog for the same provider, not any other.

Catalog checks:

- `catalog.ValidateTriggers(provider, triggerKeys)` — every trigger key must be a real trigger in the catalog for this provider (with variant fallback)
- `validateConditions(req.Conditions)` — conditions are well-formed: valid operator, non-nil value for operators that need one, valid match mode (`all` or `any`)
- `validateContextActions(catalog, req.ContextActions, provider)` — every context action references an existing catalog action, is marked as `read` access, and has a unique `as` name within the list
- `validateTerminateRules(catalog, req.TerminateOn, req.TriggerKeys, provider)` — terminate rules are valid (described below)

Any failure returns a 400 with a human-readable error message naming the offending field. Nothing gets persisted.

### What `validateContextActions` specifically enforces

```go
for idx, contextAction := range contextActions {
    if contextAction.As == "" { return "..." }
    if seenIDs[contextAction.As] { return "...duplicate..." }
    seenIDs[contextAction.As] = true

    if contextAction.Action == "" { return "..." }
    actionDef, ok := catalog.GetAction(provider, contextAction.Action)
    if !ok { return "...does not exist for provider..." }

    if actionDef.Access != catalog.AccessRead {
        return "...is a write action; only read actions are allowed for context gathering"
    }
}
```

The write-action guardrail is load-bearing. Context gathering runs before the agent even sees the event — letting it perform writes would mean one misconfigured trigger could accidentally mutate production state. The generator's access inference is what decides read vs write (see [01-catalog-architecture.md](01-catalog-architecture.md)), and this validation refuses to store any config that tries to sneak a write into the context step.

Agents can still *use* write actions, but only via tools during the run proper, where the LLM is making deliberate decisions.

### What `validateTerminateRules` enforces

```go
for idx, rule := range rules {
    if len(rule.TriggerKeys) == 0 { return "...is required" }

    for _, key := range rule.TriggerKeys {
        if key == "" { return "...empty string" }
        if parentKeys[key] {
            return "ambiguous — an event can be either a normal trigger or a terminator, not both"
        }
    }

    if err := catalog.ValidateTriggers(provider, rule.TriggerKeys); err != nil {
        return "..."
    }

    if rule.Conditions != nil {
        if errMsg := validateConditions(rule.Conditions); errMsg != "" { return "..." }
    }

    if errMsg := validateContextActions(catalog, rule.ContextActions, provider); errMsg != "" {
        return "..."
    }
}
```

Four specific checks:

1. **Non-empty `trigger_keys`** — a terminate rule with no events is a config error, not a no-op.
2. **No empty strings** — catches UI bugs that might add stray empty elements to the list.
3. **No ambiguity with parent `trigger_keys`** — the same event can't be both a normal trigger and a terminator for the same agent trigger. This would be ambiguous at runtime ("should I run the agent or close the conversation?"), and the right answer is to reject the config outright.
4. **Full recursive validation** — the rule's own conditions and context actions go through the same validators as the parent. Write-action rejection, operator checks, the whole set. Terminate rules are structurally mini-triggers, so they get the same treatment.

### Denormalized `TerminateEventKeys`

After validation passes, the handler builds two JSON/array representations:

```go
var terminateOnJSON model.RawJSON
if len(req.TerminateOn) > 0 {
    terminateBytes, _ := json.Marshal(req.TerminateOn)
    terminateOnJSON = model.RawJSON(terminateBytes)
}
terminateEventKeys := pq.StringArray(model.CollectTerminateEventKeys(req.TerminateOn))
```

The JSONB column is the source of truth. The `text[]` column is a denormalization derived from it — a flat list of every `trigger_keys` value across every rule, used by the dispatcher's `&&` array-overlap query. Both columns are written in the same transaction, so they can never drift.

`CollectTerminateEventKeys` is a small helper in the model package that deduplicates while preserving first-seen order:

```go
func CollectTerminateEventKeys(rules []TerminateRule) []string {
    seen := make(map[string]bool)
    var out []string
    for _, rule := range rules {
        for _, key := range rule.TriggerKeys {
            if key == "" || seen[key] { continue }
            seen[key] = true
            out = append(out, key)
        }
    }
    return out
}
```

### The `Update` path

`PUT /v1/agents/{agentID}/triggers/{id}` goes through a scoped subset of the same validators. Each field that appears in the request body is validated individually; fields not present are left untouched. The `terminate_on` update path also rebuilds `terminate_event_keys` from scratch, so any edit that changes the rules keeps the denormalization in sync.

Ambiguous-key validation on update uses the CURRENT `trigger_keys` of the trigger (not whatever's in the request, which may not include `trigger_keys` at all), so you can change terminate rules without re-specifying the normal trigger keys.

### The agent-create-with-trigger path

`internal/handler/agents.go` supports creating an agent and its first trigger in a single transactional request (`POST /v1/agents` with an embedded `trigger:` field). That path uses the exact same `validateTriggerRequest` function — no duplicate validation logic, no risk of the two paths drifting.

## Dispatch-time: the safety net

Save-time validation is the primary line of defense. Dispatch-time checks exist to catch drift — situations where a malformed row made it into the DB despite validation (direct SQL edits during ops work, data-migration bugs, or schema changes that bypass the handler).

### Ambiguous key detection at dispatch

```go
for _, match := range matches {
    isNormal := containsString(match.Trigger.TriggerKeys, triggerKey)
    terminateRule, hasTerminate := findMatchingTerminateRule(match.Trigger, triggerKey, input.Payload, logger)

    if isNormal && hasTerminate {
        logger.Error("dispatch: ambiguous event key, listed in both trigger_keys and terminate_on",
            "agent_trigger_id", match.Trigger.ID,
            "trigger_key", triggerKey,
        )
        run := baseRun(...)
        run.SkipReason = "ambiguous: event key is in both trigger_keys and terminate_on"
        runs = append(runs, run)
        continue
    }
    // ... normal dispatch
}
```

The run is still emitted (with `SkipReason` set) so it shows up in logs and debugging tools, but the executor won't enqueue it. This is visible instead of silent: a misconfigured trigger produces a run you can trace, not nothing at all.

### Invalid JSON in `Conditions` / `ContextActions` / `TerminateOn`

Each of these columns is JSONB, and the dispatcher parses them on read:

```go
conditions, err := parseConditions(match.Trigger.Conditions)
if err != nil {
    run.SkipReason = "invalid conditions JSON: " + err.Error()
    return run
}
```

Same for `parseContextActions` and the in-place `TerminateOn` unmarshal. If any of them is malformed (which shouldn't happen since the handler marshals them, but could after a migration), the dispatcher skips the run with a specific error message and logs it.

### Catalog misses at dispatch

`buildContextRequests` returns a `[]string` of error messages for per-action problems:

- Action not in catalog (e.g., the catalog was regenerated and an action key changed)
- Action has no execution config (e.g., a broken catalog entry)
- `ref` names a resource type that doesn't exist for the provider
- Step reference `{{$prior_step.x}}` points at an `as` name that doesn't appear earlier in the list

These are soft errors. The dispatcher logs each one and sets `SkipReason` to the first error, but still emits the run. The executor will see an empty or incomplete `ContextRequests` slice and can decide whether to proceed (e.g., with `optional: true` failures) or skip.

### Resource key required for terminate

```go
if resourceKey == "" {
    run.SkipReason = "terminate requires a resource key, but none resolved for this resource type"
    logger.Warn("dispatch: terminate skipped, no resource key")
    return run
}
```

A terminate rule attached to a resource with no `ResourceKeyTemplate` is a config error — the executor would have no way to find an existing conversation to close. The dispatcher catches it here and emits a clearly-labeled skip. This is mostly a guard against future bugs: right now the only resources with terminate-worthy semantics all have templates.

### Parent condition inheritance at terminate-time

```go
if !rule.IgnoreParentConditions {
    parentConditions, err := parseConditions(match.Trigger.Conditions)
    if err != nil {
        run.SkipReason = "invalid parent conditions JSON: " + err.Error()
        return run
    }
    if reason, passed := evaluateConditions(parentConditions, input.Payload); !passed {
        run.SkipReason = "parent " + reason
    }
}
```

The `parent ` prefix on the skip reason is deliberate. When you're debugging "why didn't my terminate fire?" and you see `parent condition 0 (pull_request.draft not_equals) failed` in the logs, you know immediately that the issue is in the PARENT's filters, not the terminate rule's own conditions.

## Catalog-time: the generator

The third layer is before the dispatcher ever runs: `cmd/fetchactions-oas3` produces the catalog JSON, and bugs here create bad data that no amount of runtime validation can fix. This is why the generator is treated as a first-class concern.

### The `inferAccess` fix

Before this PR, access inference used substring matching:

```go
// OLD — buggy
for _, prefix := range readHintPrefixes {
    if strings.Contains(keyLower, prefix) {
        return "read"
    }
}
```

The substring check matched `"view"` inside `"review"`, flagging every `*_review_*` action as read regardless of HTTP method. Six write actions were misclassified: `pulls_request_reviewers`, `pulls_create_review`, `pulls_submit_review`, `pulls_create_review_comment`, `pulls_create_reply_for_review_comment`, `actions_review_pending_deployments_for_run`.

The fix is token-based matching: split the action key on `_`, check the first two tokens against an exact-string set of read verbs. In GitHub operationIds, the verb is typically `tokens[1]` (e.g., `issues_list_comments`); for verb-first keys like `search_repos`, it's `tokens[0]`. Checking both positions handles both patterns without false positives.

Why the save-time validation didn't catch this: the validator only rejects context actions marked as write. A write endpoint mislabeled as read would pass validation — the bug was in the label, not the check. Fixing the generator is the only place this could be fixed.

### The `oneOf` body fix

GitHub wraps some endpoint bodies in `oneOf`:

```yaml
requestBody:
  content:
    application/json:
      schema:
        oneOf:
          - type: object
            properties:
              labels: { type: array, items: { type: string } }
          - type: array
            items: { type: string }
```

The old parser only walked top-level `properties`, missing the `labels` array inside the first `oneOf` alternative. Four actions were unusable: `issues_add_labels`, `issues_set_labels`, `repos_add_status_check_contexts`, `repos_set_status_check_contexts`, `repos_remove_status_check_contexts`.

The fix walks the top-level schema PLUS every object alternative in `oneOf`/`anyOf`, merging their `Properties` and `Required` lists. See `cmd/fetchactions-oas3/parse.go`.

### Missing paths in resource filtering

GitHub's resource-based filtering lists path prefixes per resource. The old config missed several paths, causing actions to silently drop during generation. Added in this PR:

- `/repos/{owner}/{repo}/compare` (for `repos_compare_commits`)
- `/repos/{owner}/{repo}/merges` (for `repos_merge`)
- `/repos/{owner}/{repo}/merge-upstream` (for `repos_merge_upstream`)
- `/repos/{owner}/{repo}/assignees` (for `issues_list_assignees` and `issues_check_user_can_be_assigned`)

Five new actions after regeneration. These had been invisible to the catalog — and therefore to the handler's validation — since the catalog revamp. No save-time check would have flagged them as missing because the handler can only validate against what's in the catalog, not what's documented upstream.

### Broken `ref_bindings` pruning

The validation audit found four ref bindings pointing at refs that don't exist on any GitHub trigger:

- `milestone.milestone_number → $refs.milestone_number`
- `organization.org → $refs.org`
- `team.org → $refs.org`
- `team.team_slug → $refs.team_slug`

At dispatch time, these would have produced `$refs.milestone_number` (unsubstituted) in resolved paths because the ref wasn't in the map. The fix was to remove the broken bindings entirely. If a trigger eventually exposes those refs (e.g., a future `milestone.created` trigger), they can be re-added.

## The three layers in practice

When a user creates a trigger, the flow is:

1. **Frontend** posts a request to `POST /v1/agents/{id}/triggers`
2. **Handler** runs `validateTriggerRequest`, rejects with 400 on any problem, otherwise inserts the row with `TerminateEventKeys` denorm computed
3. **Nothing happens** until a webhook arrives
4. **Nango handler** receives the webhook, identifies the connection, enqueues a `TypeTriggerDispatch` task
5. **Dispatcher** loads the row, parses JSONB columns, walks refs and conditions, builds `PreparedRun`
6. **Dispatch-time checks** catch any drift the handler validation couldn't see
7. **Executor** (future PR) filters skipped runs and enqueues the rest as child tasks

Each layer is optimized for the class of error it's closest to: the handler catches user-submitted bugs, the dispatcher catches drift and inherited condition mismatches, the generator catches upstream spec changes. Together they mean the runtime has almost nothing to do except walk the data — and when something does go wrong, the skip reason tells you exactly which layer failed.

## What's deliberately NOT validated

A few things could be validated but aren't, because the cost outweighs the benefit:

- **Deferred step references** (`{{$step.x}}` where `step` isn't an earlier `as`) — detected at dispatch time with a logged warning, not rejected at save time. The reason is that the user might reorder context actions in a future update and the old validation would reject intermediate states.
- **Resource key template coverage** — we don't check that every trigger whose resource has a template actually exposes the refs the template needs. If a trigger's refs are incomplete, the dispatcher's "fail closed" logic produces an empty key and the event creates a new conversation. Adding a pre-check would require cross-referencing triggers and resources at generator time, which is doable but noisy.
- **Trigger ref coverage against payload shapes** — the generator doesn't verify that trigger refs actually resolve against a canonical sample payload. The 31-agent audit did this manually, one-time; a permanent check would require shipping sample payloads alongside the catalog.
- **Terminate rule ordering for unreachable rules** — if rule 1 has no conditions and rule 2 has the same `trigger_keys`, rule 2 is unreachable. Not checked; low enough severity that catching it in review is fine.

## Where to go from here

- Exactly what's tested (and what isn't): [05-testing.md](05-testing.md)
- The catalog audit that found the generator bugs: [07-catalog-validation-report.md](07-catalog-validation-report.md)
- The deferred design decisions that drove some of the "not validated" choices: [08-design-decisions-deferred.md](08-design-decisions-deferred.md)
