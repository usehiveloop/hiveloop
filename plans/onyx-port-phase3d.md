# Phase 3D — GitHub connector

**Status:** Plan
**Branch:** `rag/phase-3d-github`
**Depends on:** 3A (RAGSource model), 3B (Connector traits + factory registry), 3C (Scheduler that drives connectors)
**Supersedes:** the high-level 3D section of `plans/onyx-port-phase3.md` (lines 442–581)

---

## Issues to Address

3C ships a periodic scheduler that scans `rag_sources` and enqueues Asynq jobs, but every job currently fails because there is no concrete connector registered. `interfaces.Lookup("github")` returns `nil`. The factory registry has slots; nobody fills them.

3D fills the GitHub slot — the first real connector — by implementing all three trait surfaces 3B defined:

- `CheckpointedConnector[GithubCheckpoint]` — paginated PR + Issue fetch with resumable checkpoints, time-window filtering, per-doc failure isolation.
- `PermSyncConnector` — translates GitHub repo visibility + team membership into the Hiveloop ACL model.
- `SlimConnector` — cheap doc-ID-only listing for prune diffing.

Plus the user-experience floor we agreed on: an `IndexingStart` column on `RAGSource` so admins of huge repos (`golang/go`, `rust-lang/rust`, etc.) can cap the initial-index window without us building a separate UI.

After 3D, the `enabled = true AND status = 'INITIAL_INDEXING'` partial index on `rag_sources` produces actual work for actual workers, batches reach the Rust `IngestBatch` gRPC, and a query against an indexed dataset returns real PR and Issue content. 3E adds the API to create those rows; 3F is the end-to-end test.

---

## Important Notes

### Auth model: Nango proxy, no token in our process

Onyx accepts a raw `github_access_token` in `credentials["github_access_token"]` (`backend/onyx/connectors/github/connector.py:457-468`). Hiveloop does not. Every external integration goes through Nango's **proxy endpoint** — the platform owns OAuth, token refresh, revocation, and the wire to GitHub. The Nango access token never leaves Nango's process.

The pattern is already in `internal/nango/client.go`:
`ProxyRequest(ctx, method, providerConfigKey, connectionID, path, queryParams, body) → response`

Hiveloop hands Nango the high-level intent (`GET /repos/acme/widget/pulls?state=all&page=1` for connection `xyz`), Nango injects the bearer header, hits GitHub, returns the response body + status + headers.

The connector consequence:

1. The admin connects GitHub through the existing Nango UI flow → an `in_connections` row is created with `provider_config_key = "github"` and a `nango_connection_id`.
2. The `RAGSource` row stores its associated `in_connection_id` (3A column already present).
3. The factory `Build(src Source, nango *nango.Client)` constructs the connector with the Nango client + connection ID. No token is fetched, persisted, or even named.
4. Every GitHub call routes through `nango.ProxyRequest`.

Operationally this is strictly better than holding a token in-process: nothing to log, nothing to leak in a goroutine dump, nothing to invalidate on rotation, no oauth2 round-trip to manage on 401. Onyx's `load_credentials(credentials: dict)` (`connector.py:457-468`) does not have a Hiveloop equivalent — there is no credentials dict; there is a connection ID.

### Direct types over `go-github`

The temptation is to import `github.com/google/go-github` for typed structs and pagination helpers. We don't, because the value-to-cost ratio is poor when our wire transport is Nango's proxy:

- We call exactly 6 endpoints (`/repos/...`, `/repos/.../pulls`, `/repos/.../issues`, `/repos/.../collaborators`, `/repos/.../teams`, `/orgs/.../members`).
- The 80 lines of structs we need are a tiny slice of go-github's type tree, and we'd have to translate them into our own `Document`/`ExternalAccess` shapes anyway.
- go-github expects to own the `*http.Client`. Routing through Nango means a `RoundTripper` that translates real HTTP into function calls into Nango — extra ceremony for marginal payoff.
- It pulls a non-trivial transitive dep tree.

