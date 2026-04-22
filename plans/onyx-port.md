# Onyx → Hiveloop: RAG Port Plan (Phases 0–1)

**Author:** architecture session, April 2026
**Scope of this document:** Phase 0 (scaffolding) and Phase 1 (data layer) only. Later phases planned separately once Phase 1 lands.
**Source project:** `/Users/bahdcoder/code/onyx` — MIT-licensed
**Target project:** `/Users/bahdcoder/code/hiveloop.com`

We are cloning Onyx's ingestion + ACL + permission-sync architecture into Hiveloop's Go/gorm/chi/asynq monolith, adapting where our architecture differs, and changing three substitutions:

| Onyx | Hiveloop |
|---|---|
| Vespa (vector store) | **LanceDB** (unofficial Go bindings, storage on **R2** prod / **MinIO** dev) |
| Vespa ACL filter | LanceDB payload filter over `acl: list<string>` |
| `ConnectorCredentialPair` + `Connector` + `Credential` | `InConnection` + `InIntegration` (already exist) + a new sibling `RAGSyncState` |
| Celery | Asynq (already in use) |
| SQLAlchemy multi-tenancy (schema-per-tenant) | Hiveloop's existing org-row-level scoping (`org_id` on every row) |
| Postgres-per-tenant | Shared Postgres, `org_id` column + index |
| Onyx `User` | Hiveloop `User` + `OrgMembership` |
| Onyx `UserGroup` (EE) | **Not cloned for Phase 1** — Hiveloop's RBAC (`OrgMembership.Role`) stays as-is for org-level permissions; RAG introduces *document-level* ACLs as a separate new axis. |

**The rule for every line of this plan:** if we are porting behavior from Onyx, the Onyx source reference is cited inline. If we are deliberately deviating, the deviation is called out with `DEVIATION:` and a justification.

---

## Locked stack decisions

| Concern | Decision |
|---|---|
| Vector DB | LanceDB via unofficial Go bindings (Phase 0 verification spike blocks further work) |
| Vector storage backing | Cloudflare R2 (prod), MinIO (dev/test) — S3-compatible |
| Checkpoint / filestore | Same R2/MinIO bucket, prefix-isolated from vector data |
| Default embedder | **Qwen3-Embedding-4B** (2560d) via SiliconFlow OpenAI-compatible endpoint |
| Embedder pluggability | **Per-org**; different orgs may pick different models; **one model per org for the lifetime of their index** |
| Reranker | Qwen3-Reranker-0.6B (SiliconFlow) |
| Search mode | Hybrid (BM25 + vector) from day one |
| Chat/answer | **Out of scope** — we expose `Search()` helper only |
| Connector auth | Nango proxy only — no direct provider HTTP clients |
| Job queue | Asynq (existing) |
| Permissions architecture | Full Onyx three-loop: ingest, perm-sync, prune — each per-connection, each independently scheduled |

**Re-embed-on-model-switch is explicit, not background:** if an org wants a different model, ops deletes their chunks + re-ingests. Keeps code simple.

---

## Testing philosophy — **non-negotiable for every tranche**

Hiveloop already runs tests against real services (see `internal/middleware/integration_test.go`: real Postgres at `localhost:5433`, real `model.AutoMigrate`). This is the only pattern we use.

### Hard rules

1. **Mocking is permitted ONLY for the embedder and reranker.** Both make paid external API calls; Phase 2C delivers their interfaces with an in-memory deterministic fake. Nothing else gets mocked — not Postgres, not Redis, not MinIO, not LanceDB, not Nango, not HTTP handlers.
2. **Integration-first.** Anything touching infrastructure is tested against a real instance of that infrastructure, via `docker-compose.test.yml` (Postgres + Redis + MinIO + LanceDB).
3. **Every test verifies business behavior, not framework behavior.** If the test could only fail because gorm/chi/asynq broke, do not write it. Concrete bans:
   - ❌ Assert that a field exists on a struct (the compiler proves this)
   - ❌ Assert that a primary key is a primary key
   - ❌ Assert that gorm reads what gorm wrote
   - ❌ Tests using `sqlmock`, `go-sqlmock`, `testify/mock` against DB code
   - ❌ Tests that set up a fixture, call a function, and assert the fixture came back
   - ❌ Tests with names like `TestCreatesSuccessfully` that don't pin behavior
4. **Pure-logic functions are tested directly — no mocks involved because no infrastructure is involved.** ACL prefix helpers, enum `IsTerminal()`, validation rules — these are pure Go and get pure table-driven tests. That is not a deviation from rule 1; there is nothing to mock.
5. **Coverage target: 100% of branchful code.** `go test -cover` must report 100% for every `internal/rag/**` package that has any conditional logic. Pure-data structs with no methods contribute zero to both numerator and denominator; they are not "uncovered."
6. **Every integration test cleans up what it created.** Use `t.Cleanup(...)` with explicit `DELETE` by `org_id` (mirror the pattern in `internal/middleware/integration_test.go:63-69`). No "just drop the schema at the end" shortcuts.
7. **No flaky tests.** If a test needs an external service to be up and it isn't, it fails loudly with "run `make test-services-up` first" — it does not skip, does not retry, does not timeout silently.

### Test harness (built in Phase 0)

Phase 0 delivers two files that every subsequent tranche uses:

- `internal/rag/testhelpers/db.go` — `ConnectTestDB(t *testing.T) *gorm.DB` that opens the existing Hiveloop test Postgres, runs `model.AutoMigrate` + `rag.AutoMigrate`, registers `t.Cleanup` to close. Parallels `internal/middleware/integration_test.go:31-60`.
- `internal/rag/testhelpers/fixtures.go` — typed fixture constructors: `NewTestOrg(t, db)`, `NewTestUser(t, db, orgID)`, `NewTestInIntegration(t, db)`, `NewTestInConnection(t, db, orgID, userID, integID)`. Each registers cleanup. Mirrors Onyx's `backend/tests/integration/common_utils/managers/` pattern.

### What "real business value" means per test type

| Test target | Why it has business value |
|---|---|
| FK cascade delete (Org → RAGDocument) | GDPR / org deletion compliance: if this breaks, orphaned docs survive tenant deletion |
| Unique constraint on `(raw_node_id, source)` | Prevents double-indexing of the same source page under two different `HierarchyNode` rows |
| Partial index `idx_rag_document_needs_sync` exists and is used by EXPLAIN | Watchdog + sync loop depend on it; full-table scan at production scale is a P0 |
| `IndexingStatus.IsTerminal()` branch coverage | Scheduler decides whether to spawn a retry based on this; wrong answer = stuck queues |
| Stale-sweep pattern on `RAGUserExternalUserGroup` | Security-critical: stale rows grant outdated permissions if sweep is wrong |
| `ACL prefix` functions produce exact Onyx-compatible strings | Filter strings must byte-match what's stored at index time; off-by-one = 0 results |
| `AutoMigrate` idempotence | CI deploys run it on every boot; non-idempotent = deploy failure |
| Seed `RAGEmbeddingModel` idempotence | Same reason |
| Schema matches Onyx columns we ported (via migration inspection, not field assertions) | Proves we actually ported the field with the right Postgres type |

