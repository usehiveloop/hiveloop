# Phase 3C — Three-loop scheduler + watchdog

**Status:** Plan
**Branch:** `rag/phase-3c-scheduler`
**Depends on:** 3A (RAGSource, RAGIndexAttempt models, partial indexes), 3B (Connector interfaces + factory registry)
**Supersedes:** the high-level 3C section of `plans/onyx-port-phase3.md` (lines 296–440)

---

## Issues to Address

3A landed the data model. 3B landed the connector trait surface. Neither does work on its own — `RAGSource` rows just sit there, no goroutine reads `last_successful_index_time` or compares it against `refresh_freq_seconds`, and no factory `Lookup` is ever called.

3C is the missing motor: a process that, on a tick, looks at every enabled source and asks **"is this one due for ingest / perm-sync / prune right now?"**, enqueues an Asynq job for each, and a separate worker pool that consumes those jobs by driving the connector and pushing batches through the existing `ragclient.IngestBatch` gRPC.

It also adds a fourth loop that nothing in 3A/3B can do: **recover from worker crashes**. Without it, an `RAGIndexAttempt` row left in `IN_PROGRESS` after a worker SIGKILL freezes its source forever, because the ingest scan's `NOT EXISTS in-flight` predicate never drains.

This phase ships:
- Three periodic Asynq tasks (`rag:scan_ingest_due`, `rag:scan_perm_sync_due`, `rag:scan_prune_due`) that scan and enqueue per-source work.
- One periodic Asynq task (`rag:watchdog_stuck_attempts`) that fails out stuck attempts.
- Three per-source Asynq handlers (`rag:ingest`, `rag:perm_sync`, `rag:prune`) that drive a Connector and write to Postgres + Rust.
- One heartbeat helper used by the per-source handlers.
- A `StubConnector` test fixture so integration tests can run without 3D's GitHub connector.

After 3C, the system can index and refresh content end-to-end; 3D plugs in the first real connector.

---

## Important Notes

### Things the codebase already gives us

The original 3C sketch in `plans/onyx-port-phase3.md` was written before 3A/3B landed. The post-3A reality is friendlier than that sketch implied:

| Component | Location | Status |
|---|---|---|
| Asynq server + client + mux | `cmd/server/worker.go`, `internal/tasks/registry.go` | production, used by ~15 task types today |
| `[]*asynq.PeriodicTaskConfig` registration pattern | `internal/tasks/periodic.go` | established; new periodic tasks just append here |
| `WorkerDeps` injection | `internal/tasks/registry.go` | extend it with `RagDeps` and pass through |
| Heartbeat columns on `RAGIndexAttempt` | `internal/rag/model/index_attempt.go` (`HeartbeatCounter`, `LastHeartbeatValue`, `LastHeartbeatTime`, `LastProgressTime`, `LastBatchesCompletedCount`) | already present, no migration needed |
| Watchdog query index | `internal/rag/model/indexes.go` → `idx_rag_index_attempt_heartbeat ON (status, last_progress_time) WHERE status = 'in_progress'` | already present |
| Ingest-scan query index | `internal/rag/model/indexes.go` → `idx_rag_sources_needs_ingest ON (org_id) WHERE enabled = true AND status IN ('ACTIVE','INITIAL_INDEXING')` | already present (3A) |
| Source columns | `internal/rag/model/rag_source.go` (`Enabled`, `Status`, `RefreshFreqSeconds`, `PruneFreqSeconds`, `PermSyncFreqSeconds`, `LastSuccessfulIndexTime`, `LastTimePermSync`, `LastPruned`) | all present |
| Connector factory `Lookup(kind)` | `internal/rag/connectors/interfaces/registry.go` | present (3B) |

### The architectural simplification this enables

The original sketch proposed per-source Redis locks (`rag:lock:ingest:<id>` etc.) plus a custom heartbeat handshake. **Drop both.**

