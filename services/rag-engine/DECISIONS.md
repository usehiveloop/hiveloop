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

---

## Tranche 2E (chunker)

### Crate versions

| Crate                    | Version | Notes                                                                      |
|--------------------------|---------|----------------------------------------------------------------------------|
| `text-splitter`          | 0.20.1  | features: `tiktoken-rs`. Rust-native replacement for Onyx's `chonkie`.     |
| `tiktoken-rs`            | 0.6.0   | `cl100k_base` BPE. Same BPE Onyx's OpenAI-family embedders target.         |
| `unicode-normalization`  | 0.1.25  | NFKC for `clean_text`.                                                     |
| `regex`                  | 1.12.3  | `_INITIAL_FILTER` parity with Onyx.                                        |

### Substitutions — where a direct Python port wasn't viable

1. **`chonkie.SentenceChunker` → `text_splitter::TextSplitter`.** Chonkie
   is Python-only. `text-splitter` offers the same "recursive, sentence-
   preferring, token-capped" splitting semantics and supports a custom
   `ChunkSizer` so we drive it with the same cl100k_base tokenizer Onyx
   uses. The only observable difference is that `text-splitter` trims
   leading/trailing whitespace from each chunk by default (Onyx also
   tends to trim but not uniformly). Downstream retrieval is not
   whitespace-sensitive so this is a non-issue.

2. **`AccumulatorState.link_offsets` keys.** Onyx computes the offset
   via `shared_precompare_cleanup(accumulator.text)` (lowercase + strip
   punctuation) so that citation-match indices line up with LLM-
   rewritten quotes. Our consumer (Lance writer + UI) uses raw byte
   offsets into the stored `content` column, so we store those directly.
   Documented inline in `src/accumulator.rs` with a `// DEVIATION:`
   marker.

3. **No `STRICT_CHUNK_TOKEN_LIMIT` / `split_text_by_tokens` fallback.**
   Onyx's `TextChunker._handle_oversized_section` runs a second pass
   against `split_text_by_tokens` when `chonkie` produces over-budget
   chunks. `text-splitter` honours the token capacity as a hard ceiling
   via the custom `ChunkSizer`, so that second pass is redundant.
   Documented inline in `src/accumulator.rs`.

4. **`clean_text` adds NFKC.** Onyx's `clean_text` does not NFKC-
   normalise — it relies on connector-side normalisation. We add NFKC
   at the chunker boundary so the same byte sequence arrives at
   tiktoken regardless of connector origin. tiktoken's cl100k_base is
   NFKC-compatible so this is neutral for token counts and strictly
   safer for deterministic output.

5. **Empty-document sentinel.** Onyx `DocumentChunker.chunk` always
   emits one empty `ChunkPayload` when `payloads.is_empty()`, even for a
   fully empty document. We only emit this sentinel when there is a
   non-empty title prefix (the practical Onyx case — title-only
   Confluence pages and Slack-channel shells). A document with no
   title AND no sections produces zero chunks. Justification: an all-
   empty record pollutes the index without any retrieval signal, and
   the ingest API can still carry the doc_id metadata on the Go side.
   Documented inline in `src/lib.rs` with a `// DEVIATION:` marker.

### Hiveloop-vs-Onyx constants

| Constant                        | Onyx | Hiveloop | Reason                                                                  |
|---------------------------------|------|----------|-------------------------------------------------------------------------|
| `CHUNK_OVERLAP` (tokens)        | 0    | 102      | 20% overlap (locked in plans/onyx-port-phase2.md §5.3, "Chunker parity"). |

Every other chunker constant matches Onyx upstream; citations are in
`crates/rag-engine-chunker/src/constants.rs`.

### Test strategy

Per `internal/rag/doc/TESTING.md`, the chunker has no infrastructure
dependencies — no mocking is needed or allowed. Tests use either the
real `cl100k_base` tokenizer (for parity tests) or `StubTokenizer` (a
word-count × 1.3 approximation that keeps unit tests fast). The stub is
never instantiated in production code.

### Not ported (explicitly out of scope)

- `generate_large_chunks` / `LARGE_CHUNK_RATIO` — multipass retrieval.
  The locked retrieval flow in Hiveloop does not use large chunks.
  Constant is still cited in `constants.rs` for future reference.
- Contextual RAG (`enable_contextual_rag`, doc summary, chunk context).
  Requires an LLM round-trip during indexing; out of scope for 2E.
- Image / tabular section chunkers. Proto only exposes text sections
  (`proto/rag_engine.proto:96-100` has no discriminator); these would
  ship in a future tranche alongside proto updates.
---

## Tranche 2G (observability — tracing, metrics, middleware)

### OpenTelemetry version set

