# 01 — Catalog Architecture

The catalog is the data layer that everything in the trigger system reads from. It's embedded in the binary at build time, has zero runtime external dependencies, and is the single source of truth for what each provider can do and what its webhook events look like.

## Layout

```
internal/mcp/catalog/
├── catalog.go              # types + accessor methods + singleton
└── providers/
    ├── github.actions.json      # 278 actions, 161 schemas, 10 resources
    ├── github.triggers.json     # 24 triggers, 15 schemas
    ├── github-app.actions.json  # same as github.actions.json (variant)
    ├── github-app-oauth.actions.json
    ├── github-pat.actions.json
    ├── intercom.actions.json
    ├── intercom.triggers.json
    ├── slack.actions.json
    ├── slack.triggers.json
    └── ...
```

Each provider has one `*.actions.json` file for its HTTP API surface and, if it supports webhooks, one `*.triggers.json` file for its event surface. Both are embedded via `//go:embed` in `catalog.go`, parsed on first access, and served via lock-free map lookups after that.

## Three top-level data types

### `ProviderActions` — one per `*.actions.json`

```go
type ProviderActions struct {
    DisplayName string
    Resources   map[string]ResourceDef       // keyed by resource type (e.g. "issue", "pull_request")
    Actions     map[string]ActionDef         // keyed by action key (e.g. "issues_get")
    Schemas     map[string]SchemaDefinition  // flattened response schemas
}
```

### `ProviderTriggers` — one per `*.triggers.json`

```go
type ProviderTriggers struct {
    DisplayName string
    Triggers    map[string]TriggerDef        // keyed by trigger key (e.g. "issues.opened")
    Schemas     map[string]SchemaDefinition  // flattened webhook payload schemas
}
```

Actions and triggers live in separate files because they're maintained differently. Actions come from the provider's OpenAPI spec and get regenerated via `cmd/fetchactions-oas3`. Triggers are hand-maintained because few providers publish a machine-readable webhook schema.

### `Catalog` — the singleton

```go
type Catalog struct {
    providers map[string]*ProviderActions
    triggers  map[string]*ProviderTriggers
}

var globalCatalog *Catalog
func Global() *Catalog { ... }
```

Call `catalog.Global()` to get the shared instance. Safe for concurrent reads. No mutation at runtime.

## Resources: the continuation primitive

`ResourceDef` is the most important struct in the catalog for the dispatcher. It defines what a "resource" is (an issue, a pull request, a conversation), what actions operate on it, and how to identify a specific instance across events.

```go
type ResourceDef struct {
    DisplayName   string
    Description   string
    IDField       string            // the resource's own ID field (e.g. "number" for issues)
    NameField     string            // display name field
    Icon          string
    ListAction    string
    RequestConfig *RequestConfig
    RefBindings   map[string]string // action_param → $refs.x template
    ResourceKeyTemplate string      // $refs.x template for stable identity
}
```

### `RefBindings` — auto-fill context action params

Resource bindings tell the dispatcher how to fill in a context action's parameters from the ref map when the user writes `ref: issue` instead of specifying every param explicitly.

Example from `internal/mcp/catalog/providers/github.actions.json`:

```json
"issue": {
  "ref_bindings": {
    "issue_number": "$refs.issue_number",
    "owner":        "$refs.owner",
    "repo":         "$refs.repo"
  }
}
```

When a user writes:

```yaml
context:
  - as: issue
    action: issues_get
    ref: issue   # ← this
```

The dispatcher looks up the `issue` resource's `ref_bindings`, substitutes each `$refs.x`, and feeds the result into the action's path template `/repos/{owner}/{repo}/issues/{issue_number}`.

### `ResourceKeyTemplate` — stable cross-event identity

A ref-template that produces a string uniquely identifying a specific resource instance within a connection. Events on the same resource produce the same key; events on different resources produce different keys.

```json
"issue":         { "resource_key_template": "$refs.owner/$refs.repo#issue-$refs.issue_number" }
"pull_request":  { "resource_key_template": "$refs.owner/$refs.repo#pr-$refs.pull_number" }
"release":       { "resource_key_template": "$refs.owner/$refs.repo#release-$refs.release_id" }
"workflow":      { "resource_key_template": "$refs.owner/$refs.repo#run-$refs.run_id" }
```

Resources with no continuation semantics (`repository`, `branch`, `label`, `milestone`, `organization`, `team`) have empty templates — events on them always produce new conversations.

