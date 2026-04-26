# Changelog

## [Unreleased]

### Added

- **Workspace artifacts (`AgentDefinition.artifacts`).** New optional config block (`upload_url`, `download_url`, `max_size_bytes`, `accepted_file_types`, `max_concurrent_uploads`, `chunk_size_bytes`, `headers`) that auto-registers an `upload_to_workspace` tool on the agent. The tool streams files from the agent's sandbox to the control plane via a tus.io v1.0.0 resumable upload protocol. Bridge handles per-chunk SHA-256 integrity checks, jittered exponential retry on 5xx/network errors (6 retries / 7 attempts), `409 Conflict` server-offset realignment, and crash-resume from a new `artifact_uploads` sqlite table when `BRIDGE_STORAGE_PATH` is set. Idempotency key is `sha256(agent_id || abs_path || file_sha256)` — re-calling the tool with the same file returns the cached control-plane response. The tool result is a JSON object (`artifact_id`, `upload_url`, `download_url`, `size`, `content_type`, `sha256`). Agent push-time validation (`AgentDefinition::validate()`) rejects empty `accepted_file_types`, zero `max_size_bytes`, malformed URLs, and zero `max_concurrent_uploads` / `chunk_size_bytes`. The `artifacts` field is also exposed on the `GET /agents/{id}` response.
- **`ArtifactsConfig` core type and `ArtifactUploadRow` storage row.** New module `bridge_core::artifacts`; new sqlite table `artifact_uploads` with `(idempotency_key, agent_id, conversation_id, location, total_size, file_sha256, bytes_sent, status, response_json, last_error, created_at, updated_at)`.
- **`config.system_reminder_refresh_turns`** — controls how often the stable system reminder (skills, subagents, todos) is re-emitted at the head of the user message. Default `10`; values `<1` clamp to `1`; always emitted on turn 0 and on turns where `turn_count % N == 0`.
- **Sandbox environment system reminder.** When `BRIDGE_STANDALONE_AGENT=true`, bridge injects a system reminder describing the sandbox's resource limits and installed tools (`crates/runtime/src/environment.rs`).
- **Stall timeout + repeat-call guard.** Resilience pass on the runtime: a per-turn stall timeout aborts hung LLM calls, and a repeat-call guard suppresses agents that re-fire the same tool with the same arguments back-to-back. (`feat(runtime): resilience pass — stall timeout, repeat-call guard, env reminder, strip fixes`)
- **`cache_control` + `tool_choice` middleware** for the LLM provider stack, with head-merge behavior to avoid history loss between provider invocations. Adds `crates/llm/src/providers/cache_control_middleware.rs` and `crates/llm/src/providers/tool_choice_middleware.rs`.

### Changed

- **Immortal mode rewritten as in-place forgecode-style compaction.** The previous LLM-driven checkpoint extractor has been removed. Compaction now replaces the eligible head of the conversation in place with one user message containing a structured summary derived from the messages it replaced — pure code, deterministic, no LLM call. `ImmortalConfig` is now `{ token_budget, retention_window, eviction_window, expose_journal_tools }`; the previous LLM-checkpoint fields (`checkpoint_prompt`, `verify_checkpoint`, `checkpoint_max_tokens`, `checkpoint_timeout_secs`, `max_previous_checkpoints`, `carry_forward_budget_fraction`) are gone. (`feat(immortal): replace LLM checkpoint with forgecode-style in-place compaction`)
- **Optional journal tools.** `journal_read` / `journal_write` are now registered only when `config.immortal` is set AND `immortal.expose_journal_tools` is true (default). Agents without immortal mode no longer see journal tools. (`feat(runtime): optional journal tools + todos-snapshot carry-forward`)
- **Bash routed through rtk.** `bash` tool invocations are routed through the rtk filter pipeline for token-efficient output. An in-process allowlist router (replacing the earlier `rtk-rewrite` dispatch) decides which commands get routed. Test-runner output (PHPUnit / Pest summary lines) is preserved verbatim — no synthetic `artisan test: ok` collapse. (`feat(bash): route tool invocations through rtk for token-efficient output`, `fix(bash): replace rtk-rewrite dispatch with in-process allowlist router`)
- **Trimmed verbose tool descriptions.** `lsp` (2862 → 1129 bytes), `todowrite` (2685 → 578), `multiedit` (2179 → 650), `journal_write` (2528 → 976). Removed tutorial-style "when to use / when not to use" sections, duplicated language lists, redundant "CRITICAL REQUIREMENTS" / "WARNING" blocks. (`fix(immortal,prompt): plug strip leak; trim system-reminder + tool descriptions`)

