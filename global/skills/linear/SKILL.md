---
name: linear
description: Use when reading or triaging Linear issues, projects, teams, workflow states, users, comments, labels, or planning data through Linear GraphQL. Provides verified curl and jq commands using LINEAR_URL and LINEAR_TOKEN, with strict response filtering to avoid dumping large GraphQL payloads into context.
---

# Linear GraphQL

Use Linear through the Hivy-provided GraphQL endpoint at `$LINEAR_URL`.

`LINEAR_URL` and `LINEAR_TOKEN` are provided by the runtime for the configured Linear connection. Always call the provided `LINEAR_URL` exactly; do not substitute another workspace or token.

## Environment

Required:

| Variable | Purpose |
|---|---|
| `LINEAR_URL` | Linear GraphQL endpoint provided by Hivy |
| `LINEAR_TOKEN` | Bearer token for the provided Linear endpoint |

Initialize once:

```bash
test -n "$LINEAR_URL" || { echo "LINEAR_URL is not set" >&2; exit 1; }
test -n "$LINEAR_TOKEN" || { echo "LINEAR_TOKEN is not set" >&2; exit 1; }
LINEAR_URL="${LINEAR_URL%/}"
```

Use this helper for every GraphQL call:

```bash
linear_graphql() {
  local query="$1"
  local vars_json
  if test "$#" -ge 2; then
    vars_json="$2"
  else
    vars_json="{}"
  fi
  jq -n --arg query "$query" --argjson vars "$vars_json" \
    '{query: $query, variables: $vars}' \
    | curl -fsS "$LINEAR_URL" \
        -H "Authorization: Bearer $LINEAR_TOKEN" \
        -H "Content-Type: application/json" \
        --data-binary @-
}
```

## Rules

- Always filter GraphQL responses with `jq` before reading or posting results.
- Never dump full issue descriptions, comments, histories, labels, attachments, or schema introspection into context.
- Prefer `first: 10` to `first: 25`; increase only when the task requires it.
- Use `identifier` like `ENG-123` for human-facing issue references. The `issue(id:)` query accepts either the UUID or the identifier.
- Use Relay pagination: request `pageInfo { hasNextPage endCursor }`, then pass `after: endCursor` only when more results are needed.
- For writes, first read the relevant object and schema input type, then make the smallest mutation that satisfies the task.
- Do not print `$LINEAR_TOKEN`.
- When reporting results to a teammate, summarize only user-relevant fields and outcomes. Do not mention proxy URLs, bearer-token mechanics, schema probing, GraphQL filtering steps, or internal query details unless troubleshooting the Linear integration itself.

## API schema discovery

If this skill does not cover an operation you need, inspect the schema before guessing. Linear supports GraphQL introspection.

List query fields matching a resource:

```bash
linear_graphql 'query QueryFields {
  __type(name: "Query") {
    fields { name }
  }
}' | jq '[.data.__type.fields[].name | select(test("issue|project|team|user|cycle|label|comment"; "i"))]'
```

Inspect one input or object type:

```bash
TYPE_NAME="IssueUpdateInput"
linear_graphql 'query TypeInfo($name: String!) {
  __type(name: $name) {
    name
    kind
    inputFields { name type { kind name ofType { kind name } } }
    fields { name type { kind name ofType { kind name } } }
  }
}' "$(jq -n --arg name "$TYPE_NAME" '{name: $name}')" \
  | jq '.data.__type
      | if .kind == "INPUT_OBJECT"
        then {name, kind, fields: [.inputFields[] | {name, type: (.type.kind + ":" + (.type.name // .type.ofType.name // ""))}]}
        else {name, kind, fields: [.fields[] | {name, type: (.type.kind + ":" + (.type.name // .type.ofType.name // ""))}]}
        end'
```

## Who am I connected as?

```bash
linear_graphql 'query Viewer {
  viewer { id name displayName email url }
  organization { id name urlKey }
}' | jq '{
  viewer: .data.viewer | {id, name, displayName, email, url},
  organization: .data.organization
}'
```

