# System Tasks Endpoint — `POST /v1/system/tasks/{taskName}`

**Goal:** give the frontend a single user-authenticated HTTP endpoint that runs a short, no-tool LLM completion using the platform's own credentials and bills the user's org. Must support streaming and non-streaming. Each task is a self-contained file declaring its model, prompt, input schema, and limits — the handler dispatches by name and validates against the task's own definition.

## Issues to address

1. The frontend has legitimate tasks that need an LLM but don't need agents/sandboxes/tools — naming a conversation, summarising a snippet, tagging an item, drafting a reply suggestion. Today there is no public path:
   - The agent path (`/v1/agents/{id}/conversations`) spins up a Daytona sandbox + bridge — minutes of latency, no fit.
   - The BYOK proxy (`/v1/proxy/*`) requires a `ptok_` minted from a *user-supplied* credential. The frontend can't use it for system tasks.
   - `internal/tasks/conversation_name.go` already does this kind of thing, but only server-to-server triggered from a webhook — not exposed to the frontend, and only for that one task.
2. We have all the substrate (system credentials, Picker, registry, CaptureTransport, credit middleware) but no caller binding them together for ad-hoc utility prompts.
3. Frontend currently can't distinguish "no system credential configured" from generic 5xx — needs a typed explicit error.
4. Adding a new task should not require touching a handler, a router, a registry init function, or anything outside one new file. It should be: drop a file, redeploy.

## Important notes

