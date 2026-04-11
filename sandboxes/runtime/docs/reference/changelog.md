# Changelog

Changes to Bridge.

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
2. Add `join` tool to parent agents
3. No code changes required

---

## Unreleased

Changes on main branch, not yet released:

- (None currently)

### v0.3.0 to v0.18.0

No breaking changes between the two documented versions — v0.18.0 is purely additive. To use per-conversation MCP:

1. Leave `BRIDGE_ALLOW_STDIO_MCP_FROM_API` unset (default `false`) unless you trust every API caller AND Bridge is sandboxed.
2. Pass `mcp_servers` with `streamable_http` transport in your `POST /agents/{id}/conversations` request body.
3. No changes required to existing agent definitions, conversations, or client code.

---

## See Also

- [GitHub Releases](https://github.com/useportal-app/bridge/releases)