### What is explicitly NOT tested (waste of time)

- Field presence, struct zero-value construction, pointer vs value semantics
- That gorm tags produce the expected DDL (we test the DDL directly via `pg_indexes` / `information_schema`)
- That uuid generation works
- Default values baked in by Postgres (we test them once via the migration inspection test, not per-model)

---

## Phase 0 — Scaffolding + LanceDB verification spike + test harness

**Owner:** 1 agent, sequential. Blocks all Phase 1 work.

### 0.1 Create package tree

```
internal/rag/
  model/              # gorm models (Phase 1 fills this)
  connectors/
    interfaces/
    github/           # stub
    notion/           # stub
  vectorstore/
  embedder/
  chunker/
  acl/
  filestore/
  pipeline/
  tasks/
  search/
  locks/
  identity/
  config/
  testhelpers/        # see 0.4
  register.go
  doc/
    ARCHITECTURE.md
    ONYX_MAPPING.md
    TESTING.md
```

**Mapping to Onyx directories:**
| Hiveloop package | Onyx equivalent |
|---|---|
| `internal/rag/model/` | `backend/onyx/db/models.py` (RAG-relevant subset) |
| `internal/rag/connectors/interfaces/` | `backend/onyx/connectors/interfaces.py` |
| `internal/rag/vectorstore/` | `backend/onyx/document_index/vespa/` |
| `internal/rag/embedder/` | `backend/onyx/indexing/embedder.py` |
| `internal/rag/chunker/` | `backend/onyx/indexing/chunking/` |
| `internal/rag/acl/` | `backend/onyx/access/` |
| `internal/rag/filestore/` | `backend/onyx/file_store/` |
| `internal/rag/pipeline/` | `backend/onyx/indexing/indexing_pipeline.py` |
| `internal/rag/tasks/` | `backend/onyx/background/celery/tasks/` |
| `internal/rag/search/` | `backend/onyx/context/search/` |
| `internal/rag/locks/` | Redis lock helpers scattered across Onyx background tasks |

### 0.2 Decision log

File: `internal/rag/doc/ARCHITECTURE.md`. Captures:
- LanceDB-over-Qdrant decision + the Rust-sidecar fallback plan
- One-model-per-org invariant
- Three-loop sync architecture, referencing `backend/onyx/background/celery/tasks/{docfetching,docprocessing,pruning,doc_permission_syncing,external_group_syncing}`
- ACL string format + the invariant that prefixes are applied on read — see Onyx `backend/onyx/access/utils.py` and `DocumentAccess.to_acl` at `backend/onyx/access/models.py:174-197`
- R2 backs both filestore and LanceDB dataset storage

### 0.3 LanceDB Go binding verification spike

**Gate: if this fails, work stops and we escalate.**

Single Go program at `internal/rag/vectorstore/spike/main.go`. Runs against local MinIO. Exercises all seven primitives:

1. Connect to LanceDB via unofficial Go bindings, backed by a local MinIO S3 endpoint
2. Create dataset with schema `id string, org_id string, vector fixed_size_list<float,2560>, acl list<string>, content text, is_public bool, doc_updated_at timestamp`
3. Upsert 100 rows with random embeddings
4. Vector search with filter `org_id = X AND (acl ∈ [...] OR is_public = true)` — returns results in <100ms
5. FTS over `content` with the same metadata filter
6. Update `acl` on an existing row without touching the vector — **critical for perm sync**, mirrors Onyx's `DocumentIndex.update_metadata` at `backend/onyx/document_index/interfaces.py`
7. Delete by `id`

**Outcome:** all 7 pass → green-light Phase 1. Any fail → write `internal/rag/doc/SPIKE_RESULT.md`, pause work, escalate.

**Test:** the spike IS the test — it's a runnable main that either exits 0 or fails with a specific failure. No separate `_test.go` needed; it's not production code.

### 0.4 Test harness

Build the two files that every Phase 1 tranche depends on.

**`internal/rag/testhelpers/db.go`**

```go
// ConnectTestDB returns a real Postgres connection with the full Hiveloop
// schema migrated (model.AutoMigrate) plus the RAG schema (rag.AutoMigrate).
// Uses the same DATABASE_URL convention as internal/middleware/integration_test.go.
// Registers t.Cleanup to close the underlying *sql.DB.
func ConnectTestDB(t *testing.T) *gorm.DB
```

**`internal/rag/testhelpers/fixtures.go`**

```go
// NewTestOrg creates a real Org row with a unique random name.
// t.Cleanup deletes the org (cascades to all RAG descendants once Phase 1 lands).
func NewTestOrg(t *testing.T, db *gorm.DB) *model.Org

// NewTestUser creates a real User + OrgMembership with role="owner".
func NewTestUser(t *testing.T, db *gorm.DB, orgID uuid.UUID) *model.User

// NewTestInIntegration creates an InIntegration (e.g. "github").
func NewTestInIntegration(t *testing.T, db *gorm.DB, provider string) *model.InIntegration

// NewTestInConnection creates an InConnection backed by a fake Nango connection ID.
func NewTestInConnection(t *testing.T, db *gorm.DB, orgID, userID, integID uuid.UUID) *model.InConnection
```

Pattern: every fixture constructor takes `*testing.T`, writes a real row, registers cleanup. No builder-pattern fluent APIs, no "insert and pray" shortcuts.

### 0.5 Testing docs

File: `internal/rag/doc/TESTING.md` — restates the rules from this plan's Testing Philosophy section so implementers don't have to chase this plan later.

### 0.6 Docker-compose for tests

Extend `docker-compose.yml` (or add `docker-compose.test.yml`):
- Postgres on 5433 (already exists per `internal/middleware/integration_test.go:26`)
- Redis on a test port (for Phase 2 locks, not used by Phase 1)
- MinIO with a pre-created `hiveloop-rag-test` bucket (used by Phase 0 spike + Phase 2 vectorstore tests)
- `make test-services-up` / `make test-services-down` targets

Phase 1 tranches need only Postgres (already up). Phase 0 also starts MinIO for the LanceDB spike.

### 0.7 Wire empty `AutoMigrate`

`internal/rag/register.go`:

```go
package rag

import "gorm.io/gorm"

func AutoMigrate(db *gorm.DB) error {
    // Phase 1 tranches append here
    return nil
}
```

Hook into `internal/model/org.go:AutoMigrate` after the existing `db.AutoMigrate(...)` block.

**Test (Phase 0 only):** `internal/rag/register_test.go` — calls `ConnectTestDB(t)` and verifies it returns without error. This proves the harness works + the stub `AutoMigrate` is wired. Single test; discarded once Phase 1 adds real tests in its place.

---

## Phase 1 — Data layer (7 tranches, 6 parallel + 1 finalizer)

**Universal constraints:**
- `OrgID uuid.UUID` FK to `orgs.id` `ON DELETE CASCADE` on every RAG model
- gorm tags following `internal/model/` patterns
- `TableName()` returns `rag_*`
- Enums are Go typed strings with `const` groups + `IsValid()` method
- JSON columns use `model.JSON` (`internal/model/json.go`)
- Struct doc comments cite the Onyx source line being ported
- **Every tranche delivers tests per the Testing Philosophy. No tranche is "done" without its test suite.**

