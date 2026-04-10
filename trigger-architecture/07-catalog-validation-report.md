# 07 — Catalog Validation Report

This is the record of a one-time audit that ran 31 parallel validation agents against the full GitHub catalog — every action in `github-app.actions.json` (273 actions), every trigger in `github.triggers.json` (24 triggers), every resource binding, and every schema reference. Each agent was assigned a focused slice of the catalog and was instructed to validate it against GitHub's official REST API and webhooks docs (sourced from `docs.github.com` and `github/rest-api-description`). The goal: find inconsistencies in structured form, with evidence URLs, so they could be fixed in one pass.

This document captures what the agents found, what was fixed, and what was intentionally deferred. It's reference material for anyone wondering "why does the catalog look the way it does" or "what do I need to watch out for if I re-run this audit for a different provider."

## Summary

| Metric | Count |
|---|---|
| Actions validated | 273 (pre-fix) → 278 (post-fix) |
| Triggers validated | 24 |
| Trigger refs validated | 148 |
| Schemas validated | 159 |
| **Total inconsistencies found** | **24** |
| Broken schema refs (`$ref` chains) | 0 |
| Orphaned schemas (defined but never referenced) | 15 (cosmetic) |

Every inconsistency that affected runtime behavior was fixed. Cosmetic issues (naming inconsistencies, orphaned schemas) were logged and deferred.

## What the audit found

### 1. Wrong `access` field — 7 write endpoints marked "read"

These were POST/PUT endpoints that mutate state but were tagged `"access": "read"`, which would let a read-scoped context action hit them unintentionally. Four came from a substring-matching bug in `inferAccess` (`strings.Contains("submit_review", "view")` → false positive). Two more came from POST endpoints without any read-keyword substring that got swept up by accident. One was a `repos_add_status_check_contexts` POST that clearly writes state.

| Action | Endpoint | Why misclassified |
|---|---|---|
| `pulls_request_reviewers` | `POST /repos/{owner}/{repo}/pulls/{pull_number}/requested_reviewers` | `review` substring |
| `pulls_create_review` | `POST /repos/{owner}/{repo}/pulls/{pull_number}/reviews` | `review` substring |
| `pulls_submit_review` | `POST /repos/{owner}/{repo}/pulls/{pull_number}/reviews/{review_id}/events` | `review` substring |
| `pulls_create_review_comment` | `POST /repos/{owner}/{repo}/pulls/{pull_number}/comments` | `review` substring |
| `pulls_create_reply_for_review_comment` | `POST /repos/{owner}/{repo}/pulls/{pull_number}/comments/{comment_id}/replies` | `review` substring |
| `actions_review_pending_deployments_for_run` | `POST /repos/{owner}/{repo}/actions/runs/{run_id}/pending_deployments` | `review` substring |
| `repos_add_status_check_contexts` | `POST /repos/{owner}/{repo}/branches/{branch}/protection/required_status_checks/contexts` | `check` substring |

**Fix**: generator logic rewrite. `inferAccess` now tokenizes the action key on `_` and checks the first two tokens against an exact-string set of read verbs, instead of substring-matching across the entire key. In GitHub operationIds the verb is typically `tokens[1]` (e.g., `issues_list_comments`); for verb-first keys like `search_repos`, it's `tokens[0]`. Both positions are checked. Fix lives in `cmd/fetchactions-oas3/parse.go`.

### 2. Missing required body parameters — 5 actions unusable

Five actions had body parameters wrapped in a `oneOf` schema that the generator never walked. The body schemas looked like this:

```yaml
requestBody:
  content:
    application/json:
      schema:
        oneOf:
          - type: object
            properties:
              labels: { type: array, items: { type: string } }
          - type: array
            items: { type: string }
```

The old parser only looked at top-level `properties` — both `oneOf` alternatives were invisible to it, so the `labels` parameter never made it into the action definition. Without the parameter, callers literally could not supply labels. Same pattern affected the `contexts` parameter on three `status_check_contexts` endpoints.

