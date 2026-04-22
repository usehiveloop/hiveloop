# rag-engine — Tranche-by-tranche decision log

This file captures version pins, fallbacks, and interpretation choices
that aren't already nailed down in `plans/onyx-port-phase2.md`. It is
append-only: downstream tranches add entries, they do not rewrite
earlier ones.

---

## Tranche 2A (scaffolding)

### Rust toolchain

- `rustc 1.94.0` (Homebrew build 4a4ef493e, 2026-03-02)
- `cargo 1.94.0`
- Edition 2021, `rust-version = "1.75"` in every crate's metadata.

No nightly, no unstable features, no `rust-toolchain.toml` file — every
downstream tranche should be able to build on a stable channel
installed by `rustup install stable`.

### Crate versions (as pinned in `Cargo.lock`)

| Crate                | Version | Notes                                                    |
|----------------------|---------|----------------------------------------------------------|
| `tonic`              | 0.12.3  | gRPC server + client                                     |
| `tonic-build`        | 0.12.x  | build-time codegen; aligned with `tonic`                 |
| `tonic-health`       | 0.12.3  | grpc.health.v1 implementation                            |
| `prost`              | 0.13.5  | proto runtime; the version tonic 0.12 pairs with         |
| `prost-types`        | 0.13.5  | `google.protobuf.Timestamp` support                      |
| `tokio`              | 1.52.1  | `full` features                                          |
| `tracing`            | 0.1.x   |                                                          |
| `tracing-subscriber` | 0.3.23  | features: `env-filter`, `fmt`, `json`                    |
| `tower`              | 0.5.x   |                                                          |
| `tower-http`         | 0.6.x   |                                                          |
| `figment`            | 0.10.19 | features: `env`, `toml`                                  |
| `serde`              | 1.x     | `derive`                                                 |
| `thiserror`          | 2.x     |                                                          |
| `anyhow`             | 1.x     | only in `main.rs` error glue and tests                   |
| `subtle`             | 2.x     | constant-time byte comparison for the shared-secret auth |
| `async-trait`        | 0.1.x   |                                                          |
| `http`               | 1.x     |                                                          |

The plan called for an OpenTelemetry dependency stack
(`tracing-opentelemetry`, `opentelemetry`, `opentelemetry-otlp`). In 2A
we deliberately do not pull these in — nothing ships traces yet; adding
the dep surface before it's used invites version drift with Tranche 2G.
2G will add the OTel crates behind a feature flag on
`rag-engine-server`.

Similarly, `tonic-reflection` is not wired in 2A. If we need
`grpcurl`-without-proto-files in prod ops, 2G adds it.

### Deviations from the tranche brief

- **`cargo-chef` version in the Dockerfile** pinned to `0.1.68` rather
  than `latest`. `latest` makes Docker layer caching less predictable
  across builder machines. Update explicitly when we bump it.
- **Clippy `result_large_err` allow** on
  `SharedSecretAuth::check`. `tonic::Status` is ~176 bytes; clippy
  flags any `Result<_, Status>` signature as "large err". Boxing would
  add an allocation on every unauthenticated request and buy nothing,
  so we opt out locally with a documented `#[allow]`. The lint stays
  `-D warnings` globally.
- **`rag-engine-server` exposes a `lib.rs` in addition to `main.rs`**
  so integration tests under `tests/` can call `TestServer::start()`
  against the real runtime code instead of shelling out to a built
  binary. Keeps test iteration fast (no rebuild between changes) while
  still hitting the live gRPC transport. `main.rs` is a thin wrapper.
- **No `Cargo.lock` in `.gitignore`.** This is a binary workspace; the
  lock file is committed to guarantee reproducible release builds.

### Locked `.proto` field numbers

Field numbers in `proto/rag_engine.proto` are LOCKED after this commit.
Never re-number. Appending new fields is fine; the contract is
backward-compatible as long as old numbers keep their types.