## List teams and workflow states

Use this before creating or moving issues. Issue creation requires `teamId`; moving an issue to a workflow state requires `stateId`.

```bash
linear_graphql 'query Teams {
  teams(first: 25) {
    nodes {
      id
      key
      name
      states(first: 20) { nodes { id name type } }
    }
  }
}' | jq '[.data.teams.nodes[] | {
  id,
  key,
  name,
  states: [.states.nodes[] | {id, name, type}]
}]'
```

## List recent issues

```bash
linear_graphql 'query RecentIssues {
  issues(first: 10, orderBy: updatedAt) {
    pageInfo { hasNextPage endCursor }
    nodes {
      id
      identifier
      title
      url
      updatedAt
      priority
      estimate
      state { id name type }
      team { id key name }
      assignee { id displayName name email }
      project { id name url }
    }
  }
}' | jq '{
  pageInfo: .data.issues.pageInfo,
  issues: [.data.issues.nodes[] | {
    id,
    identifier,
    title,
    url,
    updatedAt,
    priority,
    estimate,
    state: .state.name,
    state_type: .state.type,
    team: .team.key,
    assignee: .assignee.displayName,
    project: .project.name
  }]
}'
```

## Get one issue by identifier or UUID

```bash
ISSUE_ID="ENG-17"
linear_graphql 'query IssueById($id: String!) {
  issue(id: $id) {
    id
    identifier
    title
    url
    description
    createdAt
    updatedAt
    priority
    estimate
    state { id name type }
    team { id key name }
    assignee { id displayName name email }
    creator { id displayName name email }
    project { id name url }
    labels(first: 20) { nodes { id name color } }
    comments(first: 10) {
      nodes {
        id
        body
        createdAt
        url
        user { id displayName name email }
      }
    }
  }
}' "$(jq -n --arg id "$ISSUE_ID" '{id: $id}')" \
  | jq '{
    issue: .data.issue | {
      id,
      identifier,
      title,
      url,
      createdAt,
      updatedAt,
      priority,
      estimate,
      state: .state.name,
      state_type: .state.type,
      team: .team.key,
      assignee: .assignee.displayName,
      creator: .creator.displayName,
      project: .project.name,
      description: ((.description // "") | .[0:1200]),
      labels: [.labels.nodes[] | {id, name}],
      comments: [.comments.nodes[] | {
        id,
        user: .user.displayName,
        createdAt,
        url,
        body: (.body | .[0:800])
      }]
    }
  }'
```

## Search issues

Use `searchableContent` for broad issue text search. Use `title.containsIgnoreCase` when the user asks for title-only matching.

```bash
QUERY="memory"
linear_graphql 'query SearchIssues($q: String!) {
  issues(
    first: 10
    orderBy: updatedAt
    filter: { searchableContent: { contains: $q } }
  ) {
    nodes {
      id
      identifier
      title
      url
      updatedAt
      state { name type }
      team { key name }
      assignee { displayName email }
    }
  }
}' "$(jq -n --arg q "$QUERY" '{q: $q}')" \
  | jq '[.data.issues.nodes[] | {
    id,
    identifier,
    title,
    url,
    updatedAt,
    state: .state.name,
    state_type: .state.type,
    team: .team.key,
    assignee: .assignee.displayName
  }]'
```

Title-only search:

```bash
QUERY="memory"
linear_graphql 'query SearchIssueTitles($q: String!) {
  issues(
    first: 10
    orderBy: updatedAt
    filter: { title: { containsIgnoreCase: $q } }
  ) {
    nodes { id identifier title url state { name } team { key } }
  }
}' "$(jq -n --arg q "$QUERY" '{q: $q}')" \
  | jq '[.data.issues.nodes[] | {id, identifier, title, url, state: .state.name, team: .team.key}]'
```

## List projects