See [03-lifecycle-and-continuation.md](03-lifecycle-and-continuation.md) for how the dispatcher uses this and why it's provider-agnostic.

## Actions

An action is one HTTP operation against the provider's API, normalized to a neutral shape the executor can proxy via Nango.

```go
type ActionDef struct {
    DisplayName    string
    Description    string
    Access         string            // "read" or "write"
    ResourceType   string            // which resource this operates on
    Parameters     json.RawMessage   // JSON Schema for parameters
    Execution      *ExecutionConfig  // how to execute via Nango
    ResponseSchema string            // ref into Schemas
}

type ExecutionConfig struct {
    Method       string            // GET, POST, PUT, PATCH, DELETE
    Path         string            // /repos/{owner}/{repo}/issues/{issue_number}
    BodyMapping  map[string]string // param name → body field
    QueryMapping map[string]string // param name → query field
    Headers      map[string]string // extra headers
    ResponsePath string            // dot-path into response for list-style responses
}
```

### Access inference

The `Access` field is critical for trigger safety — context actions must be read-only to prevent agents from unintentionally mutating state during context gathering. The inference lives in `cmd/fetchactions-oas3/parse.go` and is based on the action key's verb token, not substring matching. Substring matching used to be a bug (`strings.Contains("submit_review", "view")` → false-positive read), fixed in this PR.

```go
func inferAccess(method, actionKey, path string) string {
    switch method {
    case "GET":
        return "read"
    case "POST":
        // Tokenize on "_" and check the first two tokens against the verb set.
        // Pattern for GitHub op IDs: noun_verb_rest (issues_list, pulls_get).
        // Pattern for search ops: verb_noun (search_repos).
        tokens := strings.Split(strings.ToLower(actionKey), "_")
        for i := 0; i < len(tokens) && i < 2; i++ {
            if readHintVerbs[tokens[i]] {
                return "read"
            }
        }
        // Path suffix fallback for search-style endpoints.
        for _, suffix := range readHintPathSuffixes {
            if strings.HasSuffix(strings.ToLower(path), suffix) {
                return "read"
            }
        }
        return "write"
    default:
        return "write"
    }
}
```

Read verbs: `list, get, search, find, query, read, fetch, check, show, describe, lookup, retrieve, export, download, view`.

### Body schemas with `oneOf`

GitHub wraps a few endpoint bodies in `oneOf` with two alternatives: an object form and a raw array form. Examples include `issues_add_labels`, `issues_set_labels`, `repos_add_status_check_contexts`. Before this PR, the generator only walked top-level `properties` and missed the `labels`/`contexts` arrays entirely — those actions were unusable. The parser now walks the top-level schema plus every object alternative in `oneOf`/`anyOf`, merging their properties and required lists. See `cmd/fetchactions-oas3/parse.go`.

### Variant providers

The actions catalog has separate files for provider variants (`github-app.actions.json`, `github-app-oauth.actions.json`, `github-pat.actions.json`) because the three GitHub auth modes expose different subsets of actions in practice. The generator writes identical content to all four files during regeneration — it's the `NangoProviders` list in the service config that tells the writer which filenames to produce.

## Triggers

A trigger is one webhook event the provider fires, plus metadata for extracting refs from the payload.

```go
type TriggerDef struct {
    DisplayName   string
    Description   string
    ResourceType  string            // which resource this trigger relates to
    PayloadSchema string            // ref into ProviderTriggers.Schemas
    Refs          map[string]string // ref_name → dot-path into the webhook body
}
```

### Trigger keys

Most triggers have the form `<event>.<action>` (e.g. `issues.opened`, `pull_request_review.submitted`). Actionless events use just `<event>` (e.g. `push`, `create`, `delete`). The dispatcher derives the trigger key from the HTTP header + payload in the Nango handler:

```go
func (di DispatchInput) TriggerKey() string {
    if di.EventAction == "" {
        return di.EventType
    }
    return di.EventType + "." + di.EventAction
}
```

### Refs

The most load-bearing field on a trigger. `Refs` is a map from a friendly name (used in YAML templates as `$refs.x`) to a dot-path into the webhook payload. At dispatch time, the dispatcher walks the payload and pulls out every ref defined on the matching trigger.

Example from `internal/mcp/catalog/providers/github.triggers.json`:

```json
"issues.opened": {
  "resource_type": "issue",
  "payload_schema": "issues_event",
  "refs": {
    "owner":        "repository.owner.login",
    "repo":         "repository.name",
    "repository":   "repository.full_name",
    "sender":       "sender.login",
    "issue_number": "issue.number",
    "issue_title":  "issue.title"
  }
}
```

When a real GitHub issue-opened webhook arrives, the dispatcher extracts `owner="Codertocat", repo="Hello-World", issue_number="1"`, and so on. Those values flow into:

- Context action path substitution (via `ref_bindings`)
- Instruction template substitution (via `$refs.x` and `{{$refs.x}}`)
- The resource key template (see above)

All three consumers read from the same resolved ref map.

### Trigger-to-resource relationship

Each trigger has a `resource_type` field naming which resource it belongs to. The dispatcher uses this to look up the `ResourceKeyTemplate` on the resource.

Triggers in the same resource family share identity refs. All `issues.*` triggers expose `issue_number`; all `pull_request.*` triggers expose `pull_number`. This consistency is what lets the resource-level template work — you write the template once per resource, not once per trigger.

### Variant trigger lookup

Triggers live under the base provider name (`github`), not per-variant (`github-app`). When a webhook arrives for `github-app`, the dispatcher calls `GetProviderTriggersForVariant` which progressively strips dash-suffixes until it finds a trigger set:

```go
func (c *Catalog) GetProviderTriggersForVariant(variant string) (*ProviderTriggers, bool) {
    name := variant
    for {
        idx := strings.LastIndex(name, "-")
        if idx <= 0 { return nil, false }
        name = name[:idx]
        if pt, ok := c.triggers[name]; ok { return pt, ok }
    }
}
```

The same fallback applies to `GetResourceDef` for the same reason — resources are defined on the base provider but referenced from per-variant action catalogs.

## Schemas

`SchemaDefinition` is a flattened, top-level-properties-only representation of a JSON Schema. It's a much reduced form of OpenAPI's schema model — just enough for the frontend autocompleter and the executor's response parsing.

```go
type SchemaDefinition struct {
    Type       string
    Properties map[string]SchemaPropertyDef
    Items      *SchemaRef  // for array types
}

type SchemaPropertyDef struct {
    Type        string
    Description string
    Nullable    bool
    SchemaRef   string  // references another schema by name
}

type SchemaRef struct {
    Ref string
}
```

Nested structures use `SchemaRef` to point at another entry in the same schema map. A one-level flattening avoids the circular-reference pit that OpenAPI schemas routinely fall into.

Orphaned schemas — defined but never referenced — are harmless at runtime; the frontend autocompleter is the only current consumer that would care.

## The generator

`cmd/fetchactions-oas3/` is a standalone Go program that pulls OpenAPI 3.x specs from upstream, parses them with `libopenapi`, and writes per-provider JSON files. Run it with:

```bash
go run ./cmd/fetchactions-oas3 -provider github     # one provider
go run ./cmd/fetchactions-oas3                      # all providers
go run ./cmd/fetchactions-oas3 -force               # skip the spec cache
```

Key files:

| File | Purpose |
|---|---|
| `main.go` | CLI entrypoint, per-provider loop |
| `config.go` | Per-provider config (spec URL, resources, path filters) |
| `parse.go` | OpenAPI → `ActionDef` conversion, access inference, oneOf handling |
| `write.go` | JSON output + per-variant duplication |
| `fetch.go` | HTTP spec fetching with on-disk cache |
| `naming.go` | operationId → action key conversion |

### Resource-based filtering

Most providers use prefix-based path filtering (e.g. Stripe only includes paths under `/v1/charges`, `/v1/customers`, etc.) to keep the catalog focused. GitHub uses **resource-based filtering** instead: each resource has a list of path prefixes, and actions are classified into resources by longest-prefix match. Look at `githubResources()` in `config.go` for the full set.

The resource config also carries the `ResourceKeyTemplate` and `RefBindings` that end up in the JSON, so adding a new provider with continuation support is one config entry plus one regeneration.

## Where to go from here

- How the dispatcher turns catalog data into runs: [02-dispatcher-runtime.md](02-dispatcher-runtime.md)
- How resource keys and termination work: [03-lifecycle-and-continuation.md](03-lifecycle-and-continuation.md)
- The catalog audit that drove the recent generator fixes: [07-catalog-validation-report.md](07-catalog-validation-report.md)
