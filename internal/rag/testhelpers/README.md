# `internal/rag/testhelpers`

Shared test scaffolding for the RAG subsystem. Every integration test
that touches Postgres, MinIO, or the Rust `rag-engine` goes through
this package.

## What lives here

| File | Purpose |
|---|---|
| `db.go` | `ConnectTestDB(t)` — real Postgres + `AutoMigrate`. |
| `fixtures.go` | `NewTestOrg` / `NewTestUser` / `NewTestInConnection` typed constructors with cleanup. |
| `rag_engine.go` | `StartRagEngineInTestMode(t, cfg)` — builds + runs the Rust engine. |
| `ragengine_binary.go` | Per-worktree `cargo build` cache (sync.Once). |
| `minio.go` | MinIO client + bucket/prefix lifecycle helpers. |
| `fakemodels.go` | Go port of Rust `FakeEmbedder` for precomputed vectors. |

## Copy-paste quickstart

```go
package mypkg_test

import (
    "context"
    "testing"

    "github.com/usehiveloop/hiveloop/internal/rag/ragpb"
    "github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

func TestMyFlow(t *testing.T) {
    db := testhelpers.ConnectTestDB(t)
    org := testhelpers.NewTestOrg(t, db)
    _ = org

    inst := testhelpers.StartRagEngineInTestMode(t, testhelpers.RagEngineConfig{})

    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    _, err := inst.Client.CreateDataset(ctx, &ragpb.CreateDatasetRequest{
        DatasetName:        "rag_chunks__fake__2560",
        VectorDim:          2560,
        EmbeddingPrecision: "float32",
        IdempotencyKey:     "mytest-" + t.Name(),
    })
    if err != nil { t.Fatalf("create: %v", err) }
    // ... the rest of your test
}
```

## Prerequisites

1. **MinIO must be running.** Start with `make test-services-up`.
2. **Rust toolchain must be installed.** `cargo --version` must succeed.
   The first test in a `go test` invocation pays a ~40s Rust build;
   subsequent tests reuse the artifact via `sync.Once`.
3. **Postgres must be running** for tests that use `ConnectTestDB`.

If any of these is missing, the helpers fail with a remediation message.
No silent skips — see TESTING.md Hard Rule #7.

## Running E2E against a sibling worktree

Set `RAG_ENGINE_BRANCH` to point at another worktree (useful during
parallel tranche development):

```bash
RAG_ENGINE_BRANCH=/Users/you/code/hiveloop-rag-2f \
  go test -race -p 1 -count=1 ./internal/rag -run TestE2E_
```

It also accepts a path to a pre-built binary directly.

## Fake models

The fake embedder is deterministic: same `(text, kind, dim)` → same
float32 vector, in Go and in Rust. Use `testhelpers.FakeVector(...)` to
precompute the vector the engine will produce when embedding the same
query text. Handy for asserting top-K ordering without inspecting
server-internal state.

## Per-test MinIO isolation

Every `StartRagEngineInTestMode` call allocates a UUID-based S3 prefix
under `hiveloop-rag-test/`. Cleanup deletes the prefix contents via
`DeleteObjects`. Override `RagEngineConfig.LancePrefix` to reuse an
existing prefix when that's desired (e.g. the cleanup-integration test
that seeds data before the helper runs).