```bash
linear_graphql 'query Projects {
  projects(first: 20, orderBy: updatedAt) {
    pageInfo { hasNextPage endCursor }
    nodes {
      id
      name
      url
      state
      priority
      updatedAt
      lead { id displayName name email }
      teams(first: 10) { nodes { id key name } }
    }
  }
}' | jq '{
  pageInfo: .data.projects.pageInfo,
  projects: [.data.projects.nodes[] | {
    id,
    name,
    url,
    state,
    priority,
    updatedAt,
    lead: .lead.displayName,
    teams: [.teams.nodes[] | {id, key, name}]
  }]
}'
```

## Find users

```bash
QUERY="alex"
linear_graphql 'query Users($q: String!) {
  users(
    first: 10
    filter: {
      or: [
        { name: { containsIgnoreCase: $q } }
        { displayName: { containsIgnoreCase: $q } }
        { email: { containsIgnoreCase: $q } }
      ]
    }
  ) {
    nodes { id name displayName email active url }
  }
}' "$(jq -n --arg q "$QUERY" '{q: $q}')" \
  | jq '[.data.users.nodes[] | {id, name, displayName, email, active, url}]'
```

## Pagination

Fetch the next page only when `hasNextPage` is true:

```bash
AFTER="cursor-from-previous-page"
linear_graphql 'query IssuesPage($after: String!) {
  issues(first: 10, after: $after, orderBy: updatedAt) {
    pageInfo { hasNextPage endCursor }
    nodes { id identifier title url updatedAt state { name } team { key } }
  }
}' "$(jq -n --arg after "$AFTER" '{after: $after}')" \
  | jq '{
    pageInfo: .data.issues.pageInfo,
    issues: [.data.issues.nodes[] | {id, identifier, title, url, updatedAt, state: .state.name, team: .team.key}]
  }'
```

## Write operations

Before writing, verify the exact input type in the schema. Common mutation input types:

- `IssueCreateInput`: `title`, `teamId`, optional `description`, `assigneeId`, `projectId`, `stateId`, `priority`, `estimate`, `labelIds`.
- `IssueUpdateInput`: optional `title`, `description`, `assigneeId`, `projectId`, `stateId`, `priority`, `estimate`, `addedLabelIds`, `removedLabelIds`.
- `CommentCreateInput`: `body` plus one target such as `issueId` or `projectId`.

After a write, return only the changed object fields needed to confirm the result.

Linear issue descriptions and comments accept markdown. Use markdown when it improves clarity:

- Headings for sections: `## Context`, `## Acceptance criteria`.
- Bullets or numbered lists for steps.
- Fenced code blocks for commands, stack traces, or compact logs.
- Checklists for tracked follow-up work.
- Links with `[label](url)`.

Keep markdown compact. Do not paste large logs or full payloads into descriptions or comments.

### Create an issue

First resolve the team, optional workflow state, optional assignee, and optional labels. Issue creation requires `teamId`.

```bash
TEAM_KEY="ENG"
linear_graphql 'query TeamForIssueCreate($key: String!) {
  teams(first: 1, filter: { key: { eq: $key } }) {
    nodes {
      id
      key
      name
      states(first: 20) { nodes { id name type } }
      labels(first: 20) { nodes { id name color } }
      members(first: 20) { nodes { id displayName name email active } }
    }
  }
}' "$(jq -n --arg key "$TEAM_KEY" '{key: $key}')" \
  | jq '.data.teams.nodes[0] | {
    id,
    key,
    name,
    states: [.states.nodes[] | {id, name, type}],
    labels: [.labels.nodes[] | {id, name}],
    members: [.members.nodes[] | {id, displayName, email, active}]
  }'
```

Then create the issue:

