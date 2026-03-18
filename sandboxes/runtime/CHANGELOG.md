# Changelog

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
