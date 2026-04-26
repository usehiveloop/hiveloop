# Changelog

Changes to Bridge.

---

## Unreleased

### Added

- **Workspace artifacts (`AgentDefinition.artifacts`).** New optional config block that auto-registers an `upload_to_workspace` tool. The tool streams files from the agent's sandbox to the control plane via tus.io v1.0.0 resumable chunks. Bridge handles per-chunk SHA-256 integrity checks, jittered exponential retry on 5xx/network errors (6 retries / 7 attempts), `409 Conflict` server-offset realignment, and crash-resume from a new `artifact_uploads` sqlite table when `BRIDGE_STORAGE_PATH` is set. Idempotency key is `sha256(agent_id || abs_path || file_sha256)`. Tool result: `{ artifact_id, upload_url, download_url, size, content_type, sha256 }`. Push-time validation rejects empty `accepted_file_types`, zero `max_size_bytes`, malformed URLs, zero `max_concurrent_uploads` / `chunk_size_bytes`. Surfaced on the `GET /agents/{id}` response. See [Artifacts Definition](../core-concepts/agents.md#artifacts-definition) and [`upload_to_workspace`](../tools-reference/all-tools.md#workspace-artifacts).
- **`config.system_reminder_refresh_turns`** — re-emit the stable system reminder (skills, subagents, todos) every N turns at the head of the user message. Default `10`; values `<1` clamp to `1`; always emitted on turn 0.
- **Sandbox environment system reminder.** When `BRIDGE_STANDALONE_AGENT=true`, bridge injects a system reminder describing the sandbox's resource limits and installed tools.
- **Stall timeout + repeat-call guard.** A per-turn stall timeout aborts hung LLM calls; a repeat-call guard suppresses agents that re-fire the same tool with identical arguments back-to-back.
- **`cache_control` + `tool_choice` middleware** on the LLM provider stack, with head-merge to prevent history loss between provider invocations.

### Changed

- **Immortal mode rewritten as in-place forgecode-style compaction.** The previous LLM-driven checkpoint extractor has been removed. Compaction now replaces the eligible head of the conversation in place with a single user message containing a structured summary derived from the messages it replaced — pure code, deterministic, no LLM call. `ImmortalConfig` is now `{ token_budget, retention_window, eviction_window, expose_journal_tools }`. The previous LLM-checkpoint fields (`checkpoint_prompt`, `verify_checkpoint`, `checkpoint_max_tokens`, `checkpoint_timeout_secs`, `max_previous_checkpoints`, `carry_forward_budget_fraction`) are gone. See [Immortal Mode](../core-concepts/agents.md#immortal-mode).
- **Optional journal tools.** `journal_read` / `journal_write` are registered only when the agent has `config.immortal` set AND `immortal.expose_journal_tools` is true (default). Agents without immortal mode no longer see journal tools.
- **Bash routed through rtk.** `bash` invocations are routed through the rtk filter pipeline for token-efficient output, via an in-process allowlist router (replacing the earlier `rtk-rewrite` dispatch). Test-runner output (PHPUnit / Pest summary lines) is preserved verbatim.
- **Trimmed verbose tool descriptions.** `lsp` (2862 → 1129 bytes), `todowrite` (2685 → 578), `multiedit` (2179 → 650), `journal_write` (2528 → 976). Removed tutorial-style "when to use / when not to use" sections, duplicated language lists, redundant "CRITICAL REQUIREMENTS" / "WARNING" blocks.
- **Refactor: `supervisor`, `conversation`, `agent_runner` split into directories.** `crates/runtime/src/supervisor.rs`, `conversation.rs`, and `agent_runner.rs` are now directories with focused submodules. Public API is unchanged.

### Fixed

- **`history_strip` leak inside rig's loop.** Strip previously fired only at the top of each bridge turn, so single-bridge-turn agents (everything happens inside rig's loop) saw old `Read` results, PHPUnit dumps, etc. accumulate unchecked. Strip now also fires inside `conversation/run.rs`'s resume loop after the immortal hook's cancellation history is promoted.

### Earlier additions to this Unreleased cycle

