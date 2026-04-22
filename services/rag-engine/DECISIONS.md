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
- **Dockerfile base image: `rust:1.89-slim`.** Transitive dep
  `getrandom 0.4.2` (pulled in by `rand_chacha` via `tempfile`) needs
  Cargo's `edition2024` feature, stabilised in Rust 1.85. We bumped
  from the initial 1.82 pin the plan hinted at.
- **`protoc` well-known types resolution in `build.rs`.** The naive
  `tonic_build::configure().compile_protos(&[proto], &[proto_root])`
  works on macOS (Homebrew ships the `.proto` files next to `protoc`)
  but fails in the Debian build stage because the stock
  `protobuf-compiler` package doesn't install them on a standard
  include path. The Dockerfile now installs `libprotobuf-dev` (which
  lands them at `/usr/include/google/protobuf/`) and `build.rs`
  searches `/usr/include`, `/usr/local/include`, and
  `/opt/homebrew/include` for the well-known types. Honours
  `PROTOC_INCLUDE` when set for non-standard layouts.

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

---

## Tranche 2B (LanceDB storage layer)

### Headline — GREEN: the op that killed the lance-go spike works

Metadata-only update on a `list<string>` column (`acl`) works in the
Rust `lancedb` crate via `UpdateBuilder::column("acl", "[…]")`, where
the RHS is a DataFusion SQL array literal. Round-trip of `list<utf8>`
read works via `arrow_array::ListArray::value(i).as_string::<i32>()`.
The `test_update_acl_does_not_touch_vector` test confirms BYTE-IDENTICAL
preservation of the vector column across the update.

The lance-go spike's two failure modes:
  * FTS support - GREEN: `FtsIndexBuilder` + `Query::full_text_search`.
  * Metadata-only update on `list<string>` - GREEN: see above.

Both are production-ready in this Rust crate. The decision to switch to
a Rust sidecar (Option C in `SPIKE_RESULT.md`) is therefore vindicated.

### Crate versions (2B additions)

| Crate                    | Version  | Notes                                                              |
|--------------------------|----------|--------------------------------------------------------------------|
| `lancedb`                | 0.27.2   | `features = ["aws"]` is REQUIRED to register the s3:// scheme       |
| `lance-index`            | 4.0      | re-export of `FullTextSearchQuery` — used in `search.rs`            |
| `arrow` / `arrow-array`  | 57.2     | must match lancedb's transitive pin (57.x); 58 breaks               |
| `arrow-schema`           | 57.2     | same                                                                |
| `arrow-buffer`           | 57.2     | same                                                                |
| `chrono`                 | 0.4      | `serde` feature for timestamp round-trip                            |
| `uuid`                   | 1        | `v4,serde`                                                          |
| `sha2`                   | 0.10     | for content fingerprinting (deferred to Phase 3)                    |
| `tempfile`               | 3        | dev                                                                 |
| `testcontainers`         | 0.27     | dev                                                                 |
| `testcontainers-modules` | 0.15     | `features = ["minio"]`; we override `.with_tag("latest")` in tests |
| `aws-sdk-s3`             | 1.x      | dev — used only to pre-create the test bucket                       |
| `aws-config`             | 1.x      | dev                                                                 |
| `rand`                   | 0.9      | dev — deterministic seeded RNG for tests                            |

### Toolchain bump

`rust-version` bumped from 1.75 → **1.91** (lancedb 0.27.2 minimum).
Developer machines on `rustup install stable` are fine; CI images need
a refresh.

### Storage connection options

Per the Phase 0 spike, structured S3 credentials DO NOT reach the Rust
side. We pass everything through `ConnectBuilder::storage_options`:

```rust
access_key_id, secret_access_key, region,
endpoint,                    // for MinIO / custom endpoints
allow_http,                  // MinIO runs on HTTP
aws_ec2_metadata_disabled,   // avoid 5s IMDS probe
aws_s3_allow_unsafe_rename,  // required by lance-io on S3
```

### Schema design deviation from the plan

The plan sketched `metadata: map<utf8, utf8>`. The actual implementation
uses two parallel `list<utf8>` columns (`metadata_keys`,
`metadata_values`). LanceDB's Arrow map support for SQL pushdown
filters is not as mature as list support (and Phase 2 does not filter
on metadata). Reads rehydrate to a `BTreeMap` at the crate boundary; the
proto contract is unchanged.

### Index strategy

Scalar + FTS indexes are built eagerly at `create_or_open` time because
they are cheap on an empty table:
  * `org_id`, `doc_id`: `BTree` (equality + range pushdown)
  * `is_public`: `Bitmap` (two values, high selectivity)
  * `acl`: `LabelList` (backs `array_has` / `array_has_any`)
  * `content`: `FTS` (tantivy-backed)

The vector ANN index (`IvfPq`) is deferred — `indexes::build_ann_index`
is the hook, invoked by the caller once the row threshold is crossed.
IVF_PQ training wants real data.

### Hard limit on batch size

`ingest::MAX_CHUNKS_PER_CALL = 2000` — the 2F gRPC server must split
incoming batches above this before delegating. Tranche 2F should keep
`500–1000` as the sweet spot (proto SLO envelope) and only spill into
the second sub-batch for the large tail.

### Test harness

All 15 tests boot a real MinIO via `testcontainers_modules::minio::MinIO`.
We explicitly `.with_tag("latest")` to avoid a slow pull of the
`RELEASE.2025-02-28T09-55-16Z` tag hard-coded upstream.

### Deferred to later tranches

  * `custom_sql_filter` gRPC field — Phase 2 server rejects it with
    `INVALID_ARGUMENT`; 2B has no code path for it.
  * `prune` is implemented but the gRPC RPC is wired in 2F.
  * ANN index auto-build threshold + search-latency SLO test (#20 in
    the plan) — the crate exposes the builder; wiring is on 2F.
