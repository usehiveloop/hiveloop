# Testing philosophy — non-negotiable

Every future agent reads this before writing a test for the RAG
subsystem.

Hiveloop already runs tests against real services (see
`internal/middleware/integration_test.go`: real Postgres at
`localhost:5433`, real `model.AutoMigrate`). This is the only pattern we
use.

## Hard rules

1. **Mocking is permitted ONLY for the embedder and reranker.** Both make
   paid external API calls; the embedder package ships their interfaces with an
   in-memory deterministic fake. Nothing else gets mocked — not Postgres,
   not Redis, not MinIO, not LanceDB, not Nango, not HTTP handlers.
2. **Integration-first.** Anything touching infrastructure is tested
   against a real instance of that infrastructure, via
   `docker-compose.test.yml` (Postgres + Redis + MinIO + LanceDB).
3. **Every test verifies business behavior, not framework behavior.** If
   the test could only fail because gorm/chi/asynq broke, do not write
   it. Concrete bans:
   - Assert that a field exists on a struct (the compiler proves this)
   - Assert that a primary key is a primary key
   - Assert that gorm reads what gorm wrote
   - Tests using `sqlmock`, `go-sqlmock`, `testify/mock` against DB code
   - Tests that set up a fixture, call a function, and assert the
     fixture came back
   - Tests with names like `TestCreatesSuccessfully` that don't pin
     behavior
4. **Pure-logic functions are tested directly — no mocks involved because
   no infrastructure is involved.** ACL prefix helpers, enum
   `IsTerminal()`, validation rules — these are pure Go and get pure
   table-driven tests. That is not a deviation from rule 1; there is
   nothing to mock.
5. **Coverage target: 100% of branchful code.** `go test -cover` must
   report 100% for every `internal/rag/**` package that has any
   conditional logic. Pure-data structs with no methods contribute zero
   to both numerator and denominator; they are not "uncovered."
6. **Every integration test cleans up what it created.** Use
   `t.Cleanup(...)` with explicit `DELETE` by `org_id` (mirror the
   pattern in `internal/middleware/integration_test.go:63-69`). No "just
   drop the schema at the end" shortcuts.
7. **No flaky tests.** If a test needs an external service to be up and
   it isn't, it fails loudly with "run `make test-services-up` first" —
   it does not skip, does not retry, does not timeout silently.

## Test harness (built in Phase 0)

Phase 0 delivers two files that every subsequent tranche uses:

- `internal/rag/testhelpers/db.go` — `ConnectTestDB(t *testing.T) *gorm.DB`
  that opens the existing Hiveloop test Postgres, runs
  `model.AutoMigrate` + `rag.AutoMigrate`, registers `t.Cleanup` to
  close. Parallels `internal/middleware/integration_test.go:31-60`.
- `internal/rag/testhelpers/fixtures.go` — typed fixture constructors:
  `NewTestOrg(t, db)`, `NewTestUser(t, db, orgID)`,
  `NewTestInIntegration(t, db)`,
  `NewTestInConnection(t, db, orgID, userID, integID)`. Each registers
  cleanup. Mirrors Onyx's
  `backend/tests/integration/common_utils/managers/` pattern.

## What "real business value" means per test type

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

## What is explicitly NOT tested (waste of time)

- Field presence, struct zero-value construction, pointer vs value
  semantics
- That gorm tags produce the expected DDL (we test the DDL directly via
  `pg_indexes` / `information_schema`)
- That uuid generation works
- Default values baked in by Postgres (we test them once via the
  migration inspection test, not per-model)
