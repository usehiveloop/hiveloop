---
name: bugsink
description: Use when investigating Bugsink errors, issues, projects, events, stacktraces, regressions, or production exceptions. Provides verified curl and jq commands for the canonical Bugsink API using BUGSINK_URL and BUGSINK_TOKEN, and explains how to construct dashboard links using BUGSINK_DASHBOARD_BASE_URL instead of the proxy URL.
---

# Bugsink issue investigation

Use Bugsink through the Hiveloop-provided Bugsink proxy at `$BUGSINK_URL/api/canonical/0`.

`BUGSINK_URL` is provided by the Hiveloop runtime. Always call the provided `BUGSINK_URL` exactly; the runtime handles forwarding for the configured Bugsink connection.

`BUGSINK_DASHBOARD_BASE_URL` is the real Bugsink dashboard base URL. Use it only for human-facing links. Never construct dashboard links from `BUGSINK_URL`; `BUGSINK_URL` is a Hiveloop API proxy and is not the Bugsink UI host.

## Environment

Required:

| Variable | Purpose |
|---|---|
| `BUGSINK_URL` | Hiveloop-provided Bugsink proxy base URL |
| `BUGSINK_TOKEN` | Bearer token for the provided Bugsink endpoint |

Optional:

| Variable | Purpose |
|---|---|
| `BUGSINK_DASHBOARD_BASE_URL` | Real Bugsink dashboard base URL for links shown to users |

Normalize the URL once before calling the API:

```bash
test -n "$BUGSINK_URL" || { echo "BUGSINK_URL is not set" >&2; exit 1; }
test -n "$BUGSINK_TOKEN" || { echo "BUGSINK_TOKEN is not set" >&2; exit 1; }
BUGSINK_URL="${BUGSINK_URL%/}"
BUGSINK_API="$BUGSINK_URL/api/canonical/0"
BUGSINK_DASHBOARD_BASE_URL="${BUGSINK_DASHBOARD_BASE_URL%/}"
```

Every API call must include the bearer token:

```bash
curl -fsS "$BUGSINK_API/projects/" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | jq '{project_count: (.results | length)}'
```

## Rules

- Always pipe API JSON through `jq` and keep only fields needed for the task.
- Never paste raw event detail JSON into the conversation. Event detail includes the full `data` payload and can be large.
- Prefer list endpoints first, then retrieve the single project, issue, or event needed.
- Use Bugsink internal event `id` for `/events/{id}/` and `/events/{id}/stacktrace/`; `event_id` is the event id from the client payload.
- Treat `next` and `previous` as pagination cursors/links. Follow `next` only when the user needs more results.
- Do not print `$BUGSINK_TOKEN`.

## Dashboard links

Construct human-facing Bugsink links with `BUGSINK_DASHBOARD_BASE_URL`, not `BUGSINK_URL`.

If `BUGSINK_DASHBOARD_BASE_URL` is unset, say that the dashboard base URL is unavailable and provide the issue/project/event IDs instead. Do not substitute the Hiveloop proxy URL.

Known link patterns:

```text
Projects / teams:
$BUGSINK_DASHBOARD_BASE_URL/projects/teams/

Issue latest event:
$BUGSINK_DASHBOARD_BASE_URL/issues/issue/<issue_uuid>/event/last/

Specific issue event:
$BUGSINK_DASHBOARD_BASE_URL/issues/issue/<issue_uuid>/event/<event_uuid>/

Issue event details:
$BUGSINK_DASHBOARD_BASE_URL/issues/issue/<issue_uuid>/event/<event_uuid>/details/

Issue events list:
$BUGSINK_DASHBOARD_BASE_URL/issues/issue/<issue_uuid>/events/

Issue tags:
$BUGSINK_DASHBOARD_BASE_URL/issues/issue/<issue_uuid>/tags/

Issue history:
$BUGSINK_DASHBOARD_BASE_URL/issues/issue/<issue_uuid>/history/

Issue grouping:
$BUGSINK_DASHBOARD_BASE_URL/issues/issue/<issue_uuid>/grouping/
```

Example:

```text
$BUGSINK_DASHBOARD_BASE_URL/issues/issue/878164f1-b936-4470-8341-ebbbe49a0a05/event/last/
```

When returning an issue summary, include a `dashboard_url` field if `BUGSINK_DASHBOARD_BASE_URL` is set:

```bash
jq --arg base "$BUGSINK_DASHBOARD_BASE_URL" '
  .results[]
  | {
      id,
      title,
      last_seen,
      dashboard_url: (if $base == "" then null else ($base + "/issues/issue/" + .id + "/event/last/") end)
    }
'
```

## API schema discovery

