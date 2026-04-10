# 02 — Dispatcher Runtime

The dispatcher is the pure-logic half of the trigger system. It takes a webhook envelope and a resolved connection, queries the AgentTrigger store, and produces `[]PreparedRun` blueprints that a future executor will turn into conversations. The dispatcher does not touch Nango, does not touch Bridge, does not create conversations, and does not fire context actions. Keeping that boundary hard is what makes it tractable to test.

## Package layout

```
internal/trigger/dispatch/
├── prepared_run.go            # PreparedRun, ContextRequest, DispatchInput, RunIntent, SandboxStrategy
├── dispatcher.go              # Dispatcher.Run() — the pipeline
├── stores.go                  # AgentTriggerStore interface + production GORM impl
├── stores_mem.go              # MemoryAgentTriggerStore for tests (mirrors GORM semantics)
├── refs.go                    # extractRefs from payload using catalog TriggerDef.Refs
├── conditions.go              # operator evaluation + match all/any combination
├── template.go                # $refs.x, {{$refs.x}}, {{$step.x}} substitution
├── context_builder.go         # ContextAction → ContextRequest
├── errors.go                  # ErrUnknownProvider, ErrNilConnection
├── dispatcher_helpers_test.go # newHarness, loadFixture, assertion helpers
├── dispatcher_test.go         # 17 logic tests
├── stores_postgres_test.go    # 1 real-DB test, skips if Postgres unreachable
└── testdata/github/           # real octokit webhook fixtures
```

## The entry point

`Dispatcher.Run(ctx, DispatchInput) ([]PreparedRun, error)` is the single function the asynq task handler calls. Its input is a resolved webhook envelope; its output is one `PreparedRun` per matched `AgentTrigger`, in deterministic order.

```go
type Dispatcher struct {
    Triggers AgentTriggerStore
    Catalog  *catalog.Catalog
    Logger   *slog.Logger
}

type DispatchInput struct {
    Provider    string            // "github", "github-app", "intercom", etc.
    EventType   string            // "issues" (X-GitHub-Event header)
    EventAction string            // "opened" (payload.action). Empty for actionless events.
    Payload     map[string]any    // the raw webhook body, JSON-decoded
    DeliveryID  string            // provider-assigned delivery id (for tracing/dedup)
    OrgID       uuid.UUID
    Connection  *model.Connection // already resolved by the caller, with Integration preloaded
}
```

Three things matter about this shape:

1. **The caller resolves the connection.** The dispatcher receives it pre-resolved. This keeps the Nango handler, the (future) direct-GitHub endpoint, and any custom per-provider endpoint all sharing the same dispatcher code — only the resolution step differs per caller.
2. **The payload is already parsed.** The dispatcher walks it as `map[string]any` for ref extraction and condition evaluation. No streaming, no lazy parsing; it's always the whole body in memory.
3. **The trigger key is derived on demand.** `DispatchInput.TriggerKey()` assembles `<event_type>.<event_action>` for action-bearing events and `<event_type>` for actionless ones (push, create, delete). The dispatcher matches against this composite key everywhere.

## The output

`PreparedRun` is a value type carrying everything the executor needs for one agent run:

```go
type PreparedRun struct {
    OrgID          uuid.UUID
    AgentID        uuid.UUID
    AgentTriggerID uuid.UUID
    ConnectionID   uuid.UUID
    NangoConnID    string           // copied from Connection.NangoConnectionID
    ProviderCfgKey string           // {orgID}_{integrationUniqueKey}
    Provider       string
    TriggerKey     string

    RunIntent       RunIntent        // normal | terminate
    SilentClose     bool             // only meaningful with RunIntent=terminate
    ResourceKey     string           // stable identity for continuation lookups

    SandboxStrategy SandboxStrategy  // reuse_pool | create_dedicated
    SandboxID       *uuid.UUID       // only set for reuse_pool

    Refs            map[string]string  // the resolved ref map
    ContextRequests []ContextRequest   // ordered read-only API calls
    Instructions    string             // prompt template with $refs.x substituted
    DeferredVars    []string           // {{$step.x}} placeholders for the executor to resolve

    SkipReason      string             // populated when filtered out; non-skipped runs have this empty
}
```