### Fixed

- **`history_strip` leak inside rig's loop.** Strip previously fired only at the top of each bridge turn, so single-bridge-turn agents (where everything happens inside rig's loop) saw old `Read` results, PHPUnit dumps, etc. accumulate unchecked. Strip now fires inside `conversation/run.rs`'s resume loop too, after the immortal hook's cancellation history is promoted. (`fix(immortal,prompt): plug strip leak; …`)
- **Refactor: split `supervisor`, `conversation`, `agent_runner` into sub-modules.** `crates/runtime/src/supervisor.rs`, `conversation.rs`, and `agent_runner.rs` are now directories with focused submodules (each file under ~300 lines). Public API is unchanged; references to the old single-file paths in docs have been updated. (`refactor(runtime): split supervisor/conversation/agent_runner below 300 lines`)

### Infrastructure

- New sqlite migration adds the `artifact_uploads` table with indexes on `status` and `agent_id`. Migrations remain idempotent (`IF NOT EXISTS`).
- Workspace deps: `tokio-util` features extended to `["rt", "io"]`; new entries `hex`, `mime_guess`, `bytes` (in `tools` crate). `tools` dev-deps add `axum` for the in-process TUS test server.

## [0.21.1] - 2026-04-19

### Changed

- **Subagent execution timeout is now per-agent configurable** via `AgentConfig.subagent_timeout_foreground_secs` and `AgentConfig.subagent_timeout_background_secs` (seconds, both `Option<u64>`). Each subagent's own config supplies its timeout; `__self__` self-delegation reads from the parent agent's config. Default raised to **300s (5 min)** for both foreground and background (previously hardcoded 120s foreground / 300s background).

### Infrastructure

- `openapi.json` regenerated — publishes the two new `AgentConfig` fields.

## [0.21.0] - 2026-04-19

### Added

- **Declarative tool-call requirements (`config.tool_requirements`).** Declare tools the agent MUST call per turn with cadence (`every_turn`, `first_turn_only`, `every_n_turns {n}` — "reset on call"), position (`anywhere`, `turn_start`, `turn_end` — lenient about read-only tools like `todoread` / `journal_read`), minimum call count, and enforcement variant (`next_turn_reminder` default, `warn`, `reprompt`). Tool-name matching is flexible: patterns without `__` also match MCP tools registered as `{server}__{name}`, so `"post_message"` matches `slack__post_message`. Bridge rejects pushes where a required tool also appears in `disabled_tools` (400 InvalidRequest). Violations fire a `tool_requirement_violated` event and — for non-`warn` enforcement — attach a `<system-reminder>` block to the next user message naming the missing tool(s).
- **`full_message` field on `POST /conversations/{id}/messages`.** Offload large payloads (stack traces, log dumps, file contents) to disk instead of inflating context on every turn. Bridge writes `full_message` to `{BRIDGE_ATTACHMENTS_DIR | ./.bridge-attachments}/{conversation_id}/{uuid}.txt`, appends a `<system-reminder>` to `content` pointing the agent at the absolute path, and tailors the tool hint to the agent's registered tools (`RipGrep` + `Read`, just one of them, `AstGrep`, or `bash` with a "don't `cat`" warning, or an explicit "no search tool registered" note). Missing `content` is auto-summarized from the first ~500 bytes of `full_message` rather than rejected. Attachments are cleaned up when the conversation ends. Disk failures are logged and the message is delivered without the attachment — `full_message` can never cause a send-message rejection. The `message_received` event now carries an `attachment_path` field (null when no attachment).
- **`BRIDGE_ATTACHMENTS_DIR` env var** — overrides the attachments root directory (default `./.bridge-attachments`).
- **`ChainFailed` and `ContextPressureWarning` SSE/webhook events.** `ChainFailed` fires when a chain handoff attempt errors out (the conversation continues with oversized history). `ContextPressureWarning` fires once per turn when cumulative tool-output bytes exceed ~1.5× the immortal token budget.
- **Provider-aware checkpoint prompt.** The default checkpoint extraction prompt is now provider-aware. Gemini models (detected by `ProviderType::Google` or model-name substring) automatically receive a stricter XML-delimited template with explicit per-section length caps and active-verb pruning directives. In testing this arrested Gemini 2.5 Flash's monotonic checkpoint-size growth (4k → 7k → 15k bytes over 3 chains with the old prompt) to a flat ~9k across 4 chains. Other providers fall through to the existing default. Override per-agent via `config.immortal.checkpoint_prompt`.
- **Rich `turn_completed` event payload.** Now includes `turn_latency_ms`, `cumulative_tool_calls`, `history_tokens_estimate` (tiktoken count of current history — the same signal chain checks use), `history_message_count`, and `journal_entries_committed`.
- **`ImmortalConfig` new fields.** `carry_forward_budget_fraction` (default `0.3`) caps the carry-forward tail at a fraction of `token_budget`. `verify_checkpoint` (default `false`) controls the optional phase-2 verification pass. `checkpoint_max_tokens` (default `1500`) caps checkpoint LLM output. `checkpoint_timeout_secs` (default `45`) bounds the extraction call. `max_previous_checkpoints` (default `2`) limits how many prior chain checkpoints feed the next extraction — prevents unbounded chain-over-chain growth.

