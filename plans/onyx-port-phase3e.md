# Phase 3E — RAG source admin API

**Status:** Plan
**Branch:** `rag/phase-3e-api`
**Depends on:** 3A (RAGSource model), 3B (Connector interfaces), 3C (Scheduler + Asynq enqueue helpers), 3D (GitHub connector — proves the trait surface)
**Supersedes:** the high-level 3E section of `plans/onyx-port-phase3.md` (lines 588–660)

---

## Issues to Address

After 3C+3D, the RAG plumbing works end-to-end **if** an `RAGSource` row exists in Postgres. There is no way to make one exist without typing SQL. There is also no way to:

- list what sources an org has connected
- pause / resume a misbehaving source
- force a full reindex when the schedule cadence is too slow
- delete a source cleanly (tombstone its index attempts, mark for deletion)
- inspect why a recent attempt failed

3E delivers these. The deliverable is **eleven HTTP endpoints** under `/v1/rag/`, mirroring Onyx's connector + cc-pair admin API:

```
POST   /v1/rag/sources                              create
GET    /v1/rag/sources                              list (paginated, status filter)
GET    /v1/rag/sources/:id                          detail (+ last 5 attempts)
PATCH  /v1/rag/sources/:id                          update name, pause/resume, freq config, IndexingStart
DELETE /v1/rag/sources/:id                          mark DELETING; scheduler stops enqueuing; cleanup task removes docs
POST   /v1/rag/sources/:id/sync                     enqueue rag:ingest now (optional from_beginning)
POST   /v1/rag/sources/:id/prune                    enqueue rag:prune now
POST   /v1/rag/sources/:id/perm-sync                enqueue rag:perm_sync now (gated by connector capability)
GET    /v1/rag/sources/:id/attempts                 paginated attempts
GET    /v1/rag/sources/:id/attempts/:attempt_id     attempt detail + per-doc errors
GET    /v1/rag/integrations                         picker for the admin UI: InIntegrations where SupportsRAGSource=true
```

Behind the existing `MultiAuth → RequireEmailConfirmed → ResolveOrgFlexible → RequireOrgAdmin` middleware chain. Admin-only across the board — non-admin users do not interact with the RAG control plane in this phase. (A read-only "what's indexed?" surface for end users is a separate, later concern.)

---

## Important Notes

### Onyx mapping is denser here than in earlier tranches

The Onyx surface for this is `backend/onyx/server/documents/cc_pair.py` (read/update/manual-trigger) plus `connector.py` (create/list). Several ports are 1:1 in shape but renamed and re-scoped to a Hiveloop concept:

| Onyx | Hiveloop |
|---|---|
| `Connector` row + `Credential` row + a separate `ConnectorCredentialPair` joining the two | `RAGSource` row directly references `InConnection` (which already pairs an InIntegration with a Nango connection). One row, not three. |
| `PUT /admin/cc-pair/{id}/status` flipping `ConnectorCredentialPairStatus` | `PATCH /v1/rag/sources/:id` with `{ "status": "PAUSED" }` |
| `PUT /admin/cc-pair/{id}/property` for refresh/prune frequency | same `PATCH /v1/rag/sources/:id` |
| Two-step "create connector + associate credential" with a duplicate-check on the join | single-step create; the partial unique index `uq_rag_sources_in_connection` (3A) enforces the duplicate-check at DB level |
| `POST /admin/connector/run-once` taking `connector_id` + `credential_ids[]` + `from_beginning` | `POST /v1/rag/sources/:id/sync` taking `{ from_beginning: bool }` |
| `POST /admin/cc-pair/{id}/prune` | `POST /v1/rag/sources/:id/prune` |
| `POST /admin/cc-pair/{id}/sync-permissions` (EE) | `POST /v1/rag/sources/:id/perm-sync`; non-EE because Hiveloop has no EE/CE split here |

The Hiveloop version is structurally simpler because the `RAGSource` model already collapses what Onyx splits into three tables.

### Authn lives in middleware; handlers don't re-check

The handler-pattern survey confirmed: handlers read `middleware.UserFromContext(r.Context())` and `middleware.OrgFromContext(r.Context())`, then trust them. Org-scoping for every database read is done by `WHERE org_id = ?` on the gorm query, not by a separate authorization layer.

Concretely: a `GET /v1/rag/sources/:id` handler does

```
db.Where("id = ? AND org_id = ?", sourceID, org.ID).First(&src)
```

— a cross-org probe gets `gorm.ErrRecordNotFound` and 404s, indistinguishable from a non-existent ID. This matches the `in_connections` handler pattern (`internal/handler/in_connections_create.go`).