```bash
TEAM_ID="1424ce65-a4a9-459b-accb-bcd1ffc695b6"
TITLE="Investigate webhook retry failures"
DESCRIPTION=$(cat <<'MD'
## Context
Webhook delivery retries are failing after the trigger router refactor.

## Evidence
- Bugsink shows repeated `502` responses from the delivery worker.
- Retry attempts exhaust before the employee sandbox accepts the event.

## Acceptance criteria
- [ ] Identify the failing request path
- [ ] Add a regression test
- [ ] Confirm delivery succeeds in staging
MD
)

linear_graphql 'mutation CreateIssue($input: IssueCreateInput!) {
  issueCreate(input: $input) {
    success
    issue {
      id
      identifier
      title
      url
      state { name type }
      team { key name }
      assignee { displayName email }
      labels(first: 10) { nodes { id name } }
    }
  }
}' "$(jq -n \
      --arg teamId "$TEAM_ID" \
      --arg title "$TITLE" \
      --arg description "$DESCRIPTION" \
      '{input: {teamId: $teamId, title: $title, description: $description}}')" \
  | jq '{
    success: .data.issueCreate.success,
    issue: .data.issueCreate.issue | {
      id,
      identifier,
      title,
      url,
      state: .state.name,
      team: .team.key,
      assignee: .assignee.displayName,
      labels: [.labels.nodes[] | {id, name}]
    }
  }'
```

Create an issue with a markdown code block:

```bash
TEAM_ID="1424ce65-a4a9-459b-accb-bcd1ffc695b6"
TITLE="Fix failing deploy check"
DESCRIPTION=$(cat <<'MD'
## Failing check

The deploy job fails while building the employee sandbox image.

```bash
docker build -f sandboxes/employee/Dockerfile.runtime .
```

## Expected result

The image should build and publish without manual retry.
MD
)

linear_graphql 'mutation CreateIssueWithMarkdown($input: IssueCreateInput!) {
  issueCreate(input: $input) {
    success
    issue {
      id
      identifier
      title
      url
      description
      team { key name }
    }
  }
}' "$(jq -n \
      --arg teamId "$TEAM_ID" \
      --arg title "$TITLE" \
      --arg description "$DESCRIPTION" \
      '{input: {teamId: $teamId, title: $title, description: $description}}')" \
  | jq '{
    success: .data.issueCreate.success,
    issue: .data.issueCreate.issue | {
      id,
      identifier,
      title,
      url,
      team: .team.key,
      description: ((.description // "") | .[0:1200])
    }
  }'
```

Create with assignee, labels, priority, and state:

```bash
TEAM_ID="1424ce65-a4a9-459b-accb-bcd1ffc695b6"
STATE_ID="fb76cb92-1f96-4000-88e1-17c31a73e5a4"
ASSIGNEE_ID="32da2bfa-7140-4287-b8a4-5cb1508c5e7f"
LABEL_ID="3ff660d1-3e04-4d24-8f1f-50e3b0719648"

linear_graphql 'mutation CreateIssue($input: IssueCreateInput!) {
  issueCreate(input: $input) {
    success
    issue {
      id
      identifier
      title
      url
      priority
      state { name type }
      team { key }
      assignee { displayName email }
      labels(first: 10) { nodes { id name } }
    }
  }
}' "$(jq -n \
      --arg teamId "$TEAM_ID" \
      --arg stateId "$STATE_ID" \
      --arg assigneeId "$ASSIGNEE_ID" \
      --arg labelId "$LABEL_ID" \
      '{input: {
        teamId: $teamId,
        title: "Follow up on production error reports",
        description: "Triage recent failures and summarize the root cause.",
        stateId: $stateId,
        assigneeId: $assigneeId,
        labelIds: [$labelId],
        priority: 2
      }}')" \
  | jq '{
    success: .data.issueCreate.success,
    issue: .data.issueCreate.issue | {
      id,
      identifier,
      title,
      url,
      priority,
      state: .state.name,
      team: .team.key,
      assignee: .assignee.displayName,
      labels: [.labels.nodes[] | {id, name}]
    }
  }'
```

### Update an issue

Use `issueUpdate(id:, input:)`. The `id` can be a UUID or identifier like `ENG-17`.

