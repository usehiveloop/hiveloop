# Phase 0 Spike — LanceDB Go binding research

**As of: April 22, 2026.**

> **Important correction after execution.** The API surface catalogued
> below was read from the upstream `main` branch. The actually-released
> `v0.1.2` (the only tag with prebuilt native artifacts, dated
> 2025-09-30) is a strict subset — FTS, `list<string>` IO, and custom S3
> endpoint wiring are either missing or stubbed in v0.1.2's Rust FFI.
> See `SPIKE_RESULT.md` for the empirical per-operation outcome.

## Candidates evaluated

### 1. `github.com/lancedb/lancedb-go` — OFFICIAL Go bindings

- **URL:** https://github.com/lancedb/lancedb-go
- **Go package:** `github.com/lancedb/lancedb-go/pkg/lancedb` +
  `github.com/lancedb/lancedb-go/pkg/contracts`
- **Latest version:** `v0.1.2` (published 2025-09-30 on pkg.go.dev; the
  `main` branch has commits as recent as 2026-04-13, so the project is
  actively maintained).
- **License:** Apache-2.0.
- **Maturity:** early. `v0.x` release series. Single-digit open issues
  at the time of this writing. Not yet a "battle-hardened v1", but the
  Rust core it wraps is production-grade.
- **Implementation.** CGO shim over the Rust `lancedb` crate. Native
  static libraries (`liblancedb_go.a`) must be downloaded per platform
  via `scripts/download-artifacts.sh`; `CGO_CFLAGS` and `CGO_LDFLAGS`
  must be set at build time. Supported platforms: macOS amd64, macOS
  arm64, Linux amd64, Linux arm64, Windows amd64.
- **API surface exposed** (from `pkg/contracts/i_connection.go`,
  `i_table.go`, `i_schema.go`, `types.go`, `storage_keys.go`; verified
  by reading source at `/tmp/lancedb-go/`):
  - `lancedb.Connect(ctx, uri, *ConnectionOptions) (IConnection, error)`
  - `ConnectionOptions.StorageOptions map[string]string` with
    well-known keys for S3 / MinIO / R2 / GCS / Azure (see
    `StorageAccessKeyID`, `StorageSecretAccessKey`,
    `StorageEndpoint`, `StorageAllowHTTP`,
    `StorageVirtualHostedStyleRequest`).
  - `IConnection.CreateTable(ctx, name, ISchema) (ITable, error)`,
    `OpenTable`, `TableNames`, `DropTable`.
  - `lancedb.NewSchema(*arrow.Schema)` and
    `lancedb.NewSchemaBuilder()`. Schema uses Apache Arrow types
    (`github.com/apache/arrow/go/v17/arrow`), so `FixedSizeListOf` (for
    vector columns) and `ListOf(BinaryTypes.String)` (for `list<string>`
    ACL arrays) are supported out of the box.
  - `ITable.Add(ctx, arrow.Record, *AddDataOptions)` and
    `AddRecords(ctx, []arrow.Record, *AddDataOptions)` — batched insert
    via Arrow IPC.
  - `ITable.Update(ctx, filter string, updates map[string]interface{})`
    — this is the metadata-only update path. Column-scoped, no vector
    rewrite. CRITICAL for Phase 3 perm-sync.
  - `ITable.Delete(ctx, filter)` — SQL predicate delete.
  - `ITable.VectorSearch(ctx, col, []float32, k)` and
    `VectorSearchWithFilter(ctx, col, []float32, k, filter)`.
  - `ITable.FullTextSearch(ctx, col, query)` and
    `FullTextSearchWithFilter(ctx, col, query, filter)`, enabled via
    `CreateIndex([]string{col}, IndexTypeFts)`.
  - Scalar index types (BTree, Bitmap, LabelList) plus vector index
    types (IVF-PQ, IVF-Flat, HNSW-PQ, HNSW-SQ).
- **Filter syntax.** LanceDB uses DataFusion SQL as its filter engine;
  the examples
  (`examples/hybrid_search/hybrid_search.go:420-476` and similar)
  confirm `AND`, `OR`, `IN (...)`, `=`, `<`, `>`, `LIKE` work directly.
  For `list<string>` contains we rely on DataFusion's `array_has`.

### 2. `github.com/eto-ai/lance-go`

- **URL:** Search result mentions the name, but the repo does not
  appear in the official `lancedb` GitHub org nor in the actively
  maintained "eto-ai" org. This was the historical experimental
  binding, predating the official `lancedb-go`. Not a candidate.

### 3. Community / fork packages

- None of substance. GitHub's `lang:go` filter for "lancedb" returns
  only forks of the official repo and small demo projects. Given the
  official binding exists and is recent, no third-party package is
  credible.

## Verdict

`github.com/lancedb/lancedb-go` is the only real option and it covers
all seven primitives the Phase 0 spike requires:

| Primitive | Go API |
|---|---|
| Connect with S3-compatible backing | `lancedb.Connect(ctx, "s3://bucket/prefix", &ConnectionOptions{StorageOptions: ...})` |
| Create dataset with complex schema | `lancedb.NewSchema(arrow.NewSchema([]arrow.Field{...}, nil))` |
| Upsert rows | `table.AddRecords(ctx, []arrow.Record{...}, nil)` |
| Vector search with filter | `table.VectorSearchWithFilter(ctx, "vector", queryVec, k, "org_id = '...' AND is_public = true")` |
| FTS with filter | `CreateIndex([]string{"content"}, IndexTypeFts)` + `FullTextSearchWithFilter(...)` |
| Metadata-only update | `table.Update(ctx, "id = 'x'", map[string]any{"acl": []string{"..."}})` |
| Delete by id | `table.Delete(ctx, "id = 'x'")` |

## Risks flagged for the human before green-lighting Phase 1

1. **CGO + downloaded native lib.** Production builds must run
   `scripts/download-artifacts.sh` in CI and propagate
   `CGO_CFLAGS`/`CGO_LDFLAGS` into every `go build` / `go test`
   invocation. This is an operational addition, not a showstopper, but
   it needs wiring in CI and in developer onboarding.
2. **`v0.x`.** The surface is small and stable, but we should pin
   `v0.1.2` in `go.mod` and not auto-upgrade.
3. **`list<string>` filter semantics.** DataFusion's `array_has` is
   documented and used in LanceDB's own test suite, but the exact
   dialect LanceDB accepts may drift. The spike pins the exact SQL we
   use so we notice any regression.
4. **The Rust-sidecar fallback stays live.** If any release breaks ops
   (e.g. drops metadata-only `Update` performance), we extract the
   seven operations to a sidecar without changing the callers.