---

### Tranche 1A — Core document + hierarchy

**Onyx references:**
- `Document`: `backend/onyx/db/models.py:939-1063`
- `HierarchyNode`: `backend/onyx/db/models.py:839-936`
- `DocumentByConnectorCredentialPair`: `backend/onyx/db/models.py:2512-2558`
- `HierarchyNodeByConnectorCredentialPair`: `backend/onyx/db/models.py:2480-2510`
- `HierarchyNodeType` enum: `backend/onyx/db/enums.py:306-340`
- `DocumentSource` enum: `backend/onyx/configs/constants.py:205-262`
- `PUBLIC_DOC_PAT`: `backend/onyx/configs/constants.py:27`
- `ExternalAccess`/`DocumentAccess`: `backend/onyx/access/models.py`

**Files:**
- `internal/rag/model/document.go`
- `internal/rag/model/hierarchy_node.go`
- `internal/rag/model/document_by_connection.go`
- `internal/rag/model/hierarchy_node_by_connection.go`
- `internal/rag/model/enums.go` (+ `DocumentSource`, `HierarchyNodeType`, `KGStage` placeholder)

#### `RAGDocument` (ports `Document` at `backend/onyx/db/models.py:939-1063`)

| Field | Go type | Onyx field (models.py:line) | Notes |
|---|---|---|---|
| `ID` | `string` PK | `id:943` | Source-given opaque ID |
| `OrgID` | `uuid.UUID` FK | — | DEVIATION: Onyx schema-per-tenant → org_id column |
| `FromIngestionAPI` | `bool` | `from_ingestion_api:944` | |
| `Boost` | `int` default 0 | `boost:948` | |
| `Hidden` | `bool` default false | `hidden:949` | |
| `SemanticID` | `string` | `semantic_id:950` | |
| `Link` | `*string` | `link:952` | |
| `FileID` | `*string` | `file_id:953` | |
| `DocUpdatedAt` | `*time.Time` | `doc_updated_at:960` | |
| `ChunkCount` | `*int` | `chunk_count:964` | |
| `LastModified` | `time.Time` indexed default now | `last_modified:969` | |
| `LastSynced` | `*time.Time` indexed | `last_synced:974` | |
| `PrimaryOwners` | `pq.StringArray` | `primary_owners:982` | |
| `SecondaryOwners` | `pq.StringArray` | `secondary_owners:985` | |
| `ExternalUserEmails` | `pq.StringArray` | `external_user_emails:992` | ACL |
| `ExternalUserGroupIDs` | `pq.StringArray` | `external_user_group_ids:996` | ACL |
| `IsPublic` | `bool` default false | `is_public:998` | ACL |
| `ParentHierarchyNodeID` | `*int64` FK SET NULL | `parent_hierarchy_node_id:1003` | |
| `DocMetadata` | `model.JSON` | `doc_metadata:1024` | |

**Skipped:** `kg_stage`, `kg_processing_time`, relationships to Persona/Tag/RetrievalFeedback (not Phase 1 scope).

**Indexes:**
- `idx_rag_document_org` on `(org_id)`
- `idx_rag_document_needs_sync` partial: `(id) WHERE last_modified > last_synced OR last_synced IS NULL` — port of Onyx `ix_document_needs_sync` at `backend/onyx/db/models.py:1058-1062`
- GIN on `external_user_emails` — Hiveloop explicit; Onyx relies on default ARRAY index
- GIN on `external_user_group_ids` — same
- `idx_rag_document_last_modified` — port of Onyx `index=True` at line 969

#### `RAGHierarchyNode` (ports `HierarchyNode` at `backend/onyx/db/models.py:839-936`)

Field-by-field port (fields table omitted here for brevity — see original detailed table in commit; all fields at lines 855-904 covered).

**Indexes:**
- `uq_rag_hierarchy_node_raw_id_source` unique on `(raw_node_id, source)` — port of Onyx `uq_hierarchy_node_raw_id_source` at `models.py:930-932`
- `idx_rag_hierarchy_node_source_type` on `(source, node_type)` — port of Onyx index at `models.py:934`

**`HierarchyNodeType`** — verbatim values from `backend/onyx/db/enums.py:306-340`: folder, source, shared_drive, my_drive, space, page, project, database, workspace, site, drive, channel.

#### `RAGDocumentByConnection` (adapts `DocumentByConnectorCredentialPair` at `backend/onyx/db/models.py:2512-2558`)

**DEVIATION:** Onyx keys by `(document_id, connector_id, credential_id)`. We key by `(document_id, in_connection_id)`.

| Field | Go type | Onyx field | Notes |
|---|---|---|---|
| `DocumentID` | `string` PK, FK | `id:2517` | |
| `InConnectionID` | `uuid.UUID` PK, FK CASCADE | `connector_id+credential_id:2519-2525` | |
| `HasBeenIndexed` | `bool` | `has_been_indexed:2531` | Distinguishes perm-sync-added from content-indexed (see Onyx comment 2527-2530) |

**Indexes:**
- `idx_rag_doc_conn_connection` on `(in_connection_id)` — for per-connection counts
- `idx_rag_doc_conn_counts` on `(in_connection_id, has_been_indexed)` — port of Onyx `idx_document_cc_pair_counts` at `models.py:2553-2557`

#### `RAGHierarchyNodeByConnection` (adapts `HierarchyNodeByConnectorCredentialPair` at `backend/onyx/db/models.py:2480-2510`)

Same `cc_pair → in_connection` adaptation. PK `(hierarchy_node_id, in_connection_id)`. The comment at Onyx `models.py:2481-2483` documents the pruning semantics this table enables.

#### Tranche 1A tests

**File:** `internal/rag/model/document_test.go`

All tests use `testhelpers.ConnectTestDB(t)` + `testhelpers.NewTestOrg(t, db)`.

1. `TestRAGDocument_OrgCascadeDelete` — create Org + RAGDocument, delete Org, verify doc gone via `db.First(&doc)` returning `gorm.ErrRecordNotFound`. **Business value:** org deletion compliance.
2. `TestRAGDocument_ParentHierarchyNodeSetNullOnDelete` — create HierarchyNode + Document pointing to it, delete node, verify `doc.ParentHierarchyNodeID` is null. **Business value:** deleting a folder must not delete its docs.
3. `TestRAGDocument_NeedsSyncPartialIndexExistsAndIsUsed` — after migration, query `pg_indexes` for `idx_rag_document_needs_sync`, assert `indexdef` contains `WHERE (last_modified > last_synced OR last_synced IS NULL)`. Then insert 3 docs in different sync states and run `EXPLAIN (FORMAT JSON) SELECT id FROM rag_documents WHERE last_modified > last_synced OR last_synced IS NULL`, assert the plan uses `idx_rag_document_needs_sync`. **Business value:** sync loop performance at scale.
4. `TestRAGDocument_GINIndexOnExternalUserEmails` — same pattern: verify index exists, insert 3 docs with `external_user_emails`, run `EXPLAIN` on `WHERE 'x@y.com' = ANY(external_user_emails)`, assert GIN is used. **Business value:** ACL filter performance.
5. `TestRAGHierarchyNode_UniqueRawIDSource` — insert two rows with same `(raw_node_id, source)`, second insert must return a unique-violation error. **Business value:** prevents double-indexing same source.
6. `TestRAGDocumentByConnection_ConnectionCascade` — create InConnection + Document + DocByConn, delete InConnection, verify the DocByConn row is gone but the Document row survives. **Business value:** junction cleanup without losing the doc (same doc can be indexed by multiple connections).
7. `TestRAGHierarchyNodeType_IsValid` — table-driven: every enum constant returns true; "random_string" returns false. Pure, no DB. **Business value:** API input validation.
8. `TestDocumentSource_IsValid` — same pattern.