- **Per-source locking** is unnecessary because Asynq's `asynq.Unique(ttl)` option keys on `(typename, payload)` and rejects duplicate enqueues within a TTL window. The scheduler scans every 15s; setting `Unique(refresh_freq + slack)` makes "we already enqueued this" a transport-layer guarantee. See `pkg.go.dev/github.com/hibiken/asynq#Unique`.
- The **scheduler beat lock** Onyx uses (`OnyxRedisLocks.CHECK_INDEXING_BEAT_LOCK`, `backend/onyx/background/celery/tasks/docprocessing/tasks.py:819-825`) is for preventing two beat workers from racing. Asynq's `PeriodicTaskConfigProvider` coordinates multi-instance schedulers via Redis automatically (`pkg.go.dev/github.com/hibiken/asynq#PeriodicTaskManager`), so we don't write our own.
- **Heartbeats** become a plain `UPDATE rag_index_attempts SET heartbeat_counter = heartbeat_counter+1, last_heartbeat_time = NOW(), last_progress_time = NOW()` from a goroutine inside the per-source handler. The watchdog reads `last_progress_time` directly. No locks, no Redis, just a column.

This trims roughly 40% of the originally-sketched files.

### The cron task config that already exists is the model

`internal/tasks/periodic.go` returns `[]*asynq.PeriodicTaskConfig`. Adding a 60s scheduler is two lines plus a handler. We inherit the multi-instance HA story for free.

### Onyx splits scheduling and execution by queue priority — we mirror this

Onyx beat schedule (`backend/onyx/background/celery/tasks/beat_schedule.py:68-200`):
- Indexing scan: every 15s, medium priority
- Pruning scan: every 20s, medium priority
- Doc-permission sync scan: every 30s, medium priority (EE)
- External-group sync scan: every 20s, medium priority (EE)
- Monitoring/watchdog: every 5min, low priority

The actual ingest/prune/perm-sync workers run on separate queues so a slow ingest can't starve scheduling. Asynq's queue priority feature does the same job — we register the per-source handlers on a `rag:work` queue with concurrency cap, and the periodic scans on the existing default queue.

### One file ≤ 300 lines

`scripts/check-go-file-length.sh` enforces 300 lines on hand-written Go (`MAX_LINES=300`). The allowlist is grandfathered; new code shouldn't touch it. Every file in the layout below has been sized against this budget. If a file approaches the limit during implementation, split before merging — don't allowlist.

### What stays Rust-side

3C only enqueues and orchestrates. The actual chunking, embedding, and LanceDB writes already work and stay in `services/rag-engine`. The Go ingest handler builds a `ragpb.IngestBatchRequest` and calls `ragclient.IngestBatch` — same path the E2E suite already exercises (`internal/rag/e2e_test.go`). No Rust changes in 3C.

---

## Implementation Strategy

### Layer A — `internal/rag/scheduler/` (new package)

Pure scan-and-enqueue logic. No Asynq handler types, no business logic, no connector calls. Each loop has its own file so each stays under 200 lines and is independently testable.

| File | Purpose | Onyx mapping |
|---|---|---|
| `doc.go` | Package overview | — |
| `config.go` | Tick intervals, watchdog timeout, batch size — read from env with defaults | — |
| `ingest_loop.go` | `ScanIngestDue(ctx, db, asynqClient) error` — query `rag_sources` where `enabled AND status IN ('ACTIVE','INITIAL_INDEXING') AND (last_successful_index_time IS NULL OR last_successful_index_time + refresh_freq_seconds * INTERVAL '1 second' < NOW()) AND NOT EXISTS (in-progress attempt)`, enqueue `rag:ingest` with `Unique`. | `check_for_indexing` (`backend/onyx/background/celery/tasks/docprocessing/tasks.py:788-1149`); the predicate mirrors `should_index()` (`utils.py:171-300`). |
| `perm_sync_loop.go` | `ScanPermSyncDue(...)` — same shape, only sources whose `Kind` resolves to a `PermSyncConnector` via `connectors/interfaces.Lookup`, gated by `last_time_perm_sync + perm_sync_freq_seconds`. | `check_for_doc_permissions_sync` (`backend/ee/onyx/background/celery/tasks/doc_permission_syncing/tasks.py:188-288`) + `_is_external_doc_permissions_sync_due`. |
| `prune_loop.go` | `ScanPruneDue(...)` — gated by `last_pruned + prune_freq_seconds`. | `check_for_pruning` (`backend/onyx/background/celery/tasks/pruning/tasks.py:206-314`) + `_is_pruning_due` (`tasks.py:164-197`). |
| `watchdog.go` | `ScanStuckAttempts(...)` — read `idx_rag_index_attempt_heartbeat`, mark `IN_PROGRESS` rows where `last_progress_time + watchdog_timeout < NOW()` as `FAILED`. | `monitor_indexing_attempt_progress` (`backend/onyx/background/celery/tasks/docprocessing/tasks.py:294-385`); stall logic at `tasks.py:419-465`. |