```bash
ISSUE_ID="ENG-17"
NEW_TITLE="Cloud agents: implementation plan"

linear_graphql 'mutation UpdateIssue($id: String!, $input: IssueUpdateInput!) {
  issueUpdate(id: $id, input: $input) {
    success
    issue {
      id
      identifier
      title
      url
      updatedAt
      state { name type }
      assignee { displayName email }
    }
  }
}' "$(jq -n \
      --arg id "$ISSUE_ID" \
      --arg title "$NEW_TITLE" \
      '{id: $id, input: {title: $title}}')" \
  | jq '{
    success: .data.issueUpdate.success,
    issue: .data.issueUpdate.issue | {
      id,
      identifier,
      title,
      url,
      updatedAt,
      state: .state.name,
      assignee: .assignee.displayName
    }
  }'
```

Move an issue to another workflow state:

```bash
ISSUE_ID="ENG-17"
STATE_ID="7ebe454a-2cd9-42ba-bc42-48c712b57956"

linear_graphql 'mutation MoveIssue($id: String!, $input: IssueUpdateInput!) {
  issueUpdate(id: $id, input: $input) {
    success
    issue { id identifier title url state { id name type } }
  }
}' "$(jq -n --arg id "$ISSUE_ID" --arg stateId "$STATE_ID" \
      '{id: $id, input: {stateId: $stateId}}')" \
  | jq '{
    success: .data.issueUpdate.success,
    issue: .data.issueUpdate.issue | {
      id,
      identifier,
      title,
      url,
      state: .state.name,
      state_type: .state.type
    }
  }'
```

### Add a comment to an issue

```bash
ISSUE_ID="ENG-17"
BODY=$(cat <<'MD'
### Update

I checked the latest logs and found the retry path is failing before the sandbox receives the event.

Next steps:

1. Patch the proxy path
2. Run the focused regression test
3. Confirm the staging delivery path is green

```bash
go test ./internal/handler -run 'BugsinkProxy' -count=1
```
MD
)

linear_graphql 'mutation AddIssueComment($input: CommentCreateInput!) {
  commentCreate(input: $input) {
    success
    comment {
      id
      body
      url
      createdAt
      user { displayName email }
      issue { identifier title url }
    }
  }
}' "$(jq -n --arg issueId "$ISSUE_ID" --arg body "$BODY" \
      '{input: {issueId: $issueId, body: $body}}')" \
  | jq '{
    success: .data.commentCreate.success,
    comment: .data.commentCreate.comment | {
      id,
      url,
      createdAt,
      user: .user.displayName,
      issue: .issue.identifier,
      body: (.body | .[0:500])
    }
  }'
```

### Assign or unassign an issue

Find the user first:

```bash
QUERY="alex"
linear_graphql 'query Users($q: String!) {
  users(
    first: 10
    filter: {
      or: [
        { name: { containsIgnoreCase: $q } }
        { displayName: { containsIgnoreCase: $q } }
        { email: { containsIgnoreCase: $q } }
      ]
    }
  ) {
    nodes { id name displayName email active url }
  }
}' "$(jq -n --arg q "$QUERY" '{q: $q}')" \
  | jq '[.data.users.nodes[] | {id, name, displayName, email, active, url}]'
```

Assign:

```bash
ISSUE_ID="ENG-17"
ASSIGNEE_ID="32da2bfa-7140-4287-b8a4-5cb1508c5e7f"

linear_graphql 'mutation AssignIssue($id: String!, $input: IssueUpdateInput!) {
  issueUpdate(id: $id, input: $input) {
    success
    issue { id identifier title url assignee { id displayName email } }
  }
}' "$(jq -n --arg id "$ISSUE_ID" --arg assigneeId "$ASSIGNEE_ID" \
      '{id: $id, input: {assigneeId: $assigneeId}}')" \
  | jq '{
    success: .data.issueUpdate.success,
    issue: .data.issueUpdate.issue | {
      id,
      identifier,
      title,
      url,
      assignee: .assignee | {id, displayName, email}
    }
  }'
```