No separate test for "struct is created" or "field X exists" — banned by policy.

---

### Tranche 1B — Index attempts + errors + sync record

**Onyx references:**
- `IndexAttempt`: `backend/onyx/db/models.py:2189-2343`
- `IndexAttemptError`: `backend/onyx/db/models.py:2399-2438`
- `SyncRecord`: `backend/onyx/db/models.py:2440-2478`
- `IndexingStatus` enum: `backend/onyx/db/enums.py:38-62`
- `IndexingMode` enum: `backend/onyx/db/enums.py:88-91`
- `SyncType`, `SyncStatus` enums: `backend/onyx/db/enums.py:101-141`

**Files:**
- `internal/rag/model/index_attempt.go`
- `internal/rag/model/index_attempt_error.go`
- `internal/rag/model/sync_record.go`
- Append to `internal/rag/model/enums.go`: `IndexingStatus`, `IndexingMode`, `SyncType`, `SyncStatus`

#### `RAGIndexAttempt` (ports `IndexAttempt` at `models.py:2189-2343`)

All fields from Onyx 2198-2278 ported; `search_settings_id:2222` adapts to `EmbeddingModelID` FK (see Tranche 1G).

**Methods ported from `models.py:2331-2343`:**
- `IsFinished() bool` → `Status.IsTerminal()`
- `IsCoordinationComplete() bool` → `TotalBatches != nil && CompletedBatches >= *TotalBatches`

**Indexes (direct ports from `models.py:2304-2330`):**
- `idx_rag_index_attempt_latest_for_conn` — `(in_connection_id, time_created)`
- `idx_rag_index_attempt_conn_model_updated` — `(in_connection_id, embedding_model_id, time_updated DESC)`
- `idx_rag_index_attempt_conn_model_poll` — `(in_connection_id, embedding_model_id, status, time_updated DESC)`
- `idx_rag_index_attempt_active_coord` — `(in_connection_id, embedding_model_id, status)`
- Hiveloop addition: `idx_rag_index_attempt_heartbeat` partial on `(status, last_progress_time) WHERE status='in_progress'` — watchdog scan

**`IndexingStatus`** (verbatim `enums.py:38-62`): `not_started`, `in_progress`, `success`, `canceled`, `failed`, `completed_with_errors`. Methods `IsTerminal()`, `IsSuccessful()` ported.

**`IndexingMode`** (verbatim `enums.py:88-91`): `update`, `reindex`.

#### `RAGIndexAttemptError` (verbatim port of `models.py:2399-2438`)

All fields 2402-2432.

#### `RAGSyncRecord` (subset port of `models.py:2440-2478`)