The plan specified `tracing-opentelemetry = "0.27"`, `opentelemetry = "0.27"`,
`opentelemetry_sdk = "0.27"`, `opentelemetry-otlp = "0.27"`,
`opentelemetry-semantic-conventions = "0.27"`. We landed on a partially
different set because of an ecosystem-level version alignment break:

| Crate                                 | Plan   | Landed | Why                                                                                  |
|---------------------------------------|--------|--------|--------------------------------------------------------------------------------------|
| `opentelemetry`                       | 0.27   | 0.27   | —                                                                                    |
| `opentelemetry_sdk`                   | 0.27   | 0.27   | + feature `rt-tokio` for `BatchSpanProcessor` on the Tokio runtime                   |
| `opentelemetry-otlp`                  | 0.27   | 0.27   | features `grpc-tonic`, `http-proto`, `trace`. gRPC is the default collector path.    |
| `opentelemetry-semantic-conventions`  | 0.27   | 0.27   | feature `semconv_experimental` enables `service.instance.id`                         |
| `tracing-opentelemetry`               | 0.27   | 0.28   | 0.27 targets `opentelemetry 0.26`; 0.28 is the matching bridge for `opentelemetry 0.27` |

`opentelemetry-otlp 0.27` also requires `tonic ^0.12.3`, which matches
our workspace pin from Tranche 2A. No change needed there.

### Extra dependencies pulled in by 2G

| Crate                | Version | Purpose                                                                 |
|----------------------|---------|-------------------------------------------------------------------------|
| `prometheus`         | 0.13    | process-local `Registry` + `IntCounterVec` / `HistogramVec` primitives  |
| `axum`               | 0.7     | minimal HTTP server for `/metrics`, paired with the existing `hyper 1`  |
| `ulid`               | 1       | `service.instance.id` resource attribute                                |
| `pin-project-lite`   | 0.2     | reserved for future `tower::Layer` future wrappers (not currently used) |
| `reqwest` (dev-only) | 0.12    | scraping `/metrics` from tests, `rustls-tls`                            |

### Shutdown safety

`provider.shutdown()` is sync-blocking but, when the
`BatchSpanProcessor` is driven by the Tokio runtime, it can deadlock if
`Drop` runs after the runtime has already parked. `TelemetryGuard::Drop`
offloads shutdown to a named OS thread (`"otel-shutdown"`) with a 3-second
hard timeout — on timeout it leaks the join handle and lets the process
exit. This is the only correct way to satisfy "Tokio runtime + explicit
shutdown in guard Drop" without ever hanging SIGTERM.

### Prometheus registry choice

We deliberately built a process-local `Registry` rather than using
`prometheus::default_registry()`. The default registry is a process
singleton that bleeds state across cargo test binaries running in the
same process (which rare but possible). Using our own registry lets
tests spin up an isolated `Metrics` per test (via `Metrics::new()`)
while production still funnels through `Metrics::global()`.

### Metrics server port

`metrics_addr` defaults to `0.0.0.0:9090` (env: `RAG_ENGINE_METRICS_ADDR`).
Separate listener from gRPC so auth failures don't hide `/metrics` and
so ops can firewall one without the other. Scrape command for a pod
running the defaults:

```sh
curl -s http://<pod-ip>:9090/metrics
```

### Metric rename notes for 2F/2H

The plan (§Tranche 2G Named Metrics) used `rag_rpc_requests_total`,
`rag_rpc_latency_ms`, etc. We landed on the `rag_engine_*` prefix that
the task spec actually dictated and used `_seconds` histograms (the
Prometheus convention) instead of `_ms`. Mapping:

| Plan name                              | Implemented name                               |
|----------------------------------------|------------------------------------------------|
| `rag_rpc_requests_total`               | `rag_engine_rpc_total`                         |
| `rag_rpc_latency_ms`                   | `rag_engine_rpc_duration_seconds` (histogram)  |
| `rag_lance_search_latency_ms`          | `rag_engine_lance_operation_duration_seconds` |
| `rag_lance_rows_written_total`         | (deferred to 2B — can be a labelled slice of the lance metric) |
| `rag_embedder_tokens_total`            | `rag_engine_embedding_tokens_total`            |
| `rag_inflight_ingests`                 | `rag_engine_inflight_requests{method=...}`     |
| `rag_idempotency_cache_hits_total`     | (deferred to whatever tranche adds the cache)  |

`rag_engine_inflight_requests` is label-qualified by method so ingest
and search can share one gauge (plan-split was just
`rag_inflight_ingests`). 2F/2H can add dedicated `*_ingests` /
`*_searches` gauges if they need SLI isolation, but do it additively —
do NOT rename the generic one.

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