### Changed

- **Immortal chain-event ordering.** `ChainStarted` now fires BEFORE the checkpoint extraction LLM call (previously fired after). SSE consumers can now render progress UI during the 7-75s extraction window. `ChainCompleted` payload adds `duration_ms`, `carry_forward_tokens`, `checkpoint_bytes`, `verified`.
- **Token-bounded carry-forward.** Replaces turn-count-only. `carry_forward_budget_fraction` (default 30% of budget) caps the tail, preventing a single tool-heavy turn from stuffing the new chain's context.
- **Single-phase checkpoint by default.** `verify_checkpoint` now defaults to `false` — the phase-2 verification pass rarely improves output for strong summarizer models and ~doubles cost.
- **Journal writes stage per turn.** `journal_write` tool calls now stage in-memory and commit only on turn success (or discard on failure). Prevents duplicate/orphan entries from rolled-back turns. Chain-checkpoint entries (system-generated) still persist immediately.

### Fixed

- **History restoration on mid-turn LLM errors.** When the agent's LLM call errored mid-turn (429, provider error), bridge truncated the persisted-messages side but left the in-memory rig history as the `mem::take`'d empty `Vec`. Subsequent turns silently started from empty history, defeating chain-token checks. Now restored from the pre-turn backup on the same error path the timeout/cancel paths already used.
- **Pre-stream LLM retry.** Retryable upstream errors (429/5xx/timeouts) that occur BEFORE any delta is emitted are now retried with exponential backoff (up to 3 attempts). Safe because we bail on any streaming progress.
- **`send_message` 4xx on empty body restored.** `content` is now `#[serde(default)]` so callers can supply only `full_message`, but an empty request with neither field still returns 400 InvalidRequest — preserving the pre-attachments behavior that malformed bodies like `{"invalid": true}` return 4xx.

### Infrastructure

- `openapi.json` regenerated — publishes the new `ToolRequirement`, `RequirementCadence`, `RequirementPosition`, `RequirementEnforcement` schemas and the `full_message` field on `SendMessageRequest`.
- New `scripts/immortal-real-test.mjs` standalone driver — exercises the full immortal flow with a real LLM against a live bridge, streams SSE events, and prints a deep post-run report.

## [0.20.1] - 2026-04-17

### Changed

- **`bridge install-lsp` catalog trimmed to only servers with broadly-available install methods.** Servers whose only distribution is via niche toolchains (`opam`, `gem`, `dart pub`, `dotnet tool`, `cs`/Coursier) were dropped. Placeholder entries that only `echo`'d setup instructions (`haskell`, `nixd`, `julials`, `sourcekit-lsp`, old `deno`) were dropped. Removed ids: `ocaml-lsp`, `ruby-lsp`, `ruby-lsp-official`, `dart`, `metals`, `csharp`, `haskell`, `nixd`, `julials`, `sourcekit-lsp`. The `InstallMethod::{Gem, LuaRocks, Opam, Stack}` enum variants and their install paths are deleted as dead code.
- **`deno` re-added** as a real install via the official `install.sh` with `DENO_INSTALL=$HOME/.local` so the binary lands alongside the other self-contained downloads in `~/.local/bin`.
- **Per-server install failures are now non-fatal.** `bridge install-lsp <list>` downgrades individual failures from `error!` to `warn!` and always exits 0. The final log summarises which ids were skipped so the operator can install the missing toolchain and re-run the specific id. Previously one missing `opam` (or similar) would make the whole command exit 1.