If this skill does not cover an operation you need, inspect the machine-readable API schema before guessing:

```text
$BUGSINK_URL/api/canonical/0/schema/
```

Fetch the schema and inspect only the relevant paths/components:

```bash
curl -fsS "$BUGSINK_API/schema/" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  -o /tmp/bugsink-openapi.yaml
```

List available API paths:

```bash
grep -n '^  /api/canonical/0/' /tmp/bugsink-openapi.yaml
```

Find operations related to a resource:

```bash
grep -nE 'operationId:|/api/canonical/0/(issues|events|projects|teams|releases)' \
  /tmp/bugsink-openapi.yaml \
  | sed -n '1,220p'
```

Inspect schema fields for a response type without loading the whole spec into context:

```bash
sed -n '/    Issue:/,/    PaginatedIssueList:/p' /tmp/bugsink-openapi.yaml
sed -n '/    EventDetail:/,/    EventList:/p' /tmp/bugsink-openapi.yaml
sed -n '/    ProjectDetail:/,/    ProjectList:/p' /tmp/bugsink-openapi.yaml
```

## List teams

```bash
curl -fsS "$BUGSINK_API/teams/" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | jq '[.results[] | {id, name, visibility}]'
```

## List projects

Use this first when the user gives a project name, service name, or asks "what projects exist?"

```bash
curl -fsS "$BUGSINK_API/projects/" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | jq '[.results[] | {
      id,
      name,
      slug,
      team,
      visibility,
      stored_event_count,
      digested_event_count
    }]'
```

Find one project by name or slug:

```bash
PROJECT_QUERY="api"
curl -fsS "$BUGSINK_API/projects/" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | jq --arg q "$PROJECT_QUERY" -r '
      .results[]
      | select((.name // "" | ascii_downcase) == ($q | ascii_downcase)
          or (.slug // "" | ascii_downcase) == ($q | ascii_downcase))
      | .id
    '
```

Get project details after you know the numeric project id:

```bash
PROJECT_ID=2
curl -fsS "$BUGSINK_API/projects/$PROJECT_ID/" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | jq '{
      id,
      name,
      slug,
      team,
      visibility,
      stored_event_count,
      digested_event_count,
      alert_on_new_issue,
      alert_on_regression,
      alert_on_unmute,
      retention_max_event_count
    }'
```

## List issues in a project

The issues list endpoint requires `project=<numeric_project_id>`.

```bash
PROJECT_ID=2
curl -fsS "$BUGSINK_API/issues/?project=$PROJECT_ID&sort=last_seen&order=desc" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | jq '{
      next,
      previous,
      issues: [
        .results[]
        | {
            id,
            project,
            last_seen,
            first_seen,
            event_count: .stored_event_count,
            digested_event_count,
            type: .calculated_type,
            value: .calculated_value,
            transaction,
            resolved: .is_resolved,
            resolved_by_next_release: .is_resolved_by_next_release,
            muted: .is_muted
          }
      ][0:20]
    }'
```

Supported sort/order parameters:

- `sort=digest_order` or `sort=last_seen`
- `order=asc` or `order=desc`

Filter issue list results locally by error text, type, value, or transaction:

```bash
PROJECT_ID=2
QUERY="context canceled"
curl -fsS "$BUGSINK_API/issues/?project=$PROJECT_ID&sort=last_seen&order=desc" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | jq --arg q "$QUERY" '{
      matches: [
        .results[]
        | select(
            ((.calculated_type // "")
            + " "
            + (.calculated_value // "")
            + " "
            + (.transaction // ""))
            | test($q; "i")
          )
        | {
            id,
            last_seen,
            first_seen,
            type: .calculated_type,
            value: .calculated_value,
            transaction,
            count: .stored_event_count,
            resolved: .is_resolved,
            muted: .is_muted
          }
      ]
    }'
```

## Get issue details

Use an issue UUID from the issue list.

```bash
ISSUE_ID="bc66f84f-0657-40dd-b3fc-d280acc52c27"
curl -fsS "$BUGSINK_API/issues/$ISSUE_ID/" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | jq '{
      id,
      project,
      title: ((.calculated_type // "")
        + (if (.calculated_value // "") == "" then "" else ": " + .calculated_value end)),
      first_seen,
      last_seen,
      stored_event_count,
      digested_event_count,
      transaction,
      resolved: .is_resolved,
      resolved_by_next_release: .is_resolved_by_next_release,
      muted: .is_muted
    }'
```

## List events for an issue

The events list endpoint requires `issue=<issue_uuid>`. List results are lightweight and omit the full `data` payload.

