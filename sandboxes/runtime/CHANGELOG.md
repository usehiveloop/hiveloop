# Changelog

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