`ContextRequest` is one fully-resolved API call:

```go
type ContextRequest struct {
    As           string            // context bag key (e.g. "issue", "files")
    ActionKey    string            // catalog action key (e.g. "issues_get")
    Method       string
    Path         string            // already substituted: /repos/octocat/Hello-World/issues/1347
    Query        map[string]string
    Body         map[string]any
    Headers      map[string]string
    Optional     bool              // failure doesn't block the run
    DeferredVars []string          // {{$step.x}} placeholders in this request's params
}
```

The executor iterates the slice in order, fires each request against Nango, resolves the next request's `{{$step.x}}` placeholders from previous results, and finally sends `Instructions` (with all placeholders resolved) to the agent's conversation.

## The pipeline

`Dispatcher.Run` is a straight-line function with no hidden state. Here's what happens on a single webhook:

### 1. Look up the provider's triggers

```go
triggerCatalog := d.lookupProviderTriggers(input.Provider)
if triggerCatalog == nil {
    return nil, ErrUnknownProvider
}
```

Uses the variant fallback (`github-app` → `github`) so the same trigger definitions apply across auth modes.

### 2. Look up the specific trigger definition

```go
triggerDef, hasTrigger := triggerCatalog.Triggers[triggerKey]
if !hasTrigger {
    return nil, nil  // no one is listening for this; not an error
}
```

If the event key isn't in the catalog at all (e.g. a trigger we haven't added yet), we ack the webhook silently. This is the right behavior — webhooks for unconfigured events are normal, not errors.

### 3. Extract refs from the payload

```go
refs, missing := extractRefs(input.Payload, triggerDef.Refs)
```

`extractRefs` walks the catalog's `Refs` map and does a dot-path lookup for each entry. Missing or non-scalar paths are logged but don't fail the dispatch — some refs are only present in certain sub-action payloads (e.g. `issue.pull_request` exists on PR comments but not issue comments), and the conditions system handles that case via `exists`/`not_exists`.

`stringifyScalar` converts JSON-decoded values (which come back as `float64` for all numbers) to their string form, rendering integers without trailing `.0` so paths like `/repos/octocat/Hello-World/issues/1347` look right instead of `/repos/octocat/Hello-World/issues/1347.000000`.

### 4. Resolve the resource key

```go
resourceKey := resolveResourceKey(d.Catalog, input.Provider, triggerDef.ResourceType, refs)
```

Substitutes `$refs.x` into the resource's `ResourceKeyTemplate`. If the resource type has no template, or if substitution leaves any `$refs.x` unresolved, the returned key is empty string. Empty means "always create a new conversation" — resources without continuation semantics never accidentally merge.

See [03-lifecycle-and-continuation.md](03-lifecycle-and-continuation.md) for the full semantics.

### 5. Query the agent trigger store

```go
matches, err := d.Triggers.FindMatching(ctx, input.OrgID, input.Connection.ID, []string{triggerKey})
```

One DB query. Returns every enabled `AgentTrigger` on this connection where the event key appears in either `TriggerKeys` (normal events) or `TerminateEventKeys` (terminate events). The store sorts results by trigger ID for deterministic ordering.

### 6. Per-match: decide the intent and build the run

```go
for _, match := range matches {
    isNormal := containsString(match.Trigger.TriggerKeys, triggerKey)
    terminateRule, hasTerminate := findMatchingTerminateRule(match.Trigger, triggerKey, input.Payload, logger)

    if isNormal && hasTerminate {
        // ambiguous — save-time validation should've caught this
        runs = append(runs, ambiguousSkip(...))
        continue
    }
    switch {
    case isNormal:
        runs = append(runs, d.buildNormalRun(...))
    case hasTerminate:
        runs = append(runs, d.buildTerminateRun(...))
    }
}
```

The "find rule" step walks `[]TerminateRule` in order, returning the first rule whose `TriggerKeys` contains the event key AND whose own conditions pass. First-pass-wins — this is what lets users write ordered rules like `[{merged:true, graceful}, {merged:false, silent}]`.

### 7. Return the slice

Skipped runs stay in the output with `SkipReason` set. The executor filters them out at enqueue time. Keeping them visible makes debugging "why did my trigger not fire?" a log-reading exercise instead of a mystery.

