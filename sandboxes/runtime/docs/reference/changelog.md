# Changelog

Changes to Bridge.

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

---

## See Also

- [GitHub Releases](https://github.com/useportal-app/bridge/releases)