**DEVIATION:** Onyx uses this for `DOCUMENT_SET`, `USER_GROUP`, `CONNECTOR_DELETION`, `PRUNING`, `EXTERNAL_PERMISSIONS`, `EXTERNAL_GROUP`. We port the last four only (we don't have DocumentSet or UserGroup).

`SyncType` enum — subset of `enums.py:101-111`: `connector_deletion`, `pruning`, `external_permissions`, `external_group`.
`SyncStatus` enum — verbatim `enums.py:113-127`: `in_progress`, `success`, `failed`, `canceled`, with `IsTerminal()`.

**Indexes:** direct ports of `models.py:2465-2477`.

#### Tranche 1B tests

**File:** `internal/rag/model/index_attempt_test.go`

1. `TestIndexingStatus_IsTerminal` — table-driven: `success`, `completed_with_errors`, `canceled`, `failed` → true; `not_started`, `in_progress` → false. Pure. **Business value:** scheduler depends on this to decide retry-vs-skip.
2. `TestIndexingStatus_IsSuccessful` — `success`, `completed_with_errors` → true; everything else → false. Pure.
3. `TestRAGIndexAttempt_IsCoordinationComplete` — table-driven: `{TotalBatches: nil}` → false; `{TotalBatches: 5, CompletedBatches: 4}` → false; `{TotalBatches: 5, CompletedBatches: 5}` → true; `{TotalBatches: 5, CompletedBatches: 6}` → true. Pure. **Business value:** signals end of docprocessing phase.
4. `TestRAGIndexAttempt_HeartbeatPartialIndex` — create 3 attempts (status `not_started`, `in_progress`, `success`). `EXPLAIN` the watchdog query `SELECT * FROM rag_index_attempts WHERE status='in_progress' AND last_progress_time < NOW() - INTERVAL '30 minutes'`. Assert `idx_rag_index_attempt_heartbeat` is used. **Business value:** watchdog scales to production attempt volume.
5. `TestRAGIndexAttemptError_AttemptCascadeDelete` — create IndexAttempt + Error, delete Attempt, verify Error gone. **Business value:** error log lifecycle tied to attempt.
6. `TestSyncType_Valid` / `TestSyncStatus_IsTerminal` — table-driven.

---

### Tranche 1C — Sync state + connection config + search settings

**Onyx references:**
- `ConnectorCredentialPair` (sync subset): `backend/onyx/db/models.py:723-837`
- `Connector.refresh_freq / prune_freq`: `backend/onyx/db/models.py:1886-1890`
- `ConnectorCredentialPairStatus` enum: `backend/onyx/db/enums.py:180-205`
- `AccessType` enum: `backend/onyx/db/enums.py:207-211`
- `ProcessingMode` enum: `backend/onyx/db/enums.py:93-98`
- `SearchSettings`: `backend/onyx/db/models.py:2052-2187`
- `Connector.validate_refresh_freq / validate_prune_freq`: `backend/onyx/db/models.py:1919-1929`

**ARCHITECTURAL NOTE:** Onyx's `ConnectorCredentialPair` bundles identity + schedule + sync state. We split: identity stays on Hiveloop's `InConnection`; schedule moves to `RAGConnectionConfig`; sync state moves to `RAGSyncState`. Per-org embedding config goes to `RAGSearchSettings`.

**Files:**
- `internal/rag/model/sync_state.go`
- `internal/rag/model/connection_config.go`
- `internal/rag/model/search_settings.go`
- Append to `enums.go`: `RAGConnectionStatus`, `AccessType`, `ProcessingMode`, `EmbeddingPrecision`

#### `RAGSyncState` (adapts `CCPair` sync columns at `models.py:723-837`)

One-to-one with `InConnection`, keyed by `in_connection_id` (unique). Fields ported from Onyx 739-800: `status`, `in_repeated_error_state`, `access_type`, `auto_sync_options`, `last_time_perm_sync`, `last_time_external_group_sync`, `last_successful_index_time`, `last_pruned`, `last_time_hierarchy_fetch`, `total_docs_indexed`, `indexing_trigger`, `processing_mode`, `deletion_failure_message`, `creator_id`.

Skipped: `connector_id`, `credential_id`, `name` — all live on `InConnection`.

**`RAGConnectionStatus`** (verbatim `enums.py:180-205`): `SCHEDULED`, `INITIAL_INDEXING`, `ACTIVE`, `PAUSED`, `DELETING`, `INVALID`. Methods `ActiveStatuses()`, `IndexableStatuses()`, `IsActive()` ported.

**`AccessType`** (verbatim `enums.py:207-211`): `public`, `private`, `sync`.

**`ProcessingMode`** (verbatim `enums.py:93-98`): `REGULAR`, `FILE_SYSTEM`, `RAW_BINARY`.

**Indexes:**
- Unique on `in_connection_id` (one sync state per connection)
- `idx_rag_sync_state_org_status` on `(org_id, status)`
- `idx_rag_sync_state_last_pruned` — port of Onyx `index=True` on `last_pruned` at `models.py:784`

#### `RAGConnectionConfig` (adapts `Connector.refresh_freq/prune_freq` + `CCPair.auto_sync_options`)

Fields per plan: `InConnectionID` PK, `OrgID`, `IngestConfig` (JSONB, mirrors `Connector.connector_specific_config:1872`), `RefreshFreqSeconds`, `PruneFreqSeconds`, `PermSyncFreqSeconds` (Hiveloop addition), `ExternalGroupSyncFreqSeconds` (Hiveloop addition), `IndexingStart`, `KGProcessingEnabled` (reserved).

**Validation methods** ported from `backend/onyx/db/models.py:1919-1929`:
- `ValidateRefreshFreq() error` — returns error if `RefreshFreqSeconds != nil && *RefreshFreqSeconds < 60`
- `ValidatePruneFreq() error` — returns error if `PruneFreqSeconds != nil && *PruneFreqSeconds < 300`

#### `RAGSearchSettings` (adapts `SearchSettings` at `models.py:2052-2187`)

**DEVIATION:** per-org instead of global; drop `IndexModelStatus` / `switchover_type` machinery (one-model-per-org invariant).

Fields: `OrgID` PK, `EmbeddingModelID` FK, `EmbeddingDim`, `Normalize`, `QueryPrefix`, `PassagePrefix`, `EmbeddingPrecision`, `ReducedDimension`, `MultipassIndexing`, `RerankerModelID` (Hiveloop add), `HybridAlpha` (Hiveloop add, default 0.7), `IndexName`, `EnableContextualRAG` (reserved), `ContextualRAGLLMName` (reserved), `ContextualRAGLLMProvider` (reserved).

**`EmbeddingPrecision`** — port of `enums.py:213-241`.

#### Tranche 1C tests

**File:** `internal/rag/model/sync_state_test.go`, `connection_config_test.go`, `search_settings_test.go`

1. `TestRAGSyncState_UniquePerInConnection` — create one RAGSyncState, then attempt a second for the same `in_connection_id`, expect unique-violation. **Business value:** invariant that each connection has exactly one sync state — the three-loop scheduler depends on this.
2. `TestRAGSyncState_InConnectionCascade` — delete InConnection, verify RAGSyncState row gone. **Business value:** connection removal cleans up state.
3. `TestRAGConnectionStatus_IsActive` — table-driven: `SCHEDULED`, `INITIAL_INDEXING`, `ACTIVE` → true; `PAUSED`, `DELETING`, `INVALID` → false. Pure. **Business value:** scheduler's "should I run this?" gate.
4. `TestRAGConnectionStatus_IndexableStatuses` — port of Onyx `enums.py:195-201` behavior (superset of Active including Paused).
5. `TestRAGConnectionConfig_ValidateRefreshFreq` — table-driven: `nil → ok`, `59 → err`, `60 → ok`, `3600 → ok`. Pure. **Business value:** admin API input validation; matches Onyx rule at `models.py:1923`.
6. `TestRAGConnectionConfig_ValidatePruneFreq` — `nil → ok`, `299 → err`, `300 → ok`, `86400 → ok`. Pure.
7. `TestRAGSearchSettings_OrgPK` — create settings for org A, attempt insert for org A again, expect PK violation. **Business value:** one settings row per org invariant.
8. `TestRAGSearchSettings_EmbeddingModelFK` — attempt insert with non-existent `embedding_model_id`, expect FK violation. **Business value:** catch typo'd model IDs at write time, not at search time.

---

### Tranche 1D — External groups + ACL prefix helpers

**Onyx references:**
- `User__ExternalUserGroupId`: `backend/onyx/db/models.py:4320-4350`
- `PublicExternalUserGroup`: `backend/onyx/db/models.py:4352-4380`
- `prefix_user_email`, `prefix_user_group`, `prefix_external_group`, `build_ext_group_name_for_onyx`: `backend/onyx/access/utils.py` (27 lines, verbatim port)
- `PUBLIC_DOC_PAT`: `backend/onyx/configs/constants.py:27`

**Files:**
- `internal/rag/model/external_user_group.go`
- `internal/rag/model/user_external_user_group.go`
- `internal/rag/model/public_external_user_group.go`
- `internal/rag/acl/prefix.go`

#### `RAGExternalUserGroup` (Hiveloop addition — no direct Onyx analog)

**DEVIATION:** Onyx has no table for group metadata; it's derived on-demand. We persist because (a) admin UI needs display names without source calls, (b) the stale-sweep pattern needs rows to sweep.

Fields: `ID`, `OrgID`, `InConnectionID` FK CASCADE, `ExternalUserGroupID` (source-prefixed, lowercased — see `BuildExtGroupName`), `DisplayName`, `GivesAnyoneAccess`, `MemberEmails` (denormalized for debug), `UpdatedAt`.

Unique: `(in_connection_id, external_user_group_id)`.

#### `RAGUserExternalUserGroup` (verbatim port of `User__ExternalUserGroupId` at `models.py:4320-4350`)

`cc_pair_id → in_connection_id` is the only adaptation. Fields 4328-4336 ported verbatim. Indexes `ix_user_external_group_cc_pair_stale` + `ix_user_external_group_stale` (Onyx 4338-4349) ported as `idx_rag_user_external_group_conn_stale` / `idx_rag_user_external_group_stale`.

#### `RAGPublicExternalUserGroup` (verbatim port of `PublicExternalUserGroup` at `models.py:4352-4380`)

Same `cc_pair_id → in_connection_id` adaptation. Indexes ported.

#### `internal/rag/acl/prefix.go` (verbatim port of `backend/onyx/access/utils.py`)

```go
// PrefixUserEmail — port of onyx/access/utils.py:4-8
func PrefixUserEmail(email string) string { return "user_email:" + email }

// PrefixUserGroup — port of onyx/access/utils.py:11-14
func PrefixUserGroup(name string) string { return "group:" + name }

// PrefixExternalGroup — port of onyx/access/utils.py:17-19
func PrefixExternalGroup(name string) string { return "external_group:" + name }

// BuildExtGroupName — port of onyx/access/utils.py:22-27
// Source-prefixed AND lowercased per Onyx comment 24-26.
func BuildExtGroupName(extGroupName string, source DocumentSource) string {
    return strings.ToLower(string(source) + "_" + extGroupName)
}

// PublicDocPat — port of onyx/configs/constants.py:27
const PublicDocPat = "PUBLIC"
```

#### Tranche 1D tests

**File:** `internal/rag/acl/prefix_test.go`

1. `TestPrefixUserEmail` — input `alice@example.com` → `user_email:alice@example.com`. Table-driven including empty string and unicode. **Business value:** ACL filter strings must byte-match what indexer writes; any mismatch = 0 results.
2. `TestPrefixUserGroup` — same pattern.
3. `TestPrefixExternalGroup` — same.
4. `TestBuildExtGroupName_LowercasesAndPrefixes` — `BuildExtGroupName("Backend", DocumentSourceGithub) → "github_backend"`. Also test `"MixedCase"`, `"UPPER"`, source with digits (none in our enum but still robust).
5. `TestBuildExtGroupName_Idempotent` — `BuildExtGroupName(BuildExtGroupName("x", src), src)` → a specific expected string. Not actually the same as one call (because of double-prefix) — this test pins the non-idempotent behavior so callers are aware. **Business value:** prevents accidental double-prefixing.

All pure, no DB.

**File:** `internal/rag/model/external_user_group_test.go`

6. `TestRAGExternalUserGroup_UniquePerConnection` — insert two with same `(in_connection_id, external_user_group_id)`, expect violation.
7. `TestRAGExternalUserGroup_InConnectionCascade` — delete InConnection, rows gone.
8. `TestRAGUserExternalUserGroup_CompositePK` — insert same `(user_id, external_user_group_id, in_connection_id)` twice, expect violation.
9. `TestRAGUserExternalUserGroup_StaleSweepPattern` — **the big one**:
    - Setup: org + user + connection + 3 groups
    - Insert 3 `RAGUserExternalUserGroup` rows with `stale=false`
    - Simulate sync start: `UPDATE ... SET stale=true WHERE in_connection_id=X`
    - Simulate fresh writes: 2 upserts (one overlapping, one new) with `stale=false`
    - Simulate sync end: `DELETE WHERE in_connection_id=X AND stale=true`
    - Assertions: exactly 2 rows remain, `stale=false`, matching the fresh writes, the old unrefreshed row is gone
    - **Business value:** this IS the external group sync pattern. If it's broken, users see results from deleted GitHub teams.
10. `TestRAGPublicExternalUserGroup_StaleSweepPattern` — same pattern against `rag_public_external_user_groups`.
11. `TestRAGPublicExternalUserGroup_InConnectionCascade`.

---

### Tranche 1E — External identity + OAuthAccount extension

**Onyx references:**
- `OAuthAccount`: `backend/onyx/db/models.py:299-303` (Hiveloop's analog at `internal/model/oauth_account.go`)
- External identity is a Hiveloop addition — Onyx derives it on-demand inside perm sync code

**Files:**
- `internal/rag/model/external_identity.go`
- Modify `internal/model/oauth_account.go`

#### `OAuthAccount` extension

Add to existing struct: `ProviderUserEmail *string`, `ProviderUserLogin *string`, `VerifiedEmails pq.StringArray`, `LastSyncedAt *time.Time`. All nullable — migration-safe.

#### `RAGExternalIdentity` (Hiveloop addition)

Fields: `ID`, `OrgID`, `UserID` FK CASCADE, `InConnectionID` FK CASCADE, `Provider`, `ExternalUserID`, `ExternalUserLogin`, `ExternalUserEmails`, `UpdatedAt`.

Unique: `(user_id, in_connection_id)` and `(provider, external_user_id, org_id)`.

#### Tranche 1E tests

**File:** `internal/rag/model/external_identity_test.go`

1. `TestRAGExternalIdentity_UniquePerUserConnection` — two rows for same `(user_id, in_connection_id)` → violation. **Business value:** one identity per user per connection invariant.
2. `TestRAGExternalIdentity_UniqueProviderExtIDInOrg` — two rows with same `(provider, external_user_id, org_id)` → violation. Same tuple in different orgs → allowed. **Business value:** cross-org isolation; same GitHub user can belong to two orgs.
3. `TestRAGExternalIdentity_UserCascade` — delete user → identity rows gone.
4. `TestRAGExternalIdentity_InConnectionCascade` — delete connection → identity rows gone.

**File:** `internal/model/oauth_account_test.go` (add to existing)

5. `TestOAuthAccount_MigrationAddsFieldsWithoutBreakingExistingRows` — this is critical for a live system. Procedure:
    - Simulate pre-migration state: drop the new columns if they exist, insert an OAuthAccount with only the old columns
    - Run `model.AutoMigrate(db)`
    - Fetch the pre-existing row, verify it's intact and new columns are null
    - Update the row with new fields, verify persist-and-reload works
    - **Business value:** shipping this migration against production must not corrupt existing OAuth sessions.

---

### Tranche 1G — Embedding model catalog + seed

**Onyx references:**
- `SearchSettings.model_name/dim/provider_type`: `backend/onyx/db/models.py:2056-2069`
- `CloudEmbeddingProvider`: `backend/onyx/db/models.py:3178-3197`
- `EmbeddingProvider` enum: `backend/shared_configs/enums.py:4-`

**DEVIATION:** Onyx mixes model config into `SearchSettings` + `CloudEmbeddingProvider` rows (which carry API keys). We split: a Go-side code registry, plus a read-only DB catalog seeded at startup so admin UI can enumerate models without compile-time coupling.

**Files:**
- `internal/rag/model/embedding_model.go`
- `internal/rag/embedder/registry.go` (Go-side source-of-truth for seeded rows; full embedder implementations come in Phase 2C)
- `internal/rag/embedder/seed.go` (idempotent `SeedRegistry(db *gorm.DB) error`)

#### `RAGEmbeddingModel`

Fields: `ID` PK (e.g. `siliconflow:qwen3-embedding-4b`), `Provider`, `ModelName`, `Dimension`, `MaxInputTokens`, `DatasetName` (derived, e.g. `rag_chunks__siliconflow_qwen3_embedding_4b__2560`), `QueryPrefix`, `PassagePrefix`, `PricingPer1MTokensUSD`, `IsActive`, `CreatedAt`, `UpdatedAt`.

#### Seeded rows

| ID | Provider | Model | Dim | Notes |
|---|---|---|---|---|
| `siliconflow:qwen3-embedding-4b` | siliconflow | Qwen/Qwen3-Embedding-4B | 2560 | **Default for new orgs** |
| `siliconflow:qwen3-embedding-0.6b` | siliconflow | Qwen/Qwen3-Embedding-0.6B | 1024 | Cheap tier |
| `siliconflow:qwen3-embedding-8b` | siliconflow | Qwen/Qwen3-Embedding-8B | 4096 | Quality tier |
| `openai:text-embedding-3-small` | openai | text-embedding-3-small | 1536 | Eval baseline |
| `openai:text-embedding-3-large` | openai | text-embedding-3-large | 3072 | Eval baseline |

#### Tranche 1G tests

**File:** `internal/rag/embedder/seed_test.go`

1. `TestSeedRegistry_SeedsAllModels` — call `SeedRegistry(db)`, query `SELECT id FROM rag_embedding_models`, assert the 5 expected IDs. **Business value:** admin UI depends on these rows.
2. `TestSeedRegistry_Idempotent` — call `SeedRegistry(db)` twice, assert 5 rows (not 10). **Business value:** runs on every boot; must not duplicate.
3. `TestSeedRegistry_UpdatesOnRegistryChange` — simulate registry change by calling `SeedRegistry`, then mutating a registry entry in Go, call again, assert the DB row reflects the new `PricingPer1MTokensUSD`. **Business value:** code-side registry is source of truth; DB is cache.
4. `TestRAGEmbeddingModel_DatasetNameDerivation` — pure function test on the `deriveDatasetName(provider, modelName, dim) string` helper. Table-driven: `("siliconflow", "Qwen/Qwen3-Embedding-4B", 2560) → "rag_chunks__siliconflow_qwen3_embedding_4b__2560"`; verify slashes/uppercase are normalized. **Business value:** LanceDB dataset naming must be deterministic and collision-free across models.

---

### Tranche 1F — Registration, AutoMigrate wiring, full schema verification

**Owner:** 1 agent, runs AFTER 1A–1E + 1G have all merged. Sequential — this is the single merge point.

**Files modified:**
- `internal/rag/register.go` — fills in `AutoMigrate`
- `internal/model/org.go` — confirms `rag.AutoMigrate(db)` call (stub from Phase 0)

**Work:**

1. Populate `rag.AutoMigrate`:

```go
func AutoMigrate(db *gorm.DB) error {
    if err := db.AutoMigrate(
        // 1A
        &RAGDocument{},
        &RAGHierarchyNode{},
        &RAGDocumentByConnection{},
        &RAGHierarchyNodeByConnection{},
        // 1B
        &RAGIndexAttempt{},
        &RAGIndexAttemptError{},
        &RAGSyncRecord{},
        // 1C
        &RAGSyncState{},
        &RAGConnectionConfig{},
        &RAGSearchSettings{},
        // 1D
        &RAGExternalUserGroup{},
        &RAGUserExternalUserGroup{},
        &RAGPublicExternalUserGroup{},
        // 1E
        &RAGExternalIdentity{},
        // 1G
        &RAGEmbeddingModel{},
    ); err != nil {
        return err
    }

    // Manual SQL for partial + GIN indexes.
    // Pattern matches existing internal/model/org.go:AutoMigrate approach.

    // Port of backend/onyx/db/models.py:1058-1062
    db.Exec(`CREATE INDEX IF NOT EXISTS idx_rag_document_needs_sync
             ON rag_documents (id)
             WHERE last_modified > last_synced OR last_synced IS NULL`)

    db.Exec(`CREATE INDEX IF NOT EXISTS idx_rag_document_ext_emails
             ON rag_documents USING GIN (external_user_emails)`)
    db.Exec(`CREATE INDEX IF NOT EXISTS idx_rag_document_ext_group_ids
             ON rag_documents USING GIN (external_user_group_ids)`)

    db.Exec(`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_heartbeat
             ON rag_index_attempts (status, last_progress_time)
             WHERE status = 'in_progress'`)

    return embedder.SeedRegistry(db)
}
```

2. Confirm Phase 0's call in `internal/model/org.go:AutoMigrate` is correct.

#### Tranche 1F tests — the critical suite

**File:** `internal/rag/automigrate_test.go`

This file owns verification of the entire schema. Every test hits real Postgres.

1. `TestAutoMigrate_CreatesEveryTable` — after `ConnectTestDB(t)`, query `SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name LIKE 'rag_%'`, assert the exact set: `{rag_documents, rag_hierarchy_nodes, rag_document_by_connections, rag_hierarchy_node_by_connections, rag_index_attempts, rag_index_attempt_errors, rag_sync_records, rag_sync_states, rag_connection_configs, rag_search_settings, rag_external_user_groups, rag_user_external_user_groups, rag_public_external_user_groups, rag_external_identities, rag_embedding_models}`. Any missing = fail. Any extra = fail (catches accidentally-added tables).

2. `TestAutoMigrate_CreatesEveryExpectedIndex` — query `pg_indexes` filtered to our tables. Assert a concrete list of index names. For each, also assert the `indexdef` contains expected shape (partial WHERE clauses, GIN method, etc.). Concrete assertions:
    - `idx_rag_document_needs_sync.indexdef` contains `WHERE ((last_modified > last_synced) OR (last_synced IS NULL))`
    - `idx_rag_document_ext_emails.indexdef` contains `USING gin`
    - `idx_rag_index_attempt_heartbeat.indexdef` contains `WHERE (status = 'in_progress'::text)`
    - All direct-port index names match the port mapping documented above

3. `TestAutoMigrate_AllFKConstraintsInPlace` — query `information_schema.table_constraints WHERE constraint_type='FOREIGN KEY' AND table_name LIKE 'rag_%'`. Assert concrete list per the tranche specs. For each, verify `ON DELETE` action (CASCADE vs SET NULL) via `information_schema.referential_constraints.delete_rule`.

4. `TestAutoMigrate_Idempotent` — call `rag.AutoMigrate(db)` a second time after the initial migration runs. Assert no error. Re-run test 2 and assert index count is unchanged (no duplicates).

5. `TestAutoMigrate_OrgFullCascadeDelete` — **the end-to-end cleanup test**:
    - Create org + user + integration + connection
    - Create one row in each RAG table that's org-scoped: doc, hierarchy node, doc-by-conn, hier-by-conn, index attempt + error, sync record, sync state, connection config, search settings, external user group, user-external-group, public-external-group, external identity
    - Delete the org
    - Assert: all 14+ rows gone via `SELECT COUNT(*) FROM <table> WHERE org_id=X` (where applicable) or junction-based queries
    - **Business value:** this is the single most important test in Phase 1. Proves GDPR-compliant org deletion works end-to-end against the full schema.

6. `TestAutoMigrate_SeedsEmbeddingRegistry` — post-migrate, assert the 5 `rag_embedding_models` rows exist with correct dims/providers. (Overlaps slightly with 1G's seed test but verifies it runs as part of AutoMigrate, not just when called directly.)

7. `TestAutoMigrate_PartialIndexActuallyUsed_Document` — insert 100 rows into `rag_documents` with varied `last_modified`/`last_synced`. Run `ANALYZE rag_documents`. `EXPLAIN` the sync query. Assert the plan uses `idx_rag_document_needs_sync`. **Business value:** proves planner will pick the partial index at realistic row counts (100 is enough to trip out of seq-scan territory for a partial index).

8. `TestAutoMigrate_PartialIndexActuallyUsed_Watchdog` — same pattern for the watchdog index on `rag_index_attempts`.

9. `TestAutoMigrate_GINIndexActuallyUsed` — insert docs with varied `external_user_emails`, `ANALYZE`, `EXPLAIN WHERE 'x@y.com' = ANY(external_user_emails)`, assert GIN is used.

---

## CI integration

Phase 1 introduces a new CI stage `test-rag` that:
- Runs `make test-services-up` (Postgres + MinIO)
- Runs `go test -race -cover ./internal/rag/...`
- Fails if coverage on any package with branchful code is < 100%
- Runs `make test-services-down`

Add to `.github/workflows/...` (confirm Hiveloop's CI lives there) alongside existing Hiveloop test workflows.

---

## Launch order

| Step | Agents | Mode | Blocks | Est. wall clock |
|---|---|---|---|---|
| 1 | Phase 0 scaffolding + test harness + LanceDB spike (1 agent) | sequential | everything | 3–4 h |
| 2 | Phase 1 tranches 1A, 1B, 1C, 1D, 1E, 1G (6 agents) | **parallel in worktrees** | 1F | 4–6 h |
| 3 | Phase 1 tranche 1F (1 agent) | sequential, after 2 | — | 2–3 h |
| 4 | Human review + merge | human | Phase 2 kickoff | 1 day |

**Definition of done** for each tranche:
- Files exist, gorm tags match port spec
- `go build ./...` passes
- All tests in tranche-owned test files pass under `go test -race -cover`
- Coverage 100% on branchful code in tranche-owned packages
- No mocks used (except embedder/reranker interfaces — and Phase 1 doesn't touch those)
- Integration tests run against real Postgres from the Hiveloop test docker-compose

**Total Phase 0 + Phase 1 estimate:** 2–3 engineering days with agent-assisted execution + parallel tranches.

---

## Deferred questions (answer before Phase 2)

1. Per-org billing metering for embedding + storage costs — Polar integration pattern?
2. External identity reconciliation UX when GitHub commit email ≠ Hiveloop login email — admin UI to manually link?
3. Rate-limit budget per-provider per-org — global pool or per-org cap?

---

## Onyx symbol-to-Hiveloop mapping (Phase 1 scope)

| Onyx symbol | Onyx location | Hiveloop symbol | Hiveloop location |
|---|---|---|---|
| `Document` | `models.py:939` | `RAGDocument` | `internal/rag/model/document.go` |
| `HierarchyNode` | `models.py:839` | `RAGHierarchyNode` | `internal/rag/model/hierarchy_node.go` |
| `DocumentByConnectorCredentialPair` | `models.py:2512` | `RAGDocumentByConnection` | `internal/rag/model/document_by_connection.go` |
| `HierarchyNodeByConnectorCredentialPair` | `models.py:2480` | `RAGHierarchyNodeByConnection` | `internal/rag/model/hierarchy_node_by_connection.go` |
| `IndexAttempt` | `models.py:2189` | `RAGIndexAttempt` | `internal/rag/model/index_attempt.go` |
| `IndexAttemptError` | `models.py:2399` | `RAGIndexAttemptError` | `internal/rag/model/index_attempt_error.go` |
| `SyncRecord` | `models.py:2440` | `RAGSyncRecord` | `internal/rag/model/sync_record.go` |
| `ConnectorCredentialPair` (sync state subset) | `models.py:723` | `RAGSyncState` | `internal/rag/model/sync_state.go` |
| `Connector.refresh_freq/prune_freq` + `CCPair.auto_sync_options` | `models.py:1886-1890`, `models.py:768` | `RAGConnectionConfig` | `internal/rag/model/connection_config.go` |
| `SearchSettings` | `models.py:2052` | `RAGSearchSettings` | `internal/rag/model/search_settings.go` |
| `User__ExternalUserGroupId` | `models.py:4320` | `RAGUserExternalUserGroup` | `internal/rag/model/user_external_user_group.go` |
| `PublicExternalUserGroup` | `models.py:4352` | `RAGPublicExternalUserGroup` | `internal/rag/model/public_external_user_group.go` |
| (Hiveloop addition) | — | `RAGExternalUserGroup` | `internal/rag/model/external_user_group.go` |
| (Hiveloop addition) | — | `RAGExternalIdentity` | `internal/rag/model/external_identity.go` |
| `SearchSettings.model_name/dim/provider_type` | `models.py:2056-2069` | `RAGEmbeddingModel` | `internal/rag/model/embedding_model.go` |
| `OAuthAccount` | `models.py:299` | extension | `internal/model/oauth_account.go` |
| `IndexingStatus` | `enums.py:38` | `IndexingStatus` | `internal/rag/model/enums.go` |
| `IndexingMode` | `enums.py:88` | `IndexingMode` | `internal/rag/model/enums.go` |
| `ProcessingMode` | `enums.py:93` | `ProcessingMode` | `internal/rag/model/enums.go` |
| `SyncType` (subset) | `enums.py:101` | `SyncType` | `internal/rag/model/enums.go` |
| `SyncStatus` | `enums.py:113` | `SyncStatus` | `internal/rag/model/enums.go` |
| `ConnectorCredentialPairStatus` | `enums.py:180` | `RAGConnectionStatus` | `internal/rag/model/enums.go` |
| `AccessType` | `enums.py:207` | `AccessType` | `internal/rag/model/enums.go` |
| `EmbeddingPrecision` | `enums.py:213` | `EmbeddingPrecision` | `internal/rag/model/enums.go` |
| `HierarchyNodeType` | `enums.py:306` | `HierarchyNodeType` | `internal/rag/model/enums.go` |
| `DocumentSource` | `constants.py:205` | `DocumentSource` | `internal/rag/model/enums.go` |
| `PUBLIC_DOC_PAT` | `constants.py:27` | `acl.PublicDocPat` | `internal/rag/acl/prefix.go` |
| `prefix_user_email` | `access/utils.py:4` | `acl.PrefixUserEmail` | `internal/rag/acl/prefix.go` |
| `prefix_user_group` | `access/utils.py:11` | `acl.PrefixUserGroup` | `internal/rag/acl/prefix.go` |
| `prefix_external_group` | `access/utils.py:17` | `acl.PrefixExternalGroup` | `internal/rag/acl/prefix.go` |
| `build_ext_group_name_for_onyx` | `access/utils.py:22` | `acl.BuildExtGroupName` | `internal/rag/acl/prefix.go` |

---

## What Phase 1 deliberately DOES NOT touch

| Onyx concern | Status |
|---|---|
| `UserGroup`, `User__UserGroup`, `UserGroup__ConnectorCredentialPair` | Not porting — Hiveloop RBAC stays |
| `DocumentSet`, `DocumentSet__*` | Deferred |
| `Persona`, `Tool`, `ChatSession`, `ChatMessage`, `SearchDoc` | Never — out of scope |
| `Tag`, `Document__Tag` | Deferred |
| `KGEntity*`, `KGRelationship*`, `KGTerm` | Not porting |
| `Notification`, `StandardAnswer*`, `SlackChannelConfig*`, `SlackBot*` | Not porting |
| `FederatedConnector*` | Not porting |
| `OpenSearch*MigrationRecord` | N/A (LanceDB) |
| `PersonalAccessToken`, `ApiKey`, `AccessToken` | N/A (Hiveloop has its own) |
| `LLMProvider`, `ModelConfiguration`, `VoiceProvider`, `CloudEmbeddingProvider`, etc. | N/A |
| `UsageReport` | Deferred |
| MCP tables | N/A — Hiveloop has its own |
