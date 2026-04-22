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