- **Declarative tool-call requirements (`tool_requirements`)** in agent config. Declare tools the agent must call, with cadence (every turn, first turn only, every N turns since last call), position (anywhere / turn_start / turn_end — lenient about read-only tools like `todoread` and `journal_read`), minimum call count, and enforcement variant (`next_turn_reminder` default, `warn`, `reprompt`). Tool-name matching is flexible: patterns without `__` also match MCP tools registered as `{server}__{name}`. Bridge rejects pushes where a required tool is also in `disabled_tools`. Violations fire a `tool_requirement_violated` event and, for non-warn enforcement, attach a `<system-reminder>` block to the next user message. See [Tool Requirements](../core-concepts/agents.md#tool-requirements).
- **`full_message` field on `POST /conversations/{id}/messages`.** Callers can now offload large payloads (stack traces, log dumps, file contents) by sending a short `content` summary alongside the full payload in `full_message`. Bridge writes the full payload to `{BRIDGE_ATTACHMENTS_DIR}/{conversation_id}/{uuid}.txt` (default root: `./.bridge-attachments`), appends a `<system-reminder>` to the content pointing the agent at the absolute path, and tailors the tool hint to the agent's registered tools (`RipGrep` + `Read`, just one of them, `AstGrep`, or `bash` with a "don't `cat`" warning). Missing `content` is auto-summarized from the first ~500 bytes of `full_message` rather than rejected. Attachments are cleaned up when the conversation ends. Disk failures are logged and the message is delivered without the attachment — `full_message` can never cause a send-message rejection. The `message_received` event now carries an `attachment_path` field (null when no attachment). See [Large payloads via `full_message`](../core-concepts/conversations.md#large-payloads-via-full_message) and [`BRIDGE_ATTACHMENTS_DIR`](./environment-variables.md#bridge_attachments_dir).

---

## v0.18.2 (2026-04-13)

### Removed

- **CodeDB as a first-class concept.** The `BRIDGE_CODEDB_ENABLED` and `BRIDGE_CODEDB_BINARY` config options, auto-injection, and built-in tool suppression have been removed. To use CodeDB, add it as a regular MCP server in your agent's `mcp_servers` array and use `disabled_tools` to drop built-in tools you don't want. This simplifies Bridge's codebase and configuration surface.

---

## v0.18.1 (2026-04-13)

### Added

- **Skill filesystem support.** Skills with `files` now have their supporting files (scripts, reference docs) written to `.skills/<skill-id>/` on disk at agent load time. Scripts (`.sh`, `.py`, `.rb`) are marked executable. The agent receives a location note when invoking the skill and can execute scripts directly. Files are cleaned up on agent removal or update. See [Skill Files](../core-concepts/skills.md#skill-files).
- **`file` parameter on the skill tool.** Request a specific supporting file by relative path without loading the full skill content.
- **`${CLAUDE_SKILL_DIR}` resolves to filesystem path.** The variable now substitutes to `.skills/<skill-id>` instead of the bare skill ID.
- **Tool argument validation.** Tool arguments are validated against their JSON schema before execution, catching malformed calls early without a wasted round-trip to the tool executor.

### Fixed

- Cross-platform clippy warnings in `environment.rs` (`unnecessary_cast` on statvfs fields that differ between macOS and Linux).

---

## v0.18.0 (2026-04-11)

### Added

- **Per-conversation MCP servers.** `POST /agents/{id}/conversations` now accepts an `mcp_servers` field. Attach one or more `McpServerDefinition`s scoped to a single conversation — connected at creation, torn down on every termination path (`DELETE`, abort, drain, `SIGINT`/`SIGTERM`, `max_turns`, internal error). Useful when tool surface varies per call (tenant-scoped HTTP MCP servers, dev-only tools, short-lived integrations). See [Per-Conversation MCP Servers](../core-concepts/mcp.md#per-conversation-mcp-servers) and [Conversations API](../api-reference/conversations-api.md#per-conversation-mcp).
- **`BRIDGE_ALLOW_STDIO_MCP_FROM_API` runtime flag** (default `false`). Gates the stdio MCP transport when supplied via the API. Stdio spawns an arbitrary subprocess, so it's opt-in per deployment. `streamable_http` is always allowed. Agent-level MCP servers (from control-plane-pushed definitions) are unaffected by this flag.
- **Collision detection** for MCP tool names — a per-conversation MCP server that advertises a tool whose name already exists on the agent is rejected with HTTP 400 instead of silently shadowing.

### CI

- Real-LLM e2e workflows (`e2e-approval`, `e2e-codedb`, `e2e-parallel`, `e2e-observability`) are gated off CI and now run locally with `cargo test -p bridge-e2e --test <name> -- --ignored`. Removes flakiness from upstream provider 429s.
- The three `*_native_provider` tests in `e2e_tests.rs` are marked `#[ignore]`, so `e2e-bridge` no longer needs Anthropic/Gemini/Cohere API key secrets.

### Fixed

- OpenAPI generation was broken by a stale `AgentDetailsResponse` reference in the schema registry; replaced with the current `AgentResponse` plus its nested types.

---

## v0.3.0 (2026-03-18)

### Added

- **CLI Interface** — Bridge now has a command-line interface
  - `bridge tools list --json` — List all available tools with schemas
  - `make tools` — Makefile command to list tools
- **Complete Documentation** — 56 pages of fully audited documentation

### Documentation

- Fixed all tool names, API formats, and event names
- Added comprehensive limits and constraints documentation
- Fixed webhook HMAC signature documentation
- Added missing LLM provider guides (Google, Cohere)

---

## v0.2.0 (2026-03-17)

### Added

- **Parallel agent execution** — Run up to 25 subagents concurrently
- **System reminders** — Inject skill lists and date info before each message
- **Date tracking** — Detect calendar date changes between messages
- **Skill parameters** — Template substitution with `{{args}}` in skill content
- **`join` tool** — Wait for subagents with configurable timeout

### Changed

- Updated `SkillToolArgs` to include optional `args` field
- Improved SSE stream reliability

### Fixed

- Race condition in conversation state management
- Memory leak in long-running conversations

---

## v0.1.0 (2026-02-01)

### Added

- Initial release
- HTTP API for agents and conversations
- SSE streaming
- Webhook support
- Multiple LLM providers (Anthropic, OpenAI-compatible)
- Built-in tools (filesystem, bash, search, web)
- MCP server support
- Tool permissions (allow, require_approval, deny)
- Agent draining for zero-downtime updates
- Conversation compaction

---

## Versioning

Bridge follows [Semantic Versioning](https://semver.org/):

- **MAJOR** — Breaking changes
- **MINOR** — New features, backwards compatible
- **PATCH** — Bug fixes

---

## Migration Guides

### v0.1.0 to v0.2.0

No breaking changes. To use new features:

1. Update skill definitions to use `{{args}}` templates
2. No code changes required

### v0.19.x to Unreleased — BREAKING: subagent orchestration simplified

Bridge now mirrors Claude Code's subagent model: one tool, one flag.

1. The `parallel_agent` and `join` tools have been removed.
2. The `background` parameter on `sub_agent` and `agent` has been renamed to `runInBackground`.
3. Parallel fan-out is now done by emitting multiple `sub_agent` tool_use blocks in a single assistant turn (the runtime already dispatches them concurrently).
4. Background results are auto-injected into the parent's next user turn as `[Background Agent Task Completed]` messages — there is no wait/join tool.

**Migration steps:**

- Replace `"background": true` with `"runInBackground": true` in system prompts, agent definitions, and any code that constructs tool calls.
- Replace `parallel_agent` call sites with multiple `sub_agent` tool_use blocks in the same turn.
- Remove any use of `join` — the parent now receives background outputs automatically.
- Drop `parallel_agent` and `join` from any `tools` allowlist or `disabled_tools` list.

---

## Unreleased

Changes on main branch, not yet released:

- **BREAKING:** `parallel_agent` and `join` tools removed; `sub_agent` / `agent` parameter renamed from `background` to `runInBackground`. See migration guide above.

### v0.18.0 to v0.18.1

No breaking changes. To use skill files:

1. Add a `files` map to your skill definitions with relative paths as keys and file content as values.
2. The agent will see a location note when invoking the skill and can execute scripts from `.skills/<skill-id>/`.
3. No changes required to existing skills without files.

### v0.3.0 to v0.18.0

No breaking changes between the two documented versions — v0.18.0 is purely additive. To use per-conversation MCP:

1. Leave `BRIDGE_ALLOW_STDIO_MCP_FROM_API` unset (default `false`) unless you trust every API caller AND Bridge is sandboxed.
2. Pass `mcp_servers` with `streamable_http` transport in your `POST /agents/{id}/conversations` request body.
3. No changes required to existing agent definitions, conversations, or client code.

---

## See Also

- [GitHub Releases](https://github.com/useportal-app/bridge/releases)
