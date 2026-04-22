# Onyx → Hiveloop: RAG Port Plan (Phase 2 — Rust `rag-engine` service + Go client)

**Author:** architecture session, April 2026
**Scope of this document:** Phase 2 only. Phase 0 (scaffolding + LanceDB spike) and
Phase 1 (Postgres data layer) are covered in `plans/onyx-port.md`. Phase 2 stands up
the vector-store sidecar service that Phase 0's spike (RED — see
`/Users/bahdcoder/code/hiveloop-rag-phase0/internal/rag/doc/SPIKE_RESULT.md`) forced
us to commit to. Phases 3+ (the three-loop ingest/perm-sync/prune scheduler,
connectors, retrieval surface) are planned separately.

**Source project:** `/Users/bahdcoder/code/onyx` — MIT-licensed
**Target project:** `/Users/bahdcoder/code/hiveloop.com` (Go monolith)
**New subproject (this phase):** `services/rag-engine/` — Rust binary, tonic gRPC

Why Phase 2 exists and why it's Rust: Phase 0's gating spike against
`github.com/lancedb/lancedb-go v0.1.2` failed Ops 5 (FTS) and 6
(metadata-only update on `list<string>`). Both are non-negotiable — Op 6 is the
hot path for perm-sync (Onyx Phase-3 analog) and Op 5 is our day-one hybrid
search requirement. The published Go SDK is a thin CGO wrapper over an
out-of-date prebuilt native; `main` has real implementations but no tag. Rather
than wait on upstream (Option A), vendor a self-built native (Option B — which
still leaves us at the mercy of a Go-binding surface that drops `S3Config.Endpoint`
silently), or swap to Qdrant (Option D — loses "storage-is-a-bucket"), we take
Option C from the spike result: **link the official Rust LanceDB crate directly
from a sidecar we control and expose it over gRPC.** This phase builds that
sidecar and its Go client.

## Substitutions summary (this phase only)

| Concern | Phase 0/1 | Phase 2 |
|---|---|---|
| LanceDB access | Go CGO binding (rejected per spike) | Rust binary linking `lancedb` crate natively; Go talks to it via gRPC |
| Embedder | Trait-shaped interface reserved | Rust trait `Embedder` + SiliconFlow OpenAI-compatible impl + `FakeEmbedder` |
| Reranker | Trait-shaped interface reserved | Rust trait `Reranker` + SiliconFlow impl + `FakeReranker` |
| Chunker | Python `chonkie` + custom section logic (Onyx `backend/onyx/indexing/chunker.py`) | Rust `text-splitter` + `tiktoken-rs`, Onyx-compatible semantics |
| Service boundary | In-process Go library | Separate Rust process; gRPC trust boundary |
| ACL enforcement | Go-side (Phase 3 will own) | Rust is identity-blind; accepts opaque `acl: list<string>` over gRPC |

---

## Locked stack decisions (restate — do not re-litigate)

| Concern | Decision | Source of lock |
|---|---|---|
| Vector DB access | Official Rust `lancedb` crate, out-of-process | Spike RED → Option C |
| Vector backing store | Cloudflare R2 (prod), MinIO (dev/test) — S3-compatible | `ARCHITECTURE.md §5` |
| Service language | Rust, stable channel | This plan |
| Service name + path | `rag-engine` at `services/rag-engine/` inside Hiveloop monorepo | This plan |
| Transport | gRPC via `tonic` + `prost` | This plan |
| Embedding default | SiliconFlow `Qwen/Qwen3-Embedding-4B` (2560d) | Phase 1G seeded models |
| Embedding catalog | 5 models seeded per Phase 1G — same IDs | Phase 1G |
| Pluggability | Per-org; **one model per org for the lifetime of their index** | `ARCHITECTURE.md §2` |
| Reranker | SiliconFlow `Qwen/Qwen3-Reranker-0.6B` | Phase 1 appendix |
| Hybrid search | BM25 (LanceDB native FTS via tantivy) + vector, fused Go-side | This plan |
| Chunker | `text-splitter` + `tiktoken-rs`, Onyx semantics at 512 tokens, 20% overlap | This plan (DEVIATION from Onyx upstream — see §5.3) |
| HTTP client (embed/rerank) | `reqwest` + `reqwest-middleware` + `reqwest-retry` | This plan |
| Async runtime | `tokio` | This plan |
| Config | `figment` (env + TOML + defaults) | This plan |
| Tracing/metrics | `tracing` + `tracing-opentelemetry` + `prometheus` | Correlates with existing Hiveloop OTel |
| Identity model | Rust is identity-blind; ACL is Go-responsibility | This plan |
| Dataset naming | `rag_chunks__<provider>_<model_slug>__<dim>` — deterministic per Phase 1G `deriveDatasetName` | Phase 1G test `TestRAGEmbeddingModel_DatasetNameDerivation` |
| Dataset partitioning | One LanceDB dataset per `(embedding_model_id, dim)` tuple, org-scoped via `org_id` column + filter | This plan |
| Test policy | Embedder + reranker mockable as Rust traits; LanceDB, MinIO, gRPC all real | `TESTING.md` verbatim + Rust variant |
| LLM/answer/chat | Out of scope — `Search()` helper only | `ARCHITECTURE.md` appendix |

### DEVIATION from Onyx upstream: chunk parameters

Onyx ships `DOC_EMBEDDING_CONTEXT_SIZE = 512`, `CHUNK_OVERLAP = 0`
(`backend/shared_configs/configs.py:42`, `backend/onyx/indexing/chunker.py:28`).
Per architecture session lock we adopt **512 tokens, 20% overlap (102 tokens)**
as the Hiveloop default. Rationale: Onyx's 0-overlap leans on mini-chunks
(`MINI_CHUNK_SIZE = 150`, `app_configs.py:821`) to recover adjacency signal;
we drop mini-chunks in the initial port and use overlap to compensate.
`MINI_CHUNK_SIZE` and Onyx's full AccumulatorState multi-section logic are not
ported in Phase 2 — noted in Phase 2E deferred list.

---

## Testing philosophy — non-negotiable for every tranche

**This section is verbatim from Phase 1's plan for Go, plus a Rust-side variant
appended. All hard rules carry forward unchanged.**

Hiveloop already runs tests against real services (see
`internal/middleware/integration_test.go`: real Postgres at `localhost:5433`,
real `model.AutoMigrate`). This is the only pattern we use.

### Hard rules (unchanged from Phase 1)

1. **Mocking is permitted ONLY for the embedder and reranker.** Both make paid
   external API calls; Phase 2C/2D deliver their Rust traits with in-memory
   deterministic fakes. Nothing else gets mocked — not Postgres, not Redis, not
   MinIO, not LanceDB, not Nango, not HTTP handlers, not gRPC transport.
2. **Integration-first.** Anything touching infrastructure is tested against a
   real instance of that infrastructure, via `docker-compose.test.yml` (Postgres
   + Redis + MinIO + the real `rag-engine` binary).
3. **Every test verifies business behavior, not framework behavior.** Concrete bans:
   - ❌ Assert that a proto message has a field (the codegen proves this)
   - ❌ Assert that a gRPC call is wired (the stub proves this)
   - ❌ Tests that set a mock expectation and then call the mocked method
   - ❌ Tests with names like `test_works` that don't pin behavior
4. **Pure-logic functions are tested directly — no mocks because no infra.**
5. **Coverage target: 100% of branchful code** in `services/rag-engine/src/**`
   (via `cargo llvm-cov`) and every `internal/rag/ragclient/**` Go package.
6. **Every integration test cleans up what it created.** Rust tests use the
   `tempdir` + dataset-drop pattern; Go tests use `t.Cleanup` + explicit
   `DeleteByOrg` RPC calls.
7. **No flaky tests.** If a test needs MinIO or `rag-engine` and they aren't
   up, it fails loudly with "run `make rag-test-services-up` first".

### Rust-side variant (new in Phase 2)

- **The embedder and reranker are Rust traits** (`trait Embedder`, `trait
  Reranker`) with a real SiliconFlow implementation and a `FakeEmbedder` /
  `FakeReranker` in the same crate. The fakes are the ONLY mock-like
  construct in the Rust codebase.
- **`FakeEmbedder` is deterministic**: it hashes the input string with
  SHA-256 and maps the hash to a unit-norm `Vec<f32>` of the configured
  dimension. Same input → exact same vector, across runs and machines. This
  is what makes end-to-end tests against real LanceDB deterministic without
  burning API credits.
- **`FakeReranker` is deterministic**: scores each candidate by the
  byte-length of its content (longer = lower score, or configurable via
  env). Stable ordering.
- **LanceDB is always real** in tests. Each Rust test opens a `tempdir`-backed
  dataset via the `lancedb` crate directly (not through MinIO) for speed;
  Rust *integration* tests (`tests/` dir) open against the docker-compose
  MinIO. No `mockall`, no `double`, no in-memory fake datasets.
- **gRPC transport is always real** in tests. Server-side tests in the
  `services/rag-engine/tests/` directory spawn a `tonic` server on
  `127.0.0.1:0`, connect a real client, exercise the RPC. Go-side tests do
  the same against a Rust binary built in CI.
- **Go tests spin up the real Rust binary** in test mode via a new helper
  `ragclient.StartRagEngineInTestMode(t)` (Phase 2J). The binary is compiled
  once per test run in CI; locally developers run `make rag-engine-build`
  once. Flags `--embedder=fake --reranker=fake --backend=file://$TMPDIR/ds`
  keep everything deterministic and free.

### What "real business value" means per test type in Phase 2

| Test target | Business value |
|---|---|
| `CreateDataset` with a fresh `(provider, model, dim)` triple actually produces a LanceDB dataset readable via direct crate access | Proves dataset names round-trip and schema matches what retrieval will read |
| `Ingest` stream with 1000 chunks writes 1000 rows, fresh vectors queryable | Proves the hot path works end-to-end at non-trivial volume |
| `UpdateACL` by `doc_id` rewrites only ACL column; vector bytes on disk unchanged | This IS the Op 6 failure the Go binding spiked on — the whole reason this service exists |
| `Search` with an ACL filter `array_has(acl, 'user_email:x')` returns only chunks whose ACL contains that token | Core ACL invariant; off-by-one = security bug |
| `Search` with `is_public=true` filter returns public chunks even when user ACL list is empty | PUBLIC_DOC_PAT semantics per `ARCHITECTURE.md §4` |
| `DeleteByDocID` on a doc with 12 chunks removes all 12, leaves other docs untouched | Reindex contract per Onyx `interfaces.py:220-226` |
| `DeleteByOrg` wipes everything for an org across all datasets | GDPR / tenant-deletion compliance |
| `FakeEmbedder` produces stable vectors for the same input across processes | Determinism invariant — test reproducibility |
| `Ingest` rejects a chunk whose vector dim ≠ dataset dim | Prevents silent corruption of the index |
| gRPC deadline propagates to LanceDB cancel | Client-side timeouts actually free server-side resources |
| Graceful shutdown drains in-flight ingests before exit | No half-written datasets on deploy |
| Health check returns `NOT_SERVING` when LanceDB connection is broken | Ops can detect bad pods before routing traffic |

### What is explicitly NOT tested (waste of time)

- That a prost-generated struct has a given field
- That tonic routes a given service name
- That tokio runs async code
- That serde round-trips our types to JSON