Instead, this package owns thin GitHub types in `types.go` (~80 LOC: `GithubPR`, `GithubIssue`, `GithubUser`, `GithubRepo`, `GithubTeam`, `GithubMembership` — only the fields we map to a `Document` or use for ACL), a pagination helper in `pagination.go` that parses the `Link: <...>; rel="next"` response header (~25 LOC), and a rate-limit detector in `rate_limit.go` that triggers on `403 + X-RateLimit-Remaining: 0` and parses `X-RateLimit-Reset` (~15 LOC).

The connector becomes a self-contained package: `client.go` is a thin wrapper around `nango.ProxyRequest`, `fetch_prs.go` calls it with the right path + query and `json.Unmarshal`s the bytes into `[]GithubPR`, `document.go` maps `GithubPR → Document`. Reads top-to-bottom; no library opacity.

PyGithub's `SerializedRepository` workaround (Onyx `models.py:8-17`) doesn't apply here either — our types are plain structs, no lazy loading, no serialization gymnastics. The checkpoint stores `int64 ID` + `string FullName` and re-fetches by full name when needed.

### Drop the cursor-fallback paginator

Onyx's `_get_batch_rate_limited` falls back from offset to cursor pagination when GitHub returns `422 + "cursor"` (`connector.py:166-226`). That branch is a PyGithub bug-workaround for repos with >40k results: PyGithub eagerly fetches `.raw_data` on every object, which trips a server-side over-fetch. Our direct REST calls don't have this problem — we just ask for `?page=N&per_page=100`. Offset-only, identical to Onyx's first branch.

If we ever do hit a real 422 in production, we add cursor support then. Premature port = unjustified code volume. The plan covers retry on 403/RateLimit instead, which is the realistic failure mode.

### `IndexingStart` becomes a column on `RAGSource`

Per the user discussion: default behaviour is "index every PR and Issue from the start of the repo's history" (matching Onyx — `run_docfetching.py:419-451`). For a 60k-PR repo this is ~$0.60 of embedding cost on `text-embedding-3-large`; for a 5k-PR repo it's $0.06. Both fine.

For pathological repos (Linux kernel mirrors, monorepos with hundreds of thousands of PRs) the admin needs an escape hatch. Onyx has `Connector.indexing_start` (`backend/onyx/db/models.py`); we add the same column to `RAGSource` from day 1.

```
IndexingStart *time.Time  // optional floor on the poll window; NULL = epoch
```

No migration code. Gorm `AutoMigrate` adds the column on next boot. Connector reads it via `Source.Config()` extension (Source interface returns `*RAGSource` indirectly, but direct field access on the row inside the worker is cleaner — `tasks/ingest.go` already loads the row).

### Time-window semantics match Onyx exactly

The window passed into `LoadFromCheckpoint(ctx, start, end)` is computed by the worker (in `internal/rag/tasks/ingest.go` from 3C) using the same logic as `run_docfetching.py:419-451`:

```
earliest = source.IndexingStart  ?? epoch
if from_beginning OR last_successful_index_time IS NULL:
    window_start = earliest
else:
    window_start = max(earliest, last_successful_index_time - 5 min)
window_end = now()
```

The connector then walks the start back another 3 hours internally (Onyx `connector.py:802`) to catch late-arriving updates. That 3h overlap is a connector-level invariant, not a worker-level one — it stays inside the GitHub package.

### File layout under the 300-line ceiling

