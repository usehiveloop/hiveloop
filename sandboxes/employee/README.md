# employee-bridge

Rust runtime for AI agents that act as real-world employees in Slack (and later Discord, Teams, WhatsApp). One sandbox = one workspace = one bot. Built on a provider-agnostic Rig-based agent runtime. Configured live via a control-plane HTTP API.

## Layout

```
crates/
  domain/           # shared types: AgentDefinition, SessionId, InboundEvent, ConfigStore
  gateway/          # ChannelGateway trait + Slack adapter
  agent/            # AgentRunner trait, Rig-based agent runtime
  storage/          # SQLite repos + migrations
  tools/            # bash, read_file, write_file, ...
  mcp/              # MCP server registry
  skills/           # skill prompt composition
  webhooks/         # outbound webhook outbox
  api/              # axum control-plane HTTP
  employee-bridge/  # binary
```

## Phase status

- **Phase 0 — foundations:** trait contracts and shared types (current)
- Phase 1 — five parallel tracks (Slack, agent, storage, api, webhooks)
- Phase 2 — integration; real Slack ping → reply test
- Phase 3 — tools, MCP, skills, subagents, Slack richness
- Phase 4 — observability and hardening
