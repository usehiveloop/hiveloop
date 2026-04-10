# Trigger Architecture

This folder documents the webhook trigger system: how third-party events get translated into agent runs, how the catalog is structured, how the dispatcher routes events, what decisions were made along the way, what's deliberately not built yet, and how to use all of it to build actually-working autonomous agents.

**Two sets of docs, one folder**:

- The numbered files at this level (`01-...` through `09-...`) document **how the system works internally** — architecture, pipeline, catalog model, validation layers, testing strategy. Read these if you're contributing to the trigger system or reviewing a PR that touches it.
- The [`agent-playbook/`](agent-playbook/) subfolder documents **how to use the system** to design and deploy autonomous agents — design principles, trigger strategy, safety patterns, economics, debugging, worked examples, anti-patterns, and practical limitations. Read these if you're building an agent.

Both sets cross-reference each other liberally.

## What this system does

When a customer connects a provider like GitHub or Slack and wires an agent to "fire when this event happens," the system needs to:

1. Receive the webhook from Nango (or, later, from a provider-specific endpoint)
2. Identify which agents care about this event and whether their filter conditions pass
3. Build the context the agent needs (usually one or more read API calls against the provider)
4. Start or continue a conversation with the agent, passing the enriched context as the opening message

The design splits this into two phases with a hard boundary between them:

- **Dispatch** — pure logic. Decide who runs, with what refs, producing `PreparedRun` blueprints. One DB query + CPU work, runs in microseconds. Fully unit-tested with real webhook fixtures from octokit/webhooks.
- **Execution** — I/O. Fire the context requests against Nango, ensure the agent is in Bridge, create or continue a conversation, send the final message. Deferred to a follow-up PR.

Everything in this folder documents the dispatch half, plus the catalog it reads from. Where the executor fits in is noted where relevant.

## Reading order for the technical docs

New to the codebase? Read in this order:

1. [01-catalog-architecture.md](01-catalog-architecture.md) — the data model that triggers are built on
2. [02-dispatcher-runtime.md](02-dispatcher-runtime.md) — the pipeline that turns webhooks into runs
3. [03-lifecycle-and-continuation.md](03-lifecycle-and-continuation.md) — resource keys and terminate rules
4. [06-multi-provider-patterns.md](06-multi-provider-patterns.md) — how users configure cross-source agents

Reviewing a PR that touches this area? Read:

1. [04-validation-and-safety.md](04-validation-and-safety.md) — what's enforced where
2. [05-testing.md](05-testing.md) — the test strategy and fixture provenance
3. [09-known-limitations.md](09-known-limitations.md) — known gaps to watch for

Working on the executor PR? Read:

1. [03-lifecycle-and-continuation.md](03-lifecycle-and-continuation.md) — the contract the dispatcher hands you
2. [02-dispatcher-runtime.md](02-dispatcher-runtime.md) — especially the `PreparedRun` shape
3. [08-design-decisions-deferred.md](08-design-decisions-deferred.md) — what you're expected NOT to add

Auditing the provider catalog? Read:

1. [07-catalog-validation-report.md](07-catalog-validation-report.md) — the audit from 31 parallel validation agents against GitHub's official REST + webhooks docs

## Reading order for the playbook (usage docs)

**Building your first agent?** Start with:

1. [`agent-playbook/README.md`](agent-playbook/README.md) — playbook overview
2. [`agent-playbook/01-agent-design-principles.md`](agent-playbook/01-agent-design-principles.md) — how to think about what an agent should be
3. [`agent-playbook/02-trigger-configuration-strategy.md`](agent-playbook/02-trigger-configuration-strategy.md) — how to pick events, conditions, and context
4. [`agent-playbook/04-safety-and-loop-prevention.md`](agent-playbook/04-safety-and-loop-prevention.md) — read this before deploying anything
5. [`agent-playbook/07-worked-examples.md`](agent-playbook/07-worked-examples.md) — five full configurations with commentary

**Debugging a misbehaving agent?**

- [`agent-playbook/06-observability-and-debugging.md`](agent-playbook/06-observability-and-debugging.md) — how to read dispatch logs
- [`agent-playbook/08-anti-patterns.md`](agent-playbook/08-anti-patterns.md) — common mistakes

**Planning a rollout?**

- [`agent-playbook/05-economics-and-performance.md`](agent-playbook/05-economics-and-performance.md) — cost structure
- [`agent-playbook/09-practical-limitations.md`](agent-playbook/09-practical-limitations.md) — what to plan around

## Index

### Technical architecture docs