### Validation lives on the model, not in the handler

3A's `RAGSource.ValidateRefreshFreq()` and `ValidatePruneFreq()` (`internal/rag/model/rag_source.go:154-172`) return sentinel errors `ErrSourceRefreshFreqTooSmall` / `ErrSourcePruneFreqTooSmall`. The handler's job is to call them and translate to a 422 — not to re-implement the bounds.

Other validation that does belong in the handler (because it crosses entities):
- `kind == INTEGRATION` requires a non-nil `InConnectionID`.
- `kind != INTEGRATION` rejects a non-nil `InConnectionID`.
- The pointed-to `InConnection` must belong to the caller's org and reference an `InIntegration` with `SupportsRAGSource = true` and `RevokedAt IS NULL`.
- The `Status` enum value on PATCH must be one of the legal client-settable transitions: `ACTIVE` ⇄ `PAUSED`. Clients cannot set `DELETING`, `INITIAL_INDEXING`, or `ERROR` directly — those come from server-side state machines.

The duplicate-per-InConnection check is enforced by the `uq_rag_sources_in_connection` partial unique index 3A laid down. The handler catches `pq.Error` with code `23505` (unique violation) on insert and turns it into a 409 with a clear message — no separate SELECT-then-INSERT race.

### Manual trigger endpoints reuse 3C's enqueue helpers

`internal/rag/tasks/payloads.go` already exposes:
- `NewIngestTask(payload IngestPayload, opts ...asynq.Option) (*asynq.Task, error)`
- `NewPermSyncTask(payload PermSyncPayload, opts ...) (*asynq.Task, error)`
- `NewPruneTask(payload PrunePayload, opts ...) (*asynq.Task, error)`

The trigger handlers build the payload, call the constructor, and `enq.Enqueue(task, asynqUnique(short_ttl))` — same path the periodic scheduler uses. The `Unique` TTL prevents the obvious "user clicks the button five times" footgun; subsequent clicks within the TTL silently no-op (we surface this as 202 Accepted with `{ "deduplicated": true }` in the response so the UI can reflect "we heard you, your job is already in flight").

For perm-sync, the handler needs a capability check: only kinds that registered a `PermSyncConnector` can perm-sync. Mirror 3C's `HasPermSyncCapability` predicate.

For from-beginning ingest, the handler sets `IngestPayload.FromBeginning = true` — 3C's worker handles it correctly today (it overrides the window-start computation 3D introduced).

### DELETE is a soft tombstone, not a hard drop

Onyx's `delete_connector` (`backend/onyx/db/connector.py:159`) does a hard `db_session.delete(connector)` and lets SQLAlchemy cascade through `IndexAttempt` rows. We don't do that, because:
- The cascade to `RAGDocument` and `RAGHierarchyNode` rows can be large (millions of rows for a long-lived source).
- A worker may be mid-ingest when the DELETE arrives; a hard drop races against an in-flight `INSERT INTO rag_index_attempts`.

Instead, the DELETE handler:
1. Sets `RAGSource.Status = 'DELETING'`. The 3C scheduler's predicate `status IN ('ACTIVE', 'INITIAL_INDEXING')` already excludes `DELETING`, so no new work gets enqueued.
2. Returns `202 Accepted`.
3. A cleanup task (out of scope for 3E — the 3C scheduler should grow a fourth periodic loop to drain DELETING sources, or this can live in the existing `internal/tasks/cleanup.go`) actually removes the row + cascades after a brief grace window.

For 3E, just doing step 1 + 2 is enough; the deletion-finalisation loop is a known follow-up. Document it in the response: `{ "status": "deleting", "note": "documents will be removed asynchronously" }`.

### Cost-control: one cap on list page size

`GET /v1/rag/sources?page_size=10000` is denial-of-service-shaped if not capped. Match Onyx's pattern (`cc_pair.py:82`): `page_size` validated to `[1, 100]`, default `20`. Same for `attempts`.

### File layout under the 300-line ceiling

Each of the eleven endpoints is one handler method. Splitting into 8 production files + 2 test files keeps every file under 200 lines.

