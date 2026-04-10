# 03 — Lifecycle and Continuation

The dispatcher decides whether an incoming event starts a new agent conversation or continues an existing one. It also signals when an event should end a conversation. This document describes both: the resource-key mechanism that powers continuation, and the `terminate_on` mechanism that powers graceful close. The contract it produces is what the executor (a future PR) will read to actually create or close conversations.

## The problem

Consider a pull request lifecycle:

1. PR opened → agent reviews
2. Author pushes new commits → agent re-reviews
3. Reviewer leaves feedback → agent responds
4. Reviewer approves → agent acknowledges
5. PR merged → agent posts a summary and stops tracking the PR

Without a continuation concept, every event would be a brand-new conversation. The agent's context window would reset on every webhook, losing the thread of what the PR was about, what changes it's been through, and what reviewers have already said. Agents would be stuck in perpetual first-message mode.

The design insight: **every event has a subject resource**, and events on the same subject resource should share a conversation with the agent. "This comment is on PR #42" and "this review is on PR #42" both resolve to the same resource; the agent's conversation for PR #42 should pick up both.

## Resource keys

A resource key is a stable string identifier for one specific instance of a resource, within a specific connection. For GitHub issues, the key is `octocat/Hello-World#issue-1347`. For GitHub pull requests, it's `octocat/Hello-World#pr-42`. For Intercom conversations, it would be `conv-abc123`.

The properties we need:

- **Stable across events on the same subject.** A new comment on issue #1347 and a label added to issue #1347 both produce `octocat/Hello-World#issue-1347`.
- **Unique within a connection.** Issue #1347 in repo A and issue #1347 in repo B produce different keys.
- **Computable from the webhook payload.** No out-of-band lookups; the key must be fully derivable from the ref map the dispatcher already extracted.
- **Provider-agnostic at the code level.** No Go code anywhere knows that GitHub issue keys look different from Intercom conversation keys. All of that is catalog data.

## How it's computed

Each resource in the catalog has an optional `ResourceKeyTemplate` field. The template is a string with `$refs.x` placeholders that get substituted at dispatch time from the ref map.

```go
type ResourceDef struct {
    // ... other fields ...
    ResourceKeyTemplate string
}
```

The templates currently in the GitHub catalog:

| Resource | Template |
|---|---|
| `issue` | `$refs.owner/$refs.repo#issue-$refs.issue_number` |
| `pull_request` | `$refs.owner/$refs.repo#pr-$refs.pull_number` |
| `release` | `$refs.owner/$refs.repo#release-$refs.release_id` |
| `workflow` | `$refs.owner/$refs.repo#run-$refs.run_id` |

Resources with no template — `repository`, `branch`, `label`, `milestone`, `organization`, `team` — always produce an empty key, which means "always a new conversation." This matches intuition: a push event doesn't continue a branch-level conversation, and there's no coherent "label-level" conversation to continue.

At dispatch time:

```go
run.ResourceKey = resolveResourceKey(d.Catalog, input.Provider, triggerDef.ResourceType, refs)

func resolveResourceKey(cat *catalog.Catalog, provider, resourceType string, refs map[string]string) string {
    if resourceType == "" {
        return ""
    }
    resourceDef, ok := cat.GetResourceDef(provider, resourceType)
    if !ok {
        // Try variant fallback (github-app → github)
        if baseResourceDef, found := tryVariantResource(cat, provider, resourceType); found {
            resourceDef = baseResourceDef
        } else {
            return ""
        }
    }
    if resourceDef.ResourceKeyTemplate == "" {
        return ""
    }
    resolved := substituteRefs(resourceDef.ResourceKeyTemplate, refs)
    // Fail closed: a partial resolution would silently merge unrelated resources.
    if strings.Contains(resolved, "$refs.") {
        return ""
    }
    return resolved
}
```

