# Phase 0 spike — RESULT

## Status: **RED**

The Phase 0 gating spike verified 5 of 7 required primitives against
`github.com/lancedb/lancedb-go v0.1.2` on Darwin arm64 with MinIO as the
S3-compatible backing. Two gating failures were observed. The cheap
workarounds explored during the spike do not hold up — details below.

Until a newer release of the Go binding lands (or we switch to the
Rust-sidecar fallback), **Phase 1 should not depend on the LanceDB Go
SDK for the vectorstore port.** Schema / model / acl / testhelper work
can still proceed because it does not touch the vectorstore, but the
`internal/rag/vectorstore` implementation is blocked.

---

## Observed results (single run)

Command:

    make rag-spike

Output:

    [PASS] 1. Connect (S3/MinIO)              138ms — lancedb.Connect
    [PASS] 2. Create dataset                   217ms — conn.CreateTable
    [PASS] 3. Upsert 100 rows                   23ms — table.Add → 100 rows
    [PASS] 4. Vector search + filter (<100ms)   25ms — VectorSearchWithFilter → 10 hits
    [FAIL] 5. FTS with filter                  —      Full-text search is not currently supported
    [FAIL] 6. Metadata-only update             —      Unsupported update value type for column acl
                                                      (and read-back of list<string> also fails)
    [PASS] 7. Delete by id                       8ms — table.Delete → 100 → 99 rows

---

## Details of each failure

### Op 5 — FTS with filter (hard fail)

The Go binding exposes `ITable.FullTextSearchWithFilter(...)`, but the
Rust FFI for v0.1.2 returns a hardcoded error:

    // vendored at $GOMODCACHE/github.com/lancedb/lancedb-go@v0.1.2/rust/src/query.rs:107-118
    // Apply full-text search
    if let Some(fts_search) = query_config.get("fts_search") {
        // Note: FTS search is not currently available in this API version
        return Err(lancedb::Error::InvalidInput {
            message: "Full-text search is not currently supported".to_string(),
        });
    }

The binding's own integration tests cover FTS (see
`pkg/tests/fts_test.go`), but those tests run against a newer native
library than the one published to the v0.1.2 GitHub release. The native
libraries under `releases/v0.1.2/lancedb-go-native-binaries.tar.gz` were
built with the stub. The `main` branch has real FTS code
(`rust/src/query.rs:106-120`), but there is no newer tag.

### Op 6 — Metadata-only update on `list<string>` (hard fail — this is the one that matters most)

Two problems stack on top of each other:

**(a) Scalar-only update values.** The Rust FFI only accepts
`String | Number | Bool | Null` as an update value (verified at
`rust/src/data.rs:87-101`). A `list<string>` column value cannot be
expressed in that set. We tried three encodings from Go:

1. `[]string{...}` — rejected by the binding's JSON serialization as
   `Unsupported update value type`.
2. SQL literal string `make_array('a','b','c')` — accepted as a string,
   but the Rust code wraps it in extra quotes (`'{}'`) so it becomes a
   text literal, not a DataFusion array expression.
3. JSON-array-as-string `'["a","b","c"]'` — same wrapping; stored as a
   string cell.