| File | What's in it |
|---|---|
| [01-catalog-architecture.md](01-catalog-architecture.md) | ProviderActions, ResourceDef, ActionDef, TriggerDef, schemas. How the catalog is generated, embedded, and queried. |
| [02-dispatcher-runtime.md](02-dispatcher-runtime.md) | The dispatch pipeline end-to-end. DispatchInput, PreparedRun, stores, refs, templates, conditions, context builder, asynq wiring, Nango webhook integration. |
| [03-lifecycle-and-continuation.md](03-lifecycle-and-continuation.md) | Resource keys, terminate rules, silent vs graceful close, sandbox strategies, and the contract the executor will pick up. |
| [04-validation-and-safety.md](04-validation-and-safety.md) | Save-time and dispatch-time validation, ambiguous key detection, defense-in-depth strategy. |
| [05-testing.md](05-testing.md) | The 17 logic tests, the real-Postgres GORM test, fixture provenance, what's intentionally not tested. |
| [06-multi-provider-patterns.md](06-multi-provider-patterns.md) | The "two agents, one config" pattern for cross-source agents. Why we didn't build cross-provider continuation. |
| [07-catalog-validation-report.md](07-catalog-validation-report.md) | The full findings from the 31-agent catalog audit. Wrong access flags, missing body params, missing actions, ref bugs, schema issues. |
| [08-design-decisions-deferred.md](08-design-decisions-deferred.md) | Things we explicitly chose NOT to build and why. Cross-provider continuation, multi-provider context gathering, UI bundle_id, raw payload templating, fan-out strategy. |
| [09-known-limitations.md](09-known-limitations.md) | Current gaps, their workarounds, and what would fix them. |

### Agent playbook (usage docs)

| File | What's in it |
|---|---|
| [`agent-playbook/README.md`](agent-playbook/README.md) | Overview of the playbook, audiences, reading paths. |
| [`agent-playbook/01-agent-design-principles.md`](agent-playbook/01-agent-design-principles.md) | What makes a good agent — single responsibility, scope, when to create vs. extend, state layers. |
| [`agent-playbook/02-trigger-configuration-strategy.md`](agent-playbook/02-trigger-configuration-strategy.md) | Picking events, writing conditions, choosing context actions, writing instructions that guide without over-specifying. |
| [`agent-playbook/03-lifecycle-and-state-management.md`](agent-playbook/03-lifecycle-and-state-management.md) | When conversations continue, when they end, long-running work, memory vs. payload vs. conversation state. |
| [`agent-playbook/04-safety-and-loop-prevention.md`](agent-playbook/04-safety-and-loop-prevention.md) | The biggest failure mode: self-triggering loops, multi-agent loops, write-action discipline, circuit breakers. |
| [`agent-playbook/05-economics-and-performance.md`](agent-playbook/05-economics-and-performance.md) | Per-trigger cost structure, sandbox economics, context fetch budgets, scaling considerations. |
| [`agent-playbook/06-observability-and-debugging.md`](agent-playbook/06-observability-and-debugging.md) | How to know a trigger is working, reading dispatch logs, debugging skipped runs, common workflows. |
| [`agent-playbook/07-worked-examples.md`](agent-playbook/07-worked-examples.md) | Five end-to-end realistic agent configs with full YAML and commentary. |
| [`agent-playbook/08-anti-patterns.md`](agent-playbook/08-anti-patterns.md) | 20 common mistakes with better alternatives. |
| [`agent-playbook/09-practical-limitations.md`](agent-playbook/09-practical-limitations.md) | Business-level constraints on the system today, workarounds, and when each might lift. |

## Where the code lives

| Concern | Package / file |
|---|---|
| Catalog types and accessors | `internal/mcp/catalog/catalog.go` |
| Catalog JSON (per provider) | `internal/mcp/catalog/providers/*.actions.json`, `*.triggers.json` |
| Catalog generator (OpenAPI 3.x) | `cmd/fetchactions-oas3/` |
| AgentTrigger model + types | `internal/model/agent_trigger.go` |
| Dispatcher package | `internal/trigger/dispatch/` |
| Dispatcher tests + fixtures | `internal/trigger/dispatch/*_test.go`, `testdata/github/` |
| Asynq task + handler | `internal/tasks/trigger_dispatch.go` |
| HTTP webhook wiring | `internal/handler/nango_webhooks_dispatch.go` |
| Handler validation | `internal/handler/agent_triggers.go` |

## One-sentence summary of the design

The dispatcher is a provider-agnostic pure function from `(webhook, connection, agent triggers)` to `[]PreparedRun`, driven entirely by JSON catalog data, with zero provider-specific code in the Go source and 17 tests against real webhook fixtures to keep it honest.