## Normal run construction

`buildNormalRun` — the happy path that turns a matched trigger into a blueprint:

1. Build the `baseRun` with all identity fields (OrgID, AgentID, ResourceKey, sandbox strategy)
2. Parse `AgentTrigger.Conditions` (JSONB → `TriggerMatch`) and evaluate against the payload
3. If conditions fail, set `SkipReason` but keep building — we want `ContextRequests` populated for debugging
4. Parse `AgentTrigger.ContextActions` (JSONB → `[]ContextAction`) and call `buildContextRequests`
5. Substitute `$refs.x` into the trigger's `Instructions` template
6. Collect `{{$step.x}}` placeholders into `DeferredVars`

The whole thing takes well under a millisecond per match — it's all map lookups and string substitution.

## Terminate run construction

`buildTerminateRun` is structurally similar but applies rules in a specific order:

1. Resolve sandbox + refs + resource key same as normal
2. **Fail if `ResourceKey` is empty.** A terminate run with no lookup key has nothing to close, so we skip with an explicit reason. This also catches cases where someone adds a terminate rule to a resource whose template is empty (a config bug).
3. **Inherit parent conditions.** Parse the parent `AgentTrigger.Conditions` and evaluate against the payload. If they fail, set `SkipReason` with a "parent " prefix so logs make it obvious the inherited filter rejected it. Opt out with `IgnoreParentConditions: true` if the terminate rule needs different scope.
4. **Apply rule-specific conditions.** If parent passed, evaluate the rule's own `Conditions`.
5. **If `Silent: true`, stop here.** Silent terminations carry no context actions and no instructions — the executor's job is just "look up the conversation by ResourceKey and close it."
6. **Otherwise, build the graceful final run.** Context actions from the rule, instructions from the rule, same ref substitution as normal runs.

The key insight: a terminate rule is structurally a mini-trigger with its own conditions, context, and instructions. Inheriting parent conditions by default gives the "skip drafts everywhere" ergonomics without requiring every rule to duplicate the filter.

## The stores

`AgentTriggerStore` is a tiny interface designed to be faked easily in tests:

```go
type AgentTriggerStore interface {
    FindMatching(ctx context.Context, orgID, connectionID uuid.UUID, triggerKeys []string) ([]TriggerWithAgent, error)
}

type TriggerWithAgent struct {
    Trigger model.AgentTrigger
    Agent   model.Agent
}
```

### Production: `gormAgentTriggerStore`

```go
func (s *gormAgentTriggerStore) FindMatching(ctx context.Context, orgID, connectionID uuid.UUID, triggerKeys []string) ([]TriggerWithAgent, error) {
    keys := pq.StringArray(triggerKeys)
    var triggers []model.AgentTrigger
    err := s.db.WithContext(ctx).
        Where("org_id = ? AND connection_id = ? AND enabled = TRUE AND (trigger_keys && ? OR terminate_event_keys && ?)",
            orgID, connectionID, keys, keys).
        Order("id ASC").
        Find(&triggers).Error
    // ... then load agents, filter out soft-deleted, join into []TriggerWithAgent
}
```

The `&&` operator is PostgreSQL's array overlap. It matches when ANY element of the trigger's array is in the input array. This is what lets a multi-event trigger (`["issues.opened","issues.closed","issues.reopened"]`) match on any one of them with a single query.

The query checks BOTH columns — `trigger_keys` for normal events and `terminate_event_keys` for terminators. Without the second clause, terminate-only events (events that appear only in a `TerminateRule` but not in `TriggerKeys`) would never match the store query and the terminate path would be dead on arrival.

### Tests: `MemoryAgentTriggerStore`

Mirrors the GORM store exactly. The match semantics are copied line-by-line so the in-memory tests catch logic bugs that would also surface in production. One real-Postgres test (`TestGormAgentTriggerStore_FindMatching`, see [05-testing.md](05-testing.md)) verifies that the actual SQL query behaves identically to the in-memory fake — the "no copping out" guardrail that prevents the fake from lying.

## Refs extraction

```go
func extractRefs(payload map[string]any, defs map[string]string) (refs map[string]string, missing []string)
```

