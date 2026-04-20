# 05 — Testing

The dispatcher has 17 logic tests and 1 real-Postgres integration test. All of them use real GitHub webhook payloads sourced from the octokit/webhooks repository — the canonical fixture set GitHub itself ships and that every official GitHub library depends on. None of them mock the catalog: every test runs against the actual embedded catalog data, so a test pass is a statement that real webhooks get processed correctly against real catalog data.

## Test philosophy

Three guiding principles that shaped how the suite is built:

### 1. Real fixtures, not synthesized ones

Hand-crafted test payloads look correct until you hit an edge case the author didn't think of. Real fixtures carry all the quirks of the actual API surface — nested objects you didn't expect, null vs. missing distinctions, integer vs. string IDs, odd field names. The dispatcher validates refs against these real payloads, so a regression that breaks real webhook processing also breaks a test.

The downside: we can't easily generate fixtures for every combination (e.g., PR closed with `merged: true` when the fixture ships with `merged: false`). The workaround is `patchPath(t, payload, "pull_request.merged", true)` — mutate a real fixture in-test to cover the variation you need. This keeps the fixture set small while covering the state space.

### 2. Real catalog, not mocks

Every test calls `catalog.Global()` and reads from the actual embedded `github.actions.json` + `github.triggers.json`. The reasons:

- Catalog shape is the most important contract between the dispatcher and the trigger system. A mock would encode assumptions about the shape that might drift from the real catalog.
- Trigger refs, resource ref_bindings, resource templates, action execution configs — all of them get tested end-to-end whenever a dispatcher test runs.
- Changes to the catalog (e.g., regenerating after an OpenAPI spec update) surface as test failures if they break an assumption the dispatcher is relying on.

This has saved us already. When the `inferAccess` bug was fixed in the generator, regenerating the catalog changed several actions from `read` to `write`. The dispatcher tests caught the case where a context action had been silently allowed through save-time validation before the fix but would now be rejected.

### 3. In-memory stores for dispatcher tests, real DB for the store test

The 17 dispatcher tests use `MemoryAgentTriggerStore`, which mirrors the GORM store semantics line-by-line. This keeps the test suite fast (well under a second for all 17) and hermetic (no Postgres dependency, runs anywhere Go runs). But in-memory fakes can lie — if the fake's match logic drifts from the real SQL query, tests pass but production breaks.

The "no copping out" guardrail is `TestGormAgentTriggerStore_FindMatching` in `stores_postgres_test.go`. It requires a real Postgres, uses `model.AutoMigrate` to create the real schema, seeds rows directly via GORM, and verifies the production query returns the same shape as the in-memory fake. When Postgres isn't available, the test skips gracefully with a clear message; in CI or a dev environment with `make test-setup`, it runs.

## Fixtures

All live in `internal/trigger/dispatch/testdata/github/` with provenance documented in `SOURCES.md`:

| File | Source | Purpose |
|---|---|---|
| `issues.opened.json` | `octokit/webhooks/.../issues/opened.payload.json` | Happy-path issue creation |
| `issues.labeled.json` | `.../issues/labeled.payload.json` | Cross-trigger continuation |
| `pull_request.opened.json` | `.../pull_request/opened.payload.json` | PR creation, draft=false |
| `issue_comment.created.issue.json` | `.../issue_comment/created.payload.json` | Issue comment (no PR) |
| `issue_comment.created.pr.json` | derived from `created.payload.json` | PR comment — adds `issue.pull_request` field |
| `push.json` | `.../push/payload.json` | Tag push (`refs/tags/simple-tag`) |
| `push.new-branch.json` | `.../push/with-new-branch.payload.json` | Branch push (`refs/heads/master`) |
| `workflow_run.completed.json` | `.../workflow_run/completed.payload.json` | Workflow run, conclusion=success |
| `workflow_run.completed.failure.json` | derived | conclusion patched to `failure` |
| `release.published.json` | `.../release/published.payload.json` | Release 0.0.1 published |

Two fixtures are derived from real payloads rather than taken verbatim:

- **`issue_comment.created.pr.json`**: octokit only ships the issue-comment variant. The PR variant (comment on a pull request) is derived by adding the `issue.pull_request` field GitHub sends in that case, with URL fields pointing at the correct PR endpoints. The comment body is also patched to `@zira can you take a look at this PR?` so the mention-matching tests have realistic content. Full provenance in `SOURCES.md`.
- **`workflow_run.completed.failure.json`**: octokit's fixture has `conclusion: success`. The tests need to verify the "only on failure" condition path, so we derive a fixture with `workflow_run.conclusion` patched to `failure`.