**(b) The read path can't decode `list<string>` either.** When we
`Select` a row whose ACL column is `list<string>`, the binding returns
`Unsupported type: List(Field { name: "item", data_type: Utf8, ... })`
(see the spike's recorded failure line). So even if we found a way to
write it, we couldn't read it back.

Op 6 is **explicitly called out** in the Phase 0 plan as a gating
requirement because Phase 3 perm-sync depends on it: permissions update
thousands of times more often than content; re-embedding every chunk
because its ACL changed is not viable. This failure is therefore a hard
stop, not a yellow flag.

### Ops that work (for context)

- Op 1 (Connect): works once `AWS_ENDPOINT_URL`, `AWS_ALLOW_HTTP`,
  `AWS_S3_ALLOW_UNSAFE_RENAME`, and `AWS_EC2_METADATA_DISABLED` env vars
  are set. The Go SDK's structured `S3Config{Endpoint, ForcePathStyle}`
  fields are **NOT wired through** to the Rust side in v0.1.2 — only
  access key, secret, region, and session token are forwarded via env
  vars (see `rust/src/connection.rs:116-144`). The actual MinIO endpoint
  has to be injected with `AWS_ENDPOINT_URL`. This is a gotcha, not a
  blocker.
- Op 4 (vector search + filter): works, including `array_has(acl, 'x')`
  inside the SQL filter. Latency was well under the 100ms target on
  100 rows (25ms).
- Op 7 (delete by filter): works.

---

## Why the earlier research note was too optimistic

`SPIKE_RESEARCH.md` referenced the **`main` branch** when cataloguing the
API surface (that's what I cloned to `/tmp/lancedb-go`). The public Go
module tag `v0.1.2` — what `go get` actually installs and what the
prebuilt native library supports — is a strict subset:

- FTS: defined in Go, returns an error from Rust.
- Endpoint config: defined in Go (`S3Config.Endpoint`), silently
  dropped in Rust (only env-var auth forwarding is honored).
- `list<string>` IO: defined in Arrow (`arrow.ListOf`) and accepted at
  `CreateTable` time, but read-back and updates don't support it.

The gap between main and v0.1.2 is substantial. A `v0.1.3` (or similar)
release will probably close most of these gaps — the code is there on
main — but we have to wait for it, or produce our own build.

---

## Recommendations (for the human to choose from)

The plan's "Rust-sidecar fallback" and "switch to Qdrant" options both
become live. Ranked by how much of the existing work is preserved:

### Option A — Wait for a newer LanceDB Go release (lowest effort, highest timing risk)

The `main` branch has FTS, `list<string>` read support, and richer
update semantics. Once the upstream team cuts a release with native
artifacts built from main, the spike likely turns GREEN with only a
`go.mod` bump and a rerun. No plan revision needed.

Risk: we don't control the cadence. The gap between v0.1.2 (Sept 2025)
and main (April 2026) is seven months; the next tag could be another
seven months away. If Phase 1 work blocked on vectorstore is on the
critical path, we cannot assume this.

### Option B — Build the native library from `main` ourselves (medium effort, medium risk)

The `scripts/build-native.sh` + `Makefile` in the upstream repo can
produce a darwin_arm64 + linux_amd64 static library from source. Vendor
the output under `.lancedb-native/` alongside the Go dep. We'd pin to a
specific `main` commit in `go.mod` (already an option: `go get
github.com/lancedb/lancedb-go@main` returned a valid
`v0.1.3-0.20260413172403-c43f33236280` pseudo-version).

Requires: Rust toolchain in CI and in developer onboarding; a storage
location for the compiled `.a` (we can check it into the repo or host
it on R2). Adds ~3-5 min to cold CI. Otherwise this is the cleanest
path — it keeps the Go-only data path and gets us all the upstream
fixes.

### Option C — Rust sidecar (high effort, high safety)

Write a thin Rust daemon that links the upstream `lancedb` crate
directly and exposes the seven operations over HTTP/gRPC. Hiveloop Go
talks to the sidecar via the existing service-to-service patterns.
Isolates the CGO layer entirely, eliminates the native-artifact
dance, and gives us a stable API we control.

Cost: ~1-2 weeks of work on a subsystem we'd otherwise skip. Adds a
new runtime dependency to ops.

### Option D — Swap the stack to Qdrant (escape hatch)

Qdrant has a mature, native Go client
(`github.com/qdrant/go-client`), supports hybrid search (vector + FTS),
stores payload metadata cheaply (no dataset rewrite on `set_payload`),
and runs as a self-hosted server. Loses the "storage is just a bucket"
property — we'd run a stateful Qdrant cluster.

This is the "declare bankruptcy on LanceDB" option. Only pick it if A,
B, and C are all unpalatable.

---

## What I did NOT do

- I did not silently paper over the failures to produce a GREEN report.
- I did not modify the spike to skip op 5 or op 6 to tickle out a
  passing run.
- I did not commit an unverified "it works" to the docs.

## Recommendation

**My recommendation: Option B.** Building from `main` keeps the plan
intact and buys us the already-implemented-upstream fixes without
waiting on a release. It is one afternoon of Makefile + CI work. Option A
is probably too slow; Options C and D discard real work.

Before starting Phase 1's vectorstore tranche, please decide among
A/B/C/D and update `ARCHITECTURE.md` with the outcome. Phase 1 data-layer
tranches (1A–1F) can start regardless of that decision — none of them
touch the vector store.