`defs` comes from `TriggerDef.Refs` — a map from ref name to dot-path. The function walks each path with `lookupPath`, converts the result with `stringifyScalar`, and returns both a resolved map and a list of missing refs (for logs, not errors).

Explicitly NOT supported: array indexing (`foo.0.bar`), filter expressions, regex captures. Trigger refs in the catalog always point at scalar fields on top-level or one-level-nested objects. When that's not enough (e.g., "the first label's name"), the condition system picks up the slack via its own path evaluator.

## Template substitution

Three template forms, resolved at different layers:

| Form | Resolved by | When |
|---|---|---|
| `$refs.x` | `substituteRefs()` | Dispatcher, from the refs map |
| `{{$refs.x}}` | `substituteRefs()` | Dispatcher, same as above — different surface syntax for readability |
| `{{$step.field}}` | Executor | After each context action returns |

`substituteRefs` handles the first two by replacing every occurrence in the input string with the corresponding ref value. Unresolved refs (ref name not in the map) are left in place so they show up in logs and debug output — better than silent drops.

`findStepReferences` scans for `{{$step.x}}` patterns and returns a deduped list of step names. The dispatcher records these in `DeferredVars` on the `PreparedRun` and on each `ContextRequest` so the executor knows which step results to thread through.

Path parameter substitution is separate and simpler — `substitutePathParams` replaces `{param}` placeholders in an action's `execution.path` from a values map. No brace templating there, just single-brace placeholders coming straight from the OpenAPI path.

## Conditions

The full operator set, all implemented in `conditions.go`:

| Operator | Semantics |
|---|---|
| `equals`, `not_equals` | string comparison after stringifying both sides |
| `one_of`, `not_one_of` | membership in a `[]string` |
| `contains`, `not_contains` | substring match |
| `matches` | `regexp.Compile` + `MatchString` |
| `exists`, `not_exists` | non-nil resolved path |

Conditions combine via `match: all` (AND) or `match: any` (OR), with `all` as the default when `match` is empty.

Failure reasons are human-readable and include the condition index and path/operator so logs point you straight at the failing rule:

```
condition 2 (pull_request.draft not_equals) failed
parent condition 0 (sender.login not_one_of) failed
terminate rule condition 0 (pull_request.merged equals) failed
```

The `parent` and `terminate rule` prefixes make it clear which layer of the check failed.

## Context request building

`buildContextRequests` is where a `[]ContextAction` turns into a `[]ContextRequest`:

```go
func buildContextRequests(
    cat *catalog.Catalog,
    provider string,
    actions []model.ContextAction,
    refs map[string]string,
    triggerKey string,
) (requests []ContextRequest, errs []string)
```

Param resolution follows a layered strategy, highest priority wins:

1. **Explicit params from `ContextAction.Params`** (with `$refs.x` substituted)
2. **Resource `ref_bindings`** (from the resource referenced by `ContextAction.Ref`)
3. **Bare ref map** as a last-resort fallback for path placeholders that aren't otherwise bound

Once params are resolved, the function:

