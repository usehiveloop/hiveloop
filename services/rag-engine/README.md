# rag-engine

The Rust sidecar service that holds Hiveloop's vector index. It links
the official [LanceDB](https://lancedb.github.io/lancedb/) Rust crate
directly and exposes a narrow gRPC surface so Go can drive ingest,
perm-sync, search, and deletion without going through a CGO binding.
The decision to run this out-of-process — and the decision to write it
in Rust — are both locked in
[`plans/onyx-port-phase2.md`](../../plans/onyx-port-phase2.md).

The service is **identity-blind**: it accepts opaque `acl: list<string>`
tokens and only uses them as LanceDB filter literals. All authz lives
Go-side. See `plans/onyx-port-phase2.md` §"Trust boundary invariant"
for the threat model.

## Status

**Phase 2 Tranche 2A — scaffolding only.** Every business RPC returns
`UNIMPLEMENTED`. Downstream tranches:

| Tranche | Scope                                               |
|---------|-----------------------------------------------------|
| 2B      | LanceDB storage layer (`rag-engine-lance` crate)    |
| 2C      | Embedder trait + SiliconFlow impl + FakeEmbedder    |
| 2D      | Reranker trait + SiliconFlow impl + FakeReranker    |
| 2E      | Chunker (`text-splitter` + `tiktoken-rs`)           |
| 2F      | Service handlers wired to all of the above          |
| 2G      | Observability (`tracing-opentelemetry` + Prom)      |
| 2H      | Lifecycle (panic hooks, graceful shutdown, docker)  |
| 2I      | Go client (`internal/rag/ragclient/`)               |
| 2J      | Test helpers (`ragclient.StartRagEngineInTestMode`) |

## Workspace layout

```
services/rag-engine/
├── Cargo.toml                    # workspace root (shared dep versions)
├── Cargo.lock                    # committed — this is a binary workspace
├── Dockerfile                    # multi-stage, cargo-chef cached
├── DECISIONS.md                  # version pins + any fallbacks
├── scripts/
│   └── smoke.sh                  # `make rag-engine-smoke`
└── crates/
    ├── rag-engine-proto/         # tonic-generated gRPC bindings (built once)
    ├── rag-engine-lance/         # LanceDB storage (stub in 2A)
    ├── rag-engine-embed/         # embedder trait + impls (stub in 2A)
    ├── rag-engine-rerank/        # reranker trait + impls (stub in 2A)
    ├── rag-engine-chunker/       # chunker (stub in 2A)
    └── rag-engine-server/        # the binary
```

The `.proto` contract lives at the monorepo root under
[`proto/rag_engine.proto`](../../proto/rag_engine.proto) so both the Go
client (Phase 2I) and the Rust server consume it from the same
authoritative file.

## Build and run

From `services/rag-engine/`:

```bash
cargo build --release                # produces target/release/rag-engine-server
RAG_ENGINE_SHARED_SECRET=dev cargo run --bin rag-engine-server
```

Or from the monorepo root:

```bash
make rag-engine-build
make rag-engine-run                  # defaults the secret to "localdev-secret-change-me"
make rag-engine-test                 # runs every crate's tests
make rag-engine-clippy               # clippy with -D warnings
make rag-engine-fmt                  # cargo fmt --all
make rag-engine-smoke                # boots the server, probes it, shuts down
```

## Environment variables

| Name                         | Required | Default             | Purpose                                              |
|------------------------------|----------|---------------------|------------------------------------------------------|
| `RAG_ENGINE_LISTEN_ADDR`     | no       | `0.0.0.0:50051`     | Bind address for the gRPC server                     |
| `RAG_ENGINE_SHARED_SECRET`   | **yes**  | (none — boot fails) | Bearer token required on every non-health RPC        |
| `RAG_ENGINE_LOG_LEVEL`       | no       | `info`              | `tracing` env filter (e.g. `info`, `debug`, `warn`)  |
| `RAG_ENGINE_CONFIG`          | no       | (unset)             | Optional path to a TOML config file (merged below env) |
| `RUST_LOG`                   | no       | (unset)             | Standard `tracing` override; wins over `LOG_LEVEL` when set |

The server refuses to boot if `RAG_ENGINE_SHARED_SECRET` is empty or
whitespace. This is intentional — the Rust engine is private-network
reachable from every Hiveloop pod, and an empty secret makes the trust
boundary between Go and Rust a no-op.

## Testing locally

Every test in this workspace spins up a real tonic server on an
OS-assigned ephemeral port and connects to it over loopback — **no
in-process service mounting, no transport mocks.** This mirrors
[`internal/rag/doc/TESTING.md`](../../internal/rag/doc/TESTING.md).

```bash
cd services/rag-engine
cargo test --all
```

### Smoke test against a running binary

```bash
# Terminal 1:
make rag-engine-run

# Terminal 2: if grpc_health_probe is installed
grpc_health_probe -addr=localhost:50051 -service=hiveloop.rag.v1.RagEngine
# → status: SERVING
```

## gRPC surface

See [`proto/rag_engine.proto`](../../proto/rag_engine.proto) for the
full contract. High-level:

- `CreateDataset` / `DropDataset` — dataset lifecycle, one per `(provider, model, dim)`
- `IngestBatch` — unary batch of 500–1000 raw documents; Rust chunks + embeds + writes
- `UpdateACL` — metadata-only ACL rewrite (the Op-6 primitive that forced this service)
- `Search` — hybrid vector + BM25 with ACL + org filters
- `DeleteByDocID`, `DeleteByOrg`, `Prune` — deletion paths
- Standard `grpc.health.v1.Health` — unauthenticated

All business RPCs require `authorization: Bearer <shared-secret>`.
Health RPCs do not.