| Action | Missing | Notes |
|---|---|---|
| `issues_add_labels` | `labels` (array of strings) | Without this, `POST /repos/.../labels` has no body at all |
| `issues_set_labels` | `labels` (array of strings) | Without this, `PUT` behaves as remove-all |
| `repos_add_status_check_contexts` | `contexts` (array of strings) | Plus the access-bug fix above |
| `repos_set_status_check_contexts` | `contexts` (array of strings) | |
| `repos_remove_status_check_contexts` | `contexts` (array of strings) | |
| `actions_review_pending_deployments_for_run` | `environment_ids` schema lacked `items: integer` | Generic array type only |

**Fix**: generator now walks `oneOf`/`anyOf` alternatives on request body schemas. For each object alternative, it merges the `Properties` and `Required` lists into the combined parameter set. Same file as above, `parse.go`.

### 3. Missing actions — 7 endpoints documented but absent from catalog

Seven real GitHub endpoints didn't appear in the catalog at all. The generator uses resource-based path filtering for GitHub: each resource has a list of path prefixes, and actions are included only if their path matches a prefix. Seven paths weren't in the prefix list of any resource.

| Expected key | Endpoint | Status after fix |
|---|---|---|
| `repos_compare_commits` | `GET /repos/{owner}/{repo}/compare/{basehead}` | Added via new `/compare` prefix on `repository` |
| `repos_merge` | `POST /repos/{owner}/{repo}/merges` | Added via new `/merges` prefix on `repository` |
| `repos_merge_upstream` | `POST /repos/{owner}/{repo}/merge-upstream` | Added via new exact-path entry on `repository` |
| `issues_list_assignees` | `GET /repos/{owner}/{repo}/assignees` | Added via new `/assignees` prefix on `issue` |
| `issues_check_user_can_be_assigned` | `GET /repos/{owner}/{repo}/assignees/{assignee}` | Covered by same prefix |
| `pulls_list_branches_or_pull_requests_for_head_commit` | `GET /repos/{owner}/{repo}/commits/{commit_sha}/pulls` | Already covered by existing `/commits` prefix — the audit's expected name was speculative; the real action is `repos_list_pull_requests_associated_with_commit` which is already present |
| `repos_upload_release_asset` | `POST https://uploads.github.com/...` | Not fixable — uses a separate host |

**Fix**: five actions added via `cmd/fetchactions-oas3/config.go` updates to the `githubResources()` function. Total actions went from 273 to 278.

`repos_upload_release_asset` remains missing and is a known limitation — the action's endpoint is on `uploads.github.com`, not `api.github.com`, and the current catalog assumes a single host per provider. Fixing this would require a per-action override field on `ExecutionConfig` or a separate synthetic provider entry. Deferred.

### 4. Trigger ref bugs — 3 invalid refs in `github.triggers.json`

Three refs in hand-maintained trigger definitions pointed at fields the webhook payload doesn't contain. They would resolve to empty strings at dispatch time, silently producing broken paths and broken resource keys.

**`workflow_job.completed`** (line 217):
- `run_id: workflow_run.id` → INVALID. The `workflow_job` event payload has no `workflow_run` object — it has a `workflow_job` object. Correct path: `workflow_job.run_id`.
- `workflow_name: workflow_run.name` → INVALID. Should be `workflow_job.workflow_name`.