## [0.20.0] - 2026-04-17

### Changed — BREAKING

- **Subagent orchestration simplified to match Claude Code's model.** Three tools collapsed to one:
  - Removed `parallel_agent` and `join` tools entirely.
  - `sub_agent` and `agent` rename the `background` parameter to `runInBackground`.
  - Parallel fan-out is now achieved by emitting multiple `sub_agent` tool_use blocks in a single assistant turn — the runtime already dispatches tool calls in parallel. No array-taking tool is needed.
  - Background subagent results are auto-injected into the parent's next user turn as `[Background Agent Task Completed]` messages. The `TaskRegistry` and its polling surface are gone — the existing `notification_tx` path was already doing the delivery, so `join` had become redundant double-delivery.
  - `AgentContext.task_registry` field removed. `AgentState::new` no longer takes a `task_registry` argument. `ConversationSubAgentRunner::with_task_registry` removed.
  - Net: ~1,100 lines deleted; no behavioural regressions in the workspace test suite.
  - Migration: rename `"background": true` to `"runInBackground": true` in any agent definition or prompt. Replace `parallel_agent` calls with multiple `sub_agent` tool_use blocks in the same turn. Remove any use of `join` — background results arrive automatically.

### Added

- **WebSocket Event Stream:** New `/ws/events` endpoint that delivers all events from all agents and conversations over a single persistent WebSocket connection. Efficient alternative to webhooks for high-throughput control planes.
  - Enable with `BRIDGE_WEBSOCKET_ENABLED=true`
  - Authenticate via `?token=<api_key>` query parameter
  - Global monotonic sequence numbers for ordering
  - Supports multiple concurrent clients
  - Lagged client detection with warning messages
  - Can be used alongside webhooks or as the sole event delivery mechanism

### Search tools

- Replaced the generic `grep` tool with `RipGrep` (regex/text over file contents) and `AstGrep` (structural code search using ast-grep patterns).

## [0.3.0] - 2026-03-18

### Added

- **CLI Interface:** Bridge now has a command-line interface using `clap`
  - `bridge --help` - Show CLI help
  - `bridge tools list --json` - List all available tools with their JSON schemas
  
- **Makefile Commands:**
  - `make tools` - List tools using release binary
  - `make tools-debug` - List tools using debug build

### Documentation

- **Complete Documentation Rewrite (56 pages):**
  - Fixed all tool names to match actual implementation (case-sensitive)
  - Fixed API endpoint request/response formats
  - Fixed SSE event names and payloads
  - Fixed webhook HMAC signature algorithm documentation
  - Fixed authentication error codes and messages
  - Added complete tool limits and constraints
  - Added provider type aliases and formats
  - Fixed integration tool schema documentation
  - Fixed batch tool parameter names
  - Fixed agent/conversation timeout values
  - Added missing LLM provider docs (Google, Cohere)

### Changed

- **Exports:** `register_builtin_tools` is now exported from `tools` crate

## [0.2.0] - 2026-03-17

### Added

- **New Tools:**
  - `join` tool: Wait for multiple background subagent tasks to complete
  - `parallel_agent` tool: Spawn up to 25 subagents concurrently with configurable limits

- **Parallel Execution:**
  - `TaskRegistry`: Shared state for tracking background task completion
  - `ConcurrencyLimiter`: Semaphore-based resource limiting (default: 5 concurrent tasks)
  - Background subagents now register completion for join tool visibility

- **Documentation:**
  - Detailed instruction files (.txt) for new tools following existing patterns
  - E2E tests for parallel execution capabilities

### Changed

- `AgentState` now includes `task_registry` field
- `ConversationSubAgentRunner` marks background tasks as complete in registry
- Internal tool registration updated to include new tools

## [0.1.0] - Initial Release

- Base bridge runtime with agent management
- Tool registry with built-in tools (Read, Write, Edit, Grep, Glob, Bash, etc.)
- Subagent support with depth limiting
- MCP server integration
- SSE streaming for conversations
- Webhook event delivery