Public surface from this package: four exported `Scan*` functions, one `Configs() []*asynq.PeriodicTaskConfig` helper.

### Layer B — `internal/rag/tasks/` (new package)

Asynq handlers, one file per task type. Heartbeat helper isolated.

| File | Purpose | Onyx mapping |
|---|---|---|
| `doc.go` | Package overview | — |
| `payloads.go` | JSON payload structs: `IngestPayload`, `PermSyncPayload`, `PrunePayload` — all just `RAGSourceID uuid.UUID` plus a couple of flags | — |
| `heartbeat.go` | `startHeartbeat(ctx, db, attemptID) (stop func())` — goroutine that ticks `last_heartbeat_time` and `last_progress_time` every 30s | mirrors Onyx's heartbeat loop in `docfetching_proxy_task` (`backend/onyx/background/celery/tasks/docfetching/tasks.py:312-682`) |
| `ingest.go` | `HandleIngest(ctx, t)` — load source, look up connector, open `RAGIndexAttempt`, drive `CheckpointedConnector.LoadFromCheckpoint`, batch documents, call `ragclient.IngestBatch`, save checkpoint, finalize attempt | `_kickoff_indexing_tasks` + `try_creating_docfetching_task` (`tasks.py:697-748`); actual fetch in `docfetching_task` (`tasks/docfetching/tasks.py:103-258`) |
| `perm_sync.go` | `HandlePermSync(ctx, t)` — drive `PermSyncConnector`, push ACL changes via `ragclient.UpdateACL`, no re-embed | `try_creating_permissions_sync_task` (`backend/ee/.../doc_permission_syncing/tasks.py:230-235`) |
| `prune.go` | `HandlePrune(ctx, t)` — drive `SlimConnector`, diff against `rag_documents` IDs, cascade-delete missing | `try_creating_prune_generator_task` (`backend/onyx/background/celery/tasks/pruning/tasks.py:228-235`) |
| `register.go` | `RegisterHandlers(mux *asynq.ServeMux, deps RagDeps)` — adds `rag:*` task types to the existing mux. Mounted from `internal/tasks/registry.go`. | — |
| `stub_connector_test.go` | A `StubConnector` registered into the connector factory for tests — emits N pre-canned docs, supports checkpoint, optional fixed permission set. Lets all 3C tests run without 3D's GitHub. | — |

### Layer C — Wiring

| File | Change |
|---|---|
| `internal/tasks/registry.go` | Extend `WorkerDeps` with a `Rag *rag.Deps` field. In `NewServeMux`, after existing registrations, call `ragtasks.RegisterHandlers(mux, deps.Rag)`. |
| `internal/tasks/periodic.go` | After existing periodic configs, append `scheduler.Configs()`. |
| `cmd/server/worker.go` | In `runWork`, build `rag.Deps{DB, RagClient, ConnectorFactory}` and pass to `WorkerDeps`. |

These are surgical edits — each existing file gains <20 lines.

### Layer D — Test infrastructure

| File | Purpose |
|---|---|
| `internal/rag/testhelpers/redis.go` | `ConnectTestRedis(t)` — opens a real Redis connection on the test docker-compose Redis (already running for the existing test-services-up infra), flushes the test DB index, registers `t.Cleanup`. |
| `internal/rag/testhelpers/asynq.go` | `NewTestAsynqClient(t)` + `NewTestAsynqInspector(t)` for asserting queue depth and inflight count. |

### Concurrency model summary

Three independent layers ensure at-most-one in-flight per source. Any one alone is sufficient for safety; all three give defense in depth at low cost:

1. **Asynq Unique** on enqueue — same `(typename, payload)` cannot be enqueued twice within the configured TTL. Default safety case.
2. **DB scan predicate** (`NOT EXISTS in-flight attempt`) — even if Unique TTL expires before the worker finishes, the next scan tick won't re-enqueue.
3. **Watchdog** — if a worker dies between opening the attempt and finishing it, the attempt sits in `IN_PROGRESS` until the watchdog times it out, at which point both layers above re-eligibility the source.