```
internal/handler/
  rag_sources.go               ~ 80 LOC — RAGSourceHandler struct, response shapes, writeJSON wrapper
  rag_sources_create.go        ~ 150 LOC — POST /v1/rag/sources
  rag_sources_read.go          ~ 130 LOC — GET / (list) + GET /:id (detail)
  rag_sources_update.go        ~ 140 LOC — PATCH /:id
  rag_sources_delete.go        ~ 70 LOC — DELETE /:id
  rag_sources_sync.go          ~ 160 LOC — POST /:id/{sync,prune,perm-sync} (3 thin handlers + shared enqueue helper)
  rag_sources_attempts.go      ~ 130 LOC — GET /:id/attempts + GET /:id/attempts/:attempt_id
  rag_integrations.go          ~ 60 LOC — GET /v1/rag/integrations (picker)

  rag_sources_test.go          ~ 280 LOC — endpoint integration tests
  rag_sources_helpers_test.go  ~ 200 LOC — test fixtures (createTestSource, createTestInConnection helpers)
```

Largest: `rag_sources_test.go` ~280 lines, just under the ceiling.

### Database query helpers go in `internal/rag/db/`

Per the Hiveloop convention (`CLAUDE.md`: "Put ALL db operations under the `backend/onyx/db` directory"), database queries don't live in the handler. New file:

```
internal/rag/db/
  source_queries.go           ~ 150 LOC — listSourcesForOrg, getSourceForOrg, listAttemptsForSource, listAttemptErrorsForAttempt
  source_queries_test.go      ~ 180 LOC — unit tests with real DB
```

The handler calls into these. Keeps SQL out of HTTP code, matches the rest of the codebase's layout.

---

## Implementation Strategy

### Layer A — Database query helpers (`internal/rag/db/source_queries.go`)

Pure gorm queries, all org-scoped. Each function takes `db *gorm.DB` and `orgID uuid.UUID` as the first two args.

| Function | Returns | Onyx mapping |
|---|---|---|
| `ListSourcesForOrg(db, orgID, opts ListOptions) ([]RAGSource, int64, error)` | rows + total count for pagination header | `cc_pair.py` list path (no pagination today; we add it) |
| `GetSourceForOrg(db, orgID, sourceID) (*RAGSource, error)` | single row or `gorm.ErrRecordNotFound` | `cc_pair.py:156` `get_cc_pair_full_info` |
| `ListAttemptsForSource(db, orgID, sourceID, page, pageSize) ([]RAGIndexAttempt, int64, error)` | paginated attempts | `cc_pair.py:82` paginated index-attempts endpoint |
| `GetAttemptForSource(db, orgID, sourceID, attemptID) (*RAGIndexAttempt, error)` | single attempt with org+source guard | inferred from Onyx's pattern |
| `ListAttemptErrors(db, attemptID, page, pageSize) ([]RAGIndexAttemptError, int64, error)` | paginated per-doc errors | `cc_pair.py:499` `get_cc_pair_indexing_errors` |
| `ListSupportedIntegrations(db) ([]InIntegration, error)` | InIntegrations where `supports_rag_source = true AND deleted_at IS NULL` | (no Onyx analogue — this is Hiveloop-specific UI plumbing) |

`ListOptions` covers `Page int, PageSize int, StatusFilter *RAGSourceStatus, KindFilter *RAGSourceKind`. Defaults: page 0, size 20, no filters.

### Layer B — Handler package (`internal/handler/rag_sources*.go`)