```
internal/rag/connectors/github/
  doc.go               existing 13-line stub (kept)
  init.go              ~25 lines — registers factory in init()
  config.go            ~80 lines — GithubConfig struct + JSON validation
  config_test.go       ~120 lines — config validation tests
  types.go             ~120 lines — GithubPR, GithubIssue, GithubUser, GithubRepo, GithubTeam, GithubMembership (json-tagged)
  proxy.go             ~80 lines — proxyClient interface + thin nango wrapper that decodes a json response into T
  pagination.go        ~50 lines — parse Link header, return next page (number, ok)
  rate_limit.go        ~70 lines — detect 403+X-RateLimit, parse X-RateLimit-Reset, retry helper
  rate_limit_test.go   ~110 lines — fake-proxy replay of 403 + reset-time
  checkpoint.go        ~60 lines — GithubCheckpoint struct + Stage enum + JSON
  checkpoint_test.go   ~80 lines — marshal round-trip tests
  connector.go         ~180 lines — top-level GithubConnector + ValidateConfig
  fetch_prs.go         ~180 lines — PR page-walk, filter, convert
  fetch_issues.go      ~170 lines — Issue page-walk (skip PR-shaped issues)
  document.go          ~130 lines — PR/Issue → Document mapping
  document_test.go     ~200 lines — body-text, metadata, owners, ACL mapping
  perm_sync.go         ~220 lines — visibility check + group enumeration
  perm_sync_test.go    ~250 lines — public/private/internal repo coverage
  slim.go              ~70 lines — SlimDocs implementation
  slim_test.go         ~90 lines — IDs-only round-trip
  errors.go            ~50 lines — ConnectorFailure constructors
  fixtures_test.go     ~150 lines — fake proxyClient + testdata JSON loaders
  testdata/            recorded GitHub JSON responses (PRs, Issues, repo, members, teams)
  e2e_test.go          ~200 lines — full ingest path with fake proxy
```

Largest file is `perm_sync_test.go` at ~250 lines. Headroom for growth without splits. The library-replacement files (`types.go`, `proxy.go`, `pagination.go`, `rate_limit.go`) total ~320 lines — about three days of work avoided by not pulling go-github, paid for by code we'd write anyway to translate go-github's structs into our `Document` shape.

### Onyx ACL prefix conventions match what 3A/3B already exposed

Onyx group ID forms (`backend/ee/onyx/external_permissions/github/utils.py:249-277`):

- `github__{repo_id}_collaborators`
- `github__{repo_id}_outside_collaborators`
- `github__{org_id}_organization`
- `github__{team-slug}` (raw team slug)

Hiveloop's `BuildExtGroupName(group, source)` (3B) produces lowercase `"github_<groupName>"`. Same shape, lowercase enforced. The connector uses `acl.PrefixExternalGroup` directly — no GitHub-specific prefix logic in the connector body.

Visibility translation (Onyx `utils.py:28-33`):

| GitHub repo `visibility` | Hiveloop `ExternalAccess` |
|---|---|
| `public` | `IsPublic = true`, no groups |
| `private` | `IsPublic = false`, groups = `{collaborators, outside_collaborators, all team-slugs}` |
| `internal` | `IsPublic = false`, groups = `{<org_id>_organization}` |

### What we don't port from Onyx

- **`indexing_start` validation tied to plan tier / time-bound** — Hiveloop has no per-tier ceiling concept yet. Add later if needed.
- **`repeated_error_state` connector-disable logic** — flagged in 3C plan as a follow-up; out of 3D scope. The watchdog 3C ships handles the in-flight crash case; the "this connector keeps failing every retry" pattern needs a counter on `RAGSource` and a transition to `status = 'ERROR'`. Worth ~50 LOC, but not gating 3D.
- **PR review comments / file diffs** — Onyx fetches PR body only (`connector.py:268`). We match.
- **Issue comments** — Onyx defines `_fetch_issue_comments` but never calls it from the main loop. We match (skip).
- **GitHub App auth** — Nango handles OAuth-flavored tokens only today. App-token support is a Nango concern, not 3D.
- **Enterprise base URL** — Nango handles GitHub Enterprise as a separate `providerConfigKey` ("github-enterprise"); routing is platform-level, not connector-level. The `RAGSource → InConnection` link picks the right Nango integration. No `EnterpriseBaseURL` config field needed in 3D.

---

## Implementation Strategy

### Layer A — Schema delta on `RAGSource`

| Change | Where | Notes |
|---|---|---|
| Add `IndexingStart *time.Time` column | `internal/rag/model/rag_source.go` | gorm tag `index`, nullable, default NULL. No migration script — `AutoMigrate` handles it. |
| Update `Config()` accessor (if needed) | same file | Probably unchanged — connector reads the field directly off the loaded row. |
| Add `TestRAGSource_IndexingStartFloor` | existing `rag_source_test.go` | Verify column persists, NULL default. One assertion. |

