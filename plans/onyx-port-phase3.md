# Onyx → Hiveloop: RAG Port Plan (Phase 3 — `RAGSource` + Three-Loop Scheduler + GitHub Connector)

**Author:** architecture session, April 2026
**Scope:** Phase 3 only — the real production scheduler + the first real connector.
**Prior phases:** `plans/onyx-port.md` (Phase 0+1), `plans/onyx-port-phase2.md` (Phase 2 Rust rag-engine).

Phase 2 delivered a high-performance RAG pipeline that can accept pushed documents via `IngestBatch`. Phase 3 delivers the **orchestration layer** that keeps documents in sync with their real sources (GitHub, Notion, Slack, Confluence, etc.), plus the **first real connector** (GitHub) wired end-to-end. After Phase 3, Hiveloop goes from "a RAG engine you can push docs into" to "a RAG product that automatically ingests your company's data."

---

## Locked decisions (do not re-litigate)

| Concern | Decision |
|---|---|
| **Top-level abstraction** | A new `RAGSource` table — first-class concept, replaces the current `RAGConnectionConfig` + direct `InConnection` coupling. A RAG source has a `kind`: `INTEGRATION` (referencing an `InConnection`), `WEBSITE` (root URL + scrape config), or `FILE_UPLOAD` (future). |
| **Embedder scope** | One embedder per org. All sources in an org write to the **same LanceDB dataset**. A search query retrieves across all of that org's sources simultaneously — user sees results from GitHub + Notion + Slack in a single ranked list. |
| **Integration allowlist** | Not every `InIntegration` is RAG-ingestable. Add `supports_rag_source BOOLEAN` to `in_integrations`. Admin UI's "Add RAG source" picker reads this flag. |
| **First connector** | GitHub. Implements all three connector interfaces (`CheckpointedConnector`, `PermSyncConnector`, `SlimConnector`). Used to drive the scheduler end-to-end and establish the connector template. |
| **Migration stance** | No production data yet. Phase 3 drops obsolete columns cleanly (`in_connection_id` in RAG tables, `rag_connection_configs` whole table) rather than carrying deprecated shapes. |
| **Out of scope** | Website + file-upload connectors (future phases). Answer/chat LLM layer (still user's separate product). |

---

## Testing philosophy — unchanged from Phase 1/2

Restated concisely; full text in `internal/rag/doc/TESTING.md`:

- Mocking **only** for the embedder and reranker (trait + fake impl in Rust; the in-process Rust binary used in Go tests runs with `LLM_PROVIDER=fake RERANKER_KIND=fake`).
- Everything else hits real infrastructure: real Postgres, real MinIO/R2, real LanceDB, real Asynq/Redis, real gRPC transport, **real Nango proxy** (for connector tests — with a test-tenant Nango account).
- Every test verifies real business behavior — no field-presence assertions, no framework-exercise tests, no sqlmock.
- 100% coverage on branchful code.
- Every integration test has `t.Cleanup` with explicit DELETE by `org_id`.

New for Phase 3: **Nango test tenancy.** GitHub connector tests need a working Nango connection. Maintain a dedicated test GitHub account + Nango test tenant; CI has credentials. Tests that exercise the real GitHub API are marked with a build tag `nango_integration` and skipped when the creds are absent (the sanctioned skip pattern already used for `SILICONFLOW_API_KEY`).

---

## Architecture — from source config to indexed chunk

```
┌─────────────────┐
│ RAGSource (new) │── rag_sources table: kind, config, status, org_id
│  kind: INTEGRATION  ←── references InConnection (existing Nango-backed)
│  kind: WEBSITE     ←── root_url + scrape config (Phase 4+)
│  kind: FILE_UPLOAD ←── future
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Scheduler (Phase 3C) │
│  Asynq periodic task scans rag_sources, enqueues per-source:
│    rag:ingest     (every RefreshFreqSeconds)
│    rag:perm_sync  (every PermSyncFreqSeconds)
│    rag:prune      (every PruneFreqSeconds)
│    rag:watchdog   (every 60s — scans stuck attempts)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Connector (Phase 3B interfaces, 3D GitHub impl) │
│  Connector                  — base trait
│  CheckpointedConnector[T]   — resumable ingest with checkpoint
│  PermSyncConnector          — periodic ACL refresh
│  SlimConnector              — list-all for pruning
└────────┬────────┘
         │ Document{ sections, acl, is_public, metadata, ... }
         ▼
┌─────────────────┐
│ IngestBatch gRPC (Phase 2F — already built) │
│  → Rust rag-engine: chunk → embed → LanceDB
└─────────────────┘
```

The Go scheduler + connector layer produces `Document{}` batches and pushes them through the existing `IngestBatch` gRPC to Rust. No Rust changes in Phase 3.

---

## Tranche 3A — Data model: `RAGSource` + migration

**Onyx references:**
- `ConnectorCredentialPair`: `backend/onyx/db/models.py:723-837` (conceptual analog — ours is a superset that includes non-integration kinds)
- `Connector.connector_specific_config`: `backend/onyx/db/models.py:1872` (the JSONB config pattern we reuse for `RAGSource.config`)

**Files to create:**
- `internal/rag/model/rag_source.go`
- Migration function in `internal/rag/model/automigrate_3a.go`

**Files to modify:**
- `internal/rag/model/sync_state.go` — swap `InConnectionID` PK → `RAGSourceID` PK
- `internal/rag/model/index_attempt.go` — swap `InConnectionID` → `RAGSourceID`
- `internal/rag/model/document_by_connection.go` — rename to `document_by_source.go`, swap FK
- `internal/rag/model/hierarchy_node_by_connection.go` — same
- `internal/rag/model/external_user_group.go` — swap FK
- `internal/rag/model/user_external_user_group.go` — swap composite-PK column
- `internal/rag/model/public_external_user_group.go` — swap composite-PK column
- `internal/rag/model/external_identity.go` — swap FK
- **Drop**: `internal/rag/model/connection_config.go` (`RAGConnectionConfig` — subsumed by `RAGSource.config`)
- Modify `internal/model/in_integration.go` — add `SupportsRAGSource BOOLEAN`

### `RAGSource` schema

| Field | Go type | Notes |
|---|---|---|
| `ID` | `uuid.UUID` PK | |
| `OrgID` | `uuid.UUID` FK → `orgs.id` ON DELETE CASCADE | |
| `Kind` | `RAGSourceKind` enum | `INTEGRATION` / `WEBSITE` / `FILE_UPLOAD` |
| `Name` | `string` | User-facing label, e.g. "Engineering GitHub" |
| `Status` | `RAGSourceStatus` enum | `DISCONNECTED` / `INITIAL_INDEXING` / `ACTIVE` / `PAUSED` / `ERROR` / `DELETING` |
| `Enabled` | `bool` default true | admin toggle; disabled sources are skipped by scheduler |
| `Config` | `model.JSON` (JSONB) | Kind-specific: for `INTEGRATION` holds `{}`; for `WEBSITE` holds `{root_url, scrape_depth, include_patterns, exclude_patterns}` |
| `InConnectionID` | `*uuid.UUID` FK → `in_connections.id` | Non-null iff `Kind = INTEGRATION` |
| `AccessType` | `AccessType` enum | `PUBLIC` / `PRIVATE` / `SYNC` — inherited from Phase 1C sync-state shape |
| `LastSuccessfulIndexTime` | `*time.Time` | |
| `LastTimePermSync` | `*time.Time` | |
| `LastPruned` | `*time.Time` indexed | |
| `RefreshFreqSeconds` | `*int` | Inherits `Connector.refresh_freq` (Onyx `models.py:1889`); validate ≥60 |
| `PruneFreqSeconds` | `*int` | Inherits `Connector.prune_freq` (Onyx `models.py:1890`); validate ≥300 |
| `PermSyncFreqSeconds` | `*int` | Default 21600 (6h) |
| `TotalDocsIndexed` | `int` default 0 | |
| `InRepeatedErrorState` | `bool` default false | Port of CCPair's `in_repeated_error_state` at `models.py:744` |
| `DeletionFailureMessage` | `*string` | |
| `CreatorID` | `*uuid.UUID` FK → `users.id` | |
| `CreatedAt`, `UpdatedAt` | `time.Time` | |

**Kind-specific validity**:
- If `Kind=INTEGRATION`: `InConnectionID` must be non-null, `Config.integration.X` may hold connector-specific config (e.g., GitHub repo allowlist).
- If `Kind=WEBSITE`: `InConnectionID` must be null; `Config.website.root_url` required.
- If `Kind=FILE_UPLOAD`: `InConnectionID` null; `Config.file_upload.*` TBD (future).

**Constraints:**
- Unique `(in_connection_id)` where kind=INTEGRATION — one RAGSource per InConnection
- CHECK constraint: `(kind='INTEGRATION' AND in_connection_id IS NOT NULL) OR (kind<>'INTEGRATION' AND in_connection_id IS NULL)`
- `idx_rag_sources_org_status` on `(org_id, status)`
- `idx_rag_sources_needs_ingest` partial on `(org_id)` WHERE `enabled=true AND status IN ('ACTIVE','INITIAL_INDEXING')`
- `idx_rag_sources_last_pruned` on `(last_pruned)`

### Migration steps (destructive — no production data)

1. Create `rag_sources` table.
2. Add `SupportsRAGSource BOOLEAN DEFAULT false` to `in_integrations`; backfill `true` for GitHub, Notion, Slack, Confluence, Jira, Linear, Google Drive.
3. For every table currently keyed on `in_connection_id`:
   - Add new `rag_source_id` column (nullable initially for schema add)
   - Backfill: for each distinct `in_connection_id`, create a `RAGSource` row with `kind=INTEGRATION` and set `rag_source_id` accordingly
   - Drop old `in_connection_id` column
   - Rename indexes to match
4. Drop `rag_connection_configs` table entirely (fields migrated into `RAGSource.config` + `RAGSource.*FreqSeconds`).

### Tests (`internal/rag/model/rag_source_test.go`)

All integration tests use real Postgres via `testhelpers.ConnectTestDB(t)`.

1. `TestRAGSource_OrgCascadeDelete` — delete org → source rows gone.
2. `TestRAGSource_IntegrationKindRequiresInConnection` — insert `kind=INTEGRATION` with null `in_connection_id` → CHECK violation.
3. `TestRAGSource_NonIntegrationKindRejectsInConnection` — insert `kind=WEBSITE` with non-null `in_connection_id` → CHECK violation.
4. `TestRAGSource_UniquePerInConnection` — insert two `kind=INTEGRATION` sources with same `in_connection_id` → unique violation.
5. `TestRAGSource_NeedsIngestPartialIndex` — insert sources in various statuses; `EXPLAIN` the scheduler query (`WHERE enabled AND status IN ('ACTIVE','INITIAL_INDEXING')`), assert partial index is used.
6. `TestRAGSource_ValidateRefreshFreq` / `ValidatePruneFreq` — pure validation functions; < minimum → error.
7. `TestRAGSourceStatus_IsActive` — table-driven enum method.
8. `TestRAGSourceKind_IsValid` — table-driven.
9. `TestAutoMigrate3A_DropsOldColumns` — after migration, `information_schema.columns WHERE table_name='rag_sync_states' AND column_name='in_connection_id'` returns zero rows.
10. `TestAutoMigrate3A_RenamesDocByConnection` — `document_by_source` table exists, `document_by_connection` does not.

**Definition of done:**
- All 10 tests green
- `go test -race -p 1` across `internal/rag/...` all packages still green (Phase 1 tests adapted to new column names)
- No dead code: `RAGConnectionConfig` struct + tests fully removed
- Commit message cites the Onyx symbols retired

**Estimated effort:** 2–3 days

---

## Tranche 3B — Connector framework (Go trait hierarchy)

**Onyx references:**
- `backend/onyx/connectors/interfaces.py` — full file. We port the trait design faithfully.
- `GenerateDocumentsOutput` / `GenerateSlimDocumentOutput` / `CheckpointOutput` — the output generator shapes
- `ConnectorFailure` / `DocumentFailure` / `EntityFailure`: `backend/onyx/connectors/models.py`
- `Document` / `Section` / `ExternalAccess`: same file

**Files to create:**
- `internal/rag/connectors/interfaces/connector.go` — base trait + common types
- `internal/rag/connectors/interfaces/document.go` — `Document`, `Section`, `SlimDocument`, `ExternalAccess`, `DocExternalAccess`
- `internal/rag/connectors/interfaces/checkpoint.go` — `Checkpoint` interface + JSON-serializable constraint
- `internal/rag/connectors/interfaces/failure.go` — `ConnectorFailure`, `DocumentFailure`, `EntityFailure`
- `internal/rag/connectors/interfaces/registry.go` — `Registry` that maps `InIntegration.Provider` → factory function

### Trait hierarchy

```go
// All connectors implement this.
type Connector interface {
    Kind() string                     // e.g. "github", "notion" — matches InIntegration.Provider
    ValidateConfig(ctx, *RAGSource) error
}

// Resumable ingest with checkpoint. Yields documents in batches.
type CheckpointedConnector[T Checkpoint] interface {
    Connector
    // Output is consumed by the scheduler; each item is either a Document or a
    // ConnectorFailure describing a per-doc failure that shouldn't kill the run.
    LoadFromCheckpoint(ctx context.Context, src *RAGSource, cp T, start, end time.Time) (<-chan DocumentOrFailure, error)
    DummyCheckpoint() T
    UnmarshalCheckpoint(raw json.RawMessage) (T, error)
}

// Periodic metadata-only ACL refresh. Rust-side UpdateACL path.
type PermSyncConnector interface {
    Connector
    SyncDocPermissions(ctx context.Context, src *RAGSource) (<-chan DocExternalAccess, error)
    SyncExternalGroups(ctx context.Context, src *RAGSource) (<-chan ExternalGroup, error)
}

// List all current IDs for pruning.
type SlimConnector interface {
    Connector
    ListAllSlim(ctx context.Context, src *RAGSource) (<-chan SlimDocOrFailure, error)
}
```

### Concrete types

```go
type Document struct {
    DocID            string
    SemanticID       string
    Link             string
    Sections         []Section
    ACL              []string              // opaque pre-prefixed tokens
    IsPublic         bool
    DocUpdatedAt     *time.Time
    Metadata         map[string]string
    PrimaryOwners    []string
    SecondaryOwners  []string
}

type Section struct {
    Text  string
    Link  string
    Title string
}

type SlimDocument struct {
    DocID          string
    ExternalAccess *ExternalAccess        // optional; set if perm_sync piggybacks on slim listing
}

type Checkpoint interface {
    // Marker interface — any JSON-serializable struct. Per-connector.
}

type DocumentOrFailure struct {
    Doc     *Document
    Failure *ConnectorFailure             // mutually exclusive
}

type ConnectorFailure struct {
    FailedDoc *DocumentFailure
    FailedEntity *EntityFailure
    FailureMessage string
}
```

### Registry

```go
type Factory func(*RAGSource, *nango.Client) (Connector, error)

var defaultRegistry = map[string]Factory{}

func Register(kind string, f Factory)
func Lookup(kind string) (Factory, error)
```

GitHub (Tranche 3D) calls `Register("github", NewGitHubConnector)` in its `init()`.

### Tests (`internal/rag/connectors/interfaces/interfaces_test.go`)

Pure-logic tests (no DB, no network):

1. `TestDocumentOrFailure_MutuallyExclusive` — constructors reject setting both fields.
2. `TestConnectorFailure_PropagatesOriginalError` — verify error chain.
3. `TestRegistry_RegisterAndLookup` — table-driven.
4. `TestRegistry_DuplicateKindPanics` — registering twice under same kind is a programming error.
5. `TestSectionEmpty_ShouldBeSkippableByChunker` — documents the Section.Text="" contract.
6. `TestDocument_RoundtripJSON` — Document serializes + deserializes identical (important for the gRPC proto boundary).

**Definition of done:**
- Interfaces compile, no stub returns
- Registry pattern exercised by a stub test connector in the test file (not committed as a real connector — just proves the registration shape)

**Estimated effort:** 2–3 days

---

## Tranche 3C — Three-loop scheduler (Asynq tasks + watchdog)

**Onyx references:**
- `backend/onyx/background/celery/tasks/docfetching/tasks.py` — the ingest loop
- `backend/onyx/background/celery/tasks/docprocessing/tasks.py` — chunk/embed/write (ours is Rust-side)
- `backend/onyx/background/celery/tasks/pruning/` — prune loop
- `backend/ee/onyx/background/celery/tasks/doc_permission_syncing/` — perm-sync loop
- `backend/onyx/background/celery/tasks/beat_schedule.py` — scheduling pattern
- `backend/onyx/redis/redis_connector.py` — per-connector Redis locking

**Files to create:**
- `internal/rag/tasks/scheduler.go` — periodic scan + enqueue logic
- `internal/rag/tasks/ingest_task.go` — consumes connector output, pushes batches via ragclient
- `internal/rag/tasks/perm_sync_task.go` — runs PermSyncConnector, calls Rust `UpdateACL`
- `internal/rag/tasks/prune_task.go` — SlimConnector diff + cascade delete
- `internal/rag/tasks/watchdog_task.go` — scans stuck `RAGIndexAttempt`s, marks failed
- `internal/rag/tasks/lock.go` — per-source Redis lock with heartbeat
- `internal/rag/tasks/register.go` — wire all handlers into the existing asynq ServeMux (`internal/tasks/registry.go`)

### Task types

| Type | Payload | Cadence | Lock |
|---|---|---|---|
| `rag:ingest` | `{rag_source_id, from_beginning}` | per-source `RefreshFreqSeconds` | per-source (`rag:lock:ingest:<id>`) |
| `rag:perm_sync` | `{rag_source_id}` | per-source `PermSyncFreqSeconds` | per-source (`rag:lock:perm_sync:<id>`) |
| `rag:prune` | `{rag_source_id}` | per-source `PruneFreqSeconds` | per-source (`rag:lock:prune:<id>`) |
| `rag:watchdog` | `{}` | every 60s | single global (`rag:lock:watchdog`) |

### Scheduler loop

Single cron job `rag:scheduler_tick` runs every 30s. It:
1. `SELECT id, refresh_freq_seconds, last_successful_index_time, ... FROM rag_sources WHERE enabled AND status IN ('ACTIVE','INITIAL_INDEXING')`
2. For each source, compute: is ingest due? is perm_sync due? is prune due?
3. Enqueue the appropriate task via `asynq.Enqueue(task, asynq.Unique(...))` — Asynq's `Unique` prevents duplicate enqueue within the TTL if the last enqueue hasn't been picked up yet.
4. Attempts create a `RAGIndexAttempt` row with `status=IN_PROGRESS` before work starts.

This matches Onyx's `check_for_indexing` beat task pattern (`backend/onyx/background/celery/tasks/indexing/tasks.py`).

### Ingest task worker

```go
func handleRAGIngest(ctx context.Context, t *asynq.Task) error {
    var p IngestPayload
    json.Unmarshal(t.Payload(), &p)

    // 1. Acquire per-source Redis lock; release on exit or watchdog timeout.
    lock, err := acquireLock(ctx, "rag:lock:ingest:"+p.RAGSourceID)
    if err != nil { return err }
    defer lock.Release()

    // 2. Load source + connector
    src := LoadRAGSource(db, p.RAGSourceID)
    connector := connectors.Lookup(src.Kind).(CheckpointedConnector)

    // 3. Start attempt + heartbeat goroutine
    attempt := CreateIndexAttempt(db, src.ID, FROM_BEGINNING=p.FromBeginning)
    stopHeartbeat := startHeartbeat(db, attempt.ID)
    defer stopHeartbeat()

    // 4. Load last checkpoint
    cp, _ := loadCheckpoint(attempt.CheckpointPointer)

    // 5. Drive the connector
    ch, err := connector.LoadFromCheckpoint(ctx, src, cp, pollStart, pollEnd)
    batch := []*Document{}
    for item := range ch {
        if item.Failure != nil {
            recordAttemptError(db, attempt.ID, item.Failure)
            continue
        }
        batch = append(batch, item.Doc)
        if len(batch) >= batchSize {
            flushBatch(ctx, ragclient, src, batch, attempt)
            batch = batch[:0]
        }
    }
    if len(batch) > 0 {
        flushBatch(ctx, ragclient, src, batch, attempt)
    }

    // 6. Mark success
    finalizeAttempt(db, attempt.ID, Success)
    return nil
}

func flushBatch(ctx, ragclient, src, batch, attempt) {
    req := &ragpb.IngestBatchRequest{
        DatasetName:       datasetNameForOrg(src.OrgID),   // org-wide
        OrgId:             src.OrgID.String(),
        Mode:              ragpb.IngestionMode_INGESTION_MODE_UPSERT,
        IdempotencyKey:    fmt.Sprintf("attempt-%d-batch-%d", attempt.ID, batchNum),
        DeclaredVectorDim: currentEmbedderDim(src.OrgID),
        Documents:         convertToProtoDocs(batch),
    }
    resp, err := ragclient.IngestBatch(ctx, req)
    // record per-doc outcomes into RAGIndexAttemptError as needed
}
```

### Watchdog

```go
func handleWatchdogTick(ctx context.Context, t *asynq.Task) error {
    // Find IN_PROGRESS attempts whose heartbeat hasn't updated in >5 min
    stale := db.Find(&RAGIndexAttempt{}, "status=? AND last_progress_time < ?",
        IndexingStatusInProgress, time.Now().Add(-5*time.Minute))

    for _, att := range stale {
        att.Status = IndexingStatusFailed
        att.ErrorMsg = fmt.Sprintf("watchdog: no heartbeat for %s", time.Since(att.LastProgressTime))
        db.Save(&att)
        // Increment InRepeatedErrorState on the source if this is the Nth failure in a row
    }
    return nil
}
```

### Tests (`internal/rag/tasks/*_test.go`)

All integration against real Postgres + real Redis. Use `testhelpers.ConnectTestDB` + fresh asynq.Client.

1. `TestScheduler_EnqueuesIngestWhenDue` — source with `LastSuccessfulIndexTime` older than `RefreshFreqSeconds` → scheduler enqueues; not older → skip.
2. `TestScheduler_SkipsPausedSources` — `status=PAUSED` → no enqueue regardless of timing.
3. `TestScheduler_SkipsDisabledSources` — `enabled=false` → no enqueue.
4. `TestIngest_AcquiresAndReleasesLock` — two workers racing on same source; second blocks until first finishes.
5. `TestIngest_CheckpointResumesAfterRestart` — driver consumes N docs, crashes mid-stream; next attempt resumes at checkpoint.
6. `TestIngest_HeartbeatKeepsLockAlive` — long-running attempt; lock doesn't expire during work.
7. `TestIngest_PerDocFailureDoesNotAbortBatch` — connector emits 5 docs + 2 failures; 5 succeed, 2 logged to `RAGIndexAttemptError`.
8. `TestIngest_IdempotencyKeysPreventDoubleWrite` — same attempt replayed → server dedupes, no double chunks.
9. `TestPermSync_UpdatesAclWithoutReembed` — doc has 2 chunks; perm_sync updates ACL; Rust-side `UpdateACL` called once; vector column untouched (check via direct Lance read).
10. `TestPrune_DiffsAndCascadeDeletes` — 10 docs indexed; slim listing returns 7; 3 get deleted from Postgres + Lance; junction table cleaned.
11. `TestWatchdog_MarksStaleAttemptsFailed` — attempt with old `LastProgressTime` → watchdog marks failed.
12. `TestWatchdog_DoesNotTouchFreshAttempts` — recent heartbeat → untouched.
13. `TestScheduler_UniqueEnqueuePreventsThundering` — 5 rapid scheduler ticks → only 1 ingest enqueued per source.

For tests that need a connector, register a **StubConnector** in the test setup that emits pre-canned docs from memory. No GitHub API dependency for scheduler tests.

**Definition of done:**
- All 13 tests green on real Postgres + real Redis
- `asynq-monitor` UI works (existing Hiveloop asynq monitor surface, new task types show up)
- Heartbeat + lock coordination proven under concurrent test

**Estimated effort:** 2 weeks

---

## Tranche 3D — GitHub connector (the first real one)

**Onyx references (read these first):**
- `backend/onyx/connectors/github/connector.py` — full GitHub connector (PRs + issues, checkpointed)
- `backend/onyx/connectors/github/utils.py` — auth, rate-limit helpers
- `backend/ee/onyx/external_permissions/github/` — perm sync
- `backend/ee/onyx/external_permissions/github/doc_sync.py` — ACL refresh
- `backend/ee/onyx/external_permissions/github/group_sync.py` — org team membership → ExternalUserGroup

**Files to create:**
- `internal/rag/connectors/github/connector.go` — `GitHubConnector` struct, factory, registration
- `internal/rag/connectors/github/checkpoint.go` — `GitHubCheckpoint` struct (repo iteration state, stage, cursor)
- `internal/rag/connectors/github/ingest.go` — fetch PRs, issues, discussions
- `internal/rag/connectors/github/perm_sync.go` — refresh ACLs per doc; sync org teams
- `internal/rag/connectors/github/slim.go` — list all current IDs for pruning
- `internal/rag/connectors/github/acl.go` — map repo + teams → `ExternalAccess`
- `internal/rag/connectors/github/rate_limit.go` — backoff on 403/429
- `internal/rag/connectors/github/types.go` — GitHub API response shapes we care about
- `internal/rag/connectors/github/connector_test.go`
- `internal/rag/connectors/github/perm_sync_test.go`

### What gets indexed

- **Pull requests** — title, body, every comment (user + body + created_at), base + head branch, state (open/merged/closed)
- **Issues** — title, body, every comment, labels, state
- **Discussions** — title, body, answers

We **do not** index code files, wikis, or releases in Phase 3D. Those are future tranches if product demand exists.

### Checkpoint shape (port of `GithubConnectorCheckpoint` from Onyx `connector.py`)

```go
type GitHubCheckpoint struct {
    Stage            Stage                    // PRS | ISSUES | DISCUSSIONS | DONE
    CachedRepoIDs    []int64                  // remaining repos to process
    CurrentRepo      *SerializedRepo
    CurrentCursor    *string                  // GitHub cursor-based pagination
    NumRetrieved     int
    HasMore          bool
}

type Stage string
const (
    StagePRs         Stage = "prs"
    StageIssues      Stage = "issues"
    StageDiscussions Stage = "discussions"
    StageDone        Stage = "done"
)
```

Checkpoint JSON-serialized and stored on `RAGIndexAttempt.checkpoint_pointer` (a key into the FileStore — R2 in prod, disk in dev).

### Ingest flow

```go
func (c *GitHubConnector) LoadFromCheckpoint(ctx, src, cp, start, end) (<-chan DocumentOrFailure, error) {
    out := make(chan DocumentOrFailure)
    go func() {
        defer close(out)

        // 1. If checkpoint empty, list all repos (respects include/exclude from src.Config)
        if cp.CachedRepoIDs == nil {
            cp.CachedRepoIDs = c.listRepos(ctx, src)
        }

        // 2. For each repo, for each stage, iterate via cursor
        for len(cp.CachedRepoIDs) > 0 {
            repo := popRepo(&cp)
            for _, stage := range []Stage{StagePRs, StageIssues, StageDiscussions} {
                for {
                    batch, nextCursor, err := c.fetchStage(ctx, repo, stage, cp.CurrentCursor)
                    if err != nil { ... }  // record failure, retry or abort
                    for _, item := range batch {
                        if item.UpdatedAt.Before(start) { goto nextStage }  // past the window
                        if item.UpdatedAt.After(end) { continue }           // future of the window
                        out <- DocumentOrFailure{Doc: c.buildDoc(item)}
                    }
                    if nextCursor == nil { break }
                    cp.CurrentCursor = nextCursor
                }
                nextStage:
            }
        }
    }()
    return out, nil
}
```

### ACL extraction (the security-critical piece)

```go
func (c *GitHubConnector) aclForRepo(ctx, repo) (*ExternalAccess, error) {
    // Public repo → is_public=true, no user/group scoping
    if !repo.Private {
        return &ExternalAccess{IsPublic: true}, nil
    }

    // Private repo → fetch collaborators + teams with access
    collabEmails := c.fetchCollaboratorEmails(repo)
    teamIDs := c.fetchTeamsWithAccess(repo)

    // Prefix teams as external groups per Onyx convention: external_group:github_<org>_<team_slug>
    groups := []string{}
    for _, t := range teamIDs {
        groups = append(groups, acl.BuildExtGroupName(
            fmt.Sprintf("github_%s_%s", t.Org, t.Slug), DocumentSourceGithub))
    }

    return &ExternalAccess{
        ExternalUserEmails:   collabEmails,
        ExternalUserGroupIDs: groups,
        IsPublic:             false,
    }, nil
}
```

### Nango integration

All GitHub API calls go through `nango.Client.ProxyRequest(...)`. Never a direct `github.com/google/go-github` call (matches the rule from Phase 1/2: Nango is the only way to talk to providers). The Nango connection ID lives on `InConnection.NangoConnectionID`.

### Tests

**Unit-ish (no network, test with recorded fixtures):**
1. `TestGitHubCheckpoint_RoundtripsJSON` — serialize + deserialize preserves all fields.
2. `TestACLForPublicRepo_IsPublic` — `repo.Private=false` → `ExternalAccess.IsPublic=true`, no emails/groups.
3. `TestACLForPrivateRepo_MapsCollabsAndTeams` — fixture repo w/ 3 collaborators + 2 teams → prefixed correctly.
4. `TestBuildDoc_PreservesCommentChain` — PR + 3 comments → single `Document` with sections for body + each comment.
5. `TestRateLimitBackoff_RespectsRetryAfter` — simulated 429 response with `Retry-After: 5` → sleeps 5s before retry.

**Integration (behind `nango_integration` build tag; requires test GitHub account + Nango creds):**
6. `TestGitHubIngest_FetchesRealPRs` — against a dedicated test repo with known PRs; asserts document count + content.
7. `TestGitHubIngest_CheckpointResumes` — start ingest, cancel mid-repo, resume from checkpoint; no duplicates, no missed docs.
8. `TestGitHubPermSync_DetectsCollabChanges` — add/remove collab on test repo; perm_sync produces `DocExternalAccess` with new ACL.
9. `TestGitHubSlim_ListsAllCurrentPRs` — slim listing against test repo matches `gh api` output.
10. `TestGitHubIngest_RespectsIncludeExcludePatterns` — source config includes/excludes specific repos; verified.

**Definition of done:**
- Unit tests green with fixtures
- Integration tests green against the test Nango/GitHub setup in CI
- GitHub connector registered in the registry under kind `"github"`
- Scheduler (Tranche 3C) can drive it end-to-end to ingest, perm-sync, and prune

**Estimated effort:** 2 weeks

---

## Tranche 3E — Go API endpoints

**Files to create:**
- `internal/handler/rag_sources.go` — CRUD + sync triggers
- `internal/handler/rag_sources_test.go`

### Endpoints

All behind the existing Hiveloop `RequireAuth` + `ResolveOrg` middleware, plus new `rag:source:*` scopes for API keys.

```
POST   /v1/rag/sources                           Create (kind-dispatched payload)
GET    /v1/rag/sources                           List org's sources
GET    /v1/rag/sources/:id                       Detail (inc. last 5 attempts)
PATCH  /v1/rag/sources/:id                       Update name, pause/resume, reconfigure
DELETE /v1/rag/sources/:id                       Delete → cascades to docs + chunks

POST   /v1/rag/sources/:id/sync                  Enqueue manual ingest
POST   /v1/rag/sources/:id/prune                 Enqueue manual prune
POST   /v1/rag/sources/:id/perm-sync             Enqueue manual perm_sync
GET    /v1/rag/sources/:id/attempts              Recent index attempts
GET    /v1/rag/sources/:id/attempts/:attempt_id  Detail inc. per-doc errors

GET    /v1/rag/integrations                      Lists InIntegrations where supports_rag_source=true
                                                  (used by admin UI's "Add RAG source" picker)
```

### Create payload validation

```go
type CreateSourceRequest struct {
    Kind string `json:"kind" binding:"required,oneof=integration website file_upload"`
    Name string `json:"name" binding:"required,min=1,max=120"`

    // kind=integration
    InConnectionID *uuid.UUID `json:"in_connection_id"`

    // kind=website
    RootURL         *string   `json:"root_url"`
    ScrapeDepth     *int      `json:"scrape_depth"`
    IncludePatterns []string  `json:"include_patterns"`
    ExcludePatterns []string  `json:"exclude_patterns"`

    // Shared
    RefreshFreqSeconds    *int `json:"refresh_freq_seconds"`
    PruneFreqSeconds      *int `json:"prune_freq_seconds"`
    PermSyncFreqSeconds   *int `json:"perm_sync_freq_seconds"`
}
```

Handler:
1. Validate kind-specific fields are present iff kind matches
2. If kind=integration, verify `InConnection.OrgID == current_org.ID` and `InIntegration.SupportsRAGSource=true`, and no existing RAGSource already references it
3. Enforce frequency validations (≥60s refresh, ≥300s prune)
4. Insert RAGSource with `status=INITIAL_INDEXING`
5. Enqueue an immediate `rag:ingest` task so the first index kicks off right away (don't wait for scheduler tick)
6. Return the created source

### Tests

Integration tests against real Postgres + real Redis + real asynq + real test Rust rag-engine process (using Phase 2J's `testhelpers.StartRagEngineInTestMode`).

1. `TestCreateSource_IntegrationKind_Succeeds` — with valid `InConnection`, creates source, enqueues initial ingest.
2. `TestCreateSource_IntegrationKind_WrongOrg_Forbidden` — `InConnection` belongs to different org → 403.
3. `TestCreateSource_IntegrationKind_UnsupportedIntegration` — `supports_rag_source=false` → 422.
4. `TestCreateSource_IntegrationKind_DuplicateConnection` — second source with same `InConnection` → 409.
5. `TestCreateSource_WebsiteKind_Succeeds` — with `root_url`, creates source (Phase 4+ will implement ingest).
6. `TestCreateSource_WebsiteKind_MissingURL` → 422.
7. `TestCreateSource_ValidatesRefreshFreq` — 59s → 422, 60s → 201.
8. `TestListSources_OnlyReturnsCallersOrg` — two orgs, each with sources; list scoped correctly.
9. `TestGetSource_IncludesLastAttempts` — creates a source, triggers sync, asserts attempts appear in detail response.
10. `TestPatchSource_PauseResume` — PATCH with `status=PAUSED` → subsequent scheduler scan skips it.
11. `TestDeleteSource_CascadesToChunks` — delete source; verify docs gone from Postgres and **via ragclient Search** confirm zero hits for that source's content.
12. `TestSyncEndpoint_EnqueuesIngestTask` — POST /sync → asynq queue has new task.
13. `TestAPIKey_RequiresRAGSourceScope` — API key without `rag:source:admin` → 403.

**Definition of done:**
- All 13 tests green
- OpenAPI spec regenerated (reuses existing swag annotations pattern)
- Admin UI hooks documented (no frontend code yet — Phase 9)

**Estimated effort:** 1 week

---

## Tranche 3F — Finalizer + E2E test

**Purpose:** Wire all six tranches together, prove the full loop works against real GitHub through the real scheduler into the real LanceDB.

**Files to create:**
- `internal/rag/e2e_github_test.go` — the big one
- `plans/phase3-rollout-notes.md` — runbook for deploying

### The E2E test

```go
func TestE2E_GitHubFullLoop(t *testing.T) {
    // Uses a dedicated test GitHub account (configured via env: GITHUB_TEST_TOKEN,
    // GITHUB_TEST_REPO="hiveloop-rag-test/fixture-repo").
    // This repo has a known set of PRs + issues seeded by a test helper.

    db := testhelpers.ConnectTestDB(t)
    nango := testhelpers.NangoTestClient(t)
    rag := testhelpers.StartRagEngineInTestMode(t, ...)
    org, user, integration, conn := testhelpers.NewTestGitHubSetup(t, db, nango)

    // 1. Create RAG source
    sourceResp := POST("/v1/rag/sources", CreateSourceRequest{
        Kind: "integration",
        Name: "Test GitHub",
        InConnectionID: &conn.ID,
    })

    // 2. Wait for initial ingest to complete (scheduler triggered it)
    waitForAttemptStatus(t, db, sourceResp.ID, IndexingStatusSuccess, 5*time.Minute)

    // 3. Query the rag-engine — should find PRs from the test repo
    hits := ragclient.Search(&ragpb.SearchRequest{
        DatasetName: datasetNameForOrg(org.ID),
        OrgID:       org.ID.String(),
        QueryText:   "known fixture PR title",
        Rerank:      false,
    })
    require.NotEmpty(t, hits)
    require.Contains(t, hits[0].DocID, "gh-pr-")

    // 4. Modify the repo (close a PR, add a comment via GitHub API)
    githubAPI.ClosePR(fixturePRNum)

    // 5. Trigger re-ingest; verify updated state propagates
    POST("/v1/rag/sources/" + sourceResp.ID + "/sync", nil)
    waitForAttemptStatus(...)
    hits = ragclient.Search(...)
    updatedMetadata := hits[0].Metadata["state"]
    require.Equal(t, "closed", updatedMetadata)

    // 6. Modify collab list; trigger perm_sync; verify ACL update
    githubAPI.RemoveCollaborator(fixtureCollabUser)
    POST("/v1/rag/sources/" + sourceResp.ID + "/perm-sync", nil)
    waitForPermSyncComplete(...)
    // Search as the removed collab — their results should NOT include private repo docs now
    hitsAsRemoved := ragclient.Search(WithACL([]string{"user_email:" + fixtureCollabUser}), ...)
    require.Empty(t, hitsAsRemoved)  // rerank filter removed them

    // 7. Delete a PR (force-delete via admin API), trigger prune
    githubAPI.DeletePR(anotherPRNum)
    POST("/v1/rag/sources/" + sourceResp.ID + "/prune", nil)
    waitForPruneComplete(...)
    hitsAfterPrune := ragclient.Search(WithQuery("the deleted PR title"), ...)
    require.Empty(t, hitsAfterPrune)

    // 8. Delete the source; verify all docs gone
    DELETE("/v1/rag/sources/" + sourceResp.ID)
    hitsAfterSourceDelete := ragclient.Search(...)
    require.Empty(t, hitsAfterSourceDelete)
}
```

This test proves all three loops work end-to-end against real GitHub: ingest, perm_sync, prune. It's the canonical acceptance test for Phase 3.

**Definition of done:**
- E2E test green in CI (requires test Nango + test GitHub account credentials)
- Runbook published for deploying Phase 3 to staging/prod
- All Phase 3 tranches merged to main

**Estimated effort:** 1 week (mostly CI wiring + fixture repo setup)

---

## Onyx symbol-to-Hiveloop mapping (Phase 3 additions)

| Onyx symbol | Onyx location | Hiveloop symbol | Hiveloop location |
|---|---|---|---|
| `ConnectorCredentialPair` (identity + config subset) | `models.py:723-837` | `RAGSource` | `internal/rag/model/rag_source.go` |
| `Connector.connector_specific_config` JSONB | `models.py:1872` | `RAGSource.Config` | same file |
| Connector protocol (LoadConnector, PollConnector, etc.) | `backend/onyx/connectors/interfaces.py` | `Connector`, `CheckpointedConnector[T]`, `PermSyncConnector`, `SlimConnector` | `internal/rag/connectors/interfaces/connector.go` |
| `Document`, `Section` | `backend/onyx/connectors/models.py` | `Document`, `Section` | `internal/rag/connectors/interfaces/document.go` |
| `ConnectorFailure`, `DocumentFailure`, `EntityFailure` | same | same names | same location |
| `ExternalAccess`, `DocExternalAccess` | `backend/onyx/access/models.py` | same names | same location |
| Ingest Celery task | `backend/onyx/background/celery/tasks/docfetching/tasks.py` | `handleRAGIngest` | `internal/rag/tasks/ingest_task.go` |
| Perm sync Celery task | `backend/ee/onyx/background/celery/tasks/doc_permission_syncing/` | `handleRAGPermSync` | `internal/rag/tasks/perm_sync_task.go` |
| Pruning Celery task | `backend/onyx/background/celery/tasks/pruning/` | `handleRAGPrune` | `internal/rag/tasks/prune_task.go` |
| Beat schedule | `backend/onyx/background/celery/tasks/beat_schedule.py` | `handleSchedulerTick` | `internal/rag/tasks/scheduler.go` |
| Watchdog | inline in Onyx tasks | `handleWatchdogTick` | `internal/rag/tasks/watchdog_task.go` |
| GitHub connector | `backend/onyx/connectors/github/connector.py` | `GitHubConnector` | `internal/rag/connectors/github/connector.go` |
| GitHub perm sync | `backend/ee/onyx/external_permissions/github/doc_sync.py` + `group_sync.py` | `perm_sync.go` | `internal/rag/connectors/github/perm_sync.go` |

---

## Launch order + parallelism

| Wave | Tranches | Parallel? | Deps |
|---|---|---|---|
| 1 | 3A (data model) + 3B (interfaces) | ✅ | Independent files, no shared code |
| 2 | 3C (scheduler) | ❌ | Needs 3A + 3B complete |
| 3 | 3D (GitHub connector) + 3E (API endpoints) | ✅ | Both need 3A+3B+3C; independent of each other |
| 4 | 3F (finalizer + E2E) | ❌ | Needs everything |

**Total wall clock with agent-parallel execution:** ~4 weeks
- Wave 1: 2–3 days
- Wave 2: 2 weeks
- Wave 3: 2 weeks (3D and 3E run concurrently)
- Wave 4: ~1 week

---

## Deferred to Phase 4+

| Item | Why deferred |
|---|---|
| Notion connector | Same pattern as GitHub; add after 3D proves the template |
| Slack connector | Same |
| Linear, Jira, Confluence, Google Drive connectors | Same |
| Website scraper (WebsiteConnector) | Separate from integration-kind; different auth + rate-limit model; Phase 4+ |
| File upload (`FILE_UPLOAD` kind) | Phase 5+ |
| Admin UI frontend for managing sources | Phase 9 |
| User-facing search UI | Phase 9 |
| Search API endpoint exposing rag-engine search to callers | Phase 5 (parallel to Phase 3) |
| Eval harness (Phase 11) | Can run parallel to Phase 3 |

---

## Two flags worth knowing about up-front

1. **Phase 3 locks us into "org-wide dataset"** — per your decision, all sources in an org go into the same LanceDB dataset. This means the dataset's `vector_dim` is fixed by the org's chosen embedder, and changing the embedder requires wiping + re-ingesting all of that org's data across every source. We discussed this trade-off in Phase 1G+1F (the `ReindexOrgWithModel` admin operation). It remains the simplest sustainable model.

2. **Nango test tenancy is new CI infrastructure** — Tranche 3D's integration tests need a dedicated test Nango tenant + test GitHub account. This is ~1 day of ops work to set up but belongs to Phase 3 proper, not later.