`RAGSourceHandler` struct depends on:
- `*gorm.DB` for read paths
- `enqueue.TaskEnqueuer` for the manual-trigger endpoints (the same interface 3C's scheduler uses; production wires `*asynq.Client`, tests wire a recording fake)

Every handler:
1. Reads org from context.
2. Decodes JSON body (or query params for GET).
3. Calls into `internal/rag/db` for the read.
4. For state-changing endpoints: validates the requested transition, updates via gorm, returns the updated row.
5. For trigger endpoints: builds the Asynq payload, calls `NewXTask`, calls `enq.Enqueue(task, asynqUnique(short_ttl))`. Surfaces dedup as `{ "deduplicated": true }` in the response.
6. Writes via `writeJSON(w, statusCode, response)`.

Response shapes go in `rag_sources.go`:

```
type RAGSourceResponse {
    ID, OrgID, Kind, Name, Status, Enabled,
    InConnectionID *uuid.UUID,
    Config json.RawMessage,
    IndexingStart, LastSuccessfulIndexTime, LastTimePermSync, LastPruned *time.Time,
    RefreshFreqSeconds, PruneFreqSeconds, PermSyncFreqSeconds *int,
    TotalDocsIndexed int,
    InRepeatedErrorState bool,
    CreatedAt, UpdatedAt time.Time,
}

type RAGSourceDetailResponse {
    ...embeds RAGSourceResponse,
    RecentAttempts []RAGIndexAttemptResponse  // last 5
}

type RAGIndexAttemptResponse {
    ID, Status, FromBeginning,
    NewDocsIndexed, TotalDocsIndexed, DocsRemovedFromIndex *int,
    ErrorMsg *string, ErrorCount int,
    PollRangeStart, PollRangeEnd *time.Time,
    TimeStarted, TimeUpdated *time.Time,
}
```

No `FullExceptionTrace` in the response — that goes in the per-attempt detail (`GET /:id/attempts/:attempt_id`) only, gated by admin role (which it already is via middleware). Keeps the list payload tight.

### Layer C — Route registration (`cmd/server/serve_routes_v1.go`)

Add a `/v1/rag` chi sub-router with the eleven endpoints. Mount it under the existing v1 group with the same middleware chain as `in_connections` plus `RequireOrgAdmin`. Single ~20-line change in the existing file.

### Layer D — Tests

`internal/handler/rag_sources_test.go` covers the happy/sad paths through real Postgres and a recording-fake `TaskEnqueuer`. The tests shape: chi in-process router, real DB via the existing `connectTestDB` helper, `httptest.NewRequest` + `middleware.WithUser/WithOrg` for context injection, `httptest.NewRecorder` for the response.

For the perm-sync trigger test (which checks the connector capability gate), we register a no-op stub connector under a known kind in the test setup — same pattern 3C used.

---

## Tests

Real Postgres (`make test-services-up`), real chi router in-process, fake `TaskEnqueuer` that records the payloads it received. No mocks for anything else.

### Create

1. **`TestCreateSource_IntegrationKind_HappyPath`** — admin creates with `kind=INTEGRATION`, valid `in_connection_id`; row persists with `Status=INITIAL_INDEXING`, response is 201 with the full source representation.
2. **`TestCreateSource_IntegrationKind_RejectsCrossOrgConnection`** — `in_connection_id` belongs to a different org → 404 (not 403, to avoid leaking existence).
3. **`TestCreateSource_IntegrationKind_RejectsUnsupportedIntegration`** — `InIntegration.SupportsRAGSource=false` → 422 with explicit message.
4. **`TestCreateSource_DuplicateInConnection_Returns409`** — second create against the same `in_connection_id` → 409 (DB partial unique index fires).
5. **`TestCreateSource_RefreshFreqUnder60s_Rejects`** — `refresh_freq_seconds = 30` → 422; error mentions the 60s minimum.
6. **`TestCreateSource_NonAdmin_Returns403`** — middleware gate blocks non-admins.

### List + detail

7. **`TestListSources_OrgScoped`** — two orgs, two sources each; admin in org A sees only org A's two.
8. **`TestListSources_Paginated`** — 25 sources, page_size=10 → returns 10 with `total: 25`.
9. **`TestListSources_FilterByStatus`** — `?status=PAUSED` returns only paused sources.
10. **`TestGetSourceDetail_IncludesLast5Attempts`** — source with 7 attempts → response includes the 5 most-recent ordered by `time_created DESC`.
11. **`TestGetSourceDetail_CrossOrg_Returns404`** — admin A queries source B → 404.

### Update

12. **`TestUpdateSource_PauseAndResume`** — PATCH `{ "status": "PAUSED" }` flips status and prevents the next ingest scan from picking it up; PATCH `{ "status": "ACTIVE" }` reverses it.
13. **`TestUpdateSource_RejectsClientSetDeletingStatus`** — PATCH `{ "status": "DELETING" }` → 422 (only the DELETE endpoint can set this).
14. **`TestUpdateSource_FreqValidation_PropagatesModelErrors`** — PATCH `{ "refresh_freq_seconds": 10 }` → 422 with the same message the model produces.
15. **`TestUpdateSource_IndexingStartFloor`** — PATCH `{ "indexing_start": "2024-01-01T00:00:00Z" }` → next manual sync's window-start respects the floor.

### Delete

16. **`TestDeleteSource_TombstonesAndStopsScheduler`** — DELETE flips status to `DELETING`; subsequent ingest-scan tick from 3C skips it; response is 202 with the "deletion will complete asynchronously" note.

### Manual triggers

17. **`TestSyncTrigger_EnqueuesIngestTask`** — POST `/sync` → fake enqueuer received exactly one `rag:ingest` task with `RAGSourceID = src.ID, FromBeginning = false`.
18. **`TestSyncTrigger_FromBeginning`** — POST `/sync` body `{ "from_beginning": true }` → enqueued payload has `FromBeginning = true`.
19. **`TestSyncTrigger_DedupesRapidClicks`** — second POST within the unique window → response `{ "deduplicated": true }`, fake enqueuer still has only one task.
20. **`TestPermSyncTrigger_CapabilityGate`** — kind=`stub-noperm` (no `PermSyncConnector` registered) → 422 "this connector does not support permission sync".
21. **`TestPruneTrigger_EnqueuesPruneTask`** — POST `/prune` → one `rag:prune` task on the queue.

### Attempts

22. **`TestListAttempts_Paginated`** — 12 attempts on a source, page_size=5 → 5 returned + `total: 12`.
23. **`TestGetAttemptDetail_IncludesPerDocErrors`** — attempt with 3 `rag_index_attempt_errors` rows → response includes them in `errors` array.

### Integrations picker

24. **`TestListIntegrations_OnlySupportedReturned`** — fixture has 3 InIntegrations: 2 with `SupportsRAGSource=true`, 1 with false → response has the 2.

### Definition of done

- All 24 tests pass on real Postgres (`make test-services-up`) with the chi in-process router.
- Routes registered in `serve_routes_v1.go` under `/v1/rag`; middleware chain matches `in_connections` plus `RequireOrgAdmin`.
- `scripts/check-go-file-length.sh` clean (every new file under 300 lines).
- `scripts/check-go-comment-density.sh` clean (≤10% of added Go lines are comments).
- `internal/handler` new files run inside the existing `test-core` sharded suite — no infra exclusion needed because these tests only need Postgres, which `test-core` already provisions.

---

## Onyx ↔ Hiveloop reference index

| Onyx | Hiveloop (after 3E) |
|---|---|
| `backend/onyx/server/documents/connector.py:1550` (`POST /admin/connector`) | `internal/handler/rag_sources_create.go` `Create` |
| `backend/onyx/server/documents/cc_pair.py:540` (associate credential → CC-pair) | folded into `Create` — single-step in our model |
| `backend/onyx/server/documents/connector.py:931` (`GET /admin/connector` list) | `internal/handler/rag_sources_read.go` `List` |
| `backend/onyx/server/documents/cc_pair.py:156` (`GET /admin/cc-pair/{id}`) | `internal/handler/rag_sources_read.go` `Get` |
| `backend/onyx/server/documents/cc_pair.py:259` (status flip), `:343` (rename), `:372` (freq) | `internal/handler/rag_sources_update.go` `Update` (one PATCH for all) |
| `backend/onyx/server/documents/cc_pair.py:625` (delete) | `internal/handler/rag_sources_delete.go` `Delete` (soft, not hard) |
| `backend/onyx/server/documents/connector.py:1741` (`POST /admin/connector/run-once`) | `internal/handler/rag_sources_sync.go` `TriggerSync` |
| `backend/onyx/server/documents/cc_pair.py:433` (`POST /admin/cc-pair/{id}/prune`) | `internal/handler/rag_sources_sync.go` `TriggerPrune` |
| `backend/ee/onyx/server/documents/cc_pair.py:53` (`POST /admin/cc-pair/{id}/sync-permissions`) | `internal/handler/rag_sources_sync.go` `TriggerPermSync` |
| `backend/onyx/server/documents/cc_pair.py:82` (paginated index-attempts) | `internal/handler/rag_sources_attempts.go` `ListAttempts` |
| `backend/onyx/server/documents/cc_pair.py:499` (paginated errors) | folded into `GetAttempt` (errors in the detail response) |
| `backend/onyx/db/models.py:1916` (refresh_freq validation) | `internal/rag/model/rag_source.go` `ValidateRefreshFreq` (3A — already present) |
| `backend/onyx/db/connector_credential_pair.py:563` (duplicate CC-pair rejection) | DB partial unique index `uq_rag_sources_in_connection` (3A — already present) |
| `backend/onyx/db/enums.py:374-375` (`ADD_CONNECTORS`, `MANAGE_CONNECTORS` permissions) | `RequireOrgAdmin` middleware (Hiveloop has org-admin granularity, not feature-specific permissions) |

---

## What we deliberately don't do in 3E

- **Deletion-finalisation loop** — flagged for follow-up; the soft-delete makes 3E unblocking without it.
- **Connector-disable on repeated errors** — the `InRepeatedErrorState` column exists on `RAGSource`; a separate mini-tranche should add a rule "after N consecutive failed attempts in a row, set `InRepeatedErrorState=true` and skip the source from ingest scans". Out of 3E.
- **End-user (non-admin) read endpoints** — for now, only admins talk to the RAG control plane. End-user search APIs are a separate concern.
- **Bulk operations** — no "pause all" or "delete all in status=ERROR". One row at a time, by ID. Mirrors Onyx.
- **Audit logging** — the existing `internal/admin/audit.go` (per the prior research) already logs admin actions; 3E handlers tag their actions and let that machinery log them. No new audit-log table.