Both derivations are one-line Python patches, reproducible from the original fixtures, and documented. Committing them to `testdata/` keeps tests hermetic.

## The harness

`dispatcher_helpers_test.go` holds the shared test infrastructure:

```go
type dispatchHarness struct {
    t            *testing.T
    dispatcher   *Dispatcher
    store        *MemoryAgentTriggerStore
    orgID        uuid.UUID
    connectionID uuid.UUID
    connection   *model.Connection
}

func newHarness(t *testing.T) *dispatchHarness
```

A fresh harness per test: new store, new dispatcher, new IDs, real catalog. Deterministic UUIDs (hardcoded fixture strings like `11111111-1111-1111-1111-111111111111`) so assertions don't flake on randomness.

Test helpers:

- `loadFixture(t, name)` — reads a JSON file from `testdata/github/` and unmarshals to `map[string]any`
- `harness.addTrigger(keys, conditions, context, instructions, ...customizers)` — seeds an agent + AgentTrigger row
- `harness.addTerminateTrigger(keys, conditions, context, instructions, rules, ...customizers)` — same, but with `TerminateOn` + denormalized `TerminateEventKeys` populated
- `harness.run(eventType, eventAction, payload)` — builds a `DispatchInput` and calls `dispatcher.Run`
- `assertSinglePrepared(t, runs)` — asserts exactly one run was returned; good default assertion for most tests
- `assertContextRequest(t, run, as)` — finds a request by its `as` name; fails loud with available names if not found
- `patchPath(t, payload, path, value)` — mutates a fixture in place (e.g., `patchPath(t, payload, "sender.login", "dependabot[bot]")`)
- `mustMarshalJSON(t, value)` — marshals with test-fatal error handling

The test functions themselves are short: seed, run, assert. A typical test is 30–50 lines.

## The 17 logic tests

