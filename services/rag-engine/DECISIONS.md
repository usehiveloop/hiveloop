# rag-engine — Tranche-by-tranche decision log

This file captures version pins, fallbacks, and interpretation choices
that aren't already nailed down in `plans/onyx-port-phase2.md`. It is
append-only: downstream tranches add entries, they do not rewrite
earlier ones.

---

## Tranche 2C refactor — `async-openai` as a provider-generic client

### Rationale (library-first principle)

The initial 2C implementation hand-rolled ~343 LOC of reqwest +
reqwest-middleware + reqwest-retry + serde_json wiring against the
SiliconFlow `/v1/embeddings` endpoint. SiliconFlow is OpenAI-compatible,
so that surface is shared with OpenRouter, Groq, OpenAI itself, Together,
and every other "drop-in for OpenAI" provider. Owning the wire code meant
owning its retry loop, error mapping, rate-limit heuristics, and JSON
shapes — all of which the `async-openai` crate (4.4M downloads, MIT,
actively maintained) already implements correctly. We keep only the
pieces where we have a real opinion (prefix injection, sub-batching,
dimension validation, concurrency fan-out).

### What changed

- **Removed:** `siliconflow.rs`, `tests/siliconflow.rs`, wire-level
  `EmbeddingRequest`/`EmbeddingResponse` structs in `types.rs`,
  `reqwest-middleware` and `reqwest-retry` workspace deps.
- **Added:** `openai_compat.rs` with `OpenAICompatEmbedder` +
  `OpenAICompatOptions`, backed by `async-openai`.
  `Provider::SiliconFlow` renamed to `Provider::OpenAiCompat`.
  `Provider::Fake` unchanged.
- **Added:** env-driven loader (`load_from_env_and_file`). Required env:
  `LLM_API_URL`, `LLM_API_KEY`, `LLM_MODEL`, `LLM_EMBEDDING_DIM`.
  Optional: `LLM_ID`, `LLM_QUERY_PREFIX`, `LLM_PASSAGE_PREFIX`,
  `LLM_MAX_INPUT_TOKENS`, `LLM_REQUEST_TIMEOUT_SECS`, `LLM_BATCH_SIZE`,
  `LLM_CONCURRENCY`, `LLM_MAX_RETRIES`. Qwen models get auto-defaulted
  prefixes when unset; explicit values (including empty strings) win.
- **Unchanged:** `Embedder` trait shape (`id()`, `dimension()`,
  `max_input_tokens()`, `embed()`), `FakeEmbedder`, `embed_batched`
  helper, `EmbedKind`, `EmbedError`.

### Version pins

| Crate          | Version | Notes                                                                                          |
|----------------|---------|------------------------------------------------------------------------------------------------|
| `async-openai` | 0.35.0  | Current stable as of April 2026. Default features disabled; we enable `rustls` + `embedding`.  |
| `backoff`      | 0.4     | Required at workspace level because `async-openai::Client::with_backoff` takes its type.       |
| `reqwest`      | 0.12    | Kept — we pass a custom `reqwest::Client` to `async-openai` to pin timeout + user-agent.       |

`reqwest-middleware` and `reqwest-retry` are removed: `async-openai`'s
built-in `backoff::future::retry` covers the 429 + 5xx retry logic we
previously wired by hand.

### async-openai quirk: HTTP status loss

`async-openai` discards the HTTP status code before returning:
non-2xx responses are parsed into `ApiError { message, type, param,
code }`, which has no status field. For the `RateLimited` vs `Upstream
{ status, .. }` split we used to do on 429 vs other 4xx, we now:

1. Rely on `async-openai`'s internal retry loop to retry 429 + 5xx.
   The final post-retry error still decays to `ApiError`.
2. On the final `ApiError`, heuristically match the message
   (`"rate limit"` / `"too many requests"` / `"429"`, case-insensitive)
   to flag `RateLimited`. Everything else becomes `Upstream { status: 0,
   body: message }`. This is lossy but matches what every provider we
   care about puts in their 429 bodies; if it ever bites us we can
   promote to reading raw reqwest responses via a tower layer.
3. **Load-bearing test consequence:** wiremock 4xx/429 responses MUST
   set a valid OpenAI-shaped error body (`{"error": {"message": ...}}`)
   or `async-openai` fails at JSON deserialization, masquerading as
   `InvalidResponse`. Tests construct the body with a helper.

### Feature flags

- `async-openai = { default-features = false, features = ["rustls",
  "embedding"] }` — `default = ["rustls"]` upstream, but we spell it
  out, and `embedding` is required in 0.35 to expose
  `Client::embeddings()`.
- `reqwest = { default-features = false, features = ["json",
  "rustls-tls", "gzip"] }` — matches the TLS choice above.

### Provider contract

Same struct, same env vars, different URL + key + model:

| Provider      | `LLM_API_URL`                           |
|---------------|-----------------------------------------|
| SiliconFlow   | `https://api.siliconflow.cn/v1`         |
| OpenAI        | `https://api.openai.com/v1`             |
| OpenRouter    | `https://openrouter.ai/api/v1`          |
| Groq          | `https://api.groq.com/openai/v1`        |
| Together      | `https://api.together.xyz/v1`           |

A wiremock-backed integration test
(`provider_agnostic_siliconflow_and_openrouter`) proves the embedder
doesn't encode any provider-specific assumptions: the same type
handles both with only URL/key/model swapped.

### Cargo.lock handling

Same protocol as Wave 2: `Cargo.lock` rewrites under `async-openai`'s
transitive dependency graph but is NOT committed by this branch. Run
`git update-index --assume-unchanged services/rag-engine/Cargo.lock`
after first build.

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