- Substitutes `{param}` placeholders in `action.Execution.Path` using the merged values
- Splits params into query/body according to `action.Execution.QueryMapping`/`BodyMapping`
- Copies headers from `action.Execution.Headers`
- Scans for `{{$step.x}}` placeholders and validates that the referenced step name appears in an earlier `ContextAction.As`
- Flags dangling step references with build errors (doesn't crash, just records)

Build errors are returned alongside the partial request list. The dispatcher logs them and sets `SkipReason` on the run. The executor will skip requests with empty `ActionKey` or can just skip the whole run.

## Asynq wiring

```go
const TypeTriggerDispatch = "trigger:dispatch"

type TriggerDispatchPayload struct {
    Provider     string
    EventType    string
    EventAction  string
    DeliveryID   string
    OrgID        uuid.UUID
    ConnectionID uuid.UUID
    PayloadJSON  []byte  // raw webhook body
}
```

`NewTriggerDispatchTask` produces a task with `QueueCritical`, `MaxRetry(3)`, `Timeout(30*time.Second)`. Dispatch is fast (microseconds of actual work), so the timeout is generous. Retries are low because any failure is either a transient DB issue (worth one or two retries) or a programmer error (more retries won't help).

`TriggerDispatchHandler` lives in `internal/tasks/trigger_dispatch.go`. On invocation:

1. Decode the payload envelope
2. Reload the `Connection` from the DB (freshness + current `revoked_at` check)
3. Decode the raw webhook body into `map[string]any`
4. Build a `DispatchInput` and call `Dispatcher.Run`
5. Log the per-run outcomes
6. TODO: enqueue per-agent `TypeAgentRun` child tasks for the executor PR

The handler is registered in `NewServeMux` in `internal/tasks/registry.go`.

## Nango webhook integration

`internal/handler/nango_webhooks_dispatch.go` holds the function that wires dispatch into the existing Nango webhook pipeline. The flow:

```
Nango HTTP POST → NangoWebhookHandler.Handle
                      ↓
                  identify(wh) → (orgID, integration, connection)
                      ↓
                  ┌───┴────┐
                  ↓        ↓
      dispatchTrigger    enqueueForward  ← existing org-webhook forward
      (async, fire-     (unchanged)
       and-forget)
```

`dispatchTrigger`:

1. Skips non-`forward` webhook types (auth events, connection updates — these don't carry a provider event)
2. Skips providers with no triggers in the catalog (including the variant fallback)
3. Unwraps Nango's payload envelope — it arrives either as the raw provider body or wrapped in `{headers, data}`, and this function handles both
4. Pulls the event type from the `X-GitHub-Event` (or equivalent) header when present, falling back to payload-shape inference if the header wasn't passed through
5. Enqueues a `TypeTriggerDispatch` asynq task with the raw body

The two paths (dispatch + existing org-webhook forward) run independently. A failure in dispatch doesn't block the forward, and vice versa. This non-invasiveness is deliberate — we can turn trigger dispatch off without touching the forward path.

### Payload-shape inference for GitHub

When the `X-GitHub-Event` header isn't passed through (depends on how Nango is configured), `inferGitHubEventFromPayload` guesses the event type from which top-level objects are present in the body:

- `workflow_job` object → `workflow_job`
- `workflow_run` object → `workflow_run`
- `check_run` object → `check_run`
- `review` + `pull_request` → `pull_request_review`
- `pull_request` + `comment` → `pull_request_review_comment`
- `comment` + `issue` → `issue_comment`
- `issue` (no comment) → `issues`
- `pull_request` (alone) → `pull_request`
- `release` → `release`
- `discussion` + `comment` → `discussion_comment`
- `discussion` → `discussion`
- `deployment` → `deployment_status`
- `before` + `after` → `push`

`create` and `delete` are intentionally ambiguous — they have identical body shapes and can only be distinguished by the header. This helper returns empty for that case and the dispatcher logs + drops.

## Logging

Every dispatch decision is logged at INFO or higher with structured fields:

- `delivery_id`
- `provider`
- `trigger_key`
- `org_id`
- `connection_id`
- `agent_trigger_id` (for per-match logs)
- `agent_id`
- `intent` (normal | terminate)
- `resource_key`
- `skip_reason`
- `context_requests` count
- `sandbox_strategy`

Searching logs by `delivery_id` gives the complete trace for one webhook from arrival through every matched/skipped run. This was a non-negotiable from early in the design — "extremely detailed logging" was the user's explicit ask for production debugging.

## What the dispatcher doesn't do

To keep the contract clean, these are explicitly out of scope:

- Firing Nango proxy requests
- Creating Bridge conversations
- Provisioning sandboxes
- Substituting `{{$step.x}}` placeholders
- Deciding continue-vs-new on the lookup side (it produces the `ResourceKey`; the executor does the lookup)
- Signature verification (lives in the webhook handler upstream)
- Deduplication (lives at the asynq layer via unique task IDs)

Every one of these is either upstream (handler) or downstream (executor). The dispatcher is pure in the sense that, given the same `DispatchInput` and the same DB state, it always produces the same `[]PreparedRun`. That determinism is what makes it testable against real fixtures.

## Where to go from here

- The exact contract the executor reads: [03-lifecycle-and-continuation.md](03-lifecycle-and-continuation.md)
- What the tests actually assert: [05-testing.md](05-testing.md)
- The validation that keeps bad config out: [04-validation-and-safety.md](04-validation-and-safety.md)