```bash
ISSUE_ID="bc66f84f-0657-40dd-b3fc-d280acc52c27"
curl -fsS "$BUGSINK_API/events/?issue=$ISSUE_ID&order=desc" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | jq '{
      next,
      previous,
      events: [
        .results[]
        | {
            id,
            event_id,
            project,
            timestamp,
            ingested_at,
            digested_at,
            digest_order
          }
      ][0:20]
    }'
```

## Get one event without dumping raw payload

Retrieve event detail only for one event id. The response includes the full `data` payload, so always filter aggressively:

```bash
EVENT_ID="5f6b3d20-9ac4-4c8f-ad23-e5e6c12c69d7"
curl -fsS "$BUGSINK_API/events/$EVENT_ID/" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | jq '{
      id,
      event_id,
      issue,
      project,
      timestamp,
      platform: .data.platform,
      level: .data.level,
      logger: .data.logger,
      release: .data.release,
      environment: .data.environment,
      server_name: .data.server_name,
      message: (.data.message // .data.logentry.formatted // null),
      exception: (
        (.data.exception.values? // [])
        | map({
            type,
            value,
            module,
            mechanism: .mechanism.type
          })
        | .[0:3]
      ),
      breadcrumbs: (
        (
          (if (.data.breadcrumbs | type) == "array"
           then .data.breadcrumbs
           else (.data.breadcrumbs.values? // [])
           end) // []
        )
        | map({
            timestamp,
            category,
            type,
            level,
            message
          })
        | .[-10:]
      ),
      tags: (
        (
          .data.tags // []
          | if type == "array"
            then map(if type == "array" then {key: .[0], value: .[1]} else . end)
            else to_entries
            end
        )
        | .[0:20]
      )
    }'
```

If you need SDK/runtime context, request only key names or a narrow allowlist:

```bash
EVENT_ID="5f6b3d20-9ac4-4c8f-ad23-e5e6c12c69d7"
curl -fsS "$BUGSINK_API/events/$EVENT_ID/" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | jq '{
      sdk: .data.sdk,
      contexts: {
        runtime: .data.contexts.runtime,
        os: .data.contexts.os,
        device: .data.contexts.device,
        trace: .data.contexts.trace
      },
      modules: (.data.modules // {} | keys | .[0:50])
    }'
```

## Get event stacktrace

The stacktrace endpoint returns Markdown-like text and is safe to inspect directly. It returns `_No stacktrace available._` for log-only events.

```bash
EVENT_ID="a6c8658f-3884-472c-bdd9-13f46c73a5bb"
curl -fsS "$BUGSINK_API/events/$EVENT_ID/stacktrace/" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | sed -n '1,160p'
```

## Fast triage workflow

Use this sequence for most investigations:

```bash
# 1. Find the project id.
curl -fsS "$BUGSINK_API/projects/" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | jq '[.results[] | {id, name, slug, stored_event_count}]'

# 2. List recent unresolved issues.
PROJECT_ID=2
curl -fsS "$BUGSINK_API/issues/?project=$PROJECT_ID&sort=last_seen&order=desc" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | jq '[.results[]
      | select(.is_resolved == false and .is_muted == false)
      | {
          id,
          last_seen,
          count: .stored_event_count,
          type: .calculated_type,
          value: .calculated_value,
          transaction
        }
    ][0:10]'

# 3. Pick an issue and list recent events.
ISSUE_ID="bc66f84f-0657-40dd-b3fc-d280acc52c27"
curl -fsS "$BUGSINK_API/events/?issue=$ISSUE_ID&order=desc" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | jq '[.results[] | {id, timestamp, event_id, digest_order}][0:5]'

# 4. Pick one event and read the stacktrace.
EVENT_ID="a6c8658f-3884-472c-bdd9-13f46c73a5bb"
curl -fsS "$BUGSINK_API/events/$EVENT_ID/stacktrace/" \
  -H "Authorization: Bearer $BUGSINK_TOKEN" \
  | sed -n '1,160p'
```

## Pagination

List endpoints return:

```json
{
  "next": null,
  "previous": null,
  "results": []
}
```

If `next` is non-null and the user needs more records, fetch it directly with the same authorization header:

```bash
PROJECT_ID=2
NEXT_URL=$(
  curl -fsS "$BUGSINK_API/issues/?project=$PROJECT_ID&sort=last_seen&order=desc" \
    -H "Authorization: Bearer $BUGSINK_TOKEN" \
    | jq -r '.next // empty'
)

if test -n "$NEXT_URL"; then
  curl -fsS "$NEXT_URL" \
    -H "Authorization: Bearer $BUGSINK_TOKEN" \
    | jq '{next, previous, count: (.results | length)}'
else
  echo "No next page"
fi
```