The "fail closed" step at the end is load-bearing. If the template references a ref that wasn't extracted from this particular trigger's payload (e.g., a typo in the template, or a trigger whose refs don't include the expected field), substitution leaves `$refs.x` in the output. Returning that partial string as the key would silently merge completely unrelated resources into one conversation — a serious correctness bug. The safe move is to return empty and let the event create a new conversation.

## Cross-trigger continuation

All triggers in the same resource family share the same identity refs. All `issues.*` triggers in GitHub expose `owner`, `repo`, `issue_number`. All `pull_request.*` triggers expose `owner`, `repo`, `pull_number`. And — importantly — `issue_comment.created` also exposes `issue_number` (GitHub models PR comments as issue comments, so the refs stay unified).

This means three different events — `issues.opened`, `issues.labeled`, `issue_comment.created` — on the same issue all produce the same resource key. The executor's lookup finds one conversation and appends to it. This is verified by `TestDispatch_CrossTriggerContinuation` at `internal/trigger/dispatch/dispatcher_test.go`.

## Multi-agent isolation

The resource key is scoped to `(agent_id, connection_id, resource_key)` when the executor does its lookup. Two different agents looking at the same PR each have their own conversation for it. They never share state.

This matters for the common case of multiple agents on one resource: a code-review agent, a documentation-drift agent, and a security-scan agent can all listen on the same PR events without stepping on each other. Each has its own thread with the user via its own conversation row.

## What the executor does with it

(Executor is a future PR; this is the contract the dispatcher hands off.)

```go
func (e *Executor) ensureConversation(ctx context.Context, run dispatch.PreparedRun) (uuid.UUID, bool, error) {
    if run.ResourceKey == "" {
        // Always new — no continuation semantics for this resource type.
        return e.createConversation(ctx, run)
    }

    var existing model.AgentConversation
    err := e.db.WithContext(ctx).
        Where("agent_id = ? AND connection_id = ? AND resource_key = ? AND status != ?",
            run.AgentID, run.ConnectionID, run.ResourceKey, "closed").
        Order("created_at DESC").
        First(&existing).Error

    if err == nil {
        return existing.ID, false, nil  // continuing
    }
    if !errors.Is(err, gorm.ErrRecordNotFound) {
        return uuid.Nil, false, err
    }
    // Resource key set, but no existing open conversation — create a new one.
    return e.createConversation(ctx, run)
}
```

Returned `isNew` bool tells the executor whether to call `Bridge.CreateConversation()` + `SendMessage()` (new) or just `SendMessage()` (continuing).

## Required schema changes for the executor PR

`agent_conversations` needs two new columns:

```sql
ALTER TABLE agent_conversations
  ADD COLUMN connection_id UUID,
  ADD COLUMN resource_key TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_agent_conversations_resource
  ON agent_conversations (agent_id, connection_id, resource_key)
  WHERE resource_key != '';
```

The partial index excludes user-initiated conversations (where `resource_key = ''`), so the lookup stays fast even as the table grows. User conversations never participate in the continuation lookup at all.

## Terminate rules — the lifecycle hook

Continuation handles the "keep going" case. `terminate_on` handles "stop when this event fires."

### The shape

```go
type TerminateRule struct {
    TriggerKeys            []string        `json:"trigger_keys"`
    Conditions             *TriggerMatch   `json:"conditions,omitempty"`
    ContextActions         []ContextAction `json:"context_actions,omitempty"`
    Instructions           string          `json:"instructions,omitempty"`
    Silent                 bool            `json:"silent,omitempty"`
    IgnoreParentConditions bool            `json:"ignore_parent_conditions,omitempty"`
}
```

Each rule is structurally a mini-trigger: it lists one or more event keys that should close the conversation, plus optional conditions, context actions, and instructions for a graceful final run.

`Silent: true` means "close the conversation without running the agent one more time." The default is false — most agents have something useful to say on close (merged PR → post summary; closed issue → acknowledge). Silent close is opt-in for cases where there's nothing worth saying (PR closed without merge, duplicate issue).

### Where it's stored

Two columns on `agent_triggers`:

```go
TerminateOn        RawJSON        // JSONB: []TerminateRule
TerminateEventKeys pq.StringArray // text[]: flat list of every trigger_keys value for fast lookup
```

`TerminateEventKeys` is a denormalization maintained by the handler on every Create/Update via `model.CollectTerminateEventKeys(rules)`. It exists so the dispatcher can query for triggers matching an incoming event key with a single `&&` array-overlap operator, the same pattern used for the normal `TriggerKeys` column.

The store query matches on either column:

```sql
WHERE org_id = ? AND connection_id = ? AND enabled = TRUE
  AND (trigger_keys && ? OR terminate_event_keys && ?)
```

### First-pass-wins rule ordering

When multiple rules list the same event key with different conditions, the dispatcher evaluates them in declaration order and picks the first one whose conditions pass. This supports the "merged vs. not merged" pattern:

```yaml
terminate_on:
  - trigger_keys: [pull_request.closed]
    conditions:
      match: all
      rules:
        - path: pull_request.merged
          operator: equals
          value: true
    instructions: "PR merged — post a summary"

  - trigger_keys: [pull_request.closed]
    conditions:
      match: all
      rules:
        - path: pull_request.merged
          operator: equals
          value: false
    silent: true
```

A closed-and-merged PR matches the first rule, runs the final "summarize" message, then closes. A closed-without-merge PR falls through to the second rule and closes silently.

### Parent condition inheritance

By default, terminate rules inherit the parent `AgentTrigger.Conditions`. If the parent says "skip drafts," the terminate rule also skips drafts — you never terminate a conversation you were never tracking. Opt out with `IgnoreParentConditions: true` when the terminate rule has intentionally different scope.

Implementation in `buildTerminateRun`:

```go
if !rule.IgnoreParentConditions {
    parentConditions, err := parseConditions(match.Trigger.Conditions)
    if err != nil { /* skip with reason */ }
    if reason, passed := evaluateConditions(parentConditions, input.Payload); !passed {
        run.SkipReason = "parent " + reason
        logger.Info("terminate skipped, parent conditions did not match")
    }
}
// Then apply the rule's own conditions on top.
```

Skip reasons are prefixed with `parent ` or `terminate rule ` so logs make it immediately obvious which layer rejected the event.

### Ambiguous key rejection

An event key can't appear in both `trigger_keys` and `terminate_on[*].trigger_keys` — the same event can't simultaneously be a "run this agent" trigger and a "close this agent's conversation" trigger. That would be ambiguous, and the system rejects it at two layers:

**Save-time** (handler): `validateTerminateRules` walks the rules and errors with:

```
terminate_on[N]: trigger key "X" is also in the parent trigger_keys (ambiguous — an event can be either a normal trigger or a terminator, not both)
```

**Dispatch-time** (dispatcher): if a malformed config somehow gets persisted (direct DB edit, drift, stale schema), the dispatcher's per-match logic detects it and produces a skipped run with reason `ambiguous: event key is in both trigger_keys and terminate_on`. Defense-in-depth.

See [04-validation-and-safety.md](04-validation-and-safety.md) for the full validation layering.

### What goes into the terminate `PreparedRun`

```go
PreparedRun{
    RunIntent:       RunIntentTerminate,   // tells the executor which flow to route through
    SilentClose:     rule.Silent,          // whether to skip the final run entirely
    ResourceKey:     "<stable identity>",  // the conversation to close — must be non-empty
    ContextRequests: []ContextRequest{...}, // built from the rule's context_actions (empty if Silent)
    Instructions:    "<substituted>",       // from the rule's instructions (empty if Silent)
    // ... identity fields same as normal runs
}
```

The executor routes on `RunIntent`:

```go
if run.RunIntent == RunIntentTerminate && run.SilentClose {
    return e.closeConversationSilent(ctx, run)
}

conversationID, isNew, err := e.ensureConversation(ctx, run)
if err != nil { return err }

if err := e.fireContextActions(ctx, run); err != nil { return err }
if err := e.sendInitialMessage(ctx, run, conversationID, isNew); err != nil { return err }

if run.RunIntent == RunIntentTerminate {
    return e.closeConversation(ctx, conversationID)
}
return nil
```

### Silent close semantics

`SilentClose` is only meaningful when `RunIntent == RunIntentTerminate`. When set, the executor's entire job is:

1. Find the existing conversation by `(agent_id, connection_id, resource_key)`
2. Mark it closed
3. Return

No context actions fire, no message is sent, no LLM call is made, no Nango proxy happens. The conversation just goes into the closed state, and its sandbox (if still active) will be cleaned up by the existing idle-timeout sweep.

If no existing conversation is found for the resource key, the silent close is a no-op — log it and move on. This handles webhook redelivery gracefully: the first close succeeds, the second finds nothing to close.

### Edge case: terminate arrives before any normal event

Scenario: a `pull_request.closed` webhook lands for a PR the agent never tracked (the initial `pull_request.opened` was missed, dropped, or replayed out of order). The terminate rule builds a `PreparedRun`, the executor's `ensureConversation` finds no existing conversation by the resource key, and:

- For **graceful close**: the executor creates a new conversation, sends the final message, closes it. The agent gets one short-lived run. Usually fine, sometimes annoying — the PR is already closed by the time the agent sees it.
- For **silent close**: the executor finds nothing and exits. No side effects.

If you want graceful close to also no-op on "no existing conversation," add a `require_existing: true` flag to the rule. Not built yet; propose when it becomes painful.

### What the agent sees across a lifecycle

Putting it together — a PR-review agent's conversation arc:

1. `pull_request.opened` → `resource_key = owner/repo#pr-42` → new conversation created, context: PR diff + files
2. `pull_request.synchronize` (new commits pushed) → same key → continue same conversation, context: updated diff
3. `pull_request_review.submitted` (reviewer comments) → same key → continue, context: review body
4. `pull_request.synchronize` (fix pushed) → same key → continue, context: updated diff
5. `pull_request.closed` with `merged: true` → same key → terminate-graceful run → final context fetch, agent posts summary, conversation closed
6. `pull_request_review_comment.created` (comment on the closed PR) → same key, but conversation is closed → executor creates a new conversation (status filter excludes closed ones)

This last step is a feature, not a bug. "Comment on a closed PR" is semantically a new interaction — the previous work wrapped up, and this is someone coming back to a historical thread. New conversation, fresh start.

## Sandbox strategies (separate from continuation)

A small but orthogonal lifecycle concern. Each agent is either "shared" (reuses a pool sandbox across conversations) or "dedicated" (gets a fresh sandbox per run). The dispatcher records the strategy in `PreparedRun.SandboxStrategy` based on `agent.SandboxType`:

```go
switch match.Agent.SandboxType {
case "dedicated":
    run.SandboxStrategy = SandboxStrategyCreateDedicated
default:
    run.SandboxStrategy = SandboxStrategyReusePool
    run.SandboxID = match.Agent.SandboxID
}
```

The executor uses this to decide whether to call `orchestrator.CreateDedicatedSandbox` or route through the existing pool. Continuation and sandbox strategy are independent: dedicated agents can still use resource keys to continue conversations across events. The conversation is the continuation unit, not the sandbox. When a continuation arrives, the executor ensures the conversation's current sandbox is awake (orchestrator handles this) and sends the message.

Sleeping sandboxes are free. A dedicated agent whose conversation has been idle for an hour lives in a sleeping sandbox; when the next event arrives, the orchestrator wakes it and the conversation continues. This is the behavior user specifically called out as the reason continuation works cheaply even for dedicated agents — closed conversations don't leak resources because their sandboxes sleep, and re-opening them is a wake operation, not a cold start.

## Provider-agnostic by construction

Nothing in this design knows GitHub from Intercom from Linear. The only provider-specific data is the four `resource_key_template` strings in `github.actions.json` — pure JSON configuration. Adding a new provider with continuation support is a one-line template per resource in the generator config, followed by a regeneration. Zero dispatcher code changes, zero executor code changes.

## What isn't handled

- **Cross-provider continuation** — a Linear issue's conversation doesn't continue into a GitHub PR's conversation, even when they're about the same task. See [06-multi-provider-patterns.md](06-multi-provider-patterns.md) for the intentional workaround (two agents) and [08-design-decisions-deferred.md](08-design-decisions-deferred.md) for the design discussion we consciously tabled.
- **Conversation resurrection** — once a conversation is closed, future events on the same resource create a new conversation. No "re-open" path. If you want sticky continuation across close/re-open cycles, remove the `status != 'closed'` clause from the lookup; I don't recommend this.
- **Agent-initiated linking** — there's no tool for the agent to say "link this conversation to resource X." Discussed, designed, rejected. The reasoning is in [08-design-decisions-deferred.md](08-design-decisions-deferred.md).

## Where to go from here

- What validation keeps bad configs out: [04-validation-and-safety.md](04-validation-and-safety.md)
- How the tests cover all this: [05-testing.md](05-testing.md)
- The multi-provider workaround: [06-multi-provider-patterns.md](06-multi-provider-patterns.md)