### Layer B — `internal/rag/connectors/github/` (the bulk)

Implement bottom-up — config + client + checkpoint first, then fetch + perm-sync, then registration.

| File | Purpose | Onyx mapping |
|---|---|---|
| `config.go` | `GithubConfig{RepoOwner, Repositories []string, StateFilter, IncludePRs, IncludeIssues}`. JSON-loaded from `Source.Config()`. | `connector.py:442-455` constructor args |
| `types.go` | Plain Go structs with `json` tags for the GitHub responses we consume: `GithubPR`, `GithubIssue`, `GithubUser`, `GithubRepo` (incl. `Visibility` + `Owner`), `GithubTeam`, `GithubMembership`. Field set is the strict minimum the rest of the package needs. | covers what `from github.PullRequest import *` would give us in PyGithub-land |
| `proxy.go` | `proxyClient` interface (the testing seam) + `newProxyClient(nango, connectionID)` returning the production implementation. One generic helper `getJSON[T any](ctx, p, method, path, query) (T, http.Header, error)` that calls `p.ProxyRequest`, decodes JSON, surfaces non-2xx as a typed error. | (no Onyx analogue — Python relies on PyGithub) |
| `pagination.go` | `nextPage(headers http.Header) (int, bool)` — parses GitHub's `Link: <...page=N>; rel="next"` header. Used by every list endpoint. | (no Onyx analogue — PyGithub hides this) |
| `rate_limit.go` | `isRateLimited(status int, headers http.Header) (resetAt time.Time, ok bool)` and `withRateLimitRetry(ctx, op func() error) error` — wraps a GitHub call, on rate-limited response sleeps until `resetAt + 60s` (Onyx's margin), max 5 attempts. | `rate_limit_utils.py:13-25` |
| `client.go` | `newClientForConnection(nango, connectionID) *Client` returns a `Client` struct that holds the `proxyClient` and exposes typed methods (`ListPullRequests`, `ListIssues`, `GetRepo`, `ListCollaborators`, `ListTeams`, `ListOrgMembers`). Each method is a 5–10 line wrapper around `getJSON[T]` + `withRateLimitRetry` + `nextPage`. No token enters the process. | `connector.py:457-468` (`load_credentials`) — different shape because there is no token |
| `checkpoint.go` | `GithubCheckpoint{Stage, RepoIDsRemaining []int64, CurrentRepoID *int64, CurrentRepoFullName *string, CurrPage int, LastSeenUpdatedAt *time.Time}`. `Stage` enum: `START`, `PRS`, `ISSUES`, `DONE`. JSON marshal/unmarshal + `interfaces.Checkpoint` conformance. | `connector.py:398-422` + `models.py:8-17` (no SerializedRepository — our types are plain structs) |
| `connector.go` | Top-level `GithubConnector` struct; `Kind() string`, `ValidateConfig()`, `LoadFromCheckpoint(ctx, src, ck, start, end) (<-chan DocumentOrFailure, ck, error)`. Orchestrates: pop next repo from checkpoint, dispatch to `fetch_prs.go` or `fetch_issues.go` based on stage, advance stage. | `connector.py:567-786` (`_fetch_from_github` orchestrator) |
| `fetch_prs.go` | `fetchPRs(ctx, client, repo, ck, start, end, out chan)` — page-walk `client.ListPullRequests`, reverse-chronological, early-break when `updated_at < start`, convert each via `document.go`, emit failures per-PR via `errors.go`. | `connector.py:611-690` |
| `fetch_issues.go` | Same shape; filter out PR-shaped issues (`issue.PullRequest != nil`). | `connector.py:694-754` |
| `document.go` | `prToDocument(pr GithubPR, repoExtAccess) Document`, `issueToDocument(issue GithubIssue, repoExtAccess) Document`. Maps body → `Sections[0].Content`, title → `SemanticID`, URL → `Link`, owners (author + assignees) → `PrimaryOwners`/`SecondaryOwners`, repo visibility → `IsPublic`/`ACL`. | `connector.py:247` (`_convert_pr_to_document`), `:336` (`_convert_issue_to_document`), `:283-327` (PR metadata), `:364-394` (issue metadata) |
| `perm_sync.go` | `LoadPermSync(ctx, src) <-chan ExternalGroupOrFailure`. For each configured repo, compute visibility, enumerate collaborators + outside-collaborators + teams (private) or org members (internal), emit `ExternalUserGroup{ID, UserEmails}` + the per-doc `ExternalAccess`. | `backend/ee/onyx/external_permissions/github/{doc_sync,group_sync,utils}.py` |
| `slim.go` | `SlimDocs(ctx, src, start, end) <-chan SlimDocumentOrFailure`. Same fetch loop as `connector.go` but emits `SlimDocument{ID, ExternalAccess}` instead of full `Document`. Reuses `fetch_prs.go`/`fetch_issues.go` helpers via a `slim bool` flag. | `connector.py:837-889` |
| `errors.go` | `docFailure(docID, link, message string, err error) ConnectorFailure` and `entityFailure(message string, err error) ConnectorFailure`. Thin wrappers around `interfaces.ConnectorFailure`. | `connector.py:654-668`, `:741-754` |
| `init.go` | `init() { interfaces.Register("github", build) }` where `build(src, nango) (Connector, error)` constructs `GithubConnector` from config + Nango client. | Onyx's `ConnectorBuilder` registry pattern (varies; we use a straight init-time register) |

### Layer C — Worker integration (small)

The 3C `tasks/ingest.go` handler already knows how to drive a `CheckpointedConnector`. Two small touches:

| Change | Where | Notes |
|---|---|---|
| Read `IndexingStart` when computing `window_start` | `internal/rag/tasks/ingest.go` | `earliest = src.IndexingStart ?? epoch` |
| Import `_ "internal/rag/connectors/github"` somewhere reachable from `cmd/server` | likely `internal/rag/connectors/all.go` (new) or `cmd/server/main.go` | Triggers `init()` so the factory registers |

### Layer D — Test fixtures

Tests record real GitHub JSON responses once and replay them through the fake `proxyClient`. Layout:

```
testdata/
  pulls_open_page1.json       (recorded fixture)
  pulls_open_page2.json
  issues_all_page1.json
  repos_acme_widget.json
  collaborators_widget.json
  teams_widget.json
  org_members_acme.json
```

Because every GitHub call goes through Nango's `ProxyRequest`, the test seam lives at the Nango boundary, not at `api.github.com`. The connector internally takes a small interface (defined in this package) covering only what it uses:

```
type proxyClient interface {
    ProxyRequest(ctx, method, providerConfigKey, connectionID, path string, query url.Values, body io.Reader) ([]byte, http.Header, int, error)
}
```

Production passes `*nango.Client` (which already implements that surface). Tests pass a fake whose `ProxyRequest` matches `(method, path, query)` against fixture files and returns the bytes plus the recorded headers. This is the standard Go interface-at-the-edge testing pattern; no httptest server, no fake Nango HTTP protocol.

`fixtures_test.go` provides:
- `newFakeProxy(t, testdataDir) proxyClient` — loads `testdata/*.json` keyed by `(method, path)`, returns a `proxyClient` that serves them with appropriate `X-RateLimit-*` headers.
- `(*fakeProxy).injectRateLimit(n int)` — the next N calls return `403 + X-RateLimit-Reset` 1s ahead.
- `(*fakeProxy).injectMalformed(path string)` — the next call to `path` returns invalid JSON, exercising per-doc failure isolation.

For perm-sync, the fixture covers all three visibilities (`public_repo.json`, `private_repo.json`, `internal_repo.json`) with corresponding member/team lists.

No real GitHub API in CI. A separate optionally-tagged smoke test (`//go:build smoke`) wires a real `*nango.Client` against a known public repo for sanity, run manually before each release. Out of CI scope.

---

## Tests

Same discipline as 3C — integration only, real Postgres + real Redis + real Asynq + the fake `proxyClient` in place of Nango/GitHub. The fake is a fixture replayer, not a behavioral mock; it returns whatever bytes are recorded under `testdata/`.

### Connector + config tests (`internal/rag/connectors/github/*_test.go`)

1. **`TestConfig_ValidatesRepoOwnerRequired`** — empty `RepoOwner` → `ValidateConfig` returns error.
2. **`TestConfig_ValidatesStateFilterEnum`** — `state_filter = "merged"` is not a valid GitHub state → error.
3. **`TestConfig_RepositoriesParsedAsList`** — single string `"a,b,c"` is normalized to three entries.
4. **`TestCheckpoint_MarshalRoundTrip`** — stage transitions + repo cursor + page counter survive JSON round-trip.

### Fetch tests (single fixture repo, ~25 PRs across 3 pages)

5. **`TestFetchPRs_PaginatesUntilExhausted`** — 25 PRs across 3 pages → 25 `Document`s emitted, checkpoint final stage = `DONE`.
6. **`TestFetchPRs_StateFilterOpenSkipsClosed`** — fixture has 15 open + 10 closed; `state_filter = "open"` returns 15.
7. **`TestFetchPRs_TimeWindowEarlyBreak`** — `start = T`, fixture has 10 PRs older than T; iterator stops early without fetching the older page.
8. **`TestFetchPRs_OverlapWindowCatchesLateUpdates`** — last successful = T, fixture has a PR updated at `T - 1h`; with the connector's 3h overlap, the PR is re-emitted.
9. **`TestFetchIssues_SkipsPRShapedIssues`** — fixture `/issues` returns 5 entries, 3 of which have `pull_request != nil`; only 2 issues emitted.
10. **`TestPRConvertedToDocument_FieldsMatch`** — sample PR → expected `Document{DocID, SemanticID, Link, Sections[0].Content, Metadata, PrimaryOwners}`.
11. **`TestIssueConvertedToDocument_FieldsMatch`** — same for an issue.

### Failure tests

12. **`TestPerDocFailure_ContinuesBatch`** — fixture injects malformed JSON at PR index 7 of 25; final batch has 24 documents + 1 `ConnectorFailure` for PR #7; iteration completes.
13. **`TestRateLimit_RetriesUntilReset`** — fixture returns 403 + `X-RateLimit-Reset` 1s ahead; connector waits, retries, succeeds. Asserts elapsed ≥ 1s.
14. **`TestRateLimit_AbortsAfterMaxAttempts`** — 6 consecutive 403s → connector returns terminal `EntityFailure`; checkpoint preserved for next attempt.

### Permission tests

15. **`TestPermSync_PublicRepoIsPublic`** — `visibility = "public"` → `ExternalAccess{IsPublic: true}`, no groups.
16. **`TestPermSync_PrivateRepoEnumeratesGroups`** — private repo + 3 collaborators + 1 outside-collaborator + 2 teams → 4 `ExternalUserGroup`s with prefix `external_group:github_<id>`, members listed.
17. **`TestPermSync_InternalRepoBindsOrgGroup`** — internal repo → single org group.

### Slim test

18. **`TestSlimDocs_ReturnsIDsOnly`** — same fixture as `TestFetchPRs_PaginatesUntilExhausted`; emits 25 `SlimDocument`s, none has `Sections` populated.

### IndexingStart test

19. **`TestIndexingStart_FloorsTheWindow`** — fixture has PRs from 2023-01 through 2025-04; `RAGSource.IndexingStart = 2024-01-01` → only post-2024 PRs emitted.

### Registration test

20. **`TestRegistration_GithubKindResolves`** — after import, `interfaces.Lookup("github")` returns a non-nil factory that builds a `GithubConnector`.

### End-to-end test (the validation test)

21. **`TestEndToEndIngestion_Through3CScheduler`** — wires 3C scheduler + 3D connector + fake Rust binary:
    - Insert `RAGSource` with `kind = "github"` and a Nango `connectionID`; build the connector with the fake `proxyClient` (loaded from a fixture repo of 25 PRs).
    - Trigger one ingest tick.
    - Assert: an `RAGIndexAttempt` row goes `IN_PROGRESS → COMPLETED`; the fake `IngestBatch` server received N batches totaling 25 documents; `LastSuccessfulIndexTime` advanced; checkpoint persisted with stage `DONE`.

### Definition of done

- All 21 tests pass on real Postgres + Redis (`make test-services-up`) plus the in-process fake `proxyClient`. No external network calls in CI.
- `scripts/check-go-file-length.sh` clean — no new entries in the allowlist.
- `interfaces.Lookup("github")` returns a working factory after `cmd/server` startup.
- `internal/rag/connectors/github` package added to the `PKGS` list in `.github/workflows/test-rag-e2e.yml`.
- No new dependencies added to `go.mod` — package is self-contained on top of `internal/nango`.

---

## Onyx ↔ Hiveloop reference index

| Onyx | Hiveloop (after 3D) |
|---|---|
| `backend/onyx/connectors/github/connector.py:437-1026` (`GithubConnector` class) | `internal/rag/connectors/github/connector.go` `GithubConnector` |
| `connector.py:457-468` (`load_credentials`) | `client.go` `newClientForConnection` + `proxy.go` `proxyClient` (auth lives in Nango; no token in our process) |
| `connector.py:537-551` + `:484-535` (repo selection) | `connector.go` `selectRepos` (single → list → org-wide) |
| `connector.py:567-786` (`_fetch_from_github` orchestrator) | `connector.go` `LoadFromCheckpoint` driver loop |
| `connector.py:611-690` (PR loop) | `fetch_prs.go` `fetchPRs` |
| `connector.py:694-754` (Issue loop with PR-shape skip) | `fetch_issues.go` `fetchIssues` |
| `connector.py:166-226` (`_get_batch_rate_limited`, offset pagination) | `fetch_prs.go` / `fetch_issues.go` build `?page=N&per_page=100` directly; `pagination.go` parses the `Link` response header for the next page |
| `connector.py:107-163` (cursor fallback) | NOT PORTED — direct REST calls don't have PyGithub's lazy-load 422 issue |
| `connector.py:247` (`_convert_pr_to_document`) + `:283-327` metadata | `document.go` `prToDocument` |
| `connector.py:336` (`_convert_issue_to_document`) + `:364-394` metadata | `document.go` `issueToDocument` |
| `connector.py:802` (3-hour overlap) | `connector.go` constant `pollOverlap = 3 * time.Hour` applied in `LoadFromCheckpoint` |
| `connector.py:837-889` (slim path) | `slim.go` `SlimDocs` |
| `connector.py:891-1015` (`validate_connector_settings`) | `connector.go` `ValidateConfig` (basic) + `client.go` `validateClient` (network) |
| `rate_limit_utils.py:13-25` (`sleep_after_rate_limit_exception`) | `rate_limit.go` `withRateLimitRetry` |
| `models.py:8-17` (`SerializedRepository`) | NOT PORTED — our types are plain structs, no lazy loading; checkpoint stores the repo ID and full name |
| `models.py:` `GithubConnectorCheckpoint`, `GithubConnectorStage` | `checkpoint.go` `GithubCheckpoint`, `Stage` |
| `backend/ee/onyx/external_permissions/github/doc_sync.py:34-142` | `perm_sync.go` `LoadPermSync` |
| `backend/ee/onyx/external_permissions/github/group_sync.py:14-51` | `perm_sync.go` `enumerateGroups` |
| `backend/ee/onyx/external_permissions/github/utils.py:249-277` (group ID forms) | `perm_sync.go` private helpers `collaboratorsGroupID`, `teamGroupID`, etc. — all routed through `acl.PrefixExternalGroup` |
| `backend/ee/onyx/external_permissions/github/utils.py:28-33` (visibility enum) | `perm_sync.go` `mapVisibility(github.Repository) ExternalAccess` |
| `backend/onyx/db/models.py` `Connector.indexing_start` | `internal/rag/model/rag_source.go` `RAGSource.IndexingStart` |
| `backend/onyx/background/indexing/run_docfetching.py:419-451` (window-start logic) | `internal/rag/tasks/ingest.go` (3C-owned) — small extension to read `IndexingStart` |
| Onyx `ConnectorFailure` (`connectors/models.py:486-504`) | `interfaces.ConnectorFailure` (3B-defined); thin constructors in `errors.go` |