**`check_run.completed`** (line 231):
- `workflow_name: workflow_run.name` → INVALID. `check_run` events have no `workflow_run` object, and `check_run` itself has no `workflow_name` field (check runs aren't Actions-exclusive; any GitHub App can publish them). The closest field is `check_run.name`.

**Fix**: hand-edited `github.triggers.json` to correct the refs:

```diff
  "workflow_job.completed": {
    "refs": {
-     "run_id": "workflow_run.id",
-     "workflow_name": "workflow_run.name"
+     "run_id": "workflow_job.run_id",
+     "workflow_name": "workflow_job.workflow_name"
    }
  },
  "check_run.completed": {
+   "description": "Triggered when a check run completes (any GitHub App-reported check, including CI)",
    "refs": {
-     "run_id": "check_run.id",
-     "workflow_name": "workflow_run.name"
+     "check_run_id": "check_run.id",
+     "check_run_name": "check_run.name"
    }
  }
```

Also loosened the `check_run.completed` description from "CI status check" since check runs come from any GitHub App, not just CI.

### 5. Trigger schema reference error

The `pull_request_review_comment_event` schema at line 562 used `schema_ref: "issue-comment"` for the comment object. This was wrong — GitHub's actual schema for review comments is `pull-request-review-comment`, which has a different field set (no issue reference, adds `diff_hunk`, `path`, `position`, etc.).

**Fix**: changed the `schema_ref` in `github.triggers.json`:

```diff
  "comment": {
    "type": "object",
    "description": "The review comment object including id, diff_hunk, path, position, ...",
-   "schema_ref": "issue-comment"
+   "schema_ref": "pull-request-review-comment"
  }
```

### 6. `create`/`delete` trigger payload aliasing

For GitHub's `create` and `delete` events, `payload.ref` is a **short** name (e.g., `"main"`, `"simple-tag"`), NOT the full `refs/heads/...` path the `push` event uses. The triggers exposed both `branch_name → ref` and `ref → ref`, which was misleading — consumers familiar with `push`'s full-path `ref` would expect the same semantics and get burned.

Also, `resource_type` was set to `branch`, but these events also fire for tags.

**Fix**:

```diff
  "create": {
-   "description": "Triggered when a branch or tag is created",
+   "description": "Triggered when a branch or tag is created (payload.ref is the short name, e.g. 'main' or 'v1.0', not a full refs/heads/ path)",
-   "resource_type": "branch",
+   "resource_type": "repository",
    "refs": {
      "branch_name": "ref",
-     "ref": "ref",
      "ref_type": "ref_type"
    }
  }
```

Same change for `delete`. Dropping the duplicate `ref` alias removes the footgun. Setting resource type to `repository` is more accurate because these events fire for both branches AND tags, and there's no coherent "branch vs tag" resource distinction worth encoding.

### 7. Resource ref_bindings referencing undefined refs

Four `ref_bindings` in `github.actions.json` resources pointed at refs that no trigger actually exposes. They would fail to resolve at dispatch time.

| Resource | Broken binding | Reason |
|---|---|---|
| `milestone` | `milestone_number → $refs.milestone_number` | No trigger exposes `milestone_number` |
| `organization` | `org → $refs.org` | No trigger exposes `org` |
| `team` | `org → $refs.org` | Same |
| `team` | `team_slug → $refs.team_slug` | Same |

**Fix**: removed the broken bindings from the generator config (`cmd/fetchactions-oas3/config.go`):

```diff
  "milestone": {
    RefBindings: map[string]string{
      "owner": "$refs.owner",
      "repo":  "$refs.repo",
-     "milestone_number": "$refs.milestone_number",
    },
  },
  "organization": {
-   RefBindings: map[string]string{
-     "org": "$refs.org",
-   },
  },
  "team": {
-   RefBindings: map[string]string{
-     "org":       "$refs.org",
-     "team_slug": "$refs.team_slug",
-   },
  },
```

If a future trigger eventually exposes `org` or `team_slug` refs (e.g., a hypothetical `membership.added` trigger), these can be re-added.

### 8. Naming inconsistencies — cosmetic, no runtime impact

- **`issues_add_blocked_by_dependency`** vs sibling `issues_list_dependencies_blocked_by` / `issues_remove_dependency_blocked_by`. The add action uses verb-first suffix; list and remove use resource-first. One family, two naming conventions. No runtime impact, but confusing to discover. **Deferred** — changing the key breaks any existing saved triggers referencing it.
- **`issues_list_events_for_timeline`** is misleading. GitHub's endpoint is literally "List timeline events for an issue" at `/timeline` (not `/events/timeline`). A cleaner name would be `issues_list_timeline_events`. **Deferred** — same key-stability concern.
- **`repos_sync_branch_with_upstream`** is not an official operationId; the correct key is `repos_merge_upstream`. The old catalog had it wrong but didn't include the endpoint at all (see section 3), so this is now resolved — only `repos_merge_upstream` exists in the post-fix catalog.

### 9. Orphaned schemas — 15 unreferenced but defined

Fifteen schemas are defined in the catalog but never referenced by any action's `response_schema` or by another schema's `schema_ref`:

```
branch-short, collaborator, contributor, deployment, diff-entry,
environment-approvals, hook-delivery-item, integration,
issue-event-for-issue, issue-field-value, pending-deployment,
review-comment, short-branch, status, timeline-issue-events
```

A few are clearly naming drift: `branch-short` vs `short-branch` (both defined, neither directly referenced — only `branch-short_list` is used). Singular forms like `collaborator`, `contributor`, `review-comment`, `status` exist alongside their `_list` variants, which ARE referenced. A handful (`timeline-issue-events`, `issue-event-for-issue`) look like predecessors to currently-referenced event schemas.

The audit's orphan detection was conservative — it only followed `schema_ref` fields, not `items.$ref` (which is a different field). The real orphan count is probably lower than 15 once you follow both. No runtime impact in any case.

**Status**: deferred. The schemas cost nothing to keep around, and removing them would require a careful regeneration to avoid breaking any frontend consumer that reads the raw JSON.

## What drove the audit structure

The 31 agents were divided along natural seam lines:

- **Per-resource action validators** (~10 agents) — one each for issues CRUD, pulls CRUD, pulls reviews, issues labels, milestones, branch protection, org members, teams, workflows, actions
- **Trigger validators** (~8 agents) — one each for issues triggers, PR CRUD triggers, PR review triggers, workflow triggers, push/branch triggers, release/discussion triggers, plus a cross-check validator for refs and a validator for trigger schema references
- **Cross-cutting validators** (~5 agents) — resource ref_bindings, schema references, naming conventions, orphan detection, misc actions

Each agent was given:

1. A specific slice of the catalog to validate (by action keys or trigger keys)
2. The authoritative source (GitHub REST API docs URL, GitHub webhook docs URL, or `github/rest-api-description` OpenAPI spec path)
3. A strict output format with one of three statuses: `VALID`, `INCONSISTENT`, or `MISSING`, along with evidence URLs

The strict format was critical for later consolidation. When all 31 agents returned their reports, a simple aggregation produced the findings in this document with provenance URLs for each issue.

## What this audit doesn't cover

- **Other providers**. Only GitHub was audited. Intercom, Slack, Linear, Stripe, etc. are un-audited. Their catalogs are mostly generated from OpenAPI specs and inherit the same class of bugs (oneOf, access misclassification) that were fixed in the generator — but no one has verified them end-to-end against provider docs.
- **Dynamic behaviors**. The audit validates static catalog shape against static docs. It doesn't verify that an action actually works at runtime against the provider — for that, you need integration tests against a real Nango connection, which is a separate concern.
- **Trigger payload schemas**. The trigger refs were validated against the webhook docs, but the full payload schema shapes (used by the frontend autocompleter) were not individually audited. Schema correctness here is looser.
- **Regression prevention**. This was a one-time audit. Re-running it on every catalog regeneration isn't automated, so catalog bugs can reappear with spec updates. A CI check that runs a scaled-down version of this audit on each catalog change would be worth building if the catalog starts drifting again.

## Where to go from here

- The generator fixes that landed as a result of this audit: `cmd/fetchactions-oas3/parse.go`, `config.go`
- What the generator currently validates at build time: [04-validation-and-safety.md](04-validation-and-safety.md) (section "Catalog-time: the generator")
- Limitations that this audit surfaced but deferred: [09-known-limitations.md](09-known-limitations.md)