- **System credentials** live in `credentials WHERE is_system = true`. `credentials.PostgresPicker.Pick(group)` already returns one (or `ErrNoSystemCredential`). Endpoint must surface that as a typed `errorResponse` — never a 500.
- **Cheapest-model selection** is in `internal/registry/verify.go::cheapestModel(*Provider)`. The endpoint reuses it for tasks that ask for a tier rather than a specific model.
- **Captured spend** flows through `middleware.Generation` (writes a `generations` row with parsed usage/cost) and `middleware.RequireCredits` (gates on the org's credit balance). Re-using these means billing is automatic — no new accounting code.
- **Proxy reuse vs. dedicated handler.** `/v1/proxy/*` uses `httputil.ReverseProxy`, built around the caller picking the URL/path/body. For system tasks the *backend* picks the model and shapes the request body — `httputil.ReverseProxy` is the wrong shape. We instead build a small `system.Forwarder` that does a single `http.Client.Do` through the same `proxy.CaptureTransport`, so usage capture stays identical. No middleware changes.
- **Streaming.** Same `FlushInterval: -1`/SSE-passthrough trick the proxy uses today: detect `text/event-stream`, forward chunk-by-chunk via `http.ResponseWriter.Flush()`. Non-streaming reads the body and returns once.
- **Task definitions are Go files, one per task.** Each task is `internal/system/tasks/<task_name>.go` and registers itself via `init()`. The handler does a registry lookup, runs the task's argument validation, then calls the task's `Build` to produce the LLM request. Adding a task = creating one file. The registry is deliberately not user-extensible at runtime — it's a code-owned surface.
- **Authentication.** Standard JWT via `middleware.RequireAuth`. No `ptok_`. Org comes from the token. No `org-id` header parsing on this route.
- **"Active credentials" semantics.** The picker already filters `is_system = true AND revoked_at IS NULL`. "Active" means: at least one non-revoked system credential whose `provider_id` maps to the task's `ProviderGroup`. If none, return **503 + `error_code = "system_credential_unavailable"`**. *No silent fallback to BYOK* — that would surprise the caller about who's paying.

## Surface

```
POST /v1/system/tasks/{taskName}
Body: {
  "stream": bool,            // optional; defaults to the task's DefaultStream
  "args":   { ... }          // task-specific; validated against the task's ArgSpec
}
Streaming:     Content-Type: text/event-stream  (raw provider SSE, passthrough)
Non-streaming: Content-Type: application/json   { "text": "...", "usage": {...} }
```

## Task definition shape

Each task is one file under `internal/system/tasks/`. Layout:

```
internal/system/tasks/
  conversation_name.go
  summarise.go
  ...
```

Every file follows the same structure (Go, not pseudocode — sketched only):

- `package tasks`
- An exported `var Definition = system.Task{ ... }` (or equivalent) declaring everything the handler needs.
- An `init()` that calls `system.Register(Definition)`.

The `system.Task` struct must capture, at minimum:
- `Name string` — URL slug, e.g. `"conversation_name"`. Must match the file's slug.
- `Version string` — bumped (e.g., `"v1"` → `"v2"`) whenever the prompt, schema, or model preference changes. Used as part of the cache key so a prompt change invalidates cached results without a global flush.
- `Description string` — human-readable, used in admin/logs.
- `ProviderGroup string` — `"openai"` / `"anthropic"` / `"groq"` / etc.; passed straight to `Picker.Pick`.
- `ModelTier ModelTier` — one of `Cheapest`, `Default`, or a named-model field. `Cheapest` invokes `cheapestModel(provider)` from the registry; named pins a specific model id; `Default` uses the registry's default per-provider model.
- `SystemPrompt string` — static, no template.
- `UserPromptTemplate string` — `text/template`-compatible (no Sprig). Variables come from `args`.
- `Args []ArgSpec` — declarative input schema. Each arg has `Name`, `Type` (`string` | `int` | `bool` | `string_list`), `Required`, optional `MaxLen`/`Min`/`Max`. The handler validates the request body against this list before any upstream call. No JSON Schema — keep validation Go-typed and small.
- `MaxOutputTokens int` — hard cap for safety; passed to the upstream `max_tokens`.
- `Temperature *float32` — optional.
- `DefaultStream bool` — what the handler uses when the request body omits `stream`.
- `CacheTTL time.Duration` — cache lifetime for the (task, model, args) tuple. Zero disables caching for the task. Unset defaults to `24h`.
- `Build(args map[string]any) (LLMRequest, error)` — optional escape hatch when prompt rendering needs custom logic (e.g., truncating an input field). Default implementation: render `UserPromptTemplate` with `args` and pack into a chat-completions-shaped struct. Most tasks won't override.

The handler never inspects the prompt or args by hand; everything goes through the task's declared schema.

### Registry

`internal/system/registry.go`:
- `Register(t Task)` — called from each task file's `init()`. Panics if a task with the same `Name` already exists (pure programmer error).
- `Lookup(name string) (Task, bool)` — handler's only entry point.
- `All() []Task` — for admin/diagnostic listing only.

The registry is in-memory, populated at process start by Go's `init` ordering. No DB, no flag, no runtime mutation.

### Example: `conversation_name.go`

Just the shape — actual code lives in implementation:

```go
// (sketch — see implementation)
var Definition = system.Task{
    Name:               "conversation_name",
    Description:        "Generate a 3–6 word conversation title from the first message.",
    ProviderGroup:      "openai",
    ModelTier:          system.Cheapest,
    SystemPrompt:       "...",
    UserPromptTemplate: "First message:\n\n{{.first_message}}\n\nReturn just the title.",
    Args: []system.ArgSpec{
        {Name: "first_message", Type: system.ArgString, Required: true, MaxLen: 4000},
    },
    MaxOutputTokens: 32,
    DefaultStream:   false,
}

func init() { system.Register(Definition) }
```

That single file is the entire surface for that task. Adding a `summarise` task = one new file with its own `Definition`. No router, no registry, no handler edits.

## Implementation strategy

### Wiring
- Mount in `cmd/server/serve_routes.go` under `/v1/system/tasks/{taskName}` with the same v1 middleware stack agents use:
  `RequireAuth → RequireCredits → RemainingCheck → Audit("system.task") → Generation → handler`.
- `internal/system/types.go` — `Task`, `ArgSpec`, `ArgType`, `ModelTier` constants, `LLMRequest` struct.
- `internal/system/registry.go` — global `Register`/`Lookup`/`All`.
- `internal/system/forwarder.go` — single-shot HTTP forwarder through `proxy.CaptureTransport`. Stream + non-stream branches.
- `internal/system/render.go` — default prompt renderer (text/template + arg substitution).
- `internal/system/tasks/<task>.go` — one file per task. Imports `internal/system`; calls `system.Register` from `init`.
- `internal/handler/system_tasks.go` — the new HTTP handler.
- Blank import `_ "github.com/usehiveloop/hiveloop/internal/system/tasks"` from `cmd/server` so the `init()` functions in each task file run at process start.

### Handler flow
1. Resolve task by `{taskName}` from the registry → 404 + `task_not_found` if unknown.
2. Decode request body. Default `stream` to the task's `DefaultStream` when omitted.
3. Validate `args` against `task.Args` (required, type, MaxLen, etc.) → 400 + `invalid_args` on the first violation.
4. Resolve credential: `picker.Pick(ctx, task.ProviderGroup)`.
   - On `ErrNoSystemCredential` → **503 + `system_credential_unavailable`**.
5. Resolve model from `task.ModelTier` against the registry's curated provider catalog. Missing → **503 + `system_model_unavailable`**.
6. **Cache check**: compute key = SHA-256 of canonical `(task.Name, task.Version, model_id, args_canonical_json)`. If `task.CacheTTL > 0` and the key exists, replay from cache (per the SSE/JSON branches above), record an `audit_entries` row with `cached:true`, return.
7. Build the upstream request via `task.Build(args)` (or the default renderer): system prompt, rendered user prompt, `max_tokens`, `temperature`, `stream` (Hiveloop's `stream` flag is also forwarded as OpenAI's `stream` so the upstream actually streams when we do).
8. `forwarder.Forward(ctx, credential, providerURL, requestBody, stream, w)`:
   - `http.Request` with `Authorization` injected by `proxy.AttachAuth`.
   - Wrapped in `proxy.CaptureTransport` so the existing usage parser writes spend.
   - **Streaming branch**: read upstream OpenAI SSE; for each chunk, extract `choices[0].delta.content` and emit `data: {"delta":"..."}` to the client; on `[DONE]`, emit `data: {"done":true,"usage":{...}}`. Tee the deltas into a buffer for cache write on clean completion.
   - **Non-streaming branch**: read body fully, extract `choices[0].message.content` + `usage`, write `{text, usage}` JSON to the client and to the cache.
9. `Generation` middleware writes the row with `task = "system:" + taskName`, `model`, `tokens`, `cost`. `RequireCredits`/`RemainingCheck` debit happens via the existing post-response pattern. Cache hits skip steps 7–9 entirely.

### Errors (typed via the project's `errorResponse` shape)
| Cause | HTTP | `error_code` | Message |
|---|---|---|---|
| Unknown taskName | 404 | `task_not_found` | "system task '{name}' is not defined" |
| Body decode | 400 | `invalid_request_body` | |
| Missing/typo arg, type mismatch, exceeded MaxLen | 400 | `invalid_args` | "arg '{x}': {reason}" |
| Picker returns `ErrNoSystemCredential` | 503 | `system_credential_unavailable` | "no platform credential available for provider group '{group}'" |
| No supported model on the provider | 503 | `system_model_unavailable` | "no eligible model for task '{name}'" |
| Upstream LLM 4xx | 502 | `upstream_error` | passthrough message |
| Upstream LLM 5xx / network | 502 | `upstream_error` | "provider unreachable" |

### Spend / billing
- `Generation` middleware pulls from `CapturedDataFromContext` — same context plumbing `/v1/proxy/*` uses today. No changes there.
- The generation row is tagged `task = "system:<taskName>"` so admin reporting can split utility spend from BYOK proxy spend.

## Tests

Smallest set that pins real invariants. Per the existing `internal/rag/doc/TESTING.md` philosophy + the project's "business-behavior only" rule:

- **Handler-level tests** against a fake upstream HTTP server (real Postgres, real registry, real middleware chain):
  - `TestSystemTask_StreamsHiveloopShape` — `stream:true`; fake upstream emits OpenAI SSE chunks; assert response is `text/event-stream` with Hiveloop-shaped `data: {"delta":"..."}` chunks (not raw OpenAI `choices[0].delta.content`), terminated by `data: {"done":true,"usage":{...}}`.
  - `TestSystemTask_BuffersWhenNotStreaming` — `stream:false`; single JSON response, body shape `{text, usage}`.
  - `TestSystemTask_NoSystemCredential_Returns503` — empty `credentials` table; assert 503 + `error_code = "system_credential_unavailable"`.
  - `TestSystemTask_RevokedSystemCredentialIgnored` — one revoked + one active for the same group; verify the active one is used (the fake upstream sees the matching `Authorization`).
  - `TestSystemTask_ArgValidation` — task declares `{first_message, required, MaxLen=4000}`; cases: missing → 400; >4000 chars → 400; correct → 200.
  - `TestSystemTask_UnknownTask_Returns404`.
  - `TestSystemTask_AuthRequired` — no Authorization header → 401.
  - `TestSystemTask_CreditsDebitedOnce` — fake LLM returns known token usage; assert one `generations` row with `task = "system:conversation_name"` and the correct token counts; org credit balance debited exactly once.
  - `TestSystemTask_CacheHit_NoUpstreamCall` — same `args` posted twice in a row; second call must not hit the fake upstream (assert hit count == 1) and must return the same `text` and `usage`. No second `generations` row; no second credit debit.
  - `TestSystemTask_CacheBypass_OnVersionBump` — register `conversation_name` v1, call once (populates cache); change `Version` to "v2"; same args produce a new upstream call. Pins the cache-key invalidation contract.
  - `TestSystemTask_CacheHit_StreamingShape` — cache hit on `stream:true` emits the cached text as one `data: {"delta":"..."}` chunk plus `data: {"done":true,"usage":{...},"cached":true}`. Frontend doesn't need to special-case.
  - `TestSystemTask_CacheTTLZero_NoCaching` — task with `CacheTTL: 0`; same args posted twice both hit upstream.
  - `TestSystemTask_StreamingErrorMidStream_NoCacheWrite` — fake upstream returns 200 + a few SSE chunks then errors; cache must not be populated (verified by a third call that re-hits upstream).

- **Registry-level test** (pure Go, no DB, no HTTP):
  - `TestRegistry_DuplicateRegistrationPanics` — calling `Register` twice with the same name panics. Programmer-error contract.
  - `TestRegistry_LookupUnknown_ReturnsFalse`.

That's the floor. No need for a separate test of `cheapestModel` or `Picker`; those are covered.

## Locked decisions

1. **Provider shape**: OpenAI-compat only. All system creds today (OpenAI, OpenRouter, Groq, SiliconFlow) speak `POST /v1/chat/completions` with the same wire shape. The forwarder builds OpenAI-shaped requests; it does not branch per provider in V1.
2. **SSE shape**: rewrap into a Hiveloop-shaped envelope. The forwarder consumes the upstream OpenAI SSE chunks, extracts `choices[0].delta.content`, and emits:
   ```
   data: {"delta":"<chunk text>"}\n\n
   ...
   data: {"done":true,"usage":{"input_tokens":N,"output_tokens":M}}\n\n
   ```
   This is uniform regardless of upstream provider quirks and gives the frontend one shape to handle.
3. **Caching is on by default.** Each task may set `CacheTTL time.Duration`. The default for unset is `24h`. Setting `CacheTTL: 0` disables caching for that task. Cache details:
   - **Storage**: Redis via the existing `cache.Manager` (already injected for the proxy path).
   - **Key**: SHA-256 of the canonicalised tuple `(task.Name, task.Version, model_id, args_canonical_json)`.
   - `task.Version` is a `string` field on the task definition that the author bumps whenever they change the system prompt, the user template, or the schema. Bumping it invalidates the cache for that task without a global flush.
   - **Value**: `{text, usage, model_id}` JSON.
   - **Hit on `stream=false`**: return the cached `{text, usage}` JSON; skip the LLM call.
   - **Hit on `stream=true`**: emit the entire cached `text` as a single `data: {"delta":"..."}` chunk, then `data: {"done":true,"usage":{...},"cached":true}`. The frontend doesn't have to special-case cache hits.
   - **Miss + `stream=false`**: run upstream, write the response to the cache after a clean 200, then respond.
   - **Miss + `stream=true`**: tee the upstream stream — pipe deltas to the client *and* accumulate them in a buffer. On clean upstream completion, write the buffered full text + parsed usage to cache. On error mid-stream, do not cache (avoids poisoning).
   - **Spend semantics**: cache hits **do not** invoke `Generation` or debit credits — the request never hits the upstream LLM, so there is no real spend to record. Cache hits do still write an `audit_entries` row (`task = "system:<name>"`, `cached = true`) for observability. This is the ergonomically right call: callers shouldn't be surprised that re-asking for the same conversation title charges them N times.
4. **`internal/tasks/conversation_name.go` (the webhook-driven server-to-server caller) is left as-is.** No port-over in V1.

## Implementation footprint

- `internal/system/{types.go, registry.go, render.go, forwarder.go, cache.go}` (~350 lines total)
- `internal/system/tasks/conversation_name.go` (~30 lines)
- `internal/handler/system_tasks.go` (~150 lines)
- Route registration + blank import in `cmd/server/serve_routes.go` and `cmd/server/main.go` (~10 lines)
- Tests (~280 lines)