### Cadences (defaults; overridable via env)

| Periodic task | Tick | Onyx ref |
|---|---|---|
| `rag:scan_ingest_due` | 15s | `beat_schedule.py:68-76` |
| `rag:scan_perm_sync_due` | 30s | `beat_schedule.py:182-189` |
| `rag:scan_prune_due` | 60s | `beat_schedule.py:119-127` |
| `rag:watchdog_stuck_attempts` | 60s | `beat_schedule.py:132-139` (Onyx runs every 5min; we run more often because our watchdog is the only crash-recovery path) |

Worker-side defaults:
- `rag:ingest` queue concurrency 4
- `rag:perm_sync` queue concurrency 2
- `rag:prune` queue concurrency 2
- Watchdog timeout: 30 min stale `last_progress_time`
- Heartbeat tick: 30s

All env-overridable via `RAG_*_TICK`, `RAG_*_CONCURRENCY`, `RAG_WATCHDOG_TIMEOUT`, `RAG_HEARTBEAT_TICK`.

---

## Tests

Integration only, real Postgres + real Redis + real Asynq, fake Rust binary — same hard rules as Phase 2 (TESTING.md hard rules #1, #2, #7).

Two test files mirror the package split:
- `internal/rag/scheduler/scheduler_test.go` — scan/enqueue behavior
- `internal/rag/tasks/tasks_test.go` — handler behavior

Where a test needs a connector, register a `StubConnector` (defined in `tasks/stub_connector_test.go`) into the factory inside the test's setup. `StubConnector` is parametrized:
- `Docs []Document` — what to emit
- `Failures map[int]error` — inject per-doc failures by index
- `PermSet ExternalAccess` — what `PermSyncConnector` returns
- `SlimIDs []string` — what `SlimConnector.SlimDocs` returns

### Scheduler tests (`scheduler_test.go`)

1. **`TestIngestScan_EnqueuesWhenDue`** — source with `last_successful_index_time` older than `refresh_freq_seconds` → one `rag:ingest` enqueued. Verify via Asynq inspector queue depth.
2. **`TestIngestScan_SkipsNotDue`** — `last_successful_index_time = NOW()`, `refresh_freq_seconds = 3600` → no enqueue.
3. **`TestIngestScan_SkipsDisabled`** — `enabled = false` → no enqueue regardless of timing.
4. **`TestIngestScan_SkipsPaused`** — `status = 'PAUSED'` → no enqueue.
5. **`TestIngestScan_SkipsInProgress`** — open `RAGIndexAttempt` with `status = 'IN_PROGRESS'` → no enqueue. Uses the partial-index `NOT EXISTS` predicate.
6. **`TestIngestScan_NullRefreshFreqSkips`** — `refresh_freq_seconds IS NULL` (on-demand only) → no enqueue. Mirrors Onyx's `should_index()` returning False when `connector.refresh_freq is None`.
7. **`TestPermSyncScan_OnlyForPermSyncCapableKinds`** — register two stub kinds (`stub-perm` implements `PermSyncConnector`, `stub-noperm` doesn't); only the first gets enqueued.
8. **`TestPruneScan_GatedByLastPruned`** — analogous to ingest scan, on `last_pruned + prune_freq_seconds`.
9. **`TestWatchdog_FailsStaleAttempts`** — insert `IN_PROGRESS` row with `last_progress_time = NOW() - 31min`; after watchdog, status = `FAILED`, `error_msg` set.
10. **`TestWatchdog_LeavesFreshAttemptsAlone`** — `last_progress_time = NOW() - 1min`; status unchanged.
11. **`TestUniqueEnqueue_DedupesRepeatedScans`** — fire scan tick five times in 1s with same source state; queue depth = 1.

### Handler tests (`tasks_test.go`)

12. **`TestIngest_HappyPath_StubConnector`** — `StubConnector` emits 5 docs; handler runs; `rag:ingest` task succeeds; `RAGIndexAttempt` row goes `IN_PROGRESS → COMPLETED`; `last_successful_index_time` advanced; checkpoint persisted; `ragclient.IngestBatch` called once with the 5 docs.
13. **`TestIngest_HeartbeatTicksDuringWork`** — `StubConnector` configured with a 2s delay between docs; assert `last_heartbeat_time` and `heartbeat_counter` advance during the run, not just at the end.
14. **`TestIngest_PerDocFailureDoesNotAbortBatch`** — `StubConnector` injects failures at index 1, 3; final attempt status = `COMPLETED_WITH_ERRORS`; `RAGIndexAttemptError` rows present for the two failures.
15. **`TestIngest_FatalConnectorErrorMarksFailed`** — `StubConnector.LoadFromCheckpoint` returns a fatal error before emitting anything; attempt = `FAILED`, source `Status` unchanged (re-eligible next tick).
16. **`TestIngest_CheckpointResumesAfterRestart`** — first run emits 3 of 10 docs and crashes (cancel ctx); second run starts from checkpoint and emits the remaining 7. Verify via doc count.
17. **`TestPermSync_PushesAclWithoutReembed`** — pre-seed Lance with 3 docs (via test harness); run perm-sync; assert `ragclient.UpdateACL` called; `vector` column on Lance row unchanged (read directly via the test helper).
18. **`TestPrune_DeletesDocsMissingUpstream`** — pre-seed 10 docs; `StubConnector.SlimDocs` returns 7; after prune, `rag_documents` has 7 rows; junction table cleaned; Lance rows for the 3 dropped IDs deleted.
19. **`TestWatchdogIntegration_PicksUpDeadWorker`** — start an ingest, kill the goroutine mid-flight (cancel ctx without finalizing), wait for watchdog tick, assert attempt failed and source eligible again.

### Definition of done

- All 19 tests pass on real Postgres + Redis (the existing `make test-services-up` stack)
- Existing Asynq monitor UI surfaces the four new periodic tasks and three new task types
- `go test -race -count=1 ./internal/rag/scheduler/... ./internal/rag/tasks/...` clean under the 5-min CI shard budget (these get added to `test-rag-e2e.yml`'s shard list)
- `scripts/check-go-file-length.sh` passes — every new file under 300 lines, no allowlist additions

---

## Onyx ↔ Hiveloop reference index

| Onyx | Hiveloop (after 3C) |
|---|---|
| `backend/onyx/background/celery/tasks/beat_schedule.py:68-200` | `internal/rag/scheduler/Configs()` |
| `backend/onyx/background/celery/tasks/docprocessing/tasks.py:788-1149` (`check_for_indexing`) | `internal/rag/scheduler/ingest_loop.go` `ScanIngestDue` |
| `backend/onyx/background/celery/tasks/docprocessing/utils.py:171-300` (`should_index`) | predicate inside `ScanIngestDue` query |
| `backend/onyx/background/celery/tasks/pruning/tasks.py:206-314` (`check_for_pruning`) | `internal/rag/scheduler/prune_loop.go` `ScanPruneDue` |
| `backend/ee/onyx/background/celery/tasks/doc_permission_syncing/tasks.py:188-288` | `internal/rag/scheduler/perm_sync_loop.go` `ScanPermSyncDue` |
| `backend/onyx/background/celery/tasks/docprocessing/tasks.py:294-385` (`monitor_indexing_attempt_progress`) | `internal/rag/scheduler/watchdog.go` `ScanStuckAttempts` |
| `backend/onyx/background/celery/tasks/docfetching/tasks.py:103-258` (`docfetching_task`) | `internal/rag/tasks/ingest.go` `HandleIngest` |
| `backend/onyx/background/celery/tasks/docfetching/tasks.py:312-682` (heartbeat loop in proxy task) | `internal/rag/tasks/heartbeat.go` `startHeartbeat` |
| `backend/onyx/db/models.py:2189-2342` (`IndexAttempt`) | `internal/rag/model/index_attempt.go` `RAGIndexAttempt` (already exists, no changes) |
| `backend/onyx/db/models.py:1864-1927` + `:723-823` (Connector + CCPair scheduling fields) | `internal/rag/model/rag_source.go` (already exists, no changes) |
| Onyx `OnyxRedisLocks.CHECK_INDEXING_BEAT_LOCK` | not ported — Asynq `PeriodicTaskManager` covers multi-instance scheduling |
| Onyx `OnyxRedisLocks.CONNECTOR_INDEXING` per-CCPair | not ported — Asynq `Unique` + DB `NOT EXISTS in-flight` predicate cover it |