| # | Test | What it proves |
|---|---|---|
| 1 | `TestDispatch_IssuesOpened_HappyPath` | Full happy path: refs extracted, instructions substituted, 3 context requests built with fully-resolved paths, query params, and template substitution. Also asserts `ResourceKey = Codertocat/Hello-World#issue-1`. |
| 2 | `TestDispatch_IssuesOpened_BotFiltered` | `not_one_of` operator. Patches `sender.login` to `dependabot[bot]`, asserts the run is skipped with a reason that mentions both the path and operator. Refs are still populated on the skipped run for debugging visibility. |
| 3 | `TestDispatch_PullRequestOpened_MultiEvent` | Multi-event trigger `[opened, synchronize, ready_for_review]`. Asserts the correct key matches, resource key is `Codertocat/Hello-World#pr-2`, context action path substitution works for `pulls_list_files`. |
| 4 | `TestDispatch_PullRequestOpened_DraftSkipped` | `not_equals true` operator against boolean payload field. Patches `pull_request.draft = true`, asserts skip. |
| 5 | `TestDispatch_IssueComment_MentionOnIssue` | Two conditions with `match: all`: `comment.body contains "@zira"` AND `issue.pull_request not_exists`. Exercises both operators and `{{$refs.x}}` mustache substitution in instructions. |
| 6 | `TestDispatch_IssueComment_PRCommentSkipped` | Same trigger as #5 but on the PR variant fixture. `not_exists` fails because `issue.pull_request` IS present. Assertion checks that the skip reason mentions both `issue.pull_request` and `not_exists`. |
| 7 | `TestDispatch_Push_RegexMatch` | `matches` operator with regex `^refs/heads/master$`. Uses `push.new-branch.json`. Asserts `ResourceKey == ""` because `repository` has no template. |
| 8 | `TestDispatch_Push_RegexMissMisses` | Same regex against `push.json` (tag push). Regex fails, run skipped. |
| 9 | `TestDispatch_WorkflowRun_FailureOnly` | `equals` operator on nested path `workflow_run.conclusion`. Uses the patched failure fixture. Asserts the `run_id` ref resolves correctly and the context action path substitutes it in. |
| 10 | `TestDispatch_Release_DedicatedAgent` | `agent.SandboxType = "dedicated"` produces `SandboxStrategy = CreateDedicated` and nil `SandboxID`. Also asserts release resource key substitution. |
| 11 | `TestDispatch_TwoAgents_FanOut` | Two AgentTrigger rows on the same connection listening for the same event, different agents (shared + dedicated). Deterministic ordering by trigger ID. Each run has its own sandbox strategy. |
| 12 | `TestDispatch_CrossTriggerContinuation` | Three different event types (`issues.opened`, `issues.labeled`, `issue_comment.created`) on the same issue all resolve to the same `ResourceKey`. This is the foundational test for continuation — without it, the executor's lookup can't work. |
| 13 | `TestDispatch_TerminateMergedPR_GracefulClose` | Terminate rule for `pull_request.closed` with `merged: true`. Asserts `RunIntent == terminate`, `SilentClose == false`, context request from the rule resolves, instructions substituted. |
| 14 | `TestDispatch_TerminateClosedUnmerged_SilentClose` | Two rules on the same event key with different conditions; asserts first-pass-wins picks the `merged: false, silent: true` rule. Asserts silent runs have no context requests and no instructions, but DO have a resource key (so the executor can find the conversation to close). |
| 15 | `TestDispatch_TerminateInheritsParentConditions` | Parent conditions skip drafts. A close event on a draft PR is skipped with reason prefixed by `parent `. Validates condition inheritance. |
| 16 | `TestDispatch_AmbiguousKeyRejected` | Trigger configured with `pull_request.closed` in BOTH `trigger_keys` and `terminate_on`. Dispatch-time check rejects with `ambiguous` in the skip reason. (Handler's save-time check is the primary line of defense; this is the drift safety net.) |
| 17 | `TestDispatch_TerminateOnlyKey_StoreMatches` | Store query must match triggers whose event only appears in `terminate_event_keys`, not `trigger_keys`. Proves the new column is actually queried. Without this test, terminate-only events would silently not fire and no other test would catch it. |

Every test reads a fixture, builds a trigger config, calls `harness.run`, and asserts on the returned `[]PreparedRun`. No HTTP, no asynq, no executor, no Nango — just the pure dispatcher logic.

## The real-DB test

`stores_postgres_test.go` holds `TestGormAgentTriggerStore_FindMatching`:

```go
func TestGormAgentTriggerStore_FindMatching(t *testing.T) {
    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" {
        dsn = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable"
    }
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        t.Skipf("Postgres not reachable: %v", err)
    }
    // ... ping, auto-migrate, seed, query, assert
}
```

What this test catches that in-memory tests can't:

- **SQL typos** in the WHERE clause column names
- **Misuse of `pq.StringArray`** vs `[]string` in the GORM WHERE arg
- **Wrong array operator** (e.g., `@>` instead of `&&`)
- **Schema drift** between the model and the production migration (e.g., if a new column gets added to the model but `AutoMigrate` doesn't pick it up)
- **Index missing** that would cause slow queries in production (this test doesn't benchmark, but the query is at least exercised)

The test seeds four triggers with carefully-chosen shapes:

1. A trigger that should match: enabled, right connection, key in `trigger_keys`
2. A trigger with the same key but `enabled = false` — must NOT match (GORM's bool-zero-value workaround exercised)
3. A trigger with the right key but on a DIFFERENT connection — must NOT match
4. A trigger on the right connection but with a different key — must NOT match

Then it queries three times:

- Once for the primary event key — expects only the first trigger
- Once for the SECOND element of the first trigger's TriggerKeys array — verifies the `&&` operator matches on array overlap, not just first element
- Once for a key no trigger listens for — expects empty result

When Postgres is reachable, all three queries exercise the production SQL. When it's not, the test skips with `t.Skipf` so CI environments without Postgres don't see spurious failures.

## What's intentionally not tested

Four categories deliberately sit outside this test suite:

### 1. HTTP handler behavior

`internal/handler/nango_webhooks_dispatch.go` has its own concerns — signature verification, Nango envelope unwrapping, GitHub event-type inference, task enqueueing. These belong in `internal/handler/nango_webhooks_test.go` as HTTP tests, not in the dispatcher package. None exist yet (the existing nango webhook handler tests don't cover the dispatch path). The dispatcher itself doesn't know anything about HTTP.

### 2. Asynq round-trip

`internal/tasks/trigger_dispatch.go` is a thin wrapper that unmarshals a payload, reloads a connection, and calls `Dispatcher.Run`. Its test surface is essentially "does the payload marshal/unmarshal correctly and does it pass the right input to the dispatcher?" — one smoke test is enough and belongs in `internal/tasks/`, not here. Not written yet; low urgency because the dispatcher itself is thoroughly tested and the asynq layer is just plumbing.

### 3. Executor behavior

The executor doesn't exist yet. When it does, its tests will cover:

- Conversation creation vs. continuation (`ensureConversation` lookup semantics)
- Terminate-silent vs. terminate-graceful flows
- Context action execution order and `{{$step.x}}` substitution
- Sandbox ensure/create paths
- Error handling for Nango failures
- Per-agent retry isolation

None of that exists because the executor doesn't. Future PR.

### 4. Catalog correctness

The 31-agent audit (see [07-catalog-validation-report.md](07-catalog-validation-report.md)) was a one-time cross-check against GitHub's official docs. It caught generator bugs and trigger catalog mistakes. Re-running it on every test run would be expensive, network-bound, and flaky. Catalog correctness is validated at generation time and reviewed on regeneration, not on every test run.

What the dispatcher tests DO implicitly check: the catalog is parseable, the trigger definitions match what the dispatcher expects, and ref extraction works against real payloads. If someone regenerates the catalog and breaks a trigger's refs, the test suite will fail with a useful error.

## Running the tests

```bash
# Fast: 17 logic tests, sub-second
go test ./internal/trigger/dispatch/ -v -run TestDispatch

# With the real-DB test (requires Postgres on localhost:5433)
make test-setup  # brings up docker-compose services
go test ./internal/trigger/dispatch/ -v

# All dispatcher tests + race detector
go test ./internal/trigger/dispatch/ -v -race -count=1
```

Typical output when everything works:

```
=== RUN   TestDispatch_IssuesOpened_HappyPath
--- PASS: TestDispatch_IssuesOpened_HappyPath (0.11s)
=== RUN   TestDispatch_IssuesOpened_BotFiltered
--- PASS: TestDispatch_IssuesOpened_BotFiltered (0.00s)
...
=== RUN   TestDispatch_TerminateOnlyKey_StoreMatches
--- PASS: TestDispatch_TerminateOnlyKey_StoreMatches (0.00s)
=== RUN   TestGormAgentTriggerStore_FindMatching
    stores_postgres_test.go:38: Postgres not reachable, skipping real-DB test
--- SKIP: TestGormAgentTriggerStore_FindMatching (0.00s)
PASS
ok  	github.com/usehiveloop/hiveloop/internal/trigger/dispatch	1.493s
```

## Adding new tests

Three patterns cover 90% of what you'll want to add:

### New event type coverage

Pull a real fixture from octokit/webhooks, add it to `testdata/github/` with a note in `SOURCES.md`, then write a test that:

1. Seeds a trigger listening for the event
2. Loads the fixture
3. Calls `harness.run(eventType, eventAction, payload)`
4. Asserts on the returned run's refs, resource key, context requests, and instructions

Pattern to copy from: `TestDispatch_IssuesOpened_HappyPath`.

### New operator or condition edge case

Add a test that exercises the specific operator against a real fixture, with assertions on both the pass path and the skip path. Patch the fixture in-test if you need to cover both cases.

Pattern to copy from: `TestDispatch_Push_RegexMatch` + `TestDispatch_Push_RegexMissMisses`.

### New terminate scenario

Use `addTerminateTrigger` with a specific `[]TerminateRule` shape, fire a matching event, and assert on `RunIntent`, `SilentClose`, `ResourceKey`, and the context/instructions fields.

Pattern to copy from: `TestDispatch_TerminateMergedPR_GracefulClose` or `TestDispatch_TerminateClosedUnmerged_SilentClose`.

## When tests fail

Common failure modes and what they mean:

- **"expected 1 prepared run, got 0"** — the trigger didn't match. Check: did you `addTrigger` with the right trigger keys? Is the connection ID matching? Is `Enabled: true`?
- **"skip reason: ..."** — the run matched but was filtered. The reason is human-readable and points at the failing condition.
- **"resource key = ..."** — unexpected key format. Check the catalog's `resource_key_template` for the resource, and verify the trigger's refs include every ref the template references.
- **"context request path = ..."** — path substitution went wrong. Check the action's `execution.path` template and the resource's `ref_bindings`.

Every failure message includes enough context to jump straight to the offending code or data.

## Where to go from here

- The runtime contract being tested: [02-dispatcher-runtime.md](02-dispatcher-runtime.md)
- The lifecycle semantics tests #12–17 exercise: [03-lifecycle-and-continuation.md](03-lifecycle-and-continuation.md)
- The catalog audit that validated the fixture provenance indirectly: [07-catalog-validation-report.md](07-catalog-validation-report.md)