---

## Architecture: service boundaries

### Ownership table

| Responsibility | Owner | Trust level | Notes |
|---|---|---|---|
| User identity, sessions, OAuth | Go | Trusted | Hiveloop already owns this |
| Org / membership / role authz | Go | Trusted | `internal/model/org.go`, `internal/model/org_membership.go` |
| Building the ACL string set for a query | Go | Trusted | `internal/rag/acl/prefix.go` from Phase 1D + Phase 3 query builder |
| Building the ACL string set for a chunk at index time | Go | Trusted | Same — passed into `Ingest` over gRPC as opaque strings |
| Chunk content, chunk embedding, chunk storage in LanceDB | Rust | Untrusted about identity; trusted about vectors | Sees strings in an `acl` field but does not interpret them |
| Running BM25 + vector search | Rust | Untrusted about identity | Executes filters Go supplied verbatim |
| Reranking top-K | Rust | Untrusted about identity | Calls SiliconFlow with content-only payloads |
| All Postgres / gorm state | Go | Trusted | Rust never touches Postgres |
| All Redis / asynq state | Go | Trusted | Rust never touches Redis |
| R2 / MinIO bucket creds | Both | Both sides hold creds but only Rust uses them for LanceDB; Go uses them for filestore | Separate IAM scopes recommended in prod |

### Trust boundary invariant

**The Rust service is identity-blind.** It accepts `acl: list<string>` as an
opaque field and uses them only in LanceDB `array_has` filter expressions. It
does NOT know which tokens are user emails vs group names vs public markers, and
it does NOT resolve Hiveloop users or orgs. The org_id field is treated as a
partition key, not an authz subject — it's what we filter on, but the Rust
server never asks "is this caller allowed to see org X?". **The Go client is
solely responsible for attaching the correct `org_id` and `acl` on every call.**