Unassign by setting `assigneeId` to null:

```bash
ISSUE_ID="ENG-17"
linear_graphql 'mutation UnassignIssue($id: String!, $input: IssueUpdateInput!) {
  issueUpdate(id: $id, input: $input) {
    success
    issue { id identifier title url assignee { id displayName email } }
  }
}' "$(jq -n --arg id "$ISSUE_ID" '{id: $id, input: {assigneeId: null}}')" \
  | jq '{
    success: .data.issueUpdate.success,
    issue: .data.issueUpdate.issue | {
      id,
      identifier,
      title,
      url,
      assignee: .assignee
    }
  }'
```

### Add or remove labels

Find labels by name:

```bash
QUERY="bug"
linear_graphql 'query Labels($q: String!) {
  issueLabels(first: 10, filter: { name: { containsIgnoreCase: $q } }) {
    nodes { id name color team { id key name } }
  }
}' "$(jq -n --arg q "$QUERY" '{q: $q}')" \
  | jq '[.data.issueLabels.nodes[] | {id, name, color, team: .team.key}]'
```

Add a single label:

```bash
ISSUE_ID="ENG-17"
LABEL_ID="3ff660d1-3e04-4d24-8f1f-50e3b0719648"

linear_graphql 'mutation AddIssueLabel($id: String!, $labelId: String!) {
  issueAddLabel(id: $id, labelId: $labelId) {
    success
    issue {
      id
      identifier
      title
      url
      labels(first: 20) { nodes { id name color } }
    }
  }
}' "$(jq -n --arg id "$ISSUE_ID" --arg labelId "$LABEL_ID" \
      '{id: $id, labelId: $labelId}')" \
  | jq '{
    success: .data.issueAddLabel.success,
    issue: .data.issueAddLabel.issue | {
      id,
      identifier,
      title,
      url,
      labels: [.labels.nodes[] | {id, name}]
    }
  }'
```

Remove a single label:

```bash
ISSUE_ID="ENG-17"
LABEL_ID="3ff660d1-3e04-4d24-8f1f-50e3b0719648"

linear_graphql 'mutation RemoveIssueLabel($id: String!, $labelId: String!) {
  issueRemoveLabel(id: $id, labelId: $labelId) {
    success
    issue {
      id
      identifier
      title
      url
      labels(first: 20) { nodes { id name color } }
    }
  }
}' "$(jq -n --arg id "$ISSUE_ID" --arg labelId "$LABEL_ID" \
      '{id: $id, labelId: $labelId}')" \
  | jq '{
    success: .data.issueRemoveLabel.success,
    issue: .data.issueRemoveLabel.issue | {
      id,
      identifier,
      title,
      url,
      labels: [.labels.nodes[] | {id, name}]
    }
  }'
```

Replace the issue label set in one update:

```bash
ISSUE_ID="ENG-17"
LABEL_ID_1="3ff660d1-3e04-4d24-8f1f-50e3b0719648"
LABEL_ID_2="bfc4d0ae-d9d7-47ce-83ed-ebd53512aba3"

linear_graphql 'mutation ReplaceIssueLabels($id: String!, $input: IssueUpdateInput!) {
  issueUpdate(id: $id, input: $input) {
    success
    issue {
      id
      identifier
      title
      url
      labels(first: 20) { nodes { id name color } }
    }
  }
}' "$(jq -n \
      --arg id "$ISSUE_ID" \
      --arg label1 "$LABEL_ID_1" \
      --arg label2 "$LABEL_ID_2" \
      '{id: $id, input: {labelIds: [$label1, $label2]}}')" \
  | jq '{
    success: .data.issueUpdate.success,
    issue: .data.issueUpdate.issue | {
      id,
      identifier,
      title,
      url,
      labels: [.labels.nodes[] | {id, name}]
    }
  }'
```
