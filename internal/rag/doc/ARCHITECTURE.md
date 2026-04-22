# RAG Architecture — Decision Log

**Scope:** decisions made up to and during Phase 0. Further decisions are
appended as they land. Every decision cites the Onyx source it ports from
(or calls out the deviation explicitly).

---

## 1. Vector store: LanceDB with Rust-sidecar fallback

**Decision.** Use LanceDB via the official Go bindings
`github.com/lancedb/lancedb-go` (v0.1.2, last commit 2026-04-13). The
bindings are CGO wrappers around the Rust core and require downloading a
platform-specific `liblancedb_go.a` + `lancedb.h` before building. See
`SPIKE_RESEARCH.md` for the full evaluation and `SPIKE_RESULT.md` for the
gating verification outcome.

**Why LanceDB over Qdrant / Weaviate / pgvector:**
- Storage is a plain object bucket (R2 in prod, MinIO in dev). No
  long-lived stateful server to run, scale, back up, or patch.
- Per-tenant datasets are a directory in S3 — we can wipe an org's index
  by deleting a prefix.
- Columnar layout (Arrow + Parquet) means metadata-only updates (our
  Phase-3 perm-sync hot path) are cheap — no vector rewrite.
- Hybrid search (vector + BM25 FTS) is native, matching our day-one
  requirement.

**Rust-sidecar fallback.** If the Go bindings regress — stale releases,
unfixed critical bugs, unsupported operations — we fall back to a thin
Rust binary that links the LanceDB crate directly and exposes the seven
primitives over gRPC or HTTP. The Phase 0 spike is designed so the seven
primitives map 1:1 to a sidecar API surface if we need to switch.

**Onyx reference.** Onyx uses Vespa
(`backend/onyx/document_index/vespa/index.py`). We are deliberately not
porting Vespa — it needs its own operating discipline that we don't want
to take on.

---

## 2. One embedding model per org for the lifetime of their index

**Decision.** Each org picks one embedding model at first-index time and
that model is pinned for every chunk in their dataset. Switching models
requires `DELETE FROM rag_documents WHERE org_id = ?` plus a full
re-ingest.

**Why.** Mixing embeddings of different dimensionalities or geometries in
one vector space produces silently-wrong nearest-neighbor results. Onyx
solves this with a background swap workflow
(`SearchSettings.status='PAST'|'PRESENT'|'FUTURE'` at
`backend/onyx/db/models.py:2052-2187`, plus multi-stage re-embed). We
consciously drop that machinery — the added code surface is not worth it
for our traffic profile.

**DEVIATION.** Onyx's `SearchSettings` is global + supports live
switchover; our `RAGSearchSettings` is per-org + pinned. See plan
tranche 1C.

---

## 3. Three-loop sync architecture: ingest, perm-sync, prune

**Decision.** Every `InConnection` independently schedules three loops:

| Loop | Cadence (default) | Onyx source |
|---|---|---|
| Ingest (fetch + chunk + embed + write) | `refresh_freq` seconds | `backend/onyx/background/celery/tasks/docfetching/` + `backend/onyx/background/celery/tasks/docprocessing/` |
| Perm-sync (update ACLs on existing docs, no vector rewrite) | `perm_sync_freq` seconds | `backend/onyx/background/celery/tasks/doc_permission_syncing/` |
| Prune (remove source-deleted docs from the index) | `prune_freq` seconds | `backend/onyx/background/celery/tasks/pruning/` |

Plus `external_group_syncing/` running on its own cadence for connectors
with group-based ACLs.

Each loop holds a per-connection Redis lock for the duration of its run;
the scheduler skips a connection whose lock is held. Watchdog scans a
partial index on `last_progress_time` to recover stuck runs (cited in
the plan's Tranche 1B).

**Why three loops instead of one.** Permissions on a 100k-doc corpus
change more often than the docs themselves. Coupling the two loops means
every perm change triggers a re-embed. LanceDB's cheap metadata-only
`Update` (op 6 of the Phase 0 spike) is what makes the split cheap.

---

## 4. ACL string format invariant

**Decision.** Hiveloop writes and reads ACL tokens byte-identical to
Onyx's format. The prefix functions live in `internal/rag/acl/prefix.go`
and are a verbatim port of `backend/onyx/access/utils.py`.

Tokens on a chunk look like:

    user_email:alice@example.com
    external_group:github_org_hiveloop_team_core
    PUBLIC

The `PUBLIC` literal is `PUBLIC_DOC_PAT` from
`backend/onyx/configs/constants.py:27`. A query ACL set is built in Go,
sorted deterministically, and passed into LanceDB's SQL-filter engine.

**Invariant.** If the write-side prefix and read-side prefix drift by
even one character, the filter matches zero rows and queries silently
return empty. Phase 1D therefore includes a pure-logic test that every
prefix function's output exactly equals the Onyx reference strings.

---

## 5. R2 + MinIO back both the vector store and the filestore

**Decision.** One bucket per environment. Prefix-isolated:

    <bucket>/
      lancedb/        # LanceDB datasets (per-org subdirectories)
        <org-id>/
          rag_chunks/
      filestore/      # raw payloads, checkpoints, CSV exports
        <org-id>/
          raw/
          checkpoints/

**Why one bucket.** Simpler IAM, simpler lifecycle policies, simpler
backup. Per-org isolation is logical (prefix) not physical (bucket).
Onyx uses a similar single-backend approach — see
`backend/onyx/file_store/file_store.py` where it reads/writes to S3 under
configurable prefixes.

**Dev/test.** MinIO runs in docker-compose with a pre-created
`hiveloop-rag-test` bucket. The Phase 0 spike hits it directly.

---

## 6. Shared Postgres + `org_id` column vs schema-per-tenant

**DEVIATION** from Onyx. Onyx provisions a Postgres schema per tenant and
uses SQLAlchemy's schema-switching middleware. We keep Hiveloop's
existing pattern — shared Postgres, `org_id uuid` column on every row,
row-level `WHERE org_id = ?` filtering everywhere. Rationale:

- Hiveloop already has this pattern (`internal/model/`).
- LanceDB's per-org dataset already provides physical isolation for the
  large data (vectors).
- Schema-per-tenant would force a parallel migration runner.

The cost — every query must carry `org_id` — is paid up-front in the
data-layer tests (Tranche 1A: FK cascade tests prove org-delete wipes
all descendants).

---

## Appendix: decisions deferred to later phases

These are tracked here so future agents don't re-litigate them.

- Reranker: Qwen3-Reranker-0.6B via SiliconFlow. Pluggable. Phase 2C.
- Chat / answer generation: out of scope entirely. Hiveloop already has
  its own chat subsystem; RAG only exposes `Search() -> []Chunk`.
- Connector auth: Nango only. No direct provider HTTP clients.
- UserGroup RBAC (Onyx EE): not ported. Hiveloop's `OrgMembership.Role`
  covers org-level access; RAG introduces document-level ACLs as a
  separate axis (see Decision 4).