Consequence: if the Go-side authz layer is bypassed (e.g. a rogue service speaks
gRPC to the Rust engine directly without going through Hiveloop's API), every
chunk in every org is readable. Phase 2F therefore requires mTLS or shared
secret auth on the gRPC channel in prod. In dev/test the server listens on
loopback only.

### ACL invariant (restate from Phase 1D)

Hiveloop writes and reads ACL tokens byte-identical to Onyx's format — see
`ARCHITECTURE.md §4`. Tokens on a chunk look like:

    user_email:alice@example.com
    external_group:github_org_hiveloop_team_core
    PUBLIC

Go-side prefix functions from Phase 1D (`internal/rag/acl/prefix.go`,
verbatim port of `backend/onyx/access/utils.py`) own the format. Rust never
generates ACL strings — it only filters on the strings Go supplies.

---

## gRPC API surface

This is **the contract** between Go and Rust for all of Phase 2. Every RPC
has an Onyx-interface citation to justify its existence and shape.

File: `services/rag-engine/proto/rag_engine.proto`. Go codegen drops into
`internal/rag/ragpb/`; Rust codegen into `services/rag-engine/src/pb/`.

### Full .proto (illustrative — exact field numbers locked in 2A)

```proto
syntax = "proto3";
package hiveloop.rag.v1;
option go_package = "github.com/usehiveloop/hiveloop/internal/rag/ragpb";

import "google/protobuf/timestamp.proto";

service RagEngine {
  // Dataset lifecycle — one dataset per (provider, model, dim) triple.
  // Onyx analog: Verifiable.ensure_indices_exist
  //   backend/onyx/document_index/interfaces.py:171-190
  rpc CreateDataset(CreateDatasetRequest) returns (CreateDatasetResponse);
  rpc DropDataset(DropDatasetRequest) returns (DropDatasetResponse);

  // Indexing — streaming client-side so large ingests don't buffer.
  // Onyx analog: Indexable.index
  //   backend/onyx/document_index/interfaces.py:205-243
  rpc Ingest(stream IngestChunk) returns (IngestSummary);

  // Metadata-only update — THE operation that failed the lance-go spike.
  // Onyx analog: Updatable.update_single
  //   backend/onyx/document_index/interfaces.py:268-302
  //   plus vespa impl: backend/onyx/document_index/vespa/index.py:653
  rpc UpdateACL(UpdateACLRequest) returns (UpdateACLResponse);

  // Retrieval — hybrid vector + BM25, with ACL + org + custom filters.
  // Onyx analog: HybridCapable.hybrid_retrieval
  //   backend/onyx/document_index/interfaces.py:339-383
  rpc Search(SearchRequest) returns (SearchResponse);

  // Deletion.
  // Onyx analog: Deletable.delete_single
  //   backend/onyx/document_index/interfaces.py:246-265
  rpc DeleteByDocID(DeleteByDocIDRequest) returns (DeleteByDocIDResponse);

  // Org-wide deletion — cascaded from Postgres org deletion (Phase 1 FK pattern).
  // Onyx analog: backend/onyx/document_index/vespa/index.py:913
  //   (delete_entries_by_tenant_id)
  rpc DeleteByOrg(DeleteByOrgRequest) returns (DeleteByOrgResponse);

  // Prune — delete chunks for a set of doc ids that the source no longer has.
  // Onyx analog: backend/onyx/background/celery/tasks/pruning/
  rpc Prune(PruneRequest) returns (PruneResponse);

  // Standard gRPC health: grpc.health.v1.Health (imported, not redefined).
}

message CreateDatasetRequest {
  string dataset_name = 1;              // rag_chunks__siliconflow_qwen3_embedding_4b__2560
  uint32 vector_dim = 2;                // must match embedder for this org
  string embedding_precision = 3;       // "float32" | "float16" | "int8" per Onyx EmbeddingPrecision
  string idempotency_key = 4;           // client-generated; server returns ALREADY_EXISTS if replayed
}
message CreateDatasetResponse {
  bool created = 1;                     // false = already existed and schema matches
  bool schema_ok = 2;                   // schema verified consistent
}

message DropDatasetRequest {
  string dataset_name = 1;
  bool confirm = 2;                     // must be true — safety interlock
}
message DropDatasetResponse { bool dropped = 1; }

message IngestChunk {
  // The FIRST message in the stream MUST be IngestHeader (oneof).
  oneof payload {
    IngestHeader header = 1;
    ChunkRow row = 2;
  }
}
message IngestHeader {
  string dataset_name = 1;
  string org_id = 2;                    // tenant partition key
  string ingestion_mode = 3;            // "upsert" | "reindex" (reindex wipes old chunks for each doc first)
  string idempotency_key = 4;           // dedupe stream-level replays
  uint32 declared_vector_dim = 5;       // server validates matches dataset
}
message ChunkRow {
  string chunk_id = 1;                  // "<doc_id>__<chunk_index>"
  string doc_id = 2;
  uint32 chunk_index = 3;
  string content = 4;                   // full chunk text (BM25 indexed)
  string title_prefix = 5;              // optional
  string blurb = 6;                     // first N chars — for retrieval display
  repeated float vector = 7;            // length must == header.declared_vector_dim
  repeated string acl = 8;              // OPAQUE tokens; server does not interpret
  bool is_public = 9;                   // mirrors ExternalAccess.is_public
  google.protobuf.Timestamp doc_updated_at = 10;
  map<string, string> metadata = 11;    // freeform string-string; typed metadata stays in Postgres
  // Boost, hidden, primary_owners, secondary_owners — Phase 3.
}
message IngestSummary {
  uint64 rows_written = 1;
  uint64 docs_touched = 2;
  uint64 docs_reindexed = 3;            // nonzero only if reindex mode
  uint64 bytes_written = 4;             // approximate
  repeated IngestError errors = 5;
}
message IngestError {
  string chunk_id = 1;
  string reason = 2;                    // "dim_mismatch", "nan_vector", "empty_content", ...
}

message UpdateACLRequest {
  string dataset_name = 1;
  string org_id = 2;
  repeated UpdateACLEntry entries = 3;
  string idempotency_key = 4;
}
message UpdateACLEntry {
  string doc_id = 1;                    // applies to ALL chunks with this doc_id
  repeated string acl = 2;              // full replacement (not patch)
  bool is_public = 3;
}
message UpdateACLResponse {
  uint64 docs_updated = 1;
  uint64 chunks_updated = 2;
}

message SearchRequest {
  string dataset_name = 1;
  string org_id = 2;                    // always applied as a filter
  string query_text = 3;                // for BM25 + display
  repeated float query_vector = 4;      // for vector search; server validates dim
  SearchMode mode = 5;
  repeated string acl_any_of = 6;       // array_has_any(acl, [...])
  bool include_public = 7;              // OR PUBLIC into the ACL filter
  uint32 limit = 8;
  uint32 candidate_pool = 9;            // how many to pull before fusion/rerank
  string custom_sql_filter = 10;        // escape hatch; will be Phase 3+ (Phase 2 ignores)
  double hybrid_alpha = 11;             // 0..1; 1.0 = pure vector, 0.0 = pure BM25
  bool rerank = 12;                     // if true and reranker configured, rerank before returning
  google.protobuf.Timestamp doc_updated_after = 13;
}
enum SearchMode {
  SEARCH_MODE_UNSPECIFIED = 0;
  SEARCH_MODE_HYBRID = 1;
  SEARCH_MODE_VECTOR_ONLY = 2;
  SEARCH_MODE_BM25_ONLY = 3;
}
message SearchResponse {
  repeated SearchHit hits = 1;
  uint32 bm25_candidates = 2;           // debug/observability
  uint32 vector_candidates = 3;
  uint32 after_fusion = 4;
  uint32 after_rerank = 5;
}
message SearchHit {
  string chunk_id = 1;
  string doc_id = 2;
  uint32 chunk_index = 3;
  double score = 4;                     // fused score post-rerank
  double vector_score = 5;              // raw pre-fusion
  double bm25_score = 6;                // raw pre-fusion
  double rerank_score = 7;
  string content = 8;
  string blurb = 9;
  google.protobuf.Timestamp doc_updated_at = 10;
  map<string, string> metadata = 11;
}

message DeleteByDocIDRequest {
  string dataset_name = 1;
  string org_id = 2;
  repeated string doc_ids = 3;
  string idempotency_key = 4;
}
message DeleteByDocIDResponse { uint64 chunks_deleted = 1; uint64 docs_deleted = 2; }

message DeleteByOrgRequest {
  string org_id = 1;
  repeated string dataset_names = 2;    // caller supplies ALL datasets this org has chunks in
  bool confirm = 3;
  string idempotency_key = 4;
}
message DeleteByOrgResponse { uint64 chunks_deleted = 1; }

message PruneRequest {
  string dataset_name = 1;
  string org_id = 2;
  repeated string keep_doc_ids = 3;     // opposite polarity: delete docs NOT in this set
  // When keep_doc_ids is empty, server must refuse (safety).
  string idempotency_key = 4;
}
message PruneResponse { uint64 docs_pruned = 1; uint64 chunks_pruned = 2; }
```

### RPC-level semantics (non-negotiable)

**Streaming.** Only `Ingest` is streaming (client → server). Everything else is
unary. Rationale: ingests can be 10k+ chunks each; unary would force either
memory blow-up or client-side pagination.

**Error semantics.** Every RPC returns one of these gRPC codes and nothing else:
- `OK` — happy path
- `INVALID_ARGUMENT` — bad input (dim mismatch, empty confirm, unknown dataset name with "require exists" ops)
- `ALREADY_EXISTS` — idempotency_key replay on a successful prior op
- `NOT_FOUND` — dataset doesn't exist where expected
- `FAILED_PRECONDITION` — dataset schema mismatch, backend unreachable
- `UNAVAILABLE` — LanceDB/R2 transient fault; caller should retry
- `RESOURCE_EXHAUSTED` — bounded concurrency saturated (tower limit)
- `DEADLINE_EXCEEDED` — caller deadline tripped; server must cancel underlying LanceDB op
- `INTERNAL` — bug; emits panic/error trace to stderr + tracing

**Deadline conventions.** Go client sets deadlines on every RPC:
- `Ingest`: 10 minutes default (configurable per-call)
- `Search`: 5 seconds default
- `UpdateACL`: 60 seconds
- `DeleteByDocID`, `Prune`: 5 minutes
- `DeleteByOrg`: 30 minutes (may touch every dataset)
- `CreateDataset`, `DropDataset`: 30 seconds
Server enforces deadline via `tonic`'s interceptor → `tokio::select!` with
`CancellationToken` passed to LanceDB.

**Idempotency keys.** Mutating RPCs (`CreateDataset`, `UpdateACL`,
`DeleteByDocID`, `DeleteByOrg`, `Prune`, and the `Ingest` header) carry an
`idempotency_key`. Server stores a bounded LRU (default 10k entries, TTL 1h) of
`(key, response_checksum)` tuples. On replay: same key + same request →
cached response; same key + different request → `INVALID_ARGUMENT`. On restart
the LRU is empty — idempotency is best-effort, not durable. Durable idempotency
lives Go-side via asynq task IDs (Phase 3).

**Authn.** Phase 2F: bearer token shared secret in metadata header
`x-rag-auth`; server rejects unauth'd with `UNAUTHENTICATED`. Prod upgrade to
mTLS is a Phase 2F deferred-item (noted).

---

## Rust tranches (2A–2H)

### Tranche 2A — Cargo workspace + proto codegen + runtime + config + tracing

**Goal:** a compile-clean, test-runnable Rust workspace with empty RPC stubs
that pass a trivial end-to-end "call CreateDataset, get NotImplemented" test.
Nothing else in this tranche touches real LanceDB, embedder, reranker, or chunker.

**Files:**

- `services/rag-engine/Cargo.toml` — workspace root
- `services/rag-engine/crates/rag-engine-server/Cargo.toml` — binary crate
- `services/rag-engine/crates/rag-engine-server/src/main.rs` — `#[tokio::main]`, wires tracing + config + server
- `services/rag-engine/crates/rag-engine-server/build.rs` — runs `tonic-build` for the proto
- `services/rag-engine/proto/rag_engine.proto` — contract per §5
- `services/rag-engine/crates/rag-engine-server/src/config.rs` — `figment`-backed config loader
- `services/rag-engine/crates/rag-engine-server/src/telemetry.rs` — `tracing` + OTel + Prom setup
- `services/rag-engine/crates/rag-engine-server/src/error.rs` — shared error enum → gRPC status
- `services/rag-engine/crates/rag-engine-server/src/pb/mod.rs` — generated bindings include

**Dependencies (verify current versions at implementation time — Rust crate versions churn):**

| Crate | Purpose | Version guidance (April 2026) |
|---|---|---|
| `tokio` | runtime | `1.x` current stable; we use `macros`, `rt-multi-thread`, `signal`, `sync`, `time`, `fs` features |
| `tonic` | gRPC server/client | `0.12.x` or newer; confirm proto codegen with `tonic-build` same major |
| `tonic-build` | codegen | match `tonic` |
| `prost` | proto messages | typically `0.13.x` aligned with `tonic 0.12.x` |
| `prost-types` | `google.protobuf.*` | match `prost` |
| `figment` | layered config (env + TOML) | `0.10.x`; features `toml`, `env` |
| `serde` / `serde_derive` | config structs | `1.x` |
| `tracing` | structured logs | `0.1.x` |
| `tracing-subscriber` | sinks | `0.3.x`, features `env-filter`, `fmt`, `json` |
| `tracing-opentelemetry` | OTel bridge | align with `opentelemetry` major |
| `opentelemetry` + `opentelemetry-otlp` | exporter | pin to one compatible pair |
| `prometheus` | metrics | `0.13.x` — verify; alternative: `metrics` + `metrics-exporter-prometheus` |
| `thiserror` | error derives | `1.x` |
| `anyhow` | app-level error glue (ONLY in main.rs and tests) | `1.x` |

Verify all "latest" assumptions at implementation time. If a newer major
released a breaking API, prefer the major that matches `tonic`'s ecosystem.

**Config schema (from `figment`):**

```toml
# services/rag-engine/config/default.toml — shipped defaults
[server]
listen = "0.0.0.0:7443"
health_listen = "0.0.0.0:7444"
metrics_listen = "0.0.0.0:9464"
auth_token = ""                       # required in prod; dev/test can be empty

[lance]
backend = "file:///var/lib/rag-engine" # default; overridden to s3://... in prod
default_embedding_precision = "float32"

[embedder]
provider = "siliconflow"              # | "fake"
base_url = "https://api.siliconflow.cn/v1"
api_key_env = "SILICONFLOW_API_KEY"
request_timeout_ms = 30000
max_retries = 3

[reranker]
provider = "siliconflow"
model = "Qwen/Qwen3-Reranker-0.6B"
request_timeout_ms = 15000

[limits]
max_concurrent_ingests = 8
max_concurrent_searches = 64
ingest_rps = 200

[telemetry]
service_name = "rag-engine"
otlp_endpoint = ""                    # disabled when empty
log_level = "info"
```

Env overrides use `RAG_ENGINE__SERVER__LISTEN=...` convention (double-underscore
= section separator — `figment`'s default for nested env vars).

**Tests (2A owns):**

1. `test_config_loads_defaults_then_env_overrides` — builds a `Figment` with a
   temp TOML + sets env var, asserts merge order (env > file > defaults).
   Business value: if this is wrong, prod config silently ignores overrides.
2. `test_tracing_init_idempotent` — calls `init_telemetry()` twice, second
   call is a no-op and doesn't panic. Business value: test binaries and
   benchmarks may each call it.
3. `test_server_boots_with_unimplemented_rpcs` — starts the tonic server on
   `127.0.0.1:0`, connects a client, calls `CreateDataset`, expects gRPC
   `UNIMPLEMENTED`. Business value: proves wiring end-to-end before any real
   code lands.
4. `test_health_check_serving_before_deps_wired` — queries `grpc.health.v1`,
   expects `NOT_SERVING` (because LanceDB isn't wired yet). Business value:
   health semantics are right from day 1.
5. `test_panic_in_handler_does_not_crash_server` — installs the panic hook (see
   2H), registers a test handler that panics, asserts the server keeps
   serving subsequent RPCs. Business value: one bad batch can't kill the
   process.

**Definition of done:**
- `cargo build --release` clean in `services/rag-engine/`
- `cargo test -p rag-engine-server` green
- `cargo llvm-cov --fail-under-lines 100` green on branchful code in this crate
- `rag-engine --version` prints semver + commit
- `docker build -f services/rag-engine/Dockerfile .` succeeds (full 2H content lands end of phase; 2A stubs the Dockerfile with single-stage build)

---

### Tranche 2B — LanceDB storage layer

**Goal:** a crate `rag-engine-lance` that wraps the official `lancedb` Rust
crate and exposes exactly the seven primitives the Phase 0 spike tried to
verify — plus the eighth primitive we actually need (FTS-with-filter, BM25
scoring, hybrid fusion prep).

**Onyx references (interface shapes being ported into Rust):**
- `VespaIndex.index` at `backend/onyx/document_index/vespa/index.py:463`
- `VespaIndex.update_single` at `backend/onyx/document_index/vespa/index.py:653`
- `VespaIndex.delete_single` at `backend/onyx/document_index/vespa/index.py:726`
- `VespaIndex.delete_entries_by_tenant_id` at `backend/onyx/document_index/vespa/index.py:913`
- `Verifiable.ensure_indices_exist` at `backend/onyx/document_index/interfaces.py:171-190`
- `Updatable.update_single` at `backend/onyx/document_index/interfaces.py:278-302`

**Files:**

- `services/rag-engine/crates/rag-engine-lance/Cargo.toml`
- `services/rag-engine/crates/rag-engine-lance/src/lib.rs` — re-exports
- `services/rag-engine/crates/rag-engine-lance/src/connection.rs` — `LanceConnection` struct + S3/MinIO setup
- `services/rag-engine/crates/rag-engine-lance/src/schema.rs` — Arrow schema builder keyed by `vector_dim`
- `services/rag-engine/crates/rag-engine-lance/src/dataset.rs` — `DatasetHandle` with create/drop/exists
- `services/rag-engine/crates/rag-engine-lance/src/ingest.rs` — batch upsert, reindex wipe
- `services/rag-engine/crates/rag-engine-lance/src/update.rs` — metadata-only update (the Op 6 primitive)
- `services/rag-engine/crates/rag-engine-lance/src/search.rs` — vector, FTS, filtered
- `services/rag-engine/crates/rag-engine-lance/src/delete.rs` — by doc_id, by org, prune
- `services/rag-engine/crates/rag-engine-lance/src/filters.rs` — safe SQL-filter builder (no string concat!)

**Key dependencies:**

| Crate | Purpose | Version guidance |
|---|---|---|
| `lancedb` | the whole reason this service exists | **verify current version at implementation time**; as of April 2026, upstream has FTS + list<string> update — use the latest tag with those features (probably ≥ 0.15 or whatever is current) |
| `arrow-schema` / `arrow-array` | build/bind LanceDB's Arrow types | align with `lancedb`'s re-export |
| `futures` | stream adaptors | `0.3.x` |
| `uuid` | chunk_id generation | `1.x` |
| `sha2` | optional, for content fingerprints | `0.10.x` |
| `object_store` | indirectly via lancedb; set env vars for S3/MinIO per spike result (`AWS_ENDPOINT_URL`, `AWS_ALLOW_HTTP`, `AWS_S3_ALLOW_UNSAFE_RENAME`, `AWS_EC2_METADATA_DISABLED`) | pulled in by `lancedb` |

**Schema (Arrow) for a RAG chunk row:**

```
id: utf8 (non-null)                 -- chunk_id = "<doc_id>__<chunk_index>"
org_id: utf8 (non-null)             -- tenant partition key
doc_id: utf8 (non-null)
chunk_index: uint32 (non-null)
content: utf8 (non-null, FTS-indexed)
title_prefix: utf8 (nullable)
blurb: utf8 (nullable)
vector: fixed_size_list<float32, DIM> (non-null)
acl: list<utf8> (non-null; may be empty)
is_public: bool (non-null)
doc_updated_at: timestamp<millis, UTC> (nullable)
metadata: map<utf8, utf8> (nullable; stored as struct<keys, values> on disk)
```

Port of the column surface Onyx ships in Vespa (see
`backend/onyx/document_index/vespa/app_config/schemas/danswer_chunk.sd` —
specifically the `acl`, `is_public`, `semantic_id`, `doc_updated_at` fields).
Hiveloop simplifies: we drop Vespa-specific fields (`boost`, `hidden`,
`primary_owners`, `secondary_owners`) from Phase 2; they come back in Phase 3
as typed metadata or via `UpdateACL` siblings.

**Indexes to create at `CreateDataset`:**
- Vector: ANN (IVF_PQ with `num_partitions = max(256, rows/10k)`, `num_sub_vectors = DIM/16`). Build lazily on first search or at N rows threshold (tunable).
- Scalar on `org_id` (pushdown filter)
- Scalar on `doc_id`
- **FTS on `content` via tantivy** — this is the primitive that failed
  in the Go binding spike (Op 5). The Rust crate's `Table::create_fts_index`
  API exposes it natively.

**Filter builder (`filters.rs`) — critical security surface.** All gRPC
`SearchRequest` fields become LanceDB SQL filter strings. We NEVER concatenate
user input; we use a typed builder that parametrizes via `and(..)`, `or(..)`,
`array_has(col, value)` methods. LanceDB SQL does not support proper bound
parameters — we emit pre-validated literals (org_id is UUID-shaped, acl tokens
are regex-constrained, timestamps are RFC3339). `custom_sql_filter` from
`SearchRequest` is **ignored in Phase 2** and returns `INVALID_ARGUMENT` if
non-empty; Phase 3 revisits.

**Tests (2B owns):**

All tests run against **real LanceDB** via tempdir backend; a smaller set also
runs against MinIO (docker-compose).

1. `test_create_dataset_creates_parquet_files` — call create, list files in
   tempdir, assert `data/` has ≥ 1 Parquet fragment. Business value: proves
   schema was accepted by the crate.
2. `test_create_dataset_idempotent_same_schema` — call create twice with the
   same dim, second call returns "already existed, schema ok". Business
   value: deploy-safety; we boot with "ensure dataset" at startup.
3. `test_create_dataset_rejects_schema_change` — create at dim=2560, call
   again at dim=1024, second call errors with `SchemaMismatch`. Business
   value: prevents silent mixing of dimensionalities (the Phase 0 one-model
   invariant).
4. `test_ingest_writes_exact_row_count` — ingest 1337 generated rows, assert
   dataset row count = 1337. Business value: counts are load-bearing for
   admin reports.
5. `test_ingest_reindex_mode_wipes_old_chunks_first` — ingest doc "X" with 10
   chunks; ingest doc "X" again in reindex mode with 4 chunks; assert
   chunks for "X" in the dataset == 4 (not 14, not 4 overwrites over 10).
   Business value: THIS IS the contract from Onyx `Indexable.index`
   `interfaces.py:220-226` — "when a document is reindexed/updated here, it
   must clear all of the existing document chunks before reindexing".
6. `test_ingest_rejects_dim_mismatch` — dataset at 2560, ingest a row with a
   1024-length vector, assert gRPC `INVALID_ARGUMENT` surfaced through the
   lance layer's typed error. Business value: prevents corruption.
7. `test_update_acl_rewrites_only_acl_column` — ingest a chunk, grab its
   vector fingerprint (SHA-256 of bytes). Call `UpdateACL` to change its
   `acl` + `is_public`. Read the chunk back, assert `acl` updated and
   `vector` fingerprint unchanged. Business value: proves metadata-only
   update primitive (Op 6 — the reason this service exists).
8. `test_update_acl_affects_all_chunks_of_doc` — ingest doc with 7 chunks,
   update ACL by doc_id, assert all 7 rows reflect the update. Business
   value: matches Onyx `Updatable.update_single` "Updates all chunks for a
   document" contract (`interfaces.py:289`).
9. `test_search_vector_with_org_filter` — ingest 100 rows for org A + 100 for
   org B, search with `org_id = 'A'`, assert all hits are org A. Business
   value: tenant isolation is a core invariant.
10. `test_search_bm25_with_acl_filter` — ingest 10 rows with varied ACLs, FTS
    for a term, filter `array_has(acl, 'user_email:x')`, assert only
    matching-ACL rows returned. Business value: core ACL enforcement.
11. `test_search_public_without_acl_returns_only_public` — filter
    `is_public = true`, assert private rows excluded. Business value:
    PUBLIC_DOC_PAT semantics.
12. `test_search_hybrid_rrf_merges_bm25_and_vector` — ingest 50 rows where
    some are keyword-matchy and some are vector-matchy; run hybrid with
    `hybrid_alpha = 0.5`; assert top-1 hit is the one that appears in
    BOTH top-10 of BM25 and top-10 of vector. Business value: hybrid
    fusion actually works (not just bolted together).
13. `test_delete_by_doc_id_removes_all_chunks_of_that_doc` — ingest doc with
    5 chunks + another doc with 3 chunks, delete first doc, assert 3
    chunks remain and all belong to second doc. Business value: matches
    Onyx `Deletable.delete_single` at `interfaces.py:251-265`.
14. `test_delete_by_org_wipes_that_org_only` — ingest 2 orgs, delete one,
    assert row count of the other unchanged. Business value: GDPR
    cascade from Postgres org delete.
15. `test_prune_refuses_empty_keep_list` — call prune with `keep_doc_ids = []`,
    assert `INVALID_ARGUMENT`. Business value: the "empty set means delete
    everything" footgun has bitten people; we disarm it.
16. `test_prune_removes_docs_not_in_keep_set` — ingest 5 docs, call prune
    with `keep_doc_ids = [doc1, doc3]`, assert only doc2/doc4/doc5 chunks
    gone. Business value: matches Onyx pruning semantics per
    `backend/onyx/background/celery/tasks/pruning/`.
17. `test_filter_builder_rejects_non_uuid_org_id` — pass `"';DROP TABLE"` as
    org_id, assert rejection before reaching LanceDB. Business value:
    defense in depth; even if LanceDB SQL had an injection path, we block
    upstream.
18. `test_filter_builder_escapes_acl_token_quotes` — pass ACL token
    `user_email:x'y@z.com`, assert it lands as a literal that matches that
    exact stored string (no SQL error, no wrong-match). Business value:
    ACL strings are user-derived; must not break filters.
19. `test_connection_reconnects_after_s3_404_then_recovers` — in MinIO
    test: create dataset, kill MinIO, attempt op → UNAVAILABLE; restart
    MinIO, op succeeds. Business value: transient-fault handling is right.
20. `test_ann_index_built_when_row_threshold_reached` — ingest rows past
    the N threshold, assert ANN index listed; search latency < X ms on
    1000 rows. Business value: search perf SLO.

**Definition of done:**
- All 20 tests green
- Coverage 100% on branchful code in `rag-engine-lance`
- Documentation in `services/rag-engine/doc/LANCE_NOTES.md` capturing which
  crate version + which env-var set we had to use (parallel to spike notes)

---

### Tranche 2C — Embedder trait + SiliconFlow impl + FakeEmbedder

**Goal:** a `rag-engine-embed` crate exposing `trait Embedder`, a SiliconFlow
HTTP-backed implementation, and a deterministic `FakeEmbedder`.

**Onyx references:**
- `IndexingEmbedder` abstract class: `backend/onyx/indexing/embedder.py:33-86`
- `DefaultIndexingEmbedder`: `backend/onyx/indexing/embedder.py:89-120+`
- SiliconFlow is a drop-in for OpenAI's `/v1/embeddings` surface — same wire
  format Onyx already speaks via its `EmbeddingModel` class.

**Files:**

- `services/rag-engine/crates/rag-engine-embed/src/lib.rs` — trait + re-exports
- `services/rag-engine/crates/rag-engine-embed/src/siliconflow.rs` — real impl
- `services/rag-engine/crates/rag-engine-embed/src/fake.rs` — deterministic fake
- `services/rag-engine/crates/rag-engine-embed/src/types.rs` — request/response shapes
- `services/rag-engine/crates/rag-engine-embed/src/errors.rs`

**Trait shape:**

```rust
#[async_trait::async_trait]
pub trait Embedder: Send + Sync + 'static {
    /// Embed for storage ("passage"). May use a passage_prefix.
    async fn embed_passages(&self, texts: &[String]) -> Result<Vec<Vec<f32>>, EmbedError>;
    /// Embed for querying. May use a query_prefix.
    async fn embed_query(&self, text: &str) -> Result<Vec<f32>, EmbedError>;
    fn dimension(&self) -> u32;
    fn model_id(&self) -> &str;
}
```

This mirrors Onyx's split between `passage_prefix` and `query_prefix` at
`embedder.py:52-54`, per-model in Phase 1G's registry.

**SiliconFlow implementation:**
- POSTs to `${base_url}/embeddings` (OpenAI-compatible)
- Body: `{ "model": "Qwen/Qwen3-Embedding-4B", "input": ["..."] }`
- Returns: `{ "data": [{ "embedding": [f32; 2560] }, ...], "usage": {...} }`
- Prepends `passage_prefix` / `query_prefix` per-model
- Uses `reqwest-middleware` with `reqwest-retry` for exponential backoff on
  `429`, `500`, `502`, `503`, `504`, and transient network errors
- Rate-limits with a `tokio::sync::Semaphore` sized to `embedder.max_concurrent`
- Per-request `tracing::span` with attributes `model_id`, `batch_size`, `bytes`
- Reports Prom counter `rag_embedder_requests_total{model, status}` and
  histogram `rag_embedder_latency_ms{model}`

**FakeEmbedder implementation:**
- Takes `dimension: u32, model_id: String` at construction
- `embed_query(t)` = `deterministic_vec(t, dim)` = hash `t` with SHA-256,
  expand the 32-byte digest to `dim` floats via `ChaCha20Rng::seed_from_u64`
  seeded by the hash's first 8 bytes; return unit-normalized vector
- `embed_passages(ts)` = map over the above
- No network; no semaphore; <1ms per call
- Invariant: same input + same dim → bit-identical output across processes

**Dependencies:**

| Crate | Purpose | Version guidance |
|---|---|---|
| `reqwest` | HTTP | `0.12.x`, features `json`, `rustls-tls`, `gzip` |
| `reqwest-middleware` | retry + auth middleware | align with `reqwest` |
| `reqwest-retry` | exponential backoff | align |
| `async-trait` | trait methods | `0.1.x` |
| `serde_json` | body serde | `1.x` |
| `sha2` | fake embedder | `0.10.x` |
| `rand_chacha` | fake embedder | `0.3.x` |

**Tests (2C owns):**

1. `test_fake_embedder_determinism_same_process` — call `embed_query("hi")`
   twice, assert byte-identical `Vec<f32>`. Business value: test
   reproducibility is literally what `FakeEmbedder` is for.
2. `test_fake_embedder_determinism_across_processes` — hash one result
   against a constant test vector (pin-test). Business value: catches
   accidental changes to fake determinism on upgrade.
3. `test_fake_embedder_respects_dimension` — construct at dim=1024 and
   dim=2560, assert output lengths match. Business value: multi-model
   support.
4. `test_fake_embedder_unit_norm` — assert `|v|_2 ≈ 1` (within fp epsilon).
   Business value: consistent with production normalized embeddings so
   cosine-similarity tests behave the same.
5. `test_siliconflow_serializes_request_correctly` — start a mock HTTP
   server (Rust's `wiremock`), point the client at it, assert body matches
   `{ "model": ..., "input": [...] }`. Business value: wire format
   compatibility.
6. `test_siliconflow_parses_embedding_response` — mock returns known
   payload, assert `Vec<Vec<f32>>` correct. Business value: response
   parsing.
7. `test_siliconflow_retries_on_429_then_succeeds` — mock returns 429
   twice then 200, assert final success + retry count observed via
   tracing span. Business value: backoff actually kicks in.
8. `test_siliconflow_returns_rate_limit_error_after_max_retries` — mock
   returns 429 indefinitely, assert error variant `EmbedError::RateLimited`.
   Business value: caller can distinguish transient from terminal.
9. `test_siliconflow_prefixes_are_prepended` — config passage_prefix
   `"passage: "`, embed "hello", assert wire body's `input[0]` is
   `"passage: hello"`. Business value: Qwen models need the prefix;
   forgetting it quietly degrades recall.
10. `test_siliconflow_honors_per_call_semaphore` — spawn 20 concurrent
    embeds against a mock that sleeps 100ms per call, with semaphore=5,
    assert total wall clock ≥ 400ms (4 batches). Business value:
    backpressure protects embedder API budget.

**Uses `wiremock` crate for mock HTTP server ONLY in tests inside this crate.**
This is a reasonable extension of the "mocks allowed for embedder/reranker"
rule — the wiremock instance IS a real HTTP server, just one we control.

**Definition of done:**
- All tests green
- 100% branch coverage

---

### Tranche 2D — Reranker trait + SiliconFlow impl + FakeReranker

**Goal:** mirror image of 2C, for `trait Reranker`.

**Files:**

- `services/rag-engine/crates/rag-engine-rerank/src/lib.rs`
- `services/rag-engine/crates/rag-engine-rerank/src/siliconflow.rs`
- `services/rag-engine/crates/rag-engine-rerank/src/fake.rs`

**Trait shape:**

```rust
#[async_trait::async_trait]
pub trait Reranker: Send + Sync + 'static {
    async fn rerank(
        &self,
        query: &str,
        candidates: &[Candidate],  // { id, content }
        top_k: usize,
    ) -> Result<Vec<RerankedCandidate>, RerankError>;
    fn model_id(&self) -> &str;
}
```

**SiliconFlow rerank API** (compatible with Cohere-style `/rerank` endpoint
on SiliconFlow):
- POST `${base_url}/rerank`
- Body: `{ "model": "Qwen/Qwen3-Reranker-0.6B", "query": "...", "documents": ["...", "..."], "top_n": N }`
- Response: `{ "results": [{ "index": I, "relevance_score": S }, ...] }`

**FakeReranker:**
- Scores each candidate `1.0 / (1 + content.len() as f32)` — monotonic in
  length, deterministic, stable ordering.

**Tests (2D owns):**

1. `test_fake_reranker_determinism` — call twice, same order, same scores.
2. `test_fake_reranker_top_k_bounds` — top_k=3 against 10 candidates →
   exactly 3 returned.
3. `test_fake_reranker_top_k_larger_than_inputs_returns_all` — top_k=20
   against 5 → 5 returned.
4. `test_siliconflow_rerank_parses_response` — mock server, assert order
   and scores.
5. `test_siliconflow_rerank_respects_timeout` — mock that sleeps past
   the timeout → `RerankError::Timeout`.
6. `test_siliconflow_rerank_retries_on_5xx` — mirror of 2C test 7.

**Definition of done:** all tests green, 100% branch coverage.

---

### Tranche 2E — Chunker (`text-splitter` + `tiktoken-rs`)

**Goal:** a `rag-engine-chunk` crate that takes a document's sections + text
and produces chunks with Onyx-compatible boundaries.

**Onyx references:**
- `Chunker`: `backend/onyx/indexing/chunker.py` (full class)
- `DocumentChunker`: `backend/onyx/indexing/chunking/document_chunker.py`
- `TextChunker`: `backend/onyx/indexing/chunking/text_section_chunker.py`
- Constants: `DOC_EMBEDDING_CONTEXT_SIZE = 512`
  (`backend/shared_configs/configs.py:42`), `CHUNK_OVERLAP = 0`
  (`backend/onyx/indexing/chunker.py:28`), `MINI_CHUNK_SIZE = 150`
  (`backend/onyx/configs/app_configs.py:821`), `CHUNK_MIN_CONTENT = 256`
  (`backend/onyx/indexing/chunker.py:32`).

**DEVIATION already locked above: 512 tokens, 20% overlap (102 tokens).**
Mini-chunks not ported. AccumulatorState cross-section packing not ported in
Phase 2 — Phase 3+ revisits.

**Files:**

- `services/rag-engine/crates/rag-engine-chunk/src/lib.rs` — `Chunker` struct
- `services/rag-engine/crates/rag-engine-chunk/src/tokenizer.rs` — wraps `tiktoken-rs` with the correct BPE for the embedder model family (cl100k_base as a shared default; model-specific registry)
- `services/rag-engine/crates/rag-engine-chunk/src/clean.rs` — port of `backend/onyx/utils/text_processing.py:clean_text`

**Dependencies:**

| Crate | Purpose | Version guidance |
|---|---|---|
| `text-splitter` | Rust BPE-aware recursive splitter | verify version; features `tiktoken-rs` |
| `tiktoken-rs` | OpenAI BPE tokenizer | verify; `cl100k_base` encoding |
| `unicode-normalization` | NFKC normalization in `clean_text` | `0.1.x` |
| `regex` | whitespace/control-char stripping | `1.x` |

**API:**

```rust
pub struct ChunkerConfig {
    pub content_token_limit: usize,      // 512
    pub chunk_overlap_tokens: usize,     // 102
    pub title_tokens_reserved: usize,    // up to 32
    pub metadata_tokens_reserved: usize, // up to 64
}

pub struct Chunker {
    tokenizer: TiktokenTokenizer,
    splitter: TextSplitter<TiktokenTokenizer>,
    cfg: ChunkerConfig,
}

pub struct DocumentInput {
    pub doc_id: String,
    pub title: Option<String>,
    pub sections: Vec<Section>,   // { text, link, is_tabular, is_image_caption }
    pub metadata_keyword_suffix: Option<String>,
    pub metadata_semantic_suffix: Option<String>,
}
pub struct ChunkOut {
    pub chunk_index: u32,
    pub content: String,          // what goes into `content` column + what gets embedded (with prefix)
    pub content_for_embedding: String, // with title_prefix + passage_prefix injection
    pub blurb: String,            // first 120 chars
    pub source_link: Option<String>,
    pub chunk_tok_count: u32,
}

impl Chunker {
    pub fn chunk(&self, doc: &DocumentInput) -> Vec<ChunkOut>;
}
```

**Tests (2E owns):**

1. `test_chunk_short_doc_produces_one_chunk` — 100-token doc → 1 chunk, no
   split. Business value: no wasteful empty chunks.
2. `test_chunk_doc_at_exactly_limit_produces_one_chunk` — 512-token doc → 1
   chunk. Business value: boundary behavior.
3. `test_chunk_doc_slightly_over_limit_produces_two_chunks` — 600-token
   doc → 2 chunks. Business value: splitter triggers at the right threshold.
4. `test_chunks_overlap_by_configured_tokens` — 2000-token doc → ≥ 4
   chunks, adjacent chunks share ≥ `chunk_overlap_tokens` tokens of
   content. Business value: overlap ensures retrieval doesn't miss
   sentences straddling a boundary.
5. `test_chunk_respects_sentence_boundaries_when_possible` — doc with clear
   sentences, assert no chunk ends mid-sentence unless it has to.
   Business value: ported semantic from Onyx `SentenceChunker` role.
6. `test_clean_text_strips_control_chars_matches_onyx` — table-driven:
   `"\x00hello\x07"` → `"hello"`; `"  \t\nfoo"` → `"foo"`;
   unicode NFKC idempotent. Pin against Onyx
   `backend/onyx/utils/text_processing.py:clean_text` behavior.
   Business value: corpus that round-trips Onyx's preprocessing for
   eval comparability.
7. `test_token_count_matches_tiktoken_python` — pin a few strings against
   values computed from `tiktoken` (Python). Business value: token
   budgeting parity with Onyx.
8. `test_chunk_empty_sections_produce_empty_chunk_with_doc_id` — doc with
   only a title and no content → one chunk with title_prefix applied to
   empty body. Mirrors Onyx `document_chunker.py:60-62` behavior. Business
   value: title-only documents (Confluence shells, Slack channels) still
   become searchable.
9. `test_chunk_indexes_contiguous_from_zero` — output `chunk_index` values
   are `[0, 1, 2, ...]` with no gaps. Business value: callers use the
   index to reconstruct documents.
10. `test_chunker_is_send_plus_sync` — compile-test that `Arc<Chunker>`
    crosses tasks. Business value: we share one chunker instance across
    the ingest handler's concurrency.

**Definition of done:** all tests green; table in `doc/CHUNKING.md`
documents every Onyx-upstream semantic we chose to keep and every we
chose to drop.

---

### Tranche 2F — gRPC server wiring

**Goal:** implement every RPC in `proto/rag_engine.proto` by composing the
crates built in 2B/2C/2D/2E.

**Files:**

- `services/rag-engine/crates/rag-engine-server/src/service.rs` — `RagEngineService` struct holding `Arc<dyn Embedder>`, `Arc<dyn Reranker>`, `Arc<LanceConnection>`, `Chunker`, idempotency LRU
- `services/rag-engine/crates/rag-engine-server/src/handlers/create_dataset.rs`
- `services/rag-engine/crates/rag-engine-server/src/handlers/drop_dataset.rs`
- `services/rag-engine/crates/rag-engine-server/src/handlers/ingest.rs` — streaming handler
- `services/rag-engine/crates/rag-engine-server/src/handlers/update_acl.rs`
- `services/rag-engine/crates/rag-engine-server/src/handlers/search.rs`
- `services/rag-engine/crates/rag-engine-server/src/handlers/delete_by_doc.rs`
- `services/rag-engine/crates/rag-engine-server/src/handlers/delete_by_org.rs`
- `services/rag-engine/crates/rag-engine-server/src/handlers/prune.rs`
- `services/rag-engine/crates/rag-engine-server/src/interceptors/auth.rs` — bearer-token check
- `services/rag-engine/crates/rag-engine-server/src/interceptors/deadline.rs` — propagate gRPC deadline into `CancellationToken`
- `services/rag-engine/crates/rag-engine-server/src/interceptors/idempotency.rs`
- `services/rag-engine/crates/rag-engine-server/src/interceptors/tracing.rs` — per-RPC span with deadline, peer, rpc name

**Handler specifics:**

- **`Ingest` stream.** First message MUST be `IngestHeader` or the server
  returns `INVALID_ARGUMENT`. Subsequent `ChunkRow`s are buffered in batches
  of 128 and flushed to LanceDB via `rag-engine-lance::ingest`. **We do NOT
  embed server-side in Phase 2** — the Go client is expected to have called
  an embedder (whether Rust `rag-engine-embed` via a separate gRPC RPC, a
  Go-side OpenAI client, or a future Phase-3 server-side embed path) and
  attaches the vector on each `ChunkRow`. This keeps Phase 2 simple; server
  -side embedding is a deferred item. Dim is validated against dataset dim.
- **`Search`.** If `query_vector` is empty and `mode` requires vector,
  server calls `Embedder::embed_query(query_text)` and uses the result. If
  `query_vector` is supplied, server uses it verbatim (dim-validated).
  Candidates pulled: `candidate_pool` via both vector and BM25 (per mode).
  Fusion: RRF (Reciprocal Rank Fusion) with `hybrid_alpha` bias on top-K
  from each side — standard formula
  `score(i) = alpha * (1 / (k + rank_vector(i))) + (1 - alpha) * (1 / (k + rank_bm25(i)))`
  with `k = 60` (RRF convention; Onyx uses similar). If `rerank = true`,
  send the top-`candidate_pool` to reranker, take top-`limit`.

**Tests (2F owns — server-level integration tests in `tests/`):**

1. `test_create_dataset_roundtrip` — client calls, server creates, dataset
   exists on disk via direct crate read. Business value: e2e sanity.
2. `test_ingest_stream_happy_path` — open stream, send header + 50 rows,
   receive `IngestSummary` with `rows_written=50`. Business value: the
   entire hot path works.
3. `test_ingest_stream_header_missing_returns_invalid_argument` — send a
   `ChunkRow` as the first message → error. Business value: catches
   caller bugs at the protocol boundary.
4. `test_search_vector_only_happy_path` — ingest 10 fake-embedded rows,
   search with one of the embeddings, assert it's rank 1. Business
   value: vector search integration.
5. `test_search_hybrid_with_acl_filter_excludes_non_matching` — ingest
   mixed ACL, search, assert only matching ACL present. Business value:
   end-to-end ACL.
6. `test_update_acl_then_search_reflects_new_acl` — ingest with ACL `[A]`,
   search with ACL filter `[B]` → 0 hits; update ACL to `[B]`, search
   with `[B]` → 1 hit. Business value: perm-sync round-trip.
7. `test_delete_by_org_empties_all_datasets_supplied` — ingest org into
   two datasets, delete org listing both, assert both empty. Business
   value: org deletion contract.
8. `test_prune_preserves_keep_list_only` — mirror of 2B test 16 over the
   wire.
9. `test_idempotency_key_replay_returns_cached` — call `UpdateACL` with
   key K, call again with same K + same body, second call returns same
   response without touching LanceDB (assert via row-metadata fingerprint
   unchanged-except-mtime). Business value: at-most-once semantics for
   retried tasks.
10. `test_auth_interceptor_rejects_missing_token` — call any RPC without
    `x-rag-auth`, expect `UNAUTHENTICATED`. Business value: security.
11. `test_deadline_cancels_long_search` — set 100ms deadline, mock a
    LanceDB search to sleep 1s (or use a genuinely huge query), assert
    `DEADLINE_EXCEEDED` and that the LanceDB future was dropped (assert
    via cancellation-token was-cancelled flag). Business value: client
    timeouts free server resources.
12. `test_concurrency_limiter_rejects_past_max` — spawn
    `max_concurrent_searches + 1` concurrent searches, one gets
    `RESOURCE_EXHAUSTED`. Business value: graceful degradation beats
    silent slowness.

**Definition of done:** all 12 tests green; one long-running `e2e/` test
script end-to-end ingests 10k chunks and runs 100 searches under 30s on a
laptop.

---

### Tranche 2G — Observability (logs, OTel traces, Prom metrics, health)

**Goal:** production-grade visibility; every RPC has a span; every state
change has a counter/histogram; `/metrics` served separately from gRPC;
gRPC health check reflects downstream reachability.

**Files:**

- `services/rag-engine/crates/rag-engine-server/src/telemetry.rs` (extended from 2A)
- `services/rag-engine/crates/rag-engine-server/src/metrics.rs` — Prom registry + named metrics
- `services/rag-engine/crates/rag-engine-server/src/health.rs` — `tonic_health::ServingStatus` updater

**Named metrics:**

| Name | Type | Labels | Purpose |
|---|---|---|---|
| `rag_rpc_requests_total` | counter | `rpc`, `code` | error-rate SLO |
| `rag_rpc_latency_ms` | histogram | `rpc` | p50/p95/p99 latency SLO |
| `rag_lance_rows_written_total` | counter | `dataset` | growth monitoring |
| `rag_lance_rows_deleted_total` | counter | `dataset`, `reason` | deletion audit |
| `rag_lance_search_latency_ms` | histogram | `dataset`, `mode` | perf |
| `rag_embedder_requests_total` | counter | `model`, `status` | billing tie-in |
| `rag_embedder_tokens_total` | counter | `model` | billing |
| `rag_reranker_requests_total` | counter | `model`, `status` | billing |
| `rag_inflight_ingests` | gauge | — | backpressure visibility |
| `rag_idempotency_cache_hits_total` | counter | `rpc` | retry detection |

**Tracing.** Every RPC handler wrapped in `#[tracing::instrument(skip_all, fields(rpc = "Search", org_id, dataset_name, deadline_ms))]`. Spans exported to OTLP when `telemetry.otlp_endpoint` non-empty. Spans carry `x-hiveloop-request-id` header from interceptor — this is how we correlate with Go-side OTel (Hiveloop's existing `internal/observability/` package already emits this).

**Health.** `grpc.health.v1` service registered. A background task pings LanceDB (`list_tables` against the default dataset path) every 10s; status flips to `NOT_SERVING` if ping errors for N consecutive rounds. On startup, server starts as `NOT_SERVING`, flips to `SERVING` once the first ping succeeds.

**Tests (2G owns):**

1. `test_metrics_endpoint_serves_prometheus_format` — GET `/metrics` on the
   metrics listener, expect `text/plain; version=0.0.4` body with
   `rag_rpc_requests_total` present. Business value: ops can scrape.
2. `test_rpc_counter_increments` — call `Search` 3 times, scrape
   `/metrics`, assert counter delta = 3. Business value: metric is actually
   wired, not just defined.
3. `test_error_rpc_recorded_as_code_label` — call with bad input, assert
   `rag_rpc_requests_total{rpc="Search",code="INVALID_ARGUMENT"} == 1`.
4. `test_health_goes_not_serving_when_lance_unreachable` — point lance at
   a non-existent MinIO, wait ping interval, assert health = `NOT_SERVING`.
   Business value: load balancer can drain.
5. `test_health_goes_serving_when_lance_recovers` — same test, then bring
   MinIO up, assert health flips back.
6. `test_tracing_span_includes_org_id_attribute` — capture tracing events
   into a test subscriber, call `Search(org_id="X")`, assert at least one
   span with attribute `org_id = "X"`. Business value: support debugging.

**Definition of done:** all tests green; ops runbook at
`services/rag-engine/doc/OPS.md` lists every metric, every alarm threshold,
and the three golden-path queries.

---

### Tranche 2H — Graceful shutdown, backpressure, panic handler, Dockerfile

**Goal:** the binary exits cleanly, survives bad inputs, and packages into a
single small image.

**Files:**

- `services/rag-engine/crates/rag-engine-server/src/shutdown.rs` — SIGTERM/SIGINT handler
- `services/rag-engine/crates/rag-engine-server/src/limits.rs` — `tower::limit::ConcurrencyLimit` layer
- `services/rag-engine/crates/rag-engine-server/src/panics.rs` — `std::panic::set_hook` catching + logging + metric
- `services/rag-engine/Dockerfile` — multi-stage with `cargo-chef` for layer caching
- `services/rag-engine/.dockerignore`
- `services/rag-engine/docker-compose.override.yml` (fragment merged into hiveloop.com/docker-compose.yml)

**Shutdown behavior.** On SIGTERM:
1. Health status → `NOT_SERVING` (load balancer drains).
2. Stop accepting new gRPC connections.
3. In-flight RPCs run to completion up to a 30s grace period.
4. Streaming ingests receive a final `tonic::Status::aborted("server shutting down")` if they exceed grace; their partial state is already committed at batch boundaries (LanceDB batch writes are atomic per call).
5. LanceDB connection closed; OTel exporter flushed.
6. Process exits 0.

**Backpressure.** `tower::ServiceBuilder::new().concurrency_limit(N).service(...)` wraps the whole gRPC service; N is `limits.max_concurrent_searches + max_concurrent_ingests`. Additional per-RPC semaphores enforce per-method fairness.

**Panic handler.** Custom hook logs via `tracing::error!` with
`panic.payload`, `panic.location`, `panic.backtrace`, increments
`rag_panics_total{location}`, and returns — never aborts. `tonic`'s handler
wrapping converts the `JoinError` to `INTERNAL`.

**Dockerfile (sketch):**

```
# syntax=docker/dockerfile:1.7
FROM rust:1.84-bookworm AS chef
RUN cargo install cargo-chef --locked --version X.Y.Z  # verify at impl time
WORKDIR /app

FROM chef AS planner
COPY services/rag-engine/ services/rag-engine/
RUN cd services/rag-engine && cargo chef prepare --recipe-path recipe.json

FROM chef AS builder
COPY --from=planner /app/services/rag-engine/recipe.json /app/services/rag-engine/
RUN cd services/rag-engine && cargo chef cook --release --recipe-path recipe.json
COPY services/rag-engine/ services/rag-engine/
RUN cd services/rag-engine && cargo build --release --bin rag-engine

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/services/rag-engine/target/release/rag-engine /usr/local/bin/rag-engine
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/rag-engine"]
EXPOSE 7443 7444 9464
```

**docker-compose additions (appended to `docker-compose.yml` or mirrored to a new `docker-compose.test.yml`):**

```yaml
  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    ports:
      - "9000:9000"
      - "9001:9001"
    environment:
      MINIO_ROOT_USER: minio
      MINIO_ROOT_PASSWORD: miniosecret
    volumes:
      - miniodata:/data
    healthcheck:
      test: ["CMD", "mc", "ready", "local"]
      interval: 5s
      retries: 5

  rag-engine:
    build:
      context: .
      dockerfile: services/rag-engine/Dockerfile
    ports:
      - "7443:7443"
      - "7444:7444"
      - "9464:9464"
    environment:
      RAG_ENGINE__LANCE__BACKEND: "s3://hiveloop-rag-test/lancedb"
      AWS_ENDPOINT_URL: "http://minio:9000"
      AWS_ACCESS_KEY_ID: minio
      AWS_SECRET_ACCESS_KEY: miniosecret
      AWS_REGION: us-east-1
      AWS_ALLOW_HTTP: "true"
      AWS_S3_ALLOW_UNSAFE_RENAME: "true"
      RAG_ENGINE__EMBEDDER__PROVIDER: siliconflow
      RAG_ENGINE__SERVER__AUTH_TOKEN: dev-shared-secret
    depends_on:
      minio:
        condition: service_healthy

volumes:
  miniodata:
```

**Makefile additions:**

```make
.PHONY: rag-engine-build rag-engine-run rag-engine-test rag-engine-fmt rag-engine-lint rag-e2e rag-test-services-up rag-test-services-down rag-proto-gen

rag-engine-build:
    cd services/rag-engine && cargo build --release

rag-engine-run:
    cd services/rag-engine && cargo run --release

rag-engine-test:
    cd services/rag-engine && cargo test --workspace

rag-engine-fmt:
    cd services/rag-engine && cargo fmt --all

rag-engine-lint:
    cd services/rag-engine && cargo clippy --workspace -- -D warnings

rag-proto-gen:
    cd services/rag-engine && cargo build   # build.rs emits Rust
    protoc --go_out=internal/rag/ragpb --go-grpc_out=internal/rag/ragpb services/rag-engine/proto/rag_engine.proto

rag-test-services-up:
    docker compose -f docker-compose.yml -f docker-compose.test.yml up -d postgres minio rag-engine

rag-test-services-down:
    docker compose -f docker-compose.yml -f docker-compose.test.yml down -v

rag-e2e:
    make rag-test-services-up
    go test -tags=ragengine_e2e ./internal/rag/ragclient/e2e/...
    make rag-test-services-down
```

**Tests (2H owns):**

1. `test_sigterm_drains_inflight_then_exits` — start server, fire a
   long-running `Ingest` in a goroutine, send SIGTERM, assert exit code 0
   and the ingest completed at the last batch boundary.
2. `test_panic_in_one_handler_other_rpcs_still_served` — trigger a panic
   in a test RPC, then immediately call `CreateDataset` on same server,
   expect success + panic counter = 1.
3. `test_concurrency_limit_returns_resource_exhausted` — see 2F test 12;
   duplicated here at the tower layer explicitly.
4. `test_dockerfile_image_runs_and_responds_to_health` — CI-only: build
   image, `docker run` + probe health over gRPC, expect `SERVING` once
   LanceDB is reachable.

---

## Go client tranches (2I–2J)

### Tranche 2I — Go client `internal/rag/ragclient/`

**Goal:** a typed, pooled, retry-aware, circuit-broken gRPC client that
Hiveloop business code uses to talk to `rag-engine`.

**Files:**

- `internal/rag/ragclient/client.go` — `type Client struct { ... }`, constructor, options
- `internal/rag/ragclient/pool.go` — N-connection round-robin (gRPC already multiplexes, but we keep this tiny pool for CPU-core affinity and fault isolation per Google's gRPC perf guidance)
- `internal/rag/ragclient/retry.go` — retries on `UNAVAILABLE`, `DEADLINE_EXCEEDED` (only for idempotent RPCs), and only with idempotency keys set
- `internal/rag/ragclient/breaker.go` — `github.com/sony/gobreaker` wrapping the underlying channel
- `internal/rag/ragclient/deadlines.go` — per-RPC default deadlines (see §5)
- `internal/rag/ragclient/idempotency.go` — idempotency-key generator (ULID from `github.com/oklog/ulid/v2` already in go.mod)
- `internal/rag/ragclient/errors.go` — typed error wrappers
- `internal/rag/ragclient/metrics.go` — Prom metrics mirror of Rust side (latency, error rate, retry count, breaker state)
- `internal/rag/ragclient/options.go` — functional options

**Dependencies (Go):**
- `google.golang.org/grpc` — already indirect; promote to direct dep
- `google.golang.org/grpc/health/grpc_health_v1` — for health polling
- `github.com/sony/gobreaker` — circuit breaker (verify current version)
- `google.golang.org/protobuf` — already present
- Existing Hiveloop observability per `internal/observability/`

**Client API (shape):**

```go
type Client interface {
    CreateDataset(ctx context.Context, in *ragpb.CreateDatasetRequest) (*ragpb.CreateDatasetResponse, error)
    DropDataset(ctx context.Context, in *ragpb.DropDatasetRequest) (*ragpb.DropDatasetResponse, error)

    Ingest(ctx context.Context, header *ragpb.IngestHeader) (IngestStream, error)  // returns a typed stream wrapper
    UpdateACL(ctx context.Context, in *ragpb.UpdateACLRequest) (*ragpb.UpdateACLResponse, error)
    Search(ctx context.Context, in *ragpb.SearchRequest) (*ragpb.SearchResponse, error)
    DeleteByDocID(ctx context.Context, in *ragpb.DeleteByDocIDRequest) (*ragpb.DeleteByDocIDResponse, error)
    DeleteByOrg(ctx context.Context, in *ragpb.DeleteByOrgRequest) (*ragpb.DeleteByOrgResponse, error)
    Prune(ctx context.Context, in *ragpb.PruneRequest) (*ragpb.PruneResponse, error)

    HealthCheck(ctx context.Context) (grpc_health_v1.HealthCheckResponse_ServingStatus, error)

    Close() error
}

type IngestStream interface {
    Send(row *ragpb.ChunkRow) error
    CloseAndRecv() (*ragpb.IngestSummary, error)
}
```

Constructed via:

```go
client, err := ragclient.Dial(ctx, cfg.RagEngineAddr,
    ragclient.WithAuthToken(cfg.RagEngineAuthToken),
    ragclient.WithPoolSize(4),
    ragclient.WithBreakerSettings(gobreaker.Settings{...}),
    ragclient.WithDefaultDeadlines(ragclient.DefaultDeadlines{}),
)
```

Wired into `internal/bootstrap/deps.go`:

```go
type Deps struct {
    // ... existing fields ...
    RagClient ragclient.Client
}
// in bootstrap.New(ctx):
deps.RagClient, err = ragclient.Dial(ctx, cfg.RagEngine.Addr, ...)
```

**Retry rules:**
- `Search`, `CreateDataset`, `DropDataset`, `Prune`, `DeleteByDocID`,
  `DeleteByOrg`, `UpdateACL`: retry on `UNAVAILABLE` only, max 3 attempts
  with exponential backoff (100ms → 500ms → 2s + jitter), and ONLY if
  the caller supplied an `idempotency_key` (or client auto-generated one).
- `Ingest`: **no retry** at the RPC level — the client either replays
  the full stream with the same header idempotency key, or (preferred)
  the asynq worker that owns the ingest catches the error and re-enqueues.

**Circuit breaker:** one breaker per target address. Opens after 50%
failure rate over a 20-request window (`gobreaker.Settings{
ReadyToTrip: func(counts) { ... } }`). Half-open probe every 30s. When open,
all RPCs short-circuit with a typed error `ragclient.ErrCircuitOpen`.

**Tests (2I owns):**

All tests run against the real Rust binary via a testhelper
`StartRagEngineInTestMode(t)` (delivered in 2J — 2I uses the existing
mechanism). NO mocks. The `--embedder=fake` flag makes this fast + free.

1. `TestClient_CreateDataset_Happy` — roundtrip, assert response.
2. `TestClient_Search_ReturnsHitsFromRealEngine` — ingest 3 chunks with
   fake embeddings, search, assert top-1 is the one whose content
   hashes closest.
3. `TestClient_Search_RetryOnUnavailableSucceeds` — wrap the engine
   dial with a `grpc.WithContextDialer` that returns a connection
   which fails N times then succeeds; assert client retried and
   succeeded. Business value: prod resilience.
4. `TestClient_Search_NoRetryOnInvalidArgument` — send a malformed
   request (empty dataset_name), assert one call attempt, no retries.
5. `TestClient_Ingest_NoRetryOnFailure` — break mid-stream, assert
   error bubbles up and client does NOT retry. Business value: ingests
   are too expensive to silently replay; upstream task owner decides.
6. `TestClient_Breaker_OpensAfterFailures` — inject 15 consecutive
   `UNAVAILABLE`s, assert breaker is OPEN, 16th call returns
   `ErrCircuitOpen` without touching network.
7. `TestClient_Breaker_HalfOpenProbe` — after the 30s window, probe
   succeeds, breaker closes.
8. `TestClient_DefaultDeadline_Applied` — call `Search` without passing
   a context deadline, assert outbound gRPC metadata contains a
   `grpc-timeout` that reflects the configured default.
9. `TestClient_CallerDeadlineWinsWhenShorter` — pass a ctx with 10ms
   deadline; default is 5s; assert outbound has ~10ms.
10. `TestClient_AuthTokenAttached` — assert outbound metadata carries
    `x-rag-auth`.
11. `TestClient_Close_CleansUpPool` — assert all underlying
    `*grpc.ClientConn`s are closed after `Close()`.
12. `TestClient_IdempotencyKeyAutoGenerated_WhenAbsent` — call
    `UpdateACL` without a key, capture outbound, assert it's a valid
    ULID. Business value: callers don't have to remember.
13. `TestClient_PoolRoundRobins` — issue 100 calls, assert each of N
    conns received ~100/N (±10%) calls.
14. `TestClient_MetricsEmitted` — call `Search`, assert
    `rag_client_rpc_requests_total{rpc="Search",code="OK"}` incremented.

**Definition of done:** all 14 tests green; benchmark `BenchmarkClient_Search`
shows <2ms overhead over raw gRPC on a loopback connection.

---

### Tranche 2J — Go testhelpers

**Goal:** `StartRagEngineInTestMode(t *testing.T) *ragclient.Client` that
spins up a real Rust binary, configures it to use MinIO + fakes, and
returns a ready-to-use client. Cleanup registered. Binary built ONCE
per `go test` run via sync.Once.

**Files:**

- `internal/rag/testhelpers/ragengine.go` — the helper
- `internal/rag/testhelpers/ragengine_binary.go` — build-once machinery
- `internal/rag/testhelpers/minio.go` — in-test MinIO client for bucket init (real MinIO from compose; just bucket-create idempotence)
- `internal/rag/testhelpers/fakemodels.go` — convenience functions to produce test vectors matching the `FakeEmbedder`'s deterministic output so Go-side tests can precompute expected top-K

**Helper shape:**

```go
// StartRagEngineInTestMode builds services/rag-engine once per test run,
// launches it on an ephemeral port with --embedder=fake --reranker=fake
// --backend=s3://hiveloop-rag-test/lancedb-${uuid}/ pointed at the test
// MinIO. Returns a connected Client. Registers t.Cleanup to:
//   1. Close the client.
//   2. Send SIGTERM to the process and wait up to 30s.
//   3. Delete the per-test S3 prefix.
func StartRagEngineInTestMode(t *testing.T) *ragclient.Client
```

Implementation notes:
- Binary cache dir: `$GOCACHE/ragengine-<sha-of-services-rag-engine-tree>`
- If the cached binary's hash matches the current tree, reuse; else build
  via `cargo build --release --bin rag-engine`.
- In CI, `make rag-engine-build` is an explicit step; helper skips the
  build and only runs the binary.
- MinIO must already be up (helper asserts via health probe; `t.Fatal`
  with "run `make rag-test-services-up` first" if not).

**Tests (2J owns):**

1. `TestStartRagEngineInTestMode_Boots` — call helper, assert health =
   `SERVING` within 10s.
2. `TestStartRagEngineInTestMode_ReusesBinaryAcrossTwoCalls` — call
   twice, assert the on-disk binary mtime is unchanged the second time.
3. `TestStartRagEngineInTestMode_CleanupRegisteredRemovesPrefix` — pass
   a sub-test `t`, have it create a dataset + ingest rows, let `t` end,
   assert S3 prefix empty in outer assertion. Business value: tests
   don't leak state between runs.
4. `TestStartRagEngineInTestMode_FailsLoudlyWhenMinIODown` — with MinIO
   stopped, call helper, assert `t.Fatal` message contains the exact
   string `run \`make rag-test-services-up\` first`. Business value:
   Hard Rule #7 from TESTING.md is enforced.

**Definition of done:** all 4 tests green; docs in
`internal/rag/testhelpers/README.md` show a copy-paste example.

---

## Onyx symbol-to-Rust mapping (Phase 2 scope)

| Onyx symbol | Onyx location | Rust symbol | Rust location |
|---|---|---|---|
| `Verifiable.ensure_indices_exist` | `interfaces.py:171-190` | `DatasetHandle::create` + gRPC `CreateDataset` | `rag-engine-lance/src/dataset.rs` |
| `Indexable.index` | `interfaces.py:205-243` | `DatasetHandle::ingest_batch` + gRPC `Ingest` | `rag-engine-lance/src/ingest.rs` |
| `Updatable.update_single` | `interfaces.py:268-302` | `DatasetHandle::update_acl` + gRPC `UpdateACL` | `rag-engine-lance/src/update.rs` |
| `Deletable.delete_single` | `interfaces.py:246-265` | `DatasetHandle::delete_by_doc_id` + gRPC `DeleteByDocID` | `rag-engine-lance/src/delete.rs` |
| `VespaIndex.delete_entries_by_tenant_id` | `vespa/index.py:913` | `DatasetHandle::delete_by_org` + gRPC `DeleteByOrg` | `rag-engine-lance/src/delete.rs` |
| `HybridCapable.hybrid_retrieval` | `interfaces.py:339-383` | `DatasetHandle::search_hybrid` + gRPC `Search` | `rag-engine-lance/src/search.rs` |
| Celery pruning tasks | `backend/onyx/background/celery/tasks/pruning/` | gRPC `Prune` | `rag-engine-lance/src/delete.rs` |
| `IndexingEmbedder` ABC | `indexing/embedder.py:33-86` | `trait Embedder` | `rag-engine-embed/src/lib.rs` |
| `DefaultIndexingEmbedder` | `indexing/embedder.py:89+` | `SiliconflowEmbedder` | `rag-engine-embed/src/siliconflow.rs` |
| `SentenceChunker`/`DocumentChunker` (role) | `indexing/chunking/` | `Chunker` (simplified, single-pass) | `rag-engine-chunk/src/lib.rs` |
| `clean_text` | `utils/text_processing.py` | `clean_text` | `rag-engine-chunk/src/clean.rs` |
| `IndexBatchParams` | `interfaces.py:44-53` | `IngestHeader` proto | `proto/rag_engine.proto` |
| `VespaDocumentFields` (ACL-only subset) | `interfaces.py:106-118` | `UpdateACLEntry` proto | `proto/rag_engine.proto` |
| `DocumentAccess.to_acl` | `access/models.py:174-197` | Go-side (already Phase 1D) — Rust consumes | n/a |
| `ExternalAccess` / `NodeExternalAccess` payload shapes | `access/models.py` | `acl: list<string>` + `is_public: bool` on every chunk | proto |
| Vespa health | — | `grpc.health.v1` | `rag-engine-server/src/health.rs` |

---

## What Phase 2 deliberately DOES NOT touch

| Concern | Status |
|---|---|
| Asynq task definitions for ingest/perm-sync/prune loops | Phase 3 |
| Postgres reads/writes during ingest (doc upsert, cc-pair upsert, index_attempt) | Phase 3 |
| Connector framework (GitHub/Notion/etc.) | Phase 3 |
| External group sync + user-external-group stale sweep | Phase 3 (Phase 1D only defined the tables + prefix helpers) |
| Polar/billing integration for embed + storage metering | Deferred |
| Server-side embedding path (currently Go client embeds before calling Ingest) | Optional Phase 2.5; not required |
| mTLS between Go and Rust (shared-secret only in Phase 2) | Phase 3+ |
| Re-ranker on BM25-only results (we only rerank hybrid for now) | Phase 3 |
| Onyx mini-chunks | Deferred — overlap covers the gap |
| Onyx multi-section `AccumulatorState` chunker semantics | Deferred |
| Onyx `Contextual RAG` (chunk-summary + doc-summary LLM pass) | Deferred |
| Custom SQL filter escape hatch on `Search` | Deferred to Phase 3 |
| Vespa-style `boost`, `hidden`, `primary_owners`, `secondary_owners` columns | Phase 3 (will land via `UpdateMetadata` RPC; proto field reservations noted) |
| LLM / answer layer | Out of scope forever |
| EE-only features (document_sets, user_groups RBAC on docs) | Not ported |

---

## Risks + mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Rust expertise on the team | medium | high | Phase 2 is gated on at least one engineer who has shipped Rust to prod; if not, pair-program or hire contractor. Code is intentionally narrow — one crate per concern, no clever lifetimes. |
| LanceDB Rust crate API churn | medium | medium | Pin to an exact version in `Cargo.lock`; add a canary upgrade PR monthly; keep the `lance::` imports behind our own `rag-engine-lance` facade so a crate bump only touches one file per primitive. |
| gRPC schema evolution | low | high | Use field numbers ≤ 15 only for fields expected to stay forever; reserve 1000+ for experimental. Every proto change goes through buf lint + `buf breaking` against `main`. Never remove a field; mark deprecated. |
| Build system complexity (Rust + Go + proto + Docker) | high | medium | `make rag-engine-build` is the single entry point; CI runs it. Developers onboard via `make setup-rag` which installs rustup + protoc + minio-mc. Rust build cached via `cargo-chef` layer (Dockerfile). |
| CGO escape hatch — someone proposes going back to Go bindings | medium | high | Decision log entry; pure-Rust is the committed path. Revisit only if LanceDB Go binding cuts a release with all spike ops GREEN AND publishes upstream tests proving it. |
| SiliconFlow rate limits or outage | medium | medium | Per-org API keys (Phase 3) + exponential backoff + Prom alert at 5% 429 rate. FakeEmbedder lets us keep moving in tests without depending on them. |
| MinIO/R2 S3 compatibility drift | low | high | Pin MinIO image tag; CI tests against both. LanceDB env-var kludges (`AWS_ALLOW_HTTP`, `AWS_S3_ALLOW_UNSAFE_RENAME`, `AWS_EC2_METADATA_DISABLED`) documented in `LANCE_NOTES.md`. |
| Streaming `Ingest` mid-stream failures leave partial state | medium | medium | LanceDB writes at batch boundary (128 rows); partial ingest leaves a consistent prefix. Client replays full stream with same `idempotency_key` on the header; server treats replay as reindex-mode for touched docs. |
| ACL filter injection | low | critical | `filters.rs` is a typed builder; `custom_sql_filter` is rejected in Phase 2. Unit test enumerates injection attempts. |
| Dataset-per-model explosion (5 seeded × N orgs) | low | low | Dataset count bounded by `min(num_orgs × 1 model, num_seeded_models)` — one model per org invariant caps it. Prometheus `rag_lance_datasets_total` gauge monitors. |
| Deployment: two binaries now (Hiveloop + rag-engine) | medium | low | docker-compose + Fly/Kamal target each as a separate service; graceful shutdown lets rolling deploys work without dropped ingests. |

---

## Launch order + parallelization

| Step | Tranches | Mode | Blocks | Wall clock estimate |
|---|---|---|---|---|
| 1 | 2A (workspace + proto + config + telemetry) | sequential | everything | 1 day (1 agent) |
| 2 | 2B (Lance) + 2C (Embedder) + 2D (Reranker) + 2E (Chunker) | **parallel in 4 worktrees** | 2F | 2–3 days (4 agents) |
| 3 | 2F (server wiring) | sequential, after step 2 | 2G, 2H, 2I, 2J | 1–2 days (1 agent) |
| 4 | 2G (telemetry) + 2H (shutdown/Dockerfile) | parallel | 2I/2J | 1 day (2 agents) |
| 5 | 2I (Go client) + 2J (Go testhelpers) | parallel | final integration | 1 day (2 agents) |
| 6 | Finalizer: `TestRAGEngine_EndToEnd_Ingest_Search_UpdateACL_Delete` written in Go against the real binary; Dockerfile verified in CI; OPS.md + LANCE_NOTES.md + CHUNKING.md complete | sequential | merge to main | 0.5 day |
| 7 | Human review + merge | human | Phase 3 kickoff | 1 day |

**Total Phase 2 estimate:** 5–7 engineering days wall clock with parallel
tranches. Finalizer mirrors Phase 1's tranche 1F role — the single merge
point where every tranche's deliverable is proven to compose.

**Definition of done for the whole phase:**
- `make rag-engine-test` green
- `make rag-engine-lint` clean
- `go test ./internal/rag/ragclient/...` green
- `make rag-e2e` green (ingest → search → update ACL → delete → assert)
- docker-compose brings up Postgres + MinIO + rag-engine + Hiveloop, end-to-end ingest via Go client works
- Docs: `ARCHITECTURE.md` appended with Decision 7 (Rust sidecar lock), `services/rag-engine/doc/OPS.md`, `services/rag-engine/doc/LANCE_NOTES.md`, `services/rag-engine/doc/CHUNKING.md`
- Coverage 100% branchful on every Rust crate + `ragclient` Go package
- No mocks outside `FakeEmbedder` / `FakeReranker`

---

## Deferred questions (answer before Phase 3)

1. **Server-side embedding** — should `Ingest` accept raw text + model_id and
   embed server-side, or should Go always pre-embed and pass vectors? Current
   plan: pre-embed Go-side so the Rust service is storage-only; revisit when
   Phase 3 scheduler is live and we see which pattern saves round-trips.
2. **mTLS vs shared-secret** — Phase 2 ships shared-secret only. Prod deploy
   target (Fly/Kubernetes/Kamal?) will dictate mTLS material distribution.
   Decide before prod.
3. **Dataset name re-use across orgs** — currently one dataset per
   `(provider, model, dim)` and all orgs share; org isolation is by
   `org_id` filter. Is that the right tradeoff vs per-org datasets? Argument
   for shared: ANN index amortizes training. Argument for per-org: delete is
   trivial, search latency tighter. Decide before volume.
4. **Rerank budget** — reranker is called per-search; at p99 search RPS that
   blows past SiliconFlow's free tier. Need per-org budget tracking; Polar
   integration. Deferred to Phase 3+.
5. **Chunker parity with Onyx** — we deviated on overlap and dropped
   mini-chunks. When we re-evaluate retrieval quality against Onyx upstream,
   do we need to re-add AccumulatorState cross-section packing? Re-measure
   against an eval corpus before Phase 3.
6. **Binary distribution** — `cargo install` from source at container build
   vs prebuilt binaries pushed to an internal registry? Affects CI time.
7. **Per-org rate limits on the gRPC server** — currently we enforce global
   concurrency. Per-org tower limit needs an interceptor that reads org_id
   from the request. Deferred to Phase 3.

---

## Closing

Phase 2 is the hinge point between "Hiveloop has RAG models in Postgres" and
"Hiveloop can answer questions about connected data with ACL enforcement at
scale." Phase 0 discovered we can't do that in pure Go against LanceDB today;
Phase 2 builds the bridge that unblocks the rest of the system without
sacrificing the "storage-is-a-bucket" property that made LanceDB attractive
in the first place.

Every RPC in §5 is cited against an Onyx interface so that when Phase 3 wires
the three-loop scheduler on top, the calls into the Rust service have a clear
Onyx-side analog to mirror behaviorally. Every tranche has named tests with
business-value justifications so that once the tranche is green, the
behavior is pinned. Every mock we permit is exactly what `TESTING.md` already
allowed — the embedder and reranker, and now their Rust trait siblings.