| Message                   | Field → #                                                                                                                                                                                                           |
|---------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `CreateDatasetRequest`    | `dataset_name=1`, `vector_dim=2`, `embedding_precision=3`, `idempotency_key=4`                                                                                                                                      |
| `CreateDatasetResponse`   | `created=1`, `schema_ok=2`                                                                                                                                                                                           |
| `DropDatasetRequest`      | `dataset_name=1`, `confirm=2`                                                                                                                                                                                        |
| `DropDatasetResponse`     | `dropped=1`                                                                                                                                                                                                          |
| `IngestBatchRequest`      | `dataset_name=1`, `org_id=2`, `mode=3`, `idempotency_key=4`, `declared_vector_dim=5`, `documents=6`                                                                                                                  |
| `DocumentToIngest`        | `doc_id=1`, `semantic_id=2`, `link=3`, `doc_updated_at=4`, `acl=5`, `is_public=6`, `sections=7`, `metadata=8`, `primary_owners=9`, `secondary_owners=10`                                                             |
| `Section`                 | `text=1`, `link=2`, `title=3`                                                                                                                                                                                        |
| `IngestBatchResponse`     | `results=1`, `totals=2`                                                                                                                                                                                              |
| `DocumentResult`          | `doc_id=1`, `status=2`, `chunks_written=3`, `tokens_embedded=4`, `error_code=5`, `error_reason=6`                                                                                                                    |
| `BatchTotals`             | `rows_written=1`, `bytes_written=2`, `batch_duration_ms=3`, `chunk_duration_ms=4`, `embedding_duration_ms=5`, `write_duration_ms=6`, `docs_succeeded=7`, `docs_failed=8`, `docs_skipped=9`                            |
| `UpdateACLRequest`        | `dataset_name=1`, `org_id=2`, `entries=3`, `idempotency_key=4`                                                                                                                                                       |
| `UpdateACLEntry`          | `doc_id=1`, `acl=2`, `is_public=3`                                                                                                                                                                                   |
| `UpdateACLResponse`       | `docs_updated=1`, `chunks_updated=2`                                                                                                                                                                                 |
| `SearchRequest`           | `dataset_name=1`, `org_id=2`, `query_text=3`, `query_vector=4`, `mode=5`, `acl_any_of=6`, `include_public=7`, `limit=8`, `candidate_pool=9`, `custom_sql_filter=10`, `hybrid_alpha=11`, `rerank=12`, `doc_updated_after=13` |
| `SearchResponse`          | `hits=1`, `bm25_candidates=2`, `vector_candidates=3`, `after_fusion=4`, `after_rerank=5`                                                                                                                             |
| `SearchHit`               | `chunk_id=1`, `doc_id=2`, `chunk_index=3`, `score=4`, `vector_score=5`, `bm25_score=6`, `rerank_score=7`, `content=8`, `blurb=9`, `doc_updated_at=10`, `metadata=11`                                                 |
| `DeleteByDocIDRequest`    | `dataset_name=1`, `org_id=2`, `doc_ids=3`, `idempotency_key=4`                                                                                                                                                       |
| `DeleteByDocIDResponse`   | `chunks_deleted=1`, `docs_deleted=2`                                                                                                                                                                                  |
| `DeleteByOrgRequest`      | `org_id=1`, `dataset_names=2`, `confirm=3`, `idempotency_key=4`                                                                                                                                                      |
| `DeleteByOrgResponse`     | `chunks_deleted=1`                                                                                                                                                                                                    |
| `PruneRequest`            | `dataset_name=1`, `org_id=2`, `keep_doc_ids=3`, `idempotency_key=4`                                                                                                                                                   |
| `PruneResponse`           | `docs_pruned=1`, `chunks_pruned=2`                                                                                                                                                                                    |
| `IngestionMode` (enum)    | `UNSPECIFIED=0`, `UPSERT=1`, `REINDEX=2`                                                                                                                                                                              |
| `DocumentStatus` (enum)   | `UNSPECIFIED=0`, `SUCCESS=1`, `FAILED=2`, `SKIPPED=3`                                                                                                                                                                 |
| `SearchMode` (enum)       | `UNSPECIFIED=0`, `HYBRID=1`, `VECTOR_ONLY=2`, `BM25_ONLY=3`                                                                                                                                                           |
